//go:build windows

package localsock

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
	// Mit Puffer: ohne blockiert jedes Write, bis die Gegenseite liest
	// (Tastendruck und Größenänderung kurz hintereinander hingen sonst).
	ln, err := winio.ListenPipe(p, &winio.PipeConfig{InputBufferSize: 64 << 10, OutputBufferSize: 64 << 10})
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
