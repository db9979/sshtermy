package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// stallServer beantwortet die ersten answer Keepalives und dann keine mehr
// (wie eine Leitung, die mitten in der Sitzung stumm wird).
type stallServer struct {
	addr    string
	answer  int32
	seen    atomic.Int32
	conns   chan net.Conn
	openTCP chan net.Conn
}

func startStallServer(t *testing.T, answer int32) *stallServer {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
		ServerVersion:    "SSH-2.0-OpenSSH_9.6",
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s := &stallServer{addr: ln.Addr().String(), answer: answer, openTCP: make(chan net.Conn, 4)}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			s.openTCP <- nc
			go func() {
				sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				defer sc.Close()
				go func() {
					for r := range reqs {
						if s.seen.Add(1) <= s.answer && r.WantReply {
							r.Reply(false, nil)
						}
					}
				}()
				for nch := range chans {
					nch.Reject(ssh.Prohibited, "nein")
				}
			}()
		}
	}()
	return s
}

func dialStall(t *testing.T, s *stallServer) *Client {
	t.Helper()
	hs, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	host, port, _ := net.SplitHostPort(s.addr)
	p, _ := strconv.Atoi(port)
	c := domain.Connection{Alias: "x", Host: host, Port: p, Username: "u", Auth: domain.AuthPassword,
		Password: "x", Device: domain.DeviceServer}
	cl, err := Dial(context.Background(), c, Options{HostKeys: hs, Prompt: func(string, string, string) bool { return true }, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return cl
}

func TestDroppedWhenPeerGoesAway(t *testing.T) {
	s := startStallServer(t, 100)
	cl := dialStall(t, s)
	defer cl.Close()
	if cl.Dropped() {
		t.Fatal("frische Verbindung gilt als abgebrochen")
	}
	(<-s.openTCP).Close() // Server/Netz weg
	if !cl.WaitDropped(3 * time.Second) {
		t.Fatal("Abbruch nicht erkannt")
	}
}

func TestCloseIsNotADrop(t *testing.T) {
	s := startStallServer(t, 100)
	cl := dialStall(t, s)
	cl.Close()
	if cl.WaitDropped(500 * time.Millisecond) {
		t.Fatal("eigenes Close darf nicht als Abbruch zählen")
	}
}

func TestKeepAliveDetectsSilentLine(t *testing.T) {
	s := startStallServer(t, 1)
	cl := dialStall(t, s)
	defer cl.Close()
	cl.cancel() // Standard-Keepalive (10 s) stoppen, schnelle Variante testen
	time.Sleep(50 * time.Millisecond)
	s.seen.Store(0)
	go cl.keepAliveLoop(context.Background(), 30*time.Millisecond, 300*time.Millisecond)
	if !cl.WaitDropped(3 * time.Second) {
		t.Fatal("stumme Leitung nicht erkannt")
	}
	if cl.Latency() <= 0 {
		t.Fatal("Latenz der ersten Antwort fehlt")
	}
}

func TestKeepAliveKeepsNeverAnsweringDevice(t *testing.T) {
	// Gegenstelle antwortet nie auf Keepalives (manche Switches): nicht trennen.
	s := startStallServer(t, 0)
	cl := dialStall(t, s)
	defer cl.Close()
	cl.cancel()
	go cl.keepAliveLoop(context.Background(), 20*time.Millisecond, 100*time.Millisecond)
	if cl.WaitDropped(600 * time.Millisecond) {
		t.Fatal("Verbindung ohne Keepalive-Antworten wurde getrennt")
	}
}

func TestEncoding(t *testing.T) {
	for in, want := range map[string]string{
		"de_DE.UTF-8": "UTF-8", "en_US.utf8": "UTF-8", "C": "ASCII", "": "",
		"de_DE.ISO-8859-1": "ISO-8859-1", "de_DE.UTF-8@euro": "UTF-8", "C.UTF-8": "UTF-8",
	} {
		if got := (RemoteInfo{Locale: in}).Encoding(); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestPlainName(t *testing.T) {
	for _, n := range []string{"a.txt", ".bashrc", "x y", "1:2.log"} {
		if !plainName(n) {
			t.Errorf("plainName(%q) = false", n)
		}
	}
	for _, n := range []string{"", ".", "..", "../x", "a/b", `..\..\x`, "/etc/passwd"} {
		if plainName(n) {
			t.Errorf("plainName(%q) = true", n)
		}
	}
}
