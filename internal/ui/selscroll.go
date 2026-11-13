package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Markieren über den Bildschirmrand hinaus
//
// Zieht man beim Markieren über den oberen oder unteren Rand der
// Terminalfläche, blättert sshTermy weiter, solange die Maus dort bleibt
// (je weiter weg, desto schneller), und die Markierung wächst mit.
// ⇧+Klick erweitert eine bestehende Markierung bis zur Klickstelle, auch
// nachdem man mit dem Rad weitergeblättert hat.
//
// In tmux liegt der Verlauf beim Server: Die Markierung merkt sich dort
// Zeilen in tmux-Zählung (0 = erste Zeile des Bereichs, negativ = Verlauf),
// blättert über den Copy-Mode (tmuxscroll.go) und holt den Text beim
// Loslassen mit capture-pane, wenn er nicht ganz auf dem Bildschirm steht.

const edgeEvery = 60 * time.Millisecond

type edgeTickMsg struct{ sid int }

func edgeTick(sid int) tea.Cmd {
	return tea.Tick(edgeEvery, func(time.Time) tea.Msg { return edgeTickMsg{sid} })
}

// tmuxSel: Markierung in tmux-Zeilen.
type tmuxSel struct {
	a, b     termemu.Pos
	verified bool // Position von tmux bestätigt
	assumed  int  // angenommene Position beim Klick
}

// edgeSpeed: Zeilen je Schritt, je nach Abstand zum Rand.
func edgeSpeed(dist int) int { return clamp(1+dist/2, 1, 8) }

// selEdge merkt sich beim Ziehen, ob die Maus über dem Rand steht.
// dir > 0: nach oben (zurück), < 0: nach unten.
func (a *App) selEdge(s *session, tx, ty, cols, rows int) tea.Cmd {
	s.edgeX = clamp(tx, 0, cols-1)
	switch {
	case ty < 0:
		s.edgeDir = edgeSpeed(-ty - 1)
	case ty >= rows:
		s.edgeDir = -edgeSpeed(ty - rows)
	default:
		s.edgeDir = 0
	}
	if s.edgeDir != 0 && !s.edgeTicking {
		s.edgeTicking = true
		return a.onEdgeTick(s) // gleich ein Schritt, dann im Takt
	}
	return nil
}

func (a *App) onEdgeTick(s *session) tea.Cmd {
	if s == nil || !s.selecting || s.edgeDir == 0 || s.term == nil {
		if s != nil {
			s.edgeTicking = false
		}
		return nil
	}
	cols, rows := a.termSize(s)
	row := 0
	if s.edgeDir < 0 {
		row = rows - 1
	}
	switch {
	case s.tsel != nil:
		// erst weiter, wenn tmux die letzte Aktion beantwortet hat
		if s.tmuxBusy.IsZero() || time.Since(s.tmuxBusy) > 2*time.Second {
			op, n := tmuxWheelUp, s.edgeDir
			if n < 0 {
				op, n = tmuxWheelDown, -n
			}
			s.tmuxBusy = time.Now()
			a.tmuxDoN(s, op, n)
		}
		s.tsel.b = termemu.Pos{Line: row - s.tmuxPos, Col: s.edgeX}
		a.syncTmuxSel(s)
	case s.sel != nil && !s.term.AltScreen():
		s.scroll = clamp(s.scroll+s.edgeDir, 0, s.term.ScrollbackLen())
		s.sbSeen = s.term.ScrollbackLen()
		s.sel.B = s.term.ScreenPos(clamp(s.edgeX, 0, cols-1), row, s.scroll)
	default:
		s.edgeTicking = false
		return nil
	}
	return edgeTick(s.id)
}

// tmuxSelecting: markiert sshTermy gerade in tmux (statt im eigenen Verlauf)?
func (s *session) tmuxSelectable() bool {
	return s.inTmux() && s.term.MouseTracking() == termemu.MouseOff && !s.tmuxAlt
}

// startTmuxSel: Klick in tmux beginnt eine Markierung in tmux-Zeilen.
func (a *App) startTmuxSel(s *session, x, y int) {
	p := termemu.Pos{Line: y - s.tmuxPos, Col: x}
	s.tsel = &tmuxSel{a: p, b: p, assumed: s.tmuxPos}
	a.tmuxDo(s, tmuxQuery) // wo steht tmux wirklich?
	a.syncTmuxSel(s)
}

// onTmuxState: tmux meldet seine Position (nach jeder Aktion).
func (a *App) onTmuxState(s *session, st tmuxState) {
	s.tmuxBusy = time.Time{}
	s.tmuxAlt = st.alt && !st.inMode
	if t := s.tsel; t != nil && !t.verified {
		d := t.assumed - st.pos // beim Klick sah die Zeile y wirklich y-pos
		if !s.selecting || s.edgeDir == 0 {
			t.a.Line += d
			t.b.Line += d
		} else {
			t.a.Line += d
		}
		t.verified = true
	}
	s.tmuxPos = st.pos
	if t := s.tsel; t != nil && s.selecting && s.edgeDir != 0 {
		_, rows := a.termSize(s)
		row := 0
		if s.edgeDir < 0 {
			row = rows - 1
		}
		t.b = termemu.Pos{Line: row - s.tmuxPos, Col: s.edgeX}
	}
	a.syncTmuxSel(s)
}

// syncTmuxSel zeigt den sichtbaren Teil der tmux-Markierung als s.sel.
func (a *App) syncTmuxSel(s *session) {
	t := s.tsel
	if t == nil || s.term == nil {
		return
	}
	cols, rows := a.termSize(s)
	lo, hi := t.a, t.b
	if hi.Before(lo) {
		lo, hi = hi, lo
	}
	ya, yb := lo.Line+s.tmuxPos, hi.Line+s.tmuxPos
	if yb < 0 || ya >= rows {
		s.sel = &termemu.Sel{A: termemu.Pos{Line: -1, Col: 0}, B: termemu.Pos{Line: -1, Col: 0}}
		return
	}
	ca, cb := lo.Col, hi.Col
	if ya < 0 {
		ya, ca = 0, 0
	}
	if yb >= rows {
		yb, cb = rows-1, cols-1
	}
	sb := s.term.ScrollbackLen()
	s.sel = &termemu.Sel{A: termemu.Pos{Line: sb + ya, Col: ca}, B: termemu.Pos{Line: sb + yb, Col: cb}}
}

// tmuxSelOnScreen: steht die ganze Markierung gerade auf dem Bildschirm?
func (a *App) tmuxSelOnScreen(s *session) bool {
	t := s.tsel
	_, rows := a.termSize(s)
	for _, p := range []termemu.Pos{t.a, t.b} {
		if y := p.Line + s.tmuxPos; y < 0 || y >= rows {
			return false
		}
	}
	return true
}

// copyTmuxSel kopiert die tmux-Markierung; liegt ein Teil außerhalb des
// Bildschirms, kommt der Text per capture-pane vom Server.
func (a *App) copyTmuxSel(s *session) tea.Cmd {
	if s.tsel == nil || (s.tsel.verified && a.tmuxSelOnScreen(s)) || !s.inTmux() {
		return a.copySel(s)
	}
	lo, hi := s.tsel.a, s.tsel.b
	if hi.Before(lo) {
		lo, hi = hi, lo
	}
	t, _ := tmuxTarget(s.mux.name)
	c, sid := s.client, s.id
	cmd := fmt.Sprintf("tmux capture-pane -p -S %d -E %d%s", lo.Line, hi.Line, t)
	a.flash(i18n.T("Hole die Markierung aus tmux …"))
	return func() tea.Msg {
		out, err := c.Output(cmd, 10*time.Second)
		return relayMsg{apply: func(a *App) tea.Cmd {
			if err != nil {
				a.flashErr(i18n.T("Kopieren aus tmux ging nicht: ") + err.Error())
				return nil
			}
			text := cutSelection(out, hi.Line-lo.Line+1, lo.Col, hi.Col)
			if strings.TrimSpace(text) == "" {
				return nil
			}
			go func() { _ = clipboard.WriteAll(text) }()
			if s := a.byID(sid); s != nil {
				a.flash(i18n.T("Kopiert: ") + plural(strings.Count(text, "\n")+1, i18n.T("Zeile"), i18n.T("Zeilen")) + i18n.T(" · Rechtsklick fügt ein"))
			}
			return tea.SetClipboard(text)
		}}
	}
}

// cutSelection: n Zeilen aus capture-pane, erste ab Spalte from, letzte bis
// Spalte to (einschließlich).
func cutSelection(out string, n, from, to int) string {
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for i := range lines {
		l := lines[i]
		if i == len(lines)-1 && len(lines) == n {
			l = ansi.Cut(l, 0, to+1)
		}
		if i == 0 {
			l = ansi.Cut(l, from, ansi.StringWidth(l))
		}
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}
