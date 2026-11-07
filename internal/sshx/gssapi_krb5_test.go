//go:build !windows

package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jcmturner/gofork/encoding/asn1"
	"github.com/jcmturner/gokrb5/v8/gssapi"
	"github.com/jcmturner/gokrb5/v8/iana/etypeID"
	"github.com/jcmturner/gokrb5/v8/iana/keyusage"
	"github.com/jcmturner/gokrb5/v8/iana/nametype"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/messages"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"github.com/jcmturner/gokrb5/v8/types"
	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// testKDC stellt Tickets für host/<name> aus, als käme es vom KDC.
type testKDC struct {
	kt    *keytab.Keytab
	sname types.PrincipalName
}

func newTestKDC(t *testing.T, host string) *testKDC {
	kt := keytab.New()
	if err := kt.AddEntry(KerberosSPN(host), "EXAMPLE.ORG", "dienst-geheim", time.Now(), 1, etypeID.AES256_CTS_HMAC_SHA1_96); err != nil {
		t.Fatal(err)
	}
	return &testKDC{kt: kt, sname: types.NewPrincipalName(nametype.KRB_NT_SRV_HST, KerberosSPN(host))}
}

func (k *testKDC) ticket(spn string) (messages.Ticket, types.EncryptionKey, string, types.PrincipalName, error) {
	if spn != k.sname.PrincipalNameString() {
		return messages.Ticket{}, types.EncryptionKey{}, "", types.PrincipalName{}, errors.New("unbekannter Dienst " + spn)
	}
	cname := types.NewPrincipalName(nametype.KRB_NT_PRINCIPAL, "dominik")
	now := time.Now()
	tkt, key, err := messages.NewTicket(cname, "EXAMPLE.ORG", k.sname, "EXAMPLE.ORG", asn1.BitString{Bytes: make([]byte, 4), BitLength: 32},
		k.kt, etypeID.AES256_CTS_HMAC_SHA1_96, 1, now, now, now.Add(time.Hour), now.Add(time.Hour))
	return tkt, key, "EXAMPLE.ORG", cname, err
}

// gssServer prüft wie sshd: AP-REQ mit dem Dienstschlüssel entschlüsseln,
// MIC mit dem Sitzungsschlüssel und der Folgenummer des Authenticators.
type gssServer struct {
	kdc *testKDC
	key types.EncryptionKey
	seq int64
	who string
}

func (s *gssServer) AcceptSecContext(token []byte) ([]byte, string, bool, error) {
	var kt spnego.KRB5Token
	if err := kt.Unmarshal(token); err != nil {
		return nil, "", false, err
	}
	if !kt.IsAPReq() {
		return nil, "", false, errors.New("kein AP-REQ")
	}
	req := kt.APReq
	if err := req.Ticket.DecryptEncPart(s.kdc.kt, &s.kdc.sname); err != nil {
		return nil, "", false, err
	}
	s.key = req.Ticket.DecryptedEncPart.Key
	if err := req.DecryptAuthenticator(s.key); err != nil {
		return nil, "", false, err
	}
	s.seq = req.Authenticator.SeqNumber
	s.who = req.Authenticator.CName.PrincipalNameString() + "@" + req.Authenticator.CRealm
	return nil, s.who, false, nil
}

func (s *gssServer) VerifyMIC(micField, micToken []byte) error {
	var mt gssapi.MICToken
	if err := mt.Unmarshal(micToken, false); err != nil {
		return err
	}
	if mt.SndSeqNum != uint64(s.seq) {
		return errors.New("falsche Folgenummer")
	}
	mt.Payload = micField
	ok, err := mt.Verify(s.key, keyusage.GSSAPI_INITIATOR_SIGN)
	if !ok {
		return errors.New("MIC falsch: " + errStr(err))
	}
	return nil
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *gssServer) DeleteSecContext() error { return nil }

func TestKerberosLogin(t *testing.T) {
	kdc := newTestKDC(t, "srv.example.org")
	srv := &gssServer{kdc: kdc}
	_, hpriv, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hpriv)
	logged := make(chan string, 1)
	cfg := &ssh.ServerConfig{GSSAPIWithMICConfig: &ssh.GSSAPIWithMICConfig{
		AllowLogin: func(_ ssh.ConnMetadata, who string) (*ssh.Permissions, error) { logged <- who; return nil, nil },
		Server:     srv,
	}}
	cfg.AddHostKey(hs)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
		if err != nil {
			return
		}
		defer sc.Close()
		go ssh.DiscardRequests(reqs)
		for nch := range chans {
			nch.Reject(ssh.Prohibited, "nur Login")
		}
	}()

	// Client wie in auth.go, nur mit dem Test-KDC als Ticketquelle
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	gss := &krbGSS{ticket: kdc.ticket}
	if err := gss.prepare(KerberosSPN("SRV.example.org.")); err != nil {
		t.Fatal(err)
	}
	ccfg := &ssh.ClientConfig{User: "dominik", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
		Auth: []ssh.AuthMethod{ssh.GSSAPIWithMICAuthMethod(gss, "SRV.example.org.")}}
	cl, err := ssh.Dial("tcp", "127.0.0.1:"+port, ccfg)
	if err != nil {
		t.Fatalf("Anmeldung: %v", err)
	}
	cl.Close()
	if who := <-logged; who != "dominik@EXAMPLE.ORG" {
		t.Fatalf("angemeldet als %q", who)
	}
}

// Ohne Ticket: Kerberos scheitert leise, danach kommt das Passwort.
func TestKerberosFallsBackToPassword(t *testing.T) {
	t.Setenv("KRB5CCNAME", filepath.Join(t.TempDir(), "gibts-nicht"))
	_, hpriv, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hpriv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if string(p) == "pw" {
			return nil, nil
		}
		return nil, errors.New("nein")
	}, GSSAPIWithMICConfig: &ssh.GSSAPIWithMICConfig{AllowLogin: func(ssh.ConnMetadata, string) (*ssh.Permissions, error) { return nil, nil }, Server: &gssServer{}}}
	cfg.AddHostKey(hs)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
		if err != nil {
			return
		}
		defer sc.Close()
		go ssh.DiscardRequests(reqs)
		for nch := range chans {
			nch.Reject(ssh.Prohibited, "")
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("HOME", t.TempDir())
	hk, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	c := domain.Connection{Alias: "srv", Host: host, Port: p, Username: "u", Auth: domain.AuthKerberos, Device: domain.DeviceNetwork}
	asked := false
	cl, err := Dial(context.Background(), c, Options{HostKeys: hk, Prompt: func(string, string, string) bool { return true },
		AskSecret: func(string) (string, error) { asked = true; return "pw", nil }, Timeout: 5 * time.Second, waitScale: 0.05})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	cl.Close()
	if !asked {
		t.Fatal("Passwort nicht gefragt")
	}
}
