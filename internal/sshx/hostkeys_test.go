package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
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
	if err := hs.Callback(nil)("10.0.0.1:22", remote, testKey(t)); err != ErrHostKeyChanged {
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
