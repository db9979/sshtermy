// Package telnet ist ein schlanker Telnet-Client (RFC 854) für ältere
// Switches und Konsolenserver. Er handelt nur aus, was ein Terminal braucht:
// Echo und Suppress-Go-Ahead vom Server, Fenstergröße (NAWS) und Terminaltyp
// von uns, auf Wunsch 8-Bit (BINARY). Alles andere lehnt er höflich ab.
package telnet

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Telnet-Befehle und -Optionen.
const (
	se   = 240
	sb   = 250
	will = 251
	wont = 252
	do   = 253
	dont = 254
	iac  = 255

	optBinary = 0
	optEcho   = 1
	optSGA    = 3
	optTType  = 24
	optNAWS   = 31

	ttIs   = 0
	ttSend = 1
)

// TermType: was wir als Terminaltyp melden. Geräte-CLIs kennen fast alle
// „XTERM“ oder zumindest „VT100“; der Emulator versteht beides.
var TermType = "XTERM"

// Conn ist eine offene Telnet-Sitzung.
type Conn struct {
	Done chan error // nil = Gegenstelle hat beendet (logout) oder wir haben geschlossen

	c      net.Conn
	mu     sync.Mutex // Schreiben (Tastatur, Verhandlung, Fenstergröße)
	closed atomic.Bool

	// Verhandelter Zustand. us = wir machen es, him = der Server macht es.
	// Nur der Lese-Goroutine; was Write/Resize brauchen, steht in flags.
	us, him  [256]bool
	binOut   atomic.Bool
	nawsOn   atomic.Bool
	echoOn   atomic.Bool
	cols     int
	rows     int
	ttSent   int
	sizeMu   sync.Mutex
	parseSt  int
	parseCmd byte
	sbBuf    []byte
	lastCR   bool
}

// Dial verbindet sich mit addr (host:port) und startet das Lesen.
func Dial(ctx context.Context, addr string, cols, rows int, out io.Writer) (*Conn, error) {
	d := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return newConn(nc, cols, rows, out), nil
}

func newConn(nc net.Conn, cols, rows int, out io.Writer) *Conn {
	c := &Conn{c: nc, Done: make(chan error, 1), cols: cols, rows: rows}
	go c.read(out)
	return c
}

func (c *Conn) read(out io.Writer) {
	buf := make([]byte, 4096)
	data := make([]byte, 0, 4096)
	for {
		n, err := c.c.Read(buf)
		if n > 0 {
			data = c.parse(buf[:n], data[:0])
			if len(data) > 0 {
				_, _ = out.Write(data)
			}
		}
		if err != nil {
			if c.closed.Load() || errors.Is(err, io.EOF) {
				c.Done <- nil
			} else {
				c.Done <- err
			}
			return
		}
	}
}

// Parser-Zustände
const (
	stData = iota
	stIAC
	stOpt // nach WILL/WONT/DO/DONT
	stSB
	stSBIAC
)

// parse trennt Nutzdaten von Telnet-Befehlen und beantwortet die Verhandlung.
func (c *Conn) parse(in, data []byte) []byte {
	for _, b := range in {
		switch c.parseSt {
		case stData:
			switch {
			case b == iac:
				c.parseSt = stIAC
			case b == 0 && c.lastCR:
				// „CR NUL“ heißt nur CR
				c.lastCR = false
			default:
				c.lastCR = b == '\r'
				data = append(data, b)
			}
		case stIAC:
			switch b {
			case iac: // verdoppeltes 0xFF = ein Datenbyte
				data = append(data, iac)
				c.parseSt = stData
			case will, wont, do, dont:
				c.parseCmd, c.parseSt = b, stOpt
			case sb:
				c.sbBuf, c.parseSt = c.sbBuf[:0], stSB
			default: // NOP, GA, AYT …: ignorieren
				c.parseSt = stData
			}
		case stOpt:
			c.negotiate(c.parseCmd, b)
			c.parseSt = stData
		case stSB:
			if b == iac {
				c.parseSt = stSBIAC
			} else if len(c.sbBuf) < 512 {
				c.sbBuf = append(c.sbBuf, b)
			}
		case stSBIAC:
			switch b {
			case se:
				c.subneg(c.sbBuf)
				c.parseSt = stData
			case iac:
				c.sbBuf = append(c.sbBuf, iac)
				c.parseSt = stSB
			default:
				c.parseSt = stData
			}
		}
	}
	return data
}

// negotiate antwortet nur, wenn sich der Zustand ändert (keine Endlosschleifen).
func (c *Conn) negotiate(cmd, opt byte) {
	defer c.mirror()
	switch cmd {
	case do:
		ok := opt == optNAWS || opt == optTType || opt == optBinary || opt == optSGA
		if !ok {
			c.send(iac, wont, opt)
			return
		}
		if !c.us[opt] {
			c.us[opt] = true
			c.send(iac, will, opt)
		}
		if opt == optNAWS {
			c.sendSize()
		}
	case dont:
		if c.us[opt] {
			c.us[opt] = false
			c.send(iac, wont, opt)
		}
	case will:
		ok := opt == optEcho || opt == optSGA || opt == optBinary
		if !ok {
			c.send(iac, dont, opt)
			return
		}
		if !c.him[opt] {
			c.him[opt] = true
			c.send(iac, do, opt)
		}
	case wont:
		if c.him[opt] {
			c.him[opt] = false
			c.send(iac, dont, opt)
		}
	}
}

// mirror hält die Kopien für die anderen Goroutinen aktuell.
func (c *Conn) mirror() {
	c.binOut.Store(c.us[optBinary])
	c.nawsOn.Store(c.us[optNAWS])
	c.echoOn.Store(c.him[optEcho])
}

func (c *Conn) subneg(b []byte) {
	if len(b) >= 2 && b[0] == optTType && b[1] == ttSend && c.us[optTType] {
		msg := []byte{iac, sb, optTType, ttIs}
		msg = append(msg, TermType...)
		msg = append(msg, iac, se)
		c.send(msg...)
	}
}

func (c *Conn) send(b ...byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = c.c.Write(b)
}

func (c *Conn) sendSize() {
	c.sizeMu.Lock()
	w, h := c.cols, c.rows
	c.sizeMu.Unlock()
	if w <= 0 || h <= 0 {
		return
	}
	msg := []byte{iac, sb, optNAWS}
	for _, v := range []int{w, h} {
		hi, lo := byte(v>>8), byte(v)
		msg = append(msg, hi)
		if hi == iac {
			msg = append(msg, iac)
		}
		msg = append(msg, lo)
		if lo == iac {
			msg = append(msg, iac)
		}
	}
	msg = append(msg, iac, se)
	c.send(msg...)
}

// Write schickt Tastendrücke: 0xFF wird verdoppelt, ein einzelnes CR (Enter)
// wird ohne BINARY zu „CR NUL“, wie RFC 854 es verlangt.
func (c *Conn) Write(p []byte) (int, error) {
	out := make([]byte, 0, len(p)+8)
	bin := c.binOut.Load()
	for i, b := range p {
		out = append(out, b)
		switch {
		case b == iac:
			out = append(out, iac)
		case b == '\r' && !bin && (i+1 >= len(p) || p[i+1] != '\n'):
			out = append(out, 0)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.c.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Resize meldet die neue Fenstergröße, wenn der Server NAWS will.
func (c *Conn) Resize(cols, rows int) error {
	c.sizeMu.Lock()
	changed := c.cols != cols || c.rows != rows
	c.cols, c.rows = cols, rows
	c.sizeMu.Unlock()
	if changed && c.nawsOn.Load() {
		c.sendSize()
	}
	return nil
}

// Echo meldet, ob der Server die Eingabe selbst zurückschickt.
func (c *Conn) Echo() bool { return c.echoOn.Load() }

func (c *Conn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	return c.c.Close()
}
