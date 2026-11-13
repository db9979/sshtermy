package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

// Im Ziel-Panel lässt sich Text mit der Maus markieren und kopieren:
// Ziehen, Doppelklick (Wort), Dreifachklick (Zeile), Rechtsklick.
func TestGoalPanelSelectCopy(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{Alias: "lab-01"}, lvlNormal)
	a.w, a.h, a.intro = 160, 40, nil
	a.resizeAll()
	g := &s.goal
	g.busy = false
	g.err = "Antwort des Modells nicht lesbar: XYZ123 kaputt"
	s.panelFocus = false

	at := func(word string) (int, int) {
		t.Helper()
		lines := strings.Split(ansi.Strip(a.View().Content), "\n")
		for y, l := range lines {
			if i := strings.Index(l, word); i >= 0 {
				return ansi.StringWidth(l[:i]), y
			}
		}
		t.Fatalf("%q nicht gefunden:\n%s", word, strings.Join(lines, "\n"))
		return 0, 0
	}
	x, y := at("XYZ123")
	if !a.inPanel(s, x, y) {
		t.Fatalf("XYZ123 bei %d,%d liegt nicht im Panel", x, y)
	}

	// Ziehen über „XYZ123 kaputt“ (umbrochen auf zwei Zeilen)
	kx, ky := at("kaputt")
	a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseMotionMsg{X: x + 5, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseMotionMsg{X: kx + 6, Y: ky, Button: tea.MouseLeft})
	cmd := a.mouse(tea.MouseReleaseMsg{X: kx + 6, Y: ky, Button: tea.MouseLeft})
	if got := s.panelSelText(); got != "XYZ123\nkaputt" {
		t.Fatalf("Markierung = %q\n%s", got, ansi.Strip(a.View().Content))
	}
	if cmd == nil {
		t.Fatal("Loslassen muss kopieren (OSC 52)")
	}
	s.panelAt = time.Now().Add(-time.Hour) // ohne Einblenden
	view := a.View().Content
	// View schreibt die Theme-Grundfarben in jede Zelle (fillbg.go)
	want := fillDefaultColors(a.s.Selected.Render("kaputt"), a.s.T.Text, a.s.T.Bg)
	want = want[strings.IndexByte(want, 'm')+1:]
	if !strings.Contains(view, want) {
		t.Fatal("Markierung wird nicht hervorgehoben")
	}
	plain := strings.Split(ansi.Strip(view), "\n")
	if w := ansi.StringWidth(plain[y]); w != a.w || !strings.Contains(plain[y], "XYZ123   │") {
		t.Fatalf("Panelzeile verrutscht (%d Spalten): %q", w, plain[y])
	}

	// Doppelklick: Wort
	a.click = clickState{}
	a.mouse(tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if got := s.panelSelText(); got != "XYZ123" {
		t.Fatalf("Doppelklick = %q", got)
	}
	// Dreifachklick: ganze Zeile
	a.mouse(tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if got := s.panelSelText(); !strings.HasPrefix(got, "× Antwort des Modells") || !strings.Contains(got, "XYZ123") {
		t.Fatalf("Dreifachklick = %q", got)
	}

	// mehrere Zeilen: vom Ziel bis zum Fehler
	a.click = clickState{}
	gx, gy := at("Logrotation einrichten")
	a.mouse(tea.MouseClickMsg{X: gx, Y: gy, Button: tea.MouseLeft})
	a.mouse(tea.MouseMotionMsg{X: x + 6, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: x + 6, Y: y, Button: tea.MouseLeft})
	got := s.panelSelText()
	if !strings.HasPrefix(got, "Logrotation einrichten") || !strings.HasSuffix(got, "XYZ123") || strings.Count(got, "\n") < 2 {
		t.Fatalf("mehrzeilig = %q", got)
	}
	// Rechtsklick kopiert und hebt auf
	if cmd := a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight}); cmd == nil || s.psel != nil {
		t.Fatal("Rechtsklick muss die Markierung kopieren")
	}
	// Taste hebt die Markierung auf
	a.click = clickState{}
	a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseMotionMsg{X: x + 3, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: x + 3, Y: y, Button: tea.MouseLeft})
	a.panelMsg(s, runeKey('x'))
	if s.psel != nil {
		t.Fatal("Tippen muss die Markierung aufheben")
	}
}

// Klickbare Hinweise im Panel bleiben klickbar und starten keine Markierung.
func TestGoalPanelHintsStillClick(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{Alias: "lab-01"}, lvlNormal)
	a.w, a.h, a.intro = 160, 40, nil
	a.resizeAll()
	s.goal.busy = false
	s.goal.done, s.goal.doneReason = true, "alles erledigt"
	lines := strings.Split(ansi.Strip(a.View().Content), "\n")
	for y, l := range lines {
		if i := strings.Index(l, "ganz lesen"); i >= 0 {
			a.mouse(tea.MouseClickMsg{X: ansi.StringWidth(l[:i]), Y: y, Button: tea.MouseLeft})
			if s.psel != nil {
				t.Fatal("Klick auf Hinweis darf nicht markieren")
			}
			if a.modal == nil || a.modal.kind != mText {
				t.Fatal("Klick auf „l ganz lesen“ wirkt nicht mehr")
			}
			return
		}
	}
	t.Fatal("Hinweis nicht gefunden")
}
