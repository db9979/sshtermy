package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Markieren per Tastatur (Alt+C, Idee aus herdr/tmux): ein Cursor wandert
// durch Bildschirm und Verlauf, v/Leertaste beginnt eine Markierung, V
// markiert ganze Zeilen, y oder ⏎ kopiert und beendet, Esc/q beendet.
// Bewegen: Pfeile oder h/j/k/l, w/b/e Wort, 0/^/$ Zeile, g/G Anfang/Ende,
// Bild↑/Bild↓ bzw. Strg+U/Strg+D halbe Seite. Die Ausgabe läuft weiter, der
// Server bekommt keine Taste. Gut ohne Maus (uConsole, Laptop, SSH in SSH).

type copyMode struct {
	cur    termemu.Pos  // Cursor im Gesamtpuffer
	anchor *termemu.Pos // Beginn der Markierung, nil = keine
	lines  bool         // ganze Zeilen (V)
}

func (a *App) startCopyMode(s *session) {
	if s.term == nil || s.term.AltScreen() {
		a.flash(i18n.T("Markieren per Tastatur geht nicht, solange ein Vollbildprogramm läuft (dort dessen eigene Tasten)"))
		return
	}
	x, y, _ := s.term.Cursor()
	cm := &copyMode{cur: termemu.Pos{Line: s.term.ScrollbackLen() - s.scroll + y, Col: x}}
	if s.scroll > 0 {
		cm.cur.Line = s.term.ScrollbackLen() - s.scroll // oben im zurückgeblätterten Bild
		cm.cur.Col = 0
	}
	s.copy = cm
	a.copyShow(s)
	a.flash(i18n.T("Markieren: Pfeile/hjkl bewegen · v markieren · V Zeilen · y kopieren · Esc beenden"))
}

func (a *App) stopCopyMode(s *session) {
	s.copy, s.sel, s.scroll = nil, nil, 0
}

// copyShow: Markierung (oder nur den Cursor) anzeigen und dorthin blättern.
func (a *App) copyShow(s *session) {
	cm := s.copy
	_, rows := s.term.Size()
	sb := s.term.ScrollbackLen()
	cm.cur.Line = clamp(cm.cur.Line, 0, sb+rows-1)
	top := sb - s.scroll
	switch {
	case cm.cur.Line < top:
		s.scroll = sb - cm.cur.Line
	case cm.cur.Line > top+rows-1:
		s.scroll = sb - (cm.cur.Line - rows + 1)
	}
	s.scroll = clamp(s.scroll, 0, sb)
	sel := termemu.Sel{A: cm.cur, B: cm.cur}
	if cm.anchor != nil {
		sel.A = *cm.anchor
		if cm.lines {
			w, _ := s.term.Size()
			a1, b1 := sel.A, sel.B
			if b1.Before(a1) {
				a1, b1 = b1, a1
			}
			sel = termemu.Sel{A: termemu.Pos{Line: a1.Line}, B: termemu.Pos{Line: b1.Line, Col: w - 1}}
		}
	}
	s.sel = &sel
}

// cellClass: 0 = leer, 1 = Wortzeichen, 2 = Satzzeichen. Punkt, Doppelpunkt,
// Schrägstrich, Bindestrich, @ und ~ zählen zum Wort: so ist eine IP, ein
// Pfad oder user@host mit w/e ein Wort.
func cellClass(t *termemu.Term, p termemu.Pos) int {
	c := t.CellText(p)
	r, _ := utf8.DecodeRuneInString(c)
	switch {
	case c == "" || unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.:/-@~", r):
		return 1
	}
	return 2
}

// copyWord: nächster Wortanfang (dir 1), voriger Wortanfang (-1) oder
// Wortende (e). Bleibt in der Zeile, springt am Rand in die nächste.
func copyWord(t *termemu.Term, p termemu.Pos, dir int, end bool) termemu.Pos {
	w, _ := t.Size()
	last := t.Lines() - 1
	step := func(q termemu.Pos, d int) (termemu.Pos, bool) {
		q.Col += d
		if q.Col >= w {
			if q.Line >= last {
				return q, false
			}
			q.Line, q.Col = q.Line+1, 0
		} else if q.Col < 0 {
			if q.Line == 0 {
				return q, false
			}
			q.Line, q.Col = q.Line-1, w-1
		}
		return q, true
	}
	q, ok := p, true
	switch {
	case end: // Wortende: mindestens ein Schritt, Leeres überspringen, bis zum letzten Zeichen der Klasse
		if q, ok = step(q, 1); !ok {
			return p
		}
		for cellClass(t, q) == 0 {
			if q, ok = step(q, 1); !ok {
				return p
			}
		}
		cls := cellClass(t, q)
		for {
			n, ok := step(q, 1)
			if !ok || n.Line != q.Line || cellClass(t, n) != cls {
				return q
			}
			q = n
		}
	case dir > 0:
		cls := cellClass(t, q)
		for cls != 0 { // Rest des Worts
			n, ok := step(q, 1)
			if !ok {
				return q
			}
			if n.Line != q.Line || cellClass(t, n) != cls {
				q = n
				break
			}
			q = n
		}
		for cellClass(t, q) == 0 {
			if q, ok = step(q, 1); !ok {
				return p
			}
		}
		return q
	}
	// rückwärts: Leeres davor überspringen, dann an den Anfang des Worts
	if q, ok = step(q, -1); !ok {
		return p
	}
	for cellClass(t, q) == 0 {
		if q, ok = step(q, -1); !ok {
			return p
		}
	}
	cls := cellClass(t, q)
	for {
		n, ok := step(q, -1)
		if !ok || n.Line != q.Line || cellClass(t, n) != cls {
			return q
		}
		q = n
	}
}

// lineEnd: letzte belegte Spalte der Zeile.
func lineEnd(t *termemu.Term, l int) int {
	w, _ := t.Size()
	for x := w - 1; x > 0; x-- {
		if cellClass(t, termemu.Pos{Line: l, Col: x}) != 0 {
			return x
		}
	}
	return 0
}

func (a *App) copyKey(s *session, m tea.KeyPressMsg) tea.Cmd {
	cm, t := s.copy, s.term
	if t == nil {
		s.copy = nil
		return nil
	}
	w, rows := t.Size()
	last := t.Lines() - 1
	key := m.String()
	switch key {
	case "esc", "q", "ctrl+c":
		a.stopCopyMode(s)
		return nil
	case "y", "enter", "ctrl+shift+c":
		if cm.anchor == nil {
			a.flash(i18n.T("Erst mit v (oder V für Zeilen) markieren"))
			return nil
		}
		cmd := a.copySel(s)
		a.stopCopyMode(s)
		return cmd
	case "v", "space":
		if cm.anchor != nil && !cm.lines {
			cm.anchor = nil
		} else {
			p := cm.cur
			cm.anchor, cm.lines = &p, false
		}
	case "V", "shift+v":
		if cm.anchor != nil && cm.lines {
			cm.anchor = nil
		} else {
			p := cm.cur
			cm.anchor, cm.lines = &p, true
		}
	case "left", "h":
		cm.cur.Col = max(0, cm.cur.Col-1)
	case "right", "l":
		cm.cur.Col = min(w-1, cm.cur.Col+1)
	case "up", "k":
		cm.cur.Line = max(0, cm.cur.Line-1)
	case "down", "j":
		cm.cur.Line = min(last, cm.cur.Line+1)
	case "w", "W":
		cm.cur = copyWord(t, cm.cur, 1, false)
	case "b", "B":
		cm.cur = copyWord(t, cm.cur, -1, false)
	case "e", "E":
		cm.cur = copyWord(t, cm.cur, 1, true)
	case "0", "home":
		cm.cur.Col = 0
	case "^":
		cm.cur.Col = 0
		for cm.cur.Col < w-1 && cellClass(t, cm.cur) == 0 {
			cm.cur.Col++
		}
	case "$", "end":
		cm.cur.Col = lineEnd(t, cm.cur.Line)
	case "g", "ctrl+home":
		cm.cur = termemu.Pos{}
	case "G", "shift+g", "ctrl+end":
		cm.cur = termemu.Pos{Line: last, Col: lineEnd(t, last)}
	case "pgup", "ctrl+u", "ctrl+b":
		cm.cur.Line = max(0, cm.cur.Line-max(1, rows/2))
	case "pgdown", "ctrl+d", "ctrl+f":
		cm.cur.Line = min(last, cm.cur.Line+max(1, rows/2))
	default:
		return nil // andere Tasten gehen nicht an den Server
	}
	a.copyShow(s)
	return nil
}

// copyKeys: Tastenzeile im Markiermodus.
func copyKeys(s *session) [][2]string {
	k := [][2]string{{"←↑↓→ hjkl", i18n.T("bewegen")}, {"w/b", i18n.T("Wort")}, {"0/$", i18n.T("Zeile")}, {"g/G", i18n.T("Anfang/Ende")}}
	if s.copy != nil && s.copy.anchor != nil {
		k = append(k, [2]string{"y/⏎", i18n.T("kopieren")})
	} else {
		k = append(k, [2]string{"v", i18n.T("markieren")}, [2]string{"V", i18n.T("Zeilen")})
	}
	return append(k, [2]string{"Esc", i18n.T("beenden")})
}
