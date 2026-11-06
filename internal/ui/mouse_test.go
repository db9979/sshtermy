package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestKeyFromLabel(t *testing.T) {
	for label, want := range map[string]string{
		"⏎": "enter", "Esc": "esc", "Tab": "tab", "F2": "f2", "F10": "f10", "n": "n", "/": "/", ".": ".", "~": "~",
		"Strg+S": "ctrl+s", "Strg+Space": "ctrl+space", "Strg+Q": "ctrl+q", "Strg+T": "ctrl+t", "←": "left",
	} {
		k, ok := keyFromLabel(label)
		if !ok || k.String() != want {
			t.Errorf("%q → %q (ok=%v), erwartet %q", label, k.String(), ok, want)
		}
	}
	for _, label := range []string{"↑↓", "1…9", "Alt+1…4", "Esc/F5"} {
		if _, ok := keyFromLabel(label); ok {
			t.Errorf("%q sollte nicht klickbar sein", label)
		}
	}
	if k, ok := keyFromKeyString("alt+2"); !ok || k.String() != "alt+2" {
		t.Errorf("alt+2 → %q", k.String())
	}
}

// Beendete Sitzung: die Hinweise in der Mitte („x Tab schließen“, „⏎ neu
// verbinden“) reagieren auf Klicks, obwohl sie in der Terminalfläche liegen.
func TestClosedSessionHintsClickable(t *testing.T) {
	a, st := rawTestApp(t)
	c, err := st.UpsertConnection(domain.Connection{Alias: "weg", Host: "127.0.0.1", Port: 1, Username: "root"})
	if err != nil {
		t.Fatal(err)
	}
	a.intro = nil
	s := &session{id: 1, conn: c, chipArmed: -1, state: stClosed, err: "Sitzung beendet"}
	a.sessions = []*session{s}

	at := func(word string) (int, int) {
		t.Helper()
		lines := strings.Split(ansi.Strip(a.View().Content), "\n")
		for y, l := range lines {
			if i := strings.Index(l, word); i >= 0 && y > 2 && y < len(lines)-2 { // nicht Kopf/Fußzeile
				return ansi.StringWidth(l[:i]) + 1, y
			}
		}
		t.Fatalf("%q nicht gefunden:\n%s", word, strings.Join(lines, "\n"))
		return 0, 0
	}
	click := func(x, y int) {
		a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	}

	x, y := at("neu verbinden")
	click(x, y)
	if s.state != stConnecting {
		t.Fatalf("Klick auf „neu verbinden“: Zustand %v", s.state)
	}
	s.state = stClosed
	x, y = at("Tab schließen")
	click(x, y)
	if len(a.sessions) != 0 {
		t.Fatal("Klick auf „Tab schließen“ schließt den Tab nicht")
	}
}
