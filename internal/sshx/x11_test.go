//go:build !windows

package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

func x11Setup(name string, data []byte) []byte {
	pad := func(n int) int { return (n + 3) &^ 3 }
	b := make([]byte, 12)
	b[0] = 'l'
	binary.LittleEndian.PutUint16(b[2:], 11)
	binary.LittleEndian.PutUint16(b[6:], uint16(len(name)))
	binary.LittleEndian.PutUint16(b[8:], uint16(len(data)))
	b = append(b, name...)
	b = append(b, make([]byte, pad(len(name))-len(name))...)
	b = append(b, data...)
	b = append(b, make([]byte, pad(len(data))-len(data))...)
	return b
}

func TestRewriteX11Setup(t *testing.T) {
	fake, real := bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 16)
	out, err := rewriteX11Setup(bytes.NewReader(x11Setup(x11Proto, fake)), fake, real)
	if err != nil || !bytes.Equal(out, x11Setup(x11Proto, real)) {
		t.Fatalf("Ersetzen: %v %x", err, out)
	}
	out, err = rewriteX11Setup(bytes.NewReader(x11Setup(x11Proto, fake)), fake, nil)
	if err != nil || !bytes.Equal(out, x11Setup("", nil)) {
		t.Fatalf("ohne Cookie: %v %x", err, out)
	}
	if _, err := rewriteX11Setup(bytes.NewReader(x11Setup(x11Proto, real)), fake, real); err == nil {
		t.Fatal("falsches Schein-Cookie angenommen")
	}
}

func TestParseDisplay(t *testing.T) {
	for d, want := range map[string]string{
		":0":                "unix /tmp/.X11-unix/X0",
		":1.0":              "unix /tmp/.X11-unix/X1",
		"localhost:10.0":    "tcp localhost:6010",
		"/private/tmp/xq:0": "unix /private/tmp/xq:0",
		"[::1]:2":           "tcp [::1]:6002",
	} {
		n, a, err := parseDisplay(d)
		if err != nil || n+" "+a != want {
			t.Errorf("%q: %s %s %v", d, n, a, err)
		}
	}
	if displayScreen("localhost:10.2") != 2 || displayScreen(":0") != 0 {
		t.Fatal("Bildschirmnummer")
	}
	if c := parseXauthList([]byte("host/unix:0  MIT-MAGIC-COOKIE-1  00ff10\n")); hex.EncodeToString(c) != "00ff10" {
		t.Fatalf("xauth: %x", c)
	}
}

// Ende zu Ende: Server bekommt x11-req mit Schein-Cookie, öffnet einen
// x11-Kanal, der lokale „X-Server“ sieht das echte Cookie und antwortet.
func TestX11Forwarding(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "xq:5") // wie XQuartz: Socket heißt wie das Display
	xl, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("kein Unix-Socket:", err)
	}
	defer xl.Close()
	gotSetup := make(chan []byte, 1)
	go func() {
		c, err := xl.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		b := make([]byte, 12) // ohne echtes Cookie: nur der Kopf
		_, _ = io.ReadFull(c, b)
		gotSetup <- b
		c.Write([]byte("HALLO-X"))
	}()
	// Display auf den Test-Socket: absoluter Pfad vor dem „:“
	t.Setenv("DISPLAY", sock)
	t.Setenv("PATH", dir) // kein xauth: ohne echtes Cookie weiterreichen

	_, hpriv, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hpriv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(hs)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	reply := make(chan string, 1)
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
			ch, creqs, _ := nch.Accept()
			go func() {
				var cookie []byte
				for r := range creqs {
					switch r.Type {
					case "x11-req":
						var p struct {
							Single bool
							Proto  string
							Cookie string
							Screen uint32
						}
						if ssh.Unmarshal(r.Payload, &p) != nil || p.Proto != x11Proto {
							r.Reply(false, nil)
							continue
						}
						cookie, _ = hex.DecodeString(p.Cookie)
						r.Reply(true, nil)
					case "shell":
						r.Reply(true, nil)
						go func() {
							xc, xr, err := sc.OpenChannel("x11", ssh.Marshal(&struct {
								Addr string
								Port uint32
							}{"127.0.0.1", 4711}))
							if err != nil {
								reply <- "open: " + err.Error()
								return
							}
							go ssh.DiscardRequests(xr)
							xc.Write(x11Setup(x11Proto, cookie))
							b := make([]byte, 7)
							_, _ = io.ReadFull(xc, b)
							reply <- string(b)
							ch.Write([]byte("$ "))
						}()
					default:
						r.Reply(true, nil)
					}
				}
			}()
		}
	}()

	hk, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	c := domain.Connection{Alias: "srv", Host: host, Port: p, Username: "u", Auth: domain.AuthPassword,
		Password: "x", Device: domain.DeviceServer, ForwardX11: true}
	cl, err := Dial(context.Background(), c, Options{HostKeys: hk, Prompt: func(string, string, string) bool { return true },
		Timeout: 5 * time.Second, waitScale: 0.05})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer cl.Close()
	var buf safeBuf
	sh, err := cl.OpenShell(80, 24, &buf)
	if err != nil {
		t.Fatalf("OpenShell: %v", err)
	}
	defer sh.Close()
	select {
	case r := <-reply:
		if r != "HALLO-X" {
			t.Fatalf("Antwort %q", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("keine Antwort vom X-Server")
	}
	if s := <-gotSetup; !bytes.Equal(s, x11Setup("", nil)) {
		t.Fatalf("X-Server sah %x", s)
	}
	if n := cl.X11Note(); n != "" {
		t.Fatalf("Hinweis %q", n)
	}
}
