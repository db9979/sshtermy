//go:build !windows

package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/db9979/sshtermy/internal/domain"
)

// Der Server bittet über die weitergeleitete Verbindung den lokalen Agent
// um seine Keys: Er muss genau den Key aus dem lokalen Agent sehen, darf
// aber nichts hinzufügen.
func TestAgentForwarding(t *testing.T) {
	// lokaler Agent mit einem Key
	_, kpriv, _ := ed25519.GenerateKey(rand.Reader)
	ring := agent.NewKeyring()
	if err := ring.Add(agent.AddedKey{PrivateKey: kpriv, Comment: "mein-key"}); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(t.TempDir(), "agent.sock")
	al, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("kein Unix-Socket:", err)
	}
	defer al.Close()
	go func() {
		for {
			c, err := al.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(ring, c); c.Close() }()
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", sock)

	// Server, der die Weiterleitung annimmt und dann die Keys abfragt
	_, hpriv, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hpriv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(hs)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	type result struct {
		keys   []*agent.Key
		addErr error
		err    error
	}
	got := make(chan result, 1)
	go func() {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
		if err != nil {
			got <- result{err: err}
			return
		}
		defer sc.Close()
		go ssh.DiscardRequests(reqs)
		for nch := range chans {
			ch, creqs, err := nch.Accept()
			if err != nil {
				return
			}
			go func() {
				for r := range creqs {
					switch r.Type {
					case "auth-agent-req@openssh.com", "pty-req", "env":
						r.Reply(true, nil)
					case "shell":
						r.Reply(true, nil)
						go func() {
							ac, areqs, err := sc.OpenChannel("auth-agent@openssh.com", nil)
							if err != nil {
								got <- result{err: err}
								return
							}
							go ssh.DiscardRequests(areqs)
							cl := agent.NewClient(ac)
							keys, err := cl.List()
							_, kp2, _ := ed25519.GenerateKey(rand.Reader)
							addErr := cl.Add(agent.AddedKey{PrivateKey: kp2})
							got <- result{keys: keys, addErr: addErr, err: err}
							ac.Close()
							ch.Write([]byte("$ "))
						}()
					default:
						r.Reply(false, nil)
					}
				}
			}()
		}
	}()

	hk, _ := NewHostKeyStore(filepath.Join(t.TempDir(), "known_hosts"))
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	c := domain.Connection{Alias: "srv", Host: host, Port: p, Username: "u", Auth: domain.AuthPassword,
		Password: "x", Device: domain.DeviceServer, ForwardAgent: true}
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
	case r := <-got:
		if r.err != nil {
			t.Fatalf("Server: %v", r.err)
		}
		if len(r.keys) != 1 || r.keys[0].Comment != "mein-key" {
			t.Fatalf("Keys über Weiterleitung = %v", r.keys)
		}
		if r.addErr == nil {
			t.Fatal("Server konnte einen Key zum lokalen Agent hinzufügen")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("keine Antwort über die Weiterleitung")
	}
	if n := cl.AgentNote(); n != "" {
		t.Fatalf("Hinweis = %q", n)
	}
}
