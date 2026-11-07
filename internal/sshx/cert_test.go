package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// signedKey legt im Ordner dir einen Key „id“ und ein von ca signiertes
// Zertifikat „id-cert.pub“ an.
func signedKey(t *testing.T, dir string, ca ssh.Signer, validBefore uint64) string {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	blk, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	kp := filepath.Join(dir, "id")
	if err := os.WriteFile(kp, pem.EncodeToMemory(blk), 0o600); err != nil {
		t.Fatal(err)
	}
	sp, _ := ssh.NewPublicKey(pub)
	cert := &ssh.Certificate{Key: sp, CertType: ssh.UserCert, KeyId: "dominik", ValidPrincipals: []string{"u"},
		ValidBefore: validBefore, Permissions: ssh.Permissions{Extensions: map[string]string{"permit-pty": ""}}}
	if err := cert.SignCert(rand.Reader, ca); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kp+"-cert.pub", ssh.MarshalAuthorizedKey(cert), 0o600); err != nil {
		t.Fatal(err)
	}
	return kp
}

func TestCertLogin(t *testing.T) {
	_, capriv, _ := ed25519.GenerateKey(rand.Reader)
	ca, _ := ssh.NewSignerFromKey(capriv)
	dir := t.TempDir()
	kp := signedKey(t, dir, ca, ssh.CertTimeInfinity)

	// Server vertraut nur der CA, keinem einzelnen Key
	checker := &ssh.CertChecker{IsUserAuthority: func(auth ssh.PublicKey) bool {
		return bytes.Equal(auth.Marshal(), ca.PublicKey().Marshal())
	}}
	_, hpriv, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hpriv)
	cfg := &ssh.ServerConfig{PublicKeyCallback: checker.Authenticate}
	cfg.AddHostKey(hs)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					nch.Reject(ssh.Prohibited, "no")
				}
			}()
		}
	}()
	hk, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	c := domain.Connection{Alias: "srv", Host: host, Port: p, Username: "u", Auth: domain.AuthKeyFile, KeyPath: kp, Device: domain.DeviceNetwork}
	opts := Options{HostKeys: hk, Prompt: func(string, string, string) bool { return true }, Timeout: 5 * time.Second, waitScale: 0.05}
	cl, err := Dial(context.Background(), c, opts)
	if err != nil {
		t.Fatalf("Anmeldung mit Zertifikat: %v", err)
	}
	cl.Close()

	// ohne Zertifikat lehnt der Server ab
	os.Remove(kp + "-cert.pub")
	if cl, err := Dial(context.Background(), c, opts); err == nil {
		cl.Close()
		t.Fatal("Anmeldung ohne Zertifikat angenommen")
	}
}

func TestCertExpiredAndExplicit(t *testing.T) {
	_, capriv, _ := ed25519.GenerateKey(rand.Reader)
	ca, _ := ssh.NewSignerFromKey(capriv)
	dir := t.TempDir()
	kp := signedKey(t, dir, ca, uint64(time.Now().Add(-time.Hour).Unix()))
	c := domain.Connection{KeyPath: kp}
	signer, _ := ssh.ParsePrivateKey(mustRead(t, kp))
	if _, err := withCert(c, signer, time.Now()); !errors.Is(err, ErrCertExpired) {
		t.Fatalf("abgelaufen: %v", err)
	}
	// ausdrücklich eingetragen, aber nicht da: Fehler
	c = domain.Connection{KeyPath: kp, CertPath: filepath.Join(dir, "fehlt-cert.pub")}
	if _, err := withCert(c, signer, time.Now()); err == nil {
		t.Fatal("fehlendes Zertifikat ohne Fehler")
	}
	// kein Zertifikat neben dem Key: einfach der Key
	c = domain.Connection{KeyPath: filepath.Join(dir, "anderer")}
	if s, err := withCert(c, signer, time.Now()); err != nil || s != signer {
		t.Fatalf("ohne Zertifikat: %v", err)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
