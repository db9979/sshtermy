package devfake

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"
)

// StartTelnetSwitch: derselbe Fake-ProCurve, aber per Telnet (Port 23 bei
// echten Geräten). Er handelt Echo/SGA/NAWS/Terminaltyp aus, fragt nach
// Benutzer und Passwort (beliebig) und merkt sich, was der Client meldete.
func StartTelnetSwitch(addr string) (*Switch, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	sw := &Switch{Addr: ln.Addr().String(), ln: ln, vlans: []int{1, 10, 20}}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go sw.serveTelnet(nc)
		}
	}()
	return sw, nil
}

// TelnetInfo: was der letzte Telnet-Client ausgehandelt hat.
type TelnetInfo struct {
	TermType   string
	Cols, Rows int
	User, Pass string
}

var (
	telMu   sync.Mutex
	telLast TelnetInfo
)

// LastTelnet liefert die Angaben des letzten Telnet-Clients.
func LastTelnet() TelnetInfo { telMu.Lock(); defer telMu.Unlock(); return telLast }

func (sw *Switch) serveTelnet(nc net.Conn) {
	defer nc.Close()
	_, _ = nc.Write([]byte{255, 251, 1, 255, 251, 3, 255, 253, 31, 255, 253, 24})
	in := &telnetReader{r: bufio.NewReader(nc), w: nc}
	rw := struct {
		io.Reader
		io.Writer
		io.Closer
	}{in, nc, nc}
	readLine := func(echo bool) (string, bool) {
		var b []byte
		k := make([]byte, 1)
		for {
			if _, err := in.Read(k); err != nil {
				return "", false
			}
			switch k[0] {
			case '\r', '\n':
				_, _ = nc.Write([]byte("\r\n"))
				return string(b), true
			case 0:
			default:
				b = append(b, k[0])
				if echo {
					_, _ = nc.Write(k)
				}
			}
		}
	}
	_, _ = nc.Write([]byte("\r\nHP J9773A 2530-24G-PoEP Switch\r\n\r\nUsername: "))
	u, ok := readLine(true)
	if !ok {
		return
	}
	_, _ = nc.Write([]byte("Password: "))
	p, ok := readLine(false)
	if !ok {
		return
	}
	telMu.Lock()
	telLast.User, telLast.Pass = strings.TrimSpace(u), p
	telMu.Unlock()
	sw.shell(rw)
}

// telnetReader entfernt Telnet-Befehle aus dem Eingabestrom und merkt sich
// Fenstergröße und Terminaltyp.
type telnetReader struct {
	r *bufio.Reader
	w io.Writer
}

func (t *telnetReader) Read(p []byte) (int, error) {
	n := 0
	for n == 0 {
		b, err := t.r.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != 255 {
			p[n] = b
			n++
			continue
		}
		cmd, err := t.r.ReadByte()
		if err != nil {
			return 0, err
		}
		switch cmd {
		case 255:
			p[n] = 255
			n++
		case 251, 252, 253, 254:
			opt, _ := t.r.ReadByte()
			if cmd == 251 && opt == 24 { // WILL TTYPE → SEND
				_, _ = t.w.Write([]byte{255, 250, 24, 1, 255, 240})
			}
		case 250:
			var sb []byte
			for {
				c, err := t.r.ReadByte()
				if err != nil {
					return 0, err
				}
				if c == 255 {
					c2, _ := t.r.ReadByte()
					if c2 == 240 {
						break
					}
					sb = append(sb, c2)
					continue
				}
				sb = append(sb, c)
			}
			telMu.Lock()
			switch {
			case len(sb) >= 5 && sb[0] == 31:
				telLast.Cols, telLast.Rows = int(sb[1])<<8|int(sb[2]), int(sb[3])<<8|int(sb[4])
			case len(sb) >= 2 && sb[0] == 24 && sb[1] == 0:
				telLast.TermType = string(sb[2:])
			}
			telMu.Unlock()
		}
	}
	return n, nil
}

// ServeConsole spielt den Switch auf einer seriellen Leitung (z. B. der
// Master-Seite eines Pseudo-Terminals): kein Login, nur die Konsole.
func (sw *Switch) ServeConsole(rw io.ReadWriteCloser) { sw.shell(rw) }

// NewConsole: Switch ohne Netzwerk, nur für ServeConsole.
func NewConsole() *Switch { return &Switch{vlans: []int{1, 10, 20}} }
