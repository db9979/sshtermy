package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

func testKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// Während der Dialog "Unbekannter Host" offen ist, muss der Store für andere
// (UI-Thread, Sync) benutzbar bleiben. Sonst friert die Oberfläche ein.
func TestCallbackPromptDoesNotHoldLock(t *testing.T) {
	hs, err := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	key := testKey(t)
	remote := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	asked := make(chan struct{})
	answer := make(chan bool)
	done := make(chan error, 1)
	cb := hs.Callback(func(host, fp, typ string) bool {
		close(asked)
		return <-answer
	})
	go func() { done <- cb("10.0.0.1:22", remote, key) }()
	<-asked

	free := make(chan struct{})
	go func() {
		hs.Lines()
		hs.Algorithms("10.0.0.1:22")
		_, _ = hs.AddLines(nil)
		close(free)
	}()
	select {
	case <-free:
	case <-time.After(2 * time.Second):
		t.Fatal("Store blockiert, solange die Host-Key-Rückfrage offen ist")
	}

	answer <- true
	if err := <-done; err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if len(hs.Lines()) != 1 {
		t.Fatalf("Schlüssel nicht gespeichert: %v", hs.Lines())
	}
	// Zweiter Aufruf: bekannt, keine Rückfrage.
	if err := hs.Callback(func(string, string, string) bool {
		t.Fatal("erneute Rückfrage bei bekanntem Host")
		return false
	})("10.0.0.1:22", remote, key); err != nil {
		t.Fatal(err)
	}
	// Anderer Schlüssel: Warnung statt Rückfrage.
	if err := hs.Callback(nil)("10.0.0.1:22", remote, testKey(t)); !errors.Is(err, ErrHostKeyChanged) {
		t.Fatalf("erwartet ErrHostKeyChanged, bekam %v", err)
	}
}

func TestCallbackReject(t *testing.T) {
	hs, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	err := hs.Callback(func(string, string, string) bool { return false })("h:22", &net.TCPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 22}, testKey(t))
	if err == nil || len(hs.Lines()) != 0 {
		t.Fatalf("Ablehnen: err=%v lines=%v", err, hs.Lines())
	}
}

// Wer im Host-Key-Dialog länger überlegt als die Handshake-Frist, darf danach
// nicht mit "i/o timeout" rausfliegen.
func TestSlowHostKeyAnswerKeepsHandshake(t *testing.T) {
	fs := startFakeSwitch(t, "SSH-2.0-OpenSSH_9.6")
	hs, err := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(fs.addr)
	p, _ := strconv.Atoi(port)
	c := domain.Connection{Alias: "srv", Host: host, Port: p, Username: "u",
		Auth: domain.AuthPassword, Password: "x", Device: domain.DeviceServer}
	cl, err := Dial(context.Background(), c, Options{
		HostKeys: hs,
		Prompt: func(string, string, string) bool {
			time.Sleep(700 * time.Millisecond) // länger als Timeout
			return true
		},
		Timeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Dial nach langsamer Antwort: %v", err)
	}
	cl.Close()
}

// Geänderter Schlüssel: Fehler trägt alten und neuen Fingerprint, Replace
// tauscht nur die Zeilen dieses Hosts aus.
func TestHostKeyChangedReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	hs, err := NewHostKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	old, other, fresh := testKey(t), testKey(t), testKey(t)
	remote := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 22}
	yes := func(string, string, string) bool { return true }
	if err := hs.Callback(yes)("sw1:22", remote, old); err != nil {
		t.Fatal(err)
	}
	otherRemote := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 22}
	if err := hs.Callback(yes)("sw2:22", otherRemote, other); err != nil {
		t.Fatal(err)
	}
	err = hs.Callback(yes)("sw1:22", remote, fresh)
	var hc *HostKeyChangedError
	if !errors.As(err, &hc) || !errors.Is(err, ErrHostKeyChanged) {
		t.Fatalf("erwartet HostKeyChangedError, bekam %v", err)
	}
	if len(hc.Old) == 0 || !strings.Contains(hc.Old[0], ssh.FingerprintSHA256(old)) || hc.Fingerprint() != ssh.FingerprintSHA256(fresh) {
		t.Fatalf("Fingerprints falsch: %+v", hc)
	}
	n, err := hs.Replace(hc.Host, hc.Remote, hc.Key)
	if err != nil || n == 0 {
		t.Fatalf("Replace: %d %v", n, err)
	}
	if err := hs.Callback(nil)("sw1:22", remote, fresh); err != nil {
		t.Fatalf("neuer Schlüssel nicht übernommen: %v", err)
	}
	if err := hs.Callback(nil)("sw1:22", remote, old); err == nil {
		t.Fatal("alter Schlüssel gilt noch")
	}
	if err := hs.Callback(nil)("sw2:22", otherRemote, other); err != nil {
		t.Fatalf("anderer Host verloren: %v", err)
	}
}
