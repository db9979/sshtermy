// Package localsock: lokale Verbindung zwischen zwei sshTermy-Prozessen
// desselben Benutzers (Unix-Socket bzw. Named Pipe unter Windows). Genutzt
// vom eigenen Fenster (internal/detach) und vom MCP-Server (internal/mcp).
package localsock

import (
	"errors"
	"net"
	"strings"
)

// Listen öffnet einen Socket in dir (Unix) bzw. eine Named Pipe (Windows).
// addr hat die Form "unix:/pfad" oder "pipe:\\.\pipe\…" und geht an Dial.
func Listen(dir string) (ln net.Listener, addr string, err error) { return listen(dir) }

// Dial verbindet mit einer Adresse aus Listen.
func Dial(addr string) (net.Conn, error) {
	n, a, ok := strings.Cut(strings.TrimSpace(addr), ":")
	if !ok {
		return nil, errors.New("localsock: Adresse unlesbar")
	}
	return dial(n, a)
}
