package termemu

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Maus und Markieren
//
// Fordert das Programm auf dem Server die Maus an (htop, mc, vim mit
// mouse=a, tmux …), merkt sich das Terminal den Modus; die Oberfläche leitet
// Klicks dann weiter. Sonst markiert sshTermy selbst: Positionen zählen
// über Scrollback und Bildschirm hinweg (Zeile 0 = älteste Scrollback-Zeile),
// damit eine Markierung beim Blättern stehen bleibt.

// Pos ist eine Zelle im Gesamtpuffer (Scrollback + Bildschirm).
type Pos struct{ Line, Col int }

func (p Pos) Before(o Pos) bool { return p.Line < o.Line || (p.Line == o.Line && p.Col < o.Col) }

// Sel ist eine Markierung von A bis B (beide inklusive, Reihenfolge egal).
type Sel struct{ A, B Pos }

func (s Sel) ordered() (Pos, Pos) {
	if s.B.Before(s.A) {
		return s.B, s.A
	}
	return s.A, s.B
}

func (s Sel) Contains(p Pos) bool {
	a, b := s.ordered()
	return !p.Before(a) && !b.Before(p)
}

func (s Sel) Empty() bool { return s.A == s.B }

func (t *Term) onMode(m ansi.Mode, on bool) {
	dm, ok := m.(ansi.DECMode)
	if !ok {
		return
	}
	switch dm {
	case ansi.ModeMouseX10, ansi.ModeMouseNormal, ansi.ModeMouseHighlight, ansi.ModeMouseButtonEvent, ansi.ModeMouseAnyEvent:
	default:
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.mouse == nil {
		t.mouse = map[ansi.DECMode]bool{}
	}
	if on {
		t.mouse[dm] = true
	} else {
		delete(t.mouse, dm)
	}
}

// MouseLevel: wie viel Maus das Programm haben will.
type MouseLevel int

const (
	MouseOff     MouseLevel = iota
	MousePress              // ?9    nur Drücken (X10)
	MouseButtons            // ?1000 Drücken und Loslassen
	MouseDrag               // ?1002 dazu Bewegung mit gedrückter Taste
	MouseAny                // ?1003 jede Bewegung
)

// MouseTracking: höchster vom Programm eingeschalteter Maus-Modus.
func (t *Term) MouseTracking() MouseLevel {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.mouse[ansi.ModeMouseAnyEvent]:
		return MouseAny
	case t.mouse[ansi.ModeMouseButtonEvent]:
		return MouseDrag
	case t.mouse[ansi.ModeMouseNormal], t.mouse[ansi.ModeMouseHighlight]:
		return MouseButtons
	case t.mouse[ansi.ModeMouseX10]:
		return MousePress
	}
	return MouseOff
}

// SendMouse kodiert ein Mausereignis (Koordinaten im Terminal, 0-basiert)
// passend zum Modus des Programms (X10 oder SGR) und schickt es zum Server.
func (t *Term) SendMouse(m uv.MouseEvent) { t.emu.SendMouse(m) }

// ScrollbackLen: Anzahl Zeilen oberhalb des Bildschirms (0 im Alt-Screen).
func (t *Term) ScrollbackLen() int {
	if t.AltScreen() {
		return 0
	}
	return t.emu.ScrollbackLen()
}

// cellAt liefert die Zelle im Gesamtpuffer.
func (t *Term) cellAt(sb int, p Pos) *uv.Cell {
	if p.Line < sb {
		return t.emu.ScrollbackCellAt(p.Col, p.Line)
	}
	return t.emu.CellAt(p.Col, p.Line-sb)
}

// Render zeichnet den Bildschirm, um offset Zeilen in den Scrollback
// zurückversetzt, und hebt sel (falls gesetzt) invertiert hervor.
func (t *Term) Render(offset int, sel *Sel) string {
	w, h := t.Size()
	sb := t.ScrollbackLen()
	offset = max(0, min(offset, sb))
	first := sb - offset
	rows := make([]string, h)
	line := make(uv.Line, w)
	for y := 0; y < h; y++ {
		abs := first + y
		for x := 0; x < w; x++ {
			c := t.cellAt(sb, Pos{abs, x})
			if c == nil {
				line[x] = uv.EmptyCell
			} else {
				line[x] = *c
			}
			if sel != nil && !line[x].IsZero() && sel.Contains(Pos{abs, x}) {
				if line[x].Content == "" {
					line[x].Content, line[x].Width = " ", 1
				}
				line[x].Style.Attrs ^= uv.AttrReverse
			}
		}
		r := line.Render()
		if rw := ansi.StringWidth(r); rw < w {
			r += strings.Repeat(" ", w-rw)
		}
		rows[y] = r
	}
	return strings.Join(rows, "\n")
}

// ScreenPos rechnet eine Bildschirmzeile (bei offset Zeilen Rückblick) in
// eine Position im Gesamtpuffer um.
func (t *Term) ScreenPos(x, y, offset int) Pos {
	sb := t.ScrollbackLen()
	offset = max(0, min(offset, sb))
	return Pos{sb - offset + y, x}
}

// Text liefert den markierten Text, Zeilen mit \n getrennt, ohne
// nachlaufende Leerzeichen.
func (t *Term) Text(s Sel) string {
	a, b := s.ordered()
	w, _ := t.Size()
	sb := t.ScrollbackLen()
	var out []string
	for l := a.Line; l <= b.Line; l++ {
		from, to := 0, w-1
		if l == a.Line {
			from = a.Col
		}
		if l == b.Line {
			to = min(b.Col, w-1)
		}
		var sbuf strings.Builder
		for x := from; x <= to; x++ {
			c := t.cellAt(sb, Pos{l, x})
			switch {
			case c == nil:
				sbuf.WriteByte(' ')
			case c.IsZero():
				// zweite Hälfte eines breiten Zeichens
			case c.Content == "":
				sbuf.WriteByte(' ')
			default:
				sbuf.WriteString(c.Content)
			}
		}
		out = append(out, strings.TrimRight(sbuf.String(), " "))
	}
	return strings.Join(out, "\n")
}

// WordAt: Markierung des Wortes unter p (für Doppelklick). Wortzeichen sind
// alles außer Leerzeichen und typischen Trennern, damit Pfade, Hostnamen
// und IPs am Stück markiert werden.
func (t *Term) WordAt(p Pos) Sel {
	w, _ := t.Size()
	sb := t.ScrollbackLen()
	isWord := func(x int) bool {
		c := t.cellAt(sb, Pos{p.Line, x})
		if c == nil {
			return false
		}
		if c.IsZero() {
			return true
		}
		return c.Content != "" && !strings.ContainsAny(c.Content, " \t\"'`()[]{}<>|;,")
	}
	if !isWord(p.Col) {
		return Sel{p, p}
	}
	a, b := p.Col, p.Col
	for a > 0 && isWord(a-1) {
		a--
	}
	for b < w-1 && isWord(b+1) {
		b++
	}
	return Sel{Pos{p.Line, a}, Pos{p.Line, b}}
}

// LineAt: ganze Zeile (Dreifachklick).
func (t *Term) LineAt(p Pos) Sel {
	w, _ := t.Size()
	return Sel{Pos{p.Line, 0}, Pos{p.Line, w - 1}}
}
