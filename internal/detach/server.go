package detach

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	addrFile  = "addr"
	tokenFile = "token"
)

// Server wartet auf genau ein Fenster für einen Tab.
type Server struct {
	dir   string
	ln    net.Listener
	token []byte
	once  sync.Once
}

// Listen legt den Ordner mit Adresse und Zugangswort an und öffnet den Kanal.
func Listen() (*Server, error) {
	dir, err := os.MkdirTemp("", "sshtermy-")
	if err != nil {
		return nil, err
	}
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	ln, addr, err := listen(dir)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	s := &Server{dir: dir, ln: ln, token: []byte(hex.EncodeToString(tok))}
	if err := os.WriteFile(filepath.Join(dir, addrFile), []byte(addr), 0o600); err != nil {
		s.Close()
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, tokenFile), s.token, 0o600); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Dir: Ordner, den das neue Fenster als Argument bekommt.
func (s *Server) Dir() string { return s.dir }

// Close schließt den Kanal und räumt den Ordner weg (mehrfach aufrufbar).
func (s *Server) Close() {
	s.once.Do(func() {
		s.ln.Close()
		os.RemoveAll(s.dir)
	})
}

// ErrTimeout: innerhalb der Wartezeit hat sich kein Fenster gemeldet.
var ErrTimeout = errors.New("detach: kein Fenster hat sich gemeldet")

// Accept wartet auf das Fenster, prüft das Zugangswort und liefert die
// Verbindung samt Fenstergröße. Danach nimmt der Server niemanden mehr an.
func (s *Server) Accept(timeout time.Duration) (*Conn, int, int, error) {
	type res struct {
		c          *Conn
		cols, rows int
		err        error
	}
	ch := make(chan res, 1)
	go func() {
		for {
			nc, err := s.ln.Accept()
			if err != nil {
				ch <- res{err: err}
				return
			}
			c, cols, rows, err := s.handshake(nc)
			if err != nil {
				nc.Close() // falsches Zugangswort: weiter warten
				continue
			}
			ch <- res{c, cols, rows, nil}
			return
		}
	}()
	select {
	case r := <-ch:
		s.ln.Close()
		os.Remove(filepath.Join(s.dir, tokenFile))
		return r.c, r.cols, r.rows, r.err
	case <-time.After(timeout):
		s.Close()
		return nil, 0, 0, ErrTimeout
	}
}

func (s *Server) handshake(nc net.Conn) (*Conn, int, int, error) {
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer nc.SetReadDeadline(time.Time{})
	h, err := readFrame(nc)
	if err != nil {
		return nil, 0, 0, err
	}
	if h.typ != frHello || subtle.ConstantTimeCompare(bytes.TrimSpace(h.p), s.token) != 1 {
		return nil, 0, 0, errors.New("detach: falsches Zugangswort")
	}
	r, err := readFrame(nc)
	if err != nil {
		return nil, 0, 0, err
	}
	if r.typ != frResize {
		return nil, 0, 0, errors.New("detach: Größe fehlt")
	}
	cols, rows, err := parseSize(r.p)
	if err != nil {
		return nil, 0, 0, err
	}
	return &Conn{nc: nc, fw: &frameWriter{w: nc}}, cols, rows, nil
}

// Conn ist das angedockte Fenster.
type Conn struct {
	nc   net.Conn
	fw   *frameWriter
	once sync.Once
}

// Write schickt Ausgabe der Sitzung ins Fenster.
func (c *Conn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := c.fw.write(frData, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Title setzt den Fenstertitel.
func (c *Conn) Title(t string) error { return c.fw.write(frTitle, []byte(t)) }

// Bye beendet das Fenster mit einer Meldung und schließt die Verbindung.
func (c *Conn) Bye(msg string) {
	c.once.Do(func() {
		_ = c.nc.SetWriteDeadline(time.Now().Add(time.Second))
		_ = c.fw.write(frBye, []byte(msg))
		c.nc.Close()
	})
}

// Event ist eine Nachricht aus dem Fenster.
type Event struct {
	Input      []byte // Tasten (roh, wie das Terminal sie sendet)
	Cols, Rows int    // > 0: neue Fenstergröße
}

// Read liefert das nächste Ereignis; io.EOF o. ä., wenn das Fenster zu ist.
func (c *Conn) Read() (Event, error) {
	for {
		f, err := readFrame(c.nc)
		if err != nil {
			return Event{}, err
		}
		switch f.typ {
		case frData:
			return Event{Input: f.p}, nil
		case frResize:
			if cols, rows, err := parseSize(f.p); err == nil {
				return Event{Cols: cols, Rows: rows}, nil
			}
		}
	}
}

// Close trennt ohne Meldung.
func (c *Conn) Close() { c.once.Do(func() { c.nc.Close() }) }

func readAddr(dir string) (network, addr string, err error) {
	b, err := os.ReadFile(filepath.Join(dir, addrFile))
	if err != nil {
		return "", "", err
	}
	n, a, ok := strings.Cut(strings.TrimSpace(string(b)), ":")
	if !ok {
		return "", "", errors.New("detach: Adresse unlesbar")
	}
	return n, a, nil
}
