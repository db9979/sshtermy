package relay

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// testSSHD: kleiner SSH-Server (Passwort „pw“), der auf „exec“ den Befehl zurückgibt.
func testSSHD(t *testing.T) (addr string, hostKey ssh.PublicKey) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
		if string(pw) == "pw" {
			return nil, nil
		}
		return nil, errStop
	}}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nc := range chans {
					ch, rq, _ := nc.Accept()
					go func() {
						for r := range rq {
							if r.Type == "exec" {
								_ = r.Reply(true, nil)
								_, _ = ch.Write([]byte("echo:" + string(r.Payload[4:])))
								_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								ch.Close()
							}
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String(), signer.PublicKey()
}

func TestParseLine(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	p, err := ParseLine("  sshtermy anker koppeln 'https://relay.example.org:8022/x/#termy=ABCDEF2345&fp=" + fp + "'")
	if err != nil {
		t.Fatal(err)
	}
	if p.Base != "https://relay.example.org:8022/x" || p.Code != "ABCDEF2345" || p.FP != fp {
		t.Fatalf("%+v", p)
	}
	if _, err := ParseLine("http://relay#termy=ABCDEF2345"); err == nil {
		t.Fatal("http darf nicht gehen")
	}
	if _, err := ParseLine("https://relay#termy=x"); err == nil {
		t.Fatal("zu kurzer Code")
	}
	if _, err := ParseLine("https://u:p@relay#termy=ABCDEF2345"); err == nil {
		t.Fatal("Benutzer in der URL")
	}
}

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	sshdAddr, hk := testSSHD(t)

	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv.Logf = testLogf(t)
	srv.OpenWait = 5 * time.Second
	ts := httptest.NewTLSServer(srv.Handler())
	defer ts.Close()
	fp := CertFP(ts.Certificate().Raw)
	st, _ := LoadState(dir)
	st.Name, st.URL, st.FP = "Testvermittler", ts.URL, fp
	if err := SaveState(dir, st); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Watch(ctx)

	// Falscher Pin: nichts geht
	bad, _ := NewClient(ts.URL, "", strings.Repeat("00", 32))
	if _, err := bad.Pair(ctx, "XXXXXXXX", "x"); err == nil || !strings.Contains(err.Error(), "Fingerabdruck") {
		t.Fatalf("falscher Pin: %v", err)
	}

	// Anker koppeln
	line, err := Line(dir, CodeAnchor)
	if err != nil {
		t.Fatal(err)
	}
	pl, _ := ParseLine(line)
	acfg, err := PairAnchor(ctx, pl, "web-01", sshdAddr)
	if err != nil {
		t.Fatal(err)
	}
	// Code gilt nur einmal
	if _, err := PairAnchor(ctx, pl, "web-02", sshdAddr); err == nil {
		t.Fatal("Code zweimal eingelöst")
	}
	// Code für Anker taugt nicht für Geräte
	l2, _ := Line(dir, CodeAnchor)
	p2, _ := ParseLine(l2)
	c0, _ := NewClient(p2.Base, "", p2.FP)
	if _, err := c0.Pair(ctx, p2.Code, "laptop"); err == nil {
		t.Fatal("Anker-Code als Geräte-Code")
	}

	an := &Anchor{Cfg: acfg, Logf: testLogf(t)}
	go an.Run(ctx)

	// Gerät koppeln
	cl3, _ := Line(dir, CodeClient)
	p3, _ := ParseLine(cl3)
	cl, _ := NewClient(p3.Base, "", p3.FP)
	pr, err := cl.Pair(ctx, p3.Code, "laptop")
	if err != nil || pr.Name != "Testvermittler" || pr.Kind != KindSSHTermy {
		t.Fatalf("%v %+v", err, pr)
	}

	// Liste: System online mit Host-Key
	var sys System
	for i := 0; i < 50; i++ {
		l, err := cl.Systems(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Systems) == 1 && l.Systems[0].Online {
			sys = l.Systems[0]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !sys.Online || sys.Name != "web-01" {
		t.Fatalf("System nicht online: %+v", sys)
	}
	if sys.HostKey != strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hk))) {
		t.Fatalf("Host-Key: %q", sys.HostKey)
	}

	// SSH Ende-zu-Ende durch den Vermittler
	nc, err := cl.Dial(ctx, sys.ID, "web-01.vermittler:22")
	if err != nil {
		t.Fatal(err)
	}
	if nc.RemoteAddr().String() != "web-01.vermittler:22" {
		t.Fatal(nc.RemoteAddr())
	}
	sc, chans, reqs, err := ssh.NewClientConn(nc, "web-01.vermittler:22", &ssh.ClientConfig{
		User: "x", Auth: []ssh.AuthMethod{ssh.Password("pw")}, HostKeyCallback: ssh.FixedHostKey(hk)})
	if err != nil {
		t.Fatal(err)
	}
	c := ssh.NewClient(sc, chans, reqs)
	sess, _ := c.NewSession()
	out, err := sess.Output("hallo")
	if err != nil || string(out) != "echo:hallo" {
		t.Fatalf("%q %v", out, err)
	}
	c.Close()

	// Unbekanntes System, falscher Schlüssel
	if _, err := cl.Dial(ctx, "a_000000000000", "x:22"); err == nil {
		t.Fatal("unbekanntes System")
	}
	other, _ := NewClient(ts.URL, "st_falsch", fp)
	if _, err := other.Systems(ctx); err == nil {
		t.Fatal("falscher Schlüssel")
	} else if se, ok := err.(*StatusError); !ok || se.Code != http.StatusUnauthorized {
		t.Fatal(err)
	}

	// Entfernen trennt den Anker; danach ist das System weg
	if err := RemovePeer(dir, acfg.ID); err != nil {
		t.Fatal(err)
	}
	l, err := cl.Systems(ctx)
	if err != nil || len(l.Systems) != 0 {
		t.Fatalf("nach Entfernen: %v %+v", err, l)
	}
	// Gerät entfernen: Schlüssel gilt nicht mehr
	rows, _ := ListPeers(dir)
	for _, r := range rows {
		if r.Kind == CodeClient {
			_ = RemovePeer(dir, r.ID)
		}
	}
	if _, err := cl.Systems(ctx); err == nil {
		t.Fatal("entferntes Gerät hat noch Zugriff")
	}
}

func TestCodeExpires(t *testing.T) {
	dir := t.TempDir()
	code, err := NewCode(dir, CodeClient)
	if err != nil {
		t.Fatal(err)
	}
	if takeCode(dir, code, CodeClient, time.Now().Add(CodeTTL+time.Second)) {
		t.Fatal("abgelaufener Code gilt")
	}
	code, _ = NewCode(dir, CodeClient)
	if !takeCode(dir, strings.ToLower(code), CodeClient, time.Now()) {
		t.Fatal("Code gilt nicht (Kleinschreibung)")
	}
	if takeCode(dir, code, CodeClient, time.Now()) {
		t.Fatal("Code zweimal")
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter()
	for i := 0; i < 3; i++ {
		if !l.allow("k", 3, time.Minute) {
			t.Fatal(i)
		}
	}
	if l.allow("k", 3, time.Minute) {
		t.Fatal("4. erlaubt")
	}
}
