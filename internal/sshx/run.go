package sshx

import (
	"context"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// ErrNetDevice: Exec-Abfragen sind bei Netzwerkgeräten tabu, weil z. B.
// Aruba/ProCurve dann die ganze Verbindung beenden.
var ErrNetDevice = i18n.NewError("bei Netzwerkgeräten keine Exec-Abfrage")

// RunScript führt ein sh-Skript in einer eigenen Exec-Sitzung aus ("sh -s",
// Skript über stdin), unsichtbar für das Terminal. Nur für Server.
func (c *Client) RunScript(ctx context.Context, script string) (string, error) {
	if c.NetDevice {
		return "", ErrNetDevice
	}
	s, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	s.Stdin = strings.NewReader(script)
	type res struct {
		out []byte
		err error
	}
	done := make(chan res, 1)
	go func() { out, err := s.Output("sh -s"); done <- res{out, err} }()
	select {
	case r := <-done:
		if len(r.out) > 0 {
			return string(r.out), nil // Exit-Code egal, solange etwas kam
		}
		return "", r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
