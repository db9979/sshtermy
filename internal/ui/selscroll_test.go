package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/termemu"
)

func selApp(t *testing.T, lines int) (*App, *session, int, int, rect) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{ID: "c1", Alias: "lab"}, "", "")
	a.sessions = []*session{s}
	cols, rows := a.termSize(s)
	s.term.Resize(cols, rows)
	for i := 0; i < lines; i++ {
		_, _ = s.term.Write([]byte(fmt.Sprintf("zeile %d\r\n", i)))
	}
	return a, s, cols, rows, a.termRect(s)
}

func mclick(x, y int, mod tea.KeyMod) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: mod}
}

// Ziehen über den oberen Rand blättert weiter, solange die Maus dort
// bleibt; die Markierung wächst mit (früher: nur eine Bildschirmseite).
func TestSelectionScrollsPastEdge(t *testing.T) {
	a, s, _, rows, r := selApp(t, 300)
	a.termMouse(s, mclick(r.x+2, r.y+rows-3, 0), 1)
	cmd := a.termMouse(s, tea.MouseMotionMsg{X: r.x + 2, Y: r.y - 3, Button: tea.MouseLeft}, 0)
	if cmd == nil || s.edgeDir <= 0 {
		t.Fatalf("über dem Rand kein Weiterblättern: dir=%d", s.edgeDir)
	}
	for i := 0; i < 200; i++ {
		a.onEdgeTick(s)
	}
	if s.scroll != s.term.ScrollbackLen() {
		t.Fatalf("nicht bis zum Anfang geblättert: scroll=%d von %d", s.scroll, s.term.ScrollbackLen())
	}
	a.termMouse(s, tea.MouseReleaseMsg{X: r.x + 2, Y: r.y - 3, Button: tea.MouseLeft}, 0)
	if s.edgeDir != 0 || s.selecting {
		t.Fatal("Loslassen beendet das Blättern nicht")
	}
	text := s.term.Text(*s.sel)
	if !strings.HasPrefix(text, "ile 0\nzeile 1\n") || strings.Count(text, "\n") < 250 {
		t.Fatalf("Markierung zu kurz: %d Zeilen, Anfang %q", strings.Count(text, "\n")+1, text[:min(20, len(text))])
	}
	// zurück in die Fläche: Blättern hört auf
	a.termMouse(s, mclick(r.x+2, r.y+2, 0), 1)
	a.termMouse(s, tea.MouseMotionMsg{X: r.x + 2, Y: r.y + rows + 2, Button: tea.MouseLeft}, 0)
	a.termMouse(s, tea.MouseMotionMsg{X: r.x + 2, Y: r.y + 5, Button: tea.MouseLeft}, 0)
	if s.edgeDir != 0 {
		t.Fatal("in der Fläche weitergeblättert")
	}
}

// ⇧+Klick erweitert die Markierung, auch nach dem Blättern mit dem Rad.
func TestShiftClickExtends(t *testing.T) {
	a, s, _, rows, r := selApp(t, 300)
	a.termMouse(s, mclick(r.x, r.y+rows-2, 0), 1)
	a.termMouse(s, tea.MouseMotionMsg{X: r.x + 5, Y: r.y + rows - 2, Button: tea.MouseLeft}, 0)
	a.termMouse(s, tea.MouseReleaseMsg{X: r.x + 5, Y: r.y + rows - 2, Button: tea.MouseLeft}, 0)
	s.scroll = 100 // Rad
	a.termMouse(s, mclick(r.x, r.y, tea.ModShift), 1)
	if s.selecting {
		t.Fatal("⇧+Klick soll nicht neu anfangen")
	}
	if n := strings.Count(s.term.Text(*s.sel), "\n"); n < 100 {
		t.Fatalf("Markierung nicht erweitert: %d Zeilen", n+1)
	}
}

// tmux: Markierung in tmux-Zeilen folgt dem Blättern im Copy-Mode.
func TestTmuxSelFollowsScroll(t *testing.T) {
	a, s, cols, rows, _ := selApp(t, 5)
	_, _ = s.term.Write([]byte("\x1b[?1049h"))
	s.tsel = &tmuxSel{a: termemu.Pos{Line: 10, Col: 3}, b: termemu.Pos{Line: 12, Col: 4}, verified: true}
	a.syncTmuxSel(s)
	sb := s.term.ScrollbackLen()
	if s.sel.A.Line != sb+10 || s.sel.B.Line != sb+12 {
		t.Fatalf("Bildschirm: %+v", s.sel)
	}
	a.onTmuxState(s, tmuxState{inMode: true, pos: 5})
	if s.sel.A.Line != sb+15 || s.sel.B.Line != sb+17 {
		t.Fatalf("nach 5 Zeilen hoch: %+v", s.sel)
	}
	a.onTmuxState(s, tmuxState{inMode: true, pos: rows + 20})
	if !(s.sel.A.Line > sb+rows || s.sel.A.Line < 0) && s.sel.A.Line != -1 {
		t.Fatalf("außerhalb muss leer sein: %+v", s.sel)
	}
	// Markierung reicht über den oberen Rand: oben abgeschnitten
	s.tsel = &tmuxSel{a: termemu.Pos{Line: -50, Col: 3}, b: termemu.Pos{Line: 2, Col: 4}, verified: true}
	a.onTmuxState(s, tmuxState{inMode: true, pos: 10})
	if s.sel.A != (termemu.Pos{Line: sb, Col: 0}) || s.sel.B != (termemu.Pos{Line: sb + 12, Col: 4}) {
		t.Fatalf("abgeschnitten: %+v (cols %d)", s.sel, cols)
	}
	if a.tmuxSelOnScreen(s) {
		t.Fatal("teilweise außerhalb gilt nicht als sichtbar")
	}
	// Klick vor bestätigter Position: tmux stand woanders → nachziehen
	s.tmuxPos = 0
	s.tsel = &tmuxSel{a: termemu.Pos{Line: 4, Col: 0}, b: termemu.Pos{Line: 4, Col: 0}, assumed: 0}
	a.onTmuxState(s, tmuxState{inMode: true, pos: 30})
	if s.tsel.a.Line != 4-30 {
		t.Fatalf("Klickzeile nicht nachgezogen: %d", s.tsel.a.Line)
	}
}

func TestCutSelection(t *testing.T) {
	out := "abcdef\nzweite\ndritte zeile\n"
	if got := cutSelection(out, 3, 2, 5); got != "cdef\nzweite\ndritte" {
		t.Fatalf("%q", got)
	}
	if got := cutSelection("nur eine\n", 1, 4, 6); got != "ein" {
		t.Fatalf("%q", got)
	}
}
