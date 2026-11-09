package termemu

import (
	"regexp"
	"strings"
	"unicode"

	uv "github.com/charmbracelet/ultraviolet"
)

// Suche im Verlauf
//
// Gesucht wird im Gesamtpuffer (Scrollback + Bildschirm, Zeile 0 = älteste
// Zeile, wie bei Pos). Klein geschriebene Suchbegriffe finden Groß und
// Klein, sobald ein Großbuchstabe vorkommt, wird genau gesucht (wie in vim
// mit smartcase). Der Fehlerfilter beschränkt die Treffer auf Zeilen, die
// nach Fehlern aussehen.

// Match ist ein Treffer: Zeile im Gesamtpuffer, erste Spalte und Breite in
// Spalten (nicht Bytes, wichtig für Umlaute und breite Zeichen).
type Match struct{ Line, Col, Len int }

// Lines: Anzahl Zeilen im Gesamtpuffer.
func (t *Term) Lines() int {
	_, h := t.Size()
	return t.ScrollbackLen() + h
}

// lineText liefert den Klartext einer Zeile und zu jedem Byte des Textes
// die Spalte, in der das Zeichen steht.
func (t *Term) lineText(sb, w, l int) (string, []int) {
	line := make(uv.Line, w)
	t.lineCells(sb, l, line)
	return cellsText(line)
}

// cellsText: Klartext einer Zeile aus Zellen, dazu je Byte die Spalte.
func cellsText(line uv.Line) (string, []int) {
	var b strings.Builder
	cols := make([]int, 0, len(line))
	for x := range line {
		c := &line[x]
		s := " "
		switch {
		case c.IsZero():
			continue // zweite Hälfte eines breiten Zeichens
		case c.Content != "":
			s = c.Content
		}
		for i := 0; i < len(s); i++ {
			cols = append(cols, x)
		}
		b.WriteString(s)
	}
	return b.String(), cols
}

// PlainLine: Klartext einer Zeile des Gesamtpuffers ohne nachlaufende
// Leerzeichen.
func (t *Term) PlainLine(l int) string {
	w, _ := t.Size()
	s, _ := t.lineText(t.ScrollbackLen(), w, l)
	return strings.TrimRight(s, " ")
}

// Search findet alle Vorkommen von q. Mit errorsOnly zählen nur
// Fehlerzeilen; ist q dann leer, ist jede Fehlerzeile ein Treffer.
// Reihenfolge: von oben (alt) nach unten (neu).
func (t *Term) Search(q string, errorsOnly bool) []Match {
	if q == "" && !errorsOnly {
		return nil
	}
	w, h := t.Size()
	sb := t.ScrollbackLen()
	exact := strings.IndexFunc(q, unicode.IsUpper) >= 0
	if !exact {
		q = strings.ToLower(q)
	}
	var out []Match
	for l := 0; l < sb+h; l++ {
		text, cols := t.lineText(sb, w, l)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if errorsOnly && !IsErrorLine(text) {
			continue
		}
		if q == "" {
			trimmed := strings.TrimRight(text, " ")
			lead := len(trimmed) - len(strings.TrimLeft(trimmed, " "))
			from, to := cols[lead], cols[len(trimmed)-1]
			out = append(out, Match{l, from, to - from + 1})
			continue
		}
		hay := text
		if !exact {
			hay = lowerSameLen(text)
		}
		for off := 0; off < len(hay); {
			i := strings.Index(hay[off:], q)
			if i < 0 {
				break
			}
			start, end := off+i, off+i+len(q)-1
			out = append(out, Match{l, cols[start], cols[end] - cols[start] + cellWidthAt(sb, t, l, cols[end])})
			off = start + max(1, len(q))
		}
	}
	return out
}

// cellWidthAt: Breite des Zeichens in Spalte x (1 oder 2).
func cellWidthAt(sb int, t *Term, l, x int) int {
	if c, ok := t.cellAt(sb, Pos{l, x}); ok && c.Width > 1 {
		return c.Width
	}
	return 1
}

// lowerSameLen macht Buchstaben klein, aber nur dort, wo sich die Länge in
// Bytes nicht ändert (sonst stimmte die Zuordnung zu den Spalten nicht).
func lowerSameLen(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		lr := unicode.ToLower(r)
		if len(string(lr)) == len(string(r)) {
			b.WriteRune(lr)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	errorWords = regexp.MustCompile(`(?i)(\b(error|errors|err|fehler|failed|failure|fail|fatal|panic|exception|traceback|denied|refused|unreachable|timeout|timed out|not found|no such file|nicht gefunden|cannot|can't|unable|segmentation fault|critical|crit|abort|aborted|invalid|ungültig|fehlgeschlagen|verweigert|unbekannt)\b|\bE[0-9]{3,}\b|^\s*E:)`)
	noErrors   = regexp.MustCompile(`(?i)\b(0|no|keine?|without|ohne)\s+(errors?|fehler|failures?)\b`)
)

// IsErrorLine: sieht die Zeile nach einer Fehlermeldung aus?
func IsErrorLine(s string) bool {
	return errorWords.MatchString(s) && !noErrors.MatchString(s)
}

// Mark ist ein hervorgehobener Bereich (Suchtreffer) beim Zeichnen.
type Mark struct {
	Line, Col, Len int
	Current        bool
}

// MarkStyle: Farben für Treffer und den aktuellen Treffer.
type MarkStyle struct{ Hit, Current uv.Style }

// RenderMarked zeichnet wie Render und hebt zusätzlich die Treffer hervor.
func (t *Term) RenderMarked(offset int, sel *Sel, marks []Mark, ms MarkStyle, hl *HL) string {
	if len(marks) == 0 {
		return t.RenderHL(offset, sel, hl)
	}
	return t.render(offset, sel, func(l, x int, c *uv.Cell) {
		for _, m := range marks {
			if m.Line == l && x >= m.Col && x < m.Col+m.Len {
				st := ms.Hit
				if m.Current {
					st = ms.Current
				}
				c.Style.Fg, c.Style.Bg = st.Fg, st.Bg
				c.Style.Attrs |= st.Attrs
				if c.Content == "" {
					c.Content, c.Width = " ", 1
				}
				return
			}
		}
	}, hl)
}
