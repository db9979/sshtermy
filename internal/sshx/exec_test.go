package sshx

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// startExecServer: Server mit Exec (echo/fail) und einer Switch-artigen
// Shell (Tastendruck-Banner, Seiten-Pause bei „show long“).
func startExecServer(t *testing.T) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
		ServerVersion: "SSH-2.0-OpenSSH_9.6"}
	cfg.AddHostKey(signer)
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
				defer sc.Close()
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					ch, creqs, _ := nch.Accept()
					go serveExecChan(ch, creqs)
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func exitStatus(ch ssh.Channel, code uint32) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, code)
	ch.SendRequest("exit-status", false, b)
	ch.Close()
}

func serveExecChan(ch ssh.Channel, reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "exec":
			r.Reply(true, nil)
			cmd := string(r.Payload[4:])
			switch {
			case strings.HasPrefix(cmd, "echo "):
				ch.Write([]byte(strings.TrimPrefix(cmd, "echo ") + "\n"))
				exitStatus(ch, 0)
			case cmd == "sleep":
				time.Sleep(5 * time.Second)
				exitStatus(ch, 0)
			default:
				ch.Stderr().Write([]byte("boom\n"))
				exitStatus(ch, 3)
			}
			return
		case "pty-req":
			r.Reply(true, nil)
		case "shell":
			r.Reply(true, nil)
			go fakeCLI(ch)
		default:
			r.Reply(false, nil)
		}
	}
}

func fakeCLI(ch ssh.Channel) {
	in := bufio.NewReader(ch)
	ch.Write([]byte("\x1b[2JAruba 2930F\r\nPress any key to continue"))
	in.ReadByte()
	ch.Write([]byte("\r\nsw# "))
	var line []byte
	for {
		b, err := in.ReadByte()
		if err != nil {
			return
		}
		if b != '\r' {
			line = append(line, b)
			ch.Write([]byte{b}) // Echo
			continue
		}
		cmd := string(line)
		line = nil
		ch.Write([]byte("\r\n"))
		switch cmd {
		case "show long":
			ch.Write([]byte("zeile 1\r\nzeile 2\r\n-- MORE --, next page: Space"))
			if k, _ := in.ReadByte(); k != ' ' {
				ch.Write([]byte("kein Leerzeichen!"))
			}
			ch.Write([]byte("\r                             \rzeile 3\r\nsw# "))
		default:
			ch.Write([]byte("out:" + cmd + "\r\nsw# "))
		}
	}
}

func dialAddr(t *testing.T, addr string, dev domain.DeviceKind) *Client {
	t.Helper()
	hs, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	host, port, _ := net.SplitHostPort(addr)
	p, _ := strconv.Atoi(port)
	cl, err := Dial(context.Background(), domain.Connection{Alias: "x", Host: host, Port: p, Username: "u",
		Auth: domain.AuthPassword, Password: "x", Device: dev}, Options{HostKeys: hs,
		Prompt: func(string, string, string) bool { return true }, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

func TestExec(t *testing.T) {
	cl := dialAddr(t, startExecServer(t), domain.DeviceServer)
	r, err := cl.Exec(context.Background(), "echo hallo")
	if err != nil || r.Exit != 0 || string(r.Output) != "hallo\n" {
		t.Fatalf("echo: %+v %v", r, err)
	}
	r, err = cl.Exec(context.Background(), "kaputt")
	if err != nil || r.Exit != 3 || string(r.Output) != "boom\n" {
		t.Fatalf("Fehler-Exit: %+v %q %v", r, r.Output, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := cl.Exec(ctx, "sleep"); err == nil {
		t.Fatal("Abbruch erwartet")
	}
}

func TestRunInShell(t *testing.T) {
	cl := dialAddr(t, startExecServer(t), domain.DeviceNetwork)
	r, err := cl.RunInShell(context.Background(), []string{"show version", "show long"},
		ShellTiming{Quiet: 300 * time.Millisecond, Login: 3 * time.Second, PerStep: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	out := string(r.Output)
	for _, want := range []string{"show version", "out:show version", "zeile 1", "zeile 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("fehlt %q in:\n%s", want, out)
		}
	}
	for _, bad := range []string{"MORE", "\x1b", "\r", "kein Leerzeichen", "Press any key"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q sollte nicht in der Ausgabe stehen:\n%q", bad, out)
		}
	}
	if r.Exit != -1 {
		t.Errorf("Exit = %d, erwartet -1", r.Exit)
	}
}
