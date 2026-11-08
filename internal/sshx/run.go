package sshx

import (
	"bufio"
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

// StreamScript startet ein lang laufendes sh-Skript in einer eigenen
// Exec-Sitzung und reicht jede Ausgabezeile an line weiter, bis ctx endet
// oder das Skript aufhört. Nur für Server (wie RunScript).
func (c *Client) StreamScript(ctx context.Context, script string, line func(string)) error {
	if c.NetDevice {
		return ErrNetDevice
	}
	s, err := c.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Stdin = strings.NewReader(script)
	out, err := s.StdoutPipe()
	if err != nil {
		return err
	}
	if err := s.Start("sh -s"); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = s.Close() // beendet auch das Lesen unten
	}()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if ctx.Err() != nil {
			break
		}
		line(sc.Text())
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return sc.Err()
}
