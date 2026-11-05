// Package sshx ist die SSH-Schicht: Verbindungsaufbau (inkl. Jump-Host),
// Authentifizierung, Host-Key-Prüfung, PTY-Sitzungen und Port-Forwarding.
// Basis ist golang.org/x/crypto/ssh (reines Go, kein cgo, daher problemlos für
// windows/arm64, windows/386 und darwin cross-kompilierbar).
package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// Resolver liefert eine gespeicherte Verbindung (für Jump-Hosts).
type Resolver func(id string) (domain.Connection, bool)

// Client ist eine offene SSH-Verbindung samt optionalem Jump-Host.
type Client struct {
	*ssh.Client
	Conn   domain.Connection
	jump   *Client
	Remote RemoteInfo
	cancel context.CancelFunc
}

// RemoteInfo: was wir über das Zielsystem wissen (Kontext fürs LLM).
type RemoteInfo struct {
	OS     string // z. B. "Ubuntu 24.04.1 LTS"
	Kernel string // uname -srm
	Shell  string // $SHELL
}

func (r RemoteInfo) String() string {
	parts := []string{}
	for _, p := range []string{r.OS, r.Kernel, r.Shell} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "unbekannt"
	}
	return strings.Join(parts, " · ")
}

// Options für Dial.
type Options struct {
	HostKeys  *HostKeyStore
	Prompt    HostKeyPrompt
	Resolve   Resolver
	AskSecret SecretPrompt // fehlende Passwörter/Passphrasen erfragen (optional)
	Timeout   time.Duration
}

// Dial baut die Verbindung auf. Bei JumpHostID wird zuerst der Jump-Host
// verbunden und die Zielverbindung durch ihn getunnelt (wie ssh -J).
func Dial(ctx context.Context, c domain.Connection, o Options) (*Client, error) {
	if o.Timeout == 0 {
		o.Timeout = 15 * time.Second
	}
	cfg, err := clientConfig(c, o)
	if err != nil {
		return nil, err
	}

	var (
		raw  net.Conn
		jump *Client
	)
	if c.JumpHostID != "" {
		if o.Resolve == nil {
			return nil, errors.New("Jump-Host kann nicht aufgelöst werden")
		}
		jc, ok := o.Resolve(c.JumpHostID)
		if !ok {
			return nil, errors.New("Jump-Host nicht gefunden")
		}
		if jc.JumpHostID == c.ID {
			return nil, errors.New("Jump-Host-Schleife")
		}
		jump, err = Dial(ctx, jc, o)
		if err != nil {
			return nil, fmt.Errorf("Jump-Host %s: %w", jc.Alias, err)
		}
		raw, err = jump.Dial("tcp", c.Addr())
	} else {
		d := net.Dialer{Timeout: o.Timeout, KeepAlive: 30 * time.Second}
		raw, err = d.DialContext(ctx, "tcp", c.Addr())
	}
	if err != nil {
		if jump != nil {
			jump.Close()
		}
		return nil, err
	}
	_ = raw.SetDeadline(time.Now().Add(o.Timeout)) // nur für den Handshake
	sc, chans, reqs, err := ssh.NewClientConn(raw, c.Addr(), cfg)
	if err != nil {
		raw.Close()
		if jump != nil {
			jump.Close()
		}
		return nil, err
	}
	_ = raw.SetDeadline(time.Time{})

	kctx, cancel := context.WithCancel(context.Background())
	cl := &Client{Client: ssh.NewClient(sc, chans, reqs), Conn: c, jump: jump, cancel: cancel}
	go cl.keepAlive(kctx, c.KeepAlive)
	cl.Remote = cl.probe()
	return cl, nil
}

// Close schließt Ziel und Jump-Host.
func (c *Client) Close() error {
	c.cancel()
	err := c.Client.Close()
	if c.jump != nil {
		c.jump.Close()
	}
	return err
}

func (c *Client) keepAlive(ctx context.Context, sec int) {
	if sec <= 0 {
		sec = 30
	}
	t := time.NewTicker(time.Duration(sec) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, _, err := c.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				return
			}
		}
	}
}

// probe fragt Betriebssystem, Kernel und Shell ab, in einer eigenen
// Exec-Sitzung, also unsichtbar für das Terminal des Nutzers.
func (c *Client) probe() RemoteInfo {
	s, err := c.NewSession()
	if err != nil {
		return RemoteInfo{}
	}
	defer s.Close()
	const cmd = `(. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME") || sw_vers -productName 2>/dev/null || echo; uname -srm 2>/dev/null || echo; echo "$SHELL"`
	done := make(chan []byte, 1)
	go func() { out, _ := s.Output(cmd); done <- out }()
	select {
	case out := <-done:
		lines := strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n")
		get := func(i int) string {
			if i < len(lines) {
				return strings.TrimSpace(lines[i])
			}
			return ""
		}
		ri := RemoteInfo{OS: get(0), Kernel: get(1), Shell: get(2)}
		if ri.OS == "" && ri.Kernel == "" {
			ri.OS = "Windows (OpenSSH) oder unbekannt"
		}
		return ri
	case <-time.After(4 * time.Second):
		return RemoteInfo{}
	}
}
