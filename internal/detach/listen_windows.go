//go:build windows

package detach

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// Named Pipe statt Unix-Socket: läuft auf jedem Windows und ist ohne
// Sicherheitsangabe nur für den eigenen Benutzer (und Administratoren) offen.
func listen(dir string) (net.Listener, string, error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	p := `\\.\pipe\sshtermy-` + hex.EncodeToString(b)
	ln, err := winio.ListenPipe(p, nil)
	if err != nil {
		return nil, "", err
	}
	return ln, "pipe:" + p, nil
}

func dial(network, addr string) (net.Conn, error) {
	t := 5 * time.Second
	if network == "pipe" {
		return winio.DialPipe(addr, &t)
	}
	return net.DialTimeout(network, addr, t)
}
