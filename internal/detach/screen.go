package detach

import (
	"bytes"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Screen ist das „Terminal“ des Hintergrunddienstes: bubbletea schreibt
// hinein, Screen reicht es an das gerade angedockte Fenster weiter (oder
// verwirft es, solange keines da ist). Damit ein neu angedocktes Fenster
// richtig aussieht, merkt sich Screen die eingeschalteten Terminal-Modi
// (Alt-Bildschirm, Maus, Bracketed Paste, Fokus, Tastatur-Protokoll) und
// den Titel und spielt sie beim Andocken vor; das Bild zeichnet bubbletea
// danach komplett neu.
type Screen struct {
	mu     sync.Mutex
	conn   *Conn
	modes  map[int]bool // DEC-Modi (CSI ? n h/l)
	kitty  int          // Flags des Kitty-Tastaturprotokolls (CSI > n u)
	title  string
	tail   []byte // Ende der letzten Ausgabe (Sequenzen über zwei Schreibvorgänge)
	lastCR bool   // letzte Ausgabe endete auf \r
}

func NewScreen() *Screen { return &Screen{modes: map[int]bool{}} }

var (
	decRx   = regexp.MustCompile(`\x1b\[\?([0-9;]+)([hl])`)
	kittyRx = regexp.MustCompile(`\x1b\[([>=<])([0-9;]*)u`)
	titleRx = regexp.MustCompile(`\x1b\][02];([^\x07\x1b]*)(\x07|\x1b\\)`)
)

// Write: Ausgabe von bubbletea.
func (s *Screen) Write(p []byte) (int, error) {
	n := len(p)
	s.mu.Lock()
	s.track(p)
	c := s.conn
	if runtime.GOOS != "windows" {
		// Ohne TTY geht bubbletea davon aus, dass das Terminal \n wie ein
		// echtes TTY (ONLCR) als \r\n ausgibt; das angedockte Fenster ist
		// aber roh. Also hier übersetzen, genau wie ONLCR.
		p = onlcr(p, s.lastCR)
		if len(p) > 0 {
			s.lastCR = p[len(p)-1] == '\r'
		}
	}
	s.mu.Unlock()
	if c != nil {
		_ = c.nc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write(p); err != nil {
			s.Drop(c) // Fenster hängt oder ist weg: abkoppeln, weiterlaufen
		}
	}
	return n, nil
}

// onlcr: jedes \n, vor dem kein \r steht, wird \r\n.
func onlcr(p []byte, prevCR bool) []byte {
	if bytes.IndexByte(p, '\n') < 0 {
		return p
	}
	out := make([]byte, 0, len(p)+16)
	for i, b := range p {
		if b == '\n' && !((i > 0 && p[i-1] == '\r') || (i == 0 && prevCR)) {
			out = append(out, '\r')
		}
		out = append(out, b)
	}
	return out
}

func (s *Screen) track(p []byte) {
	buf := append(append([]byte(nil), s.tail...), p...)
	for _, m := range decRx.FindAllSubmatch(buf, -1) {
		on := string(m[2]) == "h"
		for _, f := range strings.Split(string(m[1]), ";") {
			if n, err := strconv.Atoi(f); err == nil {
				s.modes[n] = on
			}
		}
	}
	for _, m := range kittyRx.FindAllSubmatch(buf, -1) {
		switch string(m[1]) {
		case ">", "=":
			n, _ := strconv.Atoi(strings.Split(string(m[2]), ";")[0])
			s.kitty = n
		case "<":
			s.kitty = 0
		}
	}
	for _, m := range titleRx.FindAllSubmatch(buf, -1) {
		s.title = string(m[1])
	}
	if len(buf) > 64 {
		buf = buf[len(buf)-64:]
	}
	s.tail = buf
}

// prelude: Modi und Titel für ein neu angedocktes Fenster.
func (s *Screen) prelude() []byte {
	var b strings.Builder
	keys := make([]int, 0, len(s.modes))
	for k, on := range s.modes {
		if on && k != 25 {
			keys = append(keys, k)
		}
	}
	sort.Ints(keys)
	// Alt-Bildschirm zuerst, sonst landen die anderen Modi im falschen Puffer
	for _, alt := range []int{1049, 1047, 47} {
		if s.modes[alt] {
			b.WriteString("\x1b[?" + strconv.Itoa(alt) + "h")
		}
	}
	for _, k := range keys {
		if k == 1049 || k == 1047 || k == 47 {
			continue
		}
		b.WriteString("\x1b[?" + strconv.Itoa(k) + "h")
	}
	if on, ok := s.modes[25]; ok && !on {
		b.WriteString("\x1b[?25l")
	}
	if s.kitty > 0 {
		b.WriteString("\x1b[>" + strconv.Itoa(s.kitty) + "u")
	}
	if s.title != "" {
		b.WriteString("\x1b]0;" + s.title + "\x07")
	}
	return []byte(b.String())
}

// Attach: c ist jetzt das Fenster; ein vorher angedocktes bekommt msg.
func (s *Screen) Attach(c *Conn, msg string) {
	s.mu.Lock()
	old := s.conn
	s.conn = c
	pre := s.prelude()
	s.mu.Unlock()
	if old != nil {
		old.Bye(msg)
	}
	_, _ = c.Write(pre)
}

// Drop: c ist weg (oder hängt). Nur wenn es noch das aktuelle Fenster ist.
func (s *Screen) Drop(c *Conn) bool {
	s.mu.Lock()
	cur := s.conn == c
	if cur {
		s.conn = nil
	}
	s.mu.Unlock()
	if cur {
		c.Close()
	}
	return cur
}

// Bye: das angedockte Fenster mit einer Meldung entlassen (Abkoppeln, Ende).
func (s *Screen) Bye(msg string) {
	s.mu.Lock()
	c := s.conn
	s.conn = nil
	s.mu.Unlock()
	if c != nil {
		c.Bye(msg)
	}
}

// Attached: ist gerade ein Fenster angedockt?
func (s *Screen) Attached() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn != nil
}
