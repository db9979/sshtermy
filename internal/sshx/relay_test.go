package sshx

import (
	"context"
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
