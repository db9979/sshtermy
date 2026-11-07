package termemu

import (
	"sync"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
)

// Platzsparender Verlauf (Scrollback)
//
// Der Emulator (charmbracelet/x/vt) hebt jede Zeile, die oben aus dem Bild
// läuft, als volle Zellen auf: 112 Byte pro Zeichen (Text, Stil mit drei
// Farben, Link, Breite). Bei 5000 Zeilen à 150 Zeichen sind das rund 80 MB
// pro Tab, mit 8 Tabs also mehrere hundert MB. Hier liegt jede Zeile
// kompakt: 8 Byte pro Zeichen, Stile und Links nur einmal pro Zeile. Nach
// jedem Write zieht Term die neuen Zeilen aus dem Emulator ab (drain) und
// leert dessen Puffer wieder. Der Ringpuffer verwirft die älteste Zeile in
// O(1) statt das ganze Feld umzukopieren.

// sbCell: Zeichen (rune, < 0 = Index in extra), Stil- und Linknummer der
// Zeile (0 = keiner) und Breite.
type sbCell struct {
	r      int32
	st, ln uint8
	w      uint8
}

type sbLine struct {
	cells  []sbCell
	styles []uv.Style // styles[0] = kein Stil
	links  []uv.Link  // links[0] = kein Link
	extra  []string   // Grapheme aus mehreren Runen (Emoji, Akzente)
	raw    uv.Line    // Ausweichweg für Zeilen mit über 255 Stilen
}

type scrollback struct {
	mu    sync.Mutex
	lines []sbLine // Ring, head = älteste Zeile
	head  int
	n     int
	max   int
}

func newScrollback(max int) *scrollback { return &scrollback{max: max} }

func (s *scrollback) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func (s *scrollback) clear() {
	s.mu.Lock()
	s.lines, s.head, s.n = nil, 0, 0
	s.mu.Unlock()
}

// push hängt eine Zeile an (die Zellen werden kopiert, line darf danach
// wiederverwendet werden).
func (s *scrollback) push(line uv.Line) {
	l := encodeLine(line)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.max <= 0 {
		return
	}
	if s.n < s.max {
		if len(s.lines) < s.max {
			s.lines = append(s.lines, l)
		} else {
			s.lines[(s.head+s.n)%s.max] = l
		}
		s.n++
		return
	}
	s.lines[s.head] = l
	s.head = (s.head + 1) % s.max
}

func (s *scrollback) at(y int) *sbLine {
	if y < 0 || y >= s.n {
		return nil
	}
	return &s.lines[(s.head+y)%len(s.lines)]
}

// cell liefert Zelle x der Zeile y (0 = älteste). ok = false hinter dem
// Zeilenende oder außerhalb des Verlaufs.
func (s *scrollback) cell(x, y int) (uv.Cell, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.at(y)
	if l == nil {
		return uv.Cell{}, false
	}
	return l.cell(x)
}

// line liefert die ganze Zeile y als Zellen (für Klartext).
func (s *scrollback) line(y int) uv.Line {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.at(y)
	if l == nil {
		return nil
	}
	if l.raw != nil {
		return l.raw
	}
	out := make(uv.Line, len(l.cells))
	for x := range out {
		out[x], _ = l.cell(x)
	}
	return out
}

func (l *sbLine) cell(x int) (uv.Cell, bool) {
	if l.raw != nil {
		if x < 0 || x >= len(l.raw) {
			return uv.Cell{}, false
		}
		return l.raw[x], true
	}
	if x < 0 || x >= len(l.cells) {
		return uv.Cell{}, false
	}
	c := l.cells[x]
	out := uv.Cell{Width: int(c.w)}
	switch {
	case c.r < 0:
		out.Content = l.extra[-c.r-1]
	case c.r == 0:
	case c.r < utf8.RuneSelf:
		out.Content = asciiStr[c.r]
	default:
		out.Content = string(rune(c.r))
	}
	if c.st > 0 {
		out.Style = l.styles[c.st]
	}
	if c.ln > 0 {
		out.Link = l.links[c.ln]
	}
	return out, true
}

var asciiStr = func() (t [utf8.RuneSelf]string) {
	for i := range t {
		t[i] = string(rune(i))
	}
	return t
}()

func encodeLine(line uv.Line) sbLine {
	var l sbLine
	l.cells = make([]sbCell, len(line))
	for i := range line {
		c := &line[i]
		var sc sbCell
		if c.Width >= 0 && c.Width < 256 {
			sc.w = uint8(c.Width)
		} else {
			return rawLine(line)
		}
		switch r, size := utf8.DecodeRuneInString(c.Content); {
		case c.Content == "":
		case size == len(c.Content) && r != utf8.RuneError && r != 0:
			sc.r = r
		default:
			l.extra = append(l.extra, c.Content)
			sc.r = -int32(len(l.extra))
		}
		if !c.Style.IsZero() {
			k := -1
			for j := 1; j < len(l.styles); j++ {
				if l.styles[j] == c.Style {
					k = j
					break
				}
			}
			if k < 0 {
				if len(l.styles) == 0 {
					l.styles = append(l.styles, uv.Style{})
				}
				if len(l.styles) > 255 {
					return rawLine(line)
				}
				l.styles = append(l.styles, c.Style)
				k = len(l.styles) - 1
			}
			sc.st = uint8(k)
		}
		if c.Link != (uv.Link{}) {
			k := -1
			for j := 1; j < len(l.links); j++ {
				if l.links[j] == c.Link {
					k = j
					break
				}
			}
			if k < 0 {
				if len(l.links) == 0 {
					l.links = append(l.links, uv.Link{})
				}
				if len(l.links) > 255 {
					return rawLine(line)
				}
				l.links = append(l.links, c.Link)
				k = len(l.links) - 1
			}
			sc.ln = uint8(k)
		}
		l.cells[i] = sc
	}
	return l
}

func rawLine(line uv.Line) sbLine {
	return sbLine{raw: append(uv.Line(nil), line...)}
}
