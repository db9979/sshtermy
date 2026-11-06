package sshx

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/relay"
)

// Verbindungen über einen Vermittler: der vom Anker gemeldete Host-Key gilt
// ohne Rückfrage, ein anderer wird abgelehnt (nie „trotzdem verbinden“).
func TestRelayHostKey(t *testing.T) {
	addr := startExecServer(t)
	good, err := relay.ProbeHostKey(addr)
	if err != nil {
		t.Fatal(err)
	}
	other := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(testKey(t))))
	c := domain.Connection{Alias: "r", Host: "web", RelayID: "rabc", RelaySystem: "a_1", Username: "u",
		Auth: domain.AuthPassword, Password: "x", Device: domain.DeviceServer}
	dial := func(hk string, prompt HostKeyPrompt) (*Client, error) {
		hs, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
		return Dial(context.Background(), c, Options{HostKeys: hs, Prompt: prompt, Timeout: 5 * time.Second,
			Relay: func(ctx context.Context, cc domain.Connection) (net.Conn, string, error) {
				if cc.Addr() != "a_1.rabc.vermittler:22" {
					t.Errorf("Addr %q", cc.Addr())
				}
				nc, err := net.Dial("tcp", addr)
				return nc, hk, err
			}})
	}
	never := func(string, string, string) bool { t.Error("Rückfrage trotz gemeldetem Host-Key"); return false }
	cl, err := dial(good, never)
	if err != nil {
		t.Fatal(err)
	}
	cl.Close()
	if _, err := dial(other, func(string, string, string) bool { return true }); !errors.Is(err, ErrRelayHostKey) {
		t.Fatalf("falscher Host-Key: %v", err)
	}
	asked := false
	cl, err = dial("", func(string, string, string) bool { asked = true; return true })
	if err != nil || !asked {
		t.Fatalf("ohne gemeldeten Key fragen: %v %v", err, asked)
	}
	cl.Close()
	if _, err := Dial(context.Background(), c, Options{HostKeys: &HostKeyStore{}}); err == nil {
		t.Fatal("ohne RelayDialer")
	}
}

// Server mit mehreren Host-Keys (wie jedes OpenSSH: ecdsa + ed25519): der Anker
// meldet ed25519, der Client muss genau diesen Typ aushandeln.
func TestRelayHostKeyMultiKeyServer(t *testing.T) {
	_, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	ecPriv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	edS, _ := ssh.NewSignerFromKey(edPriv)
	ecS, _ := ssh.NewSignerFromKey(ecPriv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(ecS)
	cfg.AddHostKey(edS)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
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
				go ssh.DiscardRequests(reqs)
				go func() {
					for nch := range chans {
						nch.Reject(ssh.Prohibited, "nein")
					}
				}()
				_ = sc.Wait()
			}()
		}
	}()
	addr := ln.Addr().String()
	good, err := relay.ProbeHostKey(addr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(good, ssh.KeyAlgoED25519) {
		t.Fatalf("Anker meldet %q, erwartet ed25519", good)
	}
	hs, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	c := domain.Connection{Alias: "r", Host: "web", RelayID: "rabc", RelaySystem: "a_1", Username: "u",
		Auth: domain.AuthPassword, Password: "x", Device: domain.DeviceServer}
	cl, err := Dial(context.Background(), c, Options{HostKeys: hs, Timeout: 5 * time.Second,
		Prompt: func(string, string, string) bool { t.Error("Rückfrage trotz gemeldetem Host-Key"); return false },
		Relay: func(ctx context.Context, cc domain.Connection) (net.Conn, string, error) {
			nc, err := net.Dial("tcp", addr)
			return nc, good, err
		}})
	if err != nil {
		t.Fatalf("Server mit ecdsa+ed25519: %v", err)
	}
	cl.Close()
}
