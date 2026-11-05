package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestAIInputWrapsAndGrows(t *testing.T) {
	in := newAIInput("")
	in.SetWidth(20)
	in.Focus()
	in.SetValue(strings.Repeat("wort ", 12)) // 60 Zeichen → mehrere Zeilen
	in.SetWidth(20)
	v := in.View()
	if h := lipgloss.Height(v); h < 3 {
		t.Fatalf("Feld sollte mitwachsen, Höhe %d:\n%s", h, v)
	}
	for _, l := range strings.Split(v, "\n") {
		if w := lipgloss.Width(l); w > 20 {
			t.Fatalf("Zeile breiter als Feld (%d): %q", w, l)
		}
	}
}

func TestAIInputNewlineKeys(t *testing.T) {
	in := newAIInput("")
	in.Focus()
	in.SetValue("a")
	in, _ = in.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if in.Value() != "a" {
		t.Fatalf("⏎ darf keine Zeile einfügen: %q", in.Value())
	}
	in, _ = in.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	in, _ = in.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if in.Value() != "a\n\n" {
		t.Fatalf("Shift/Alt+⏎ sollte Zeilen einfügen: %q", in.Value())
	}
}

func TestCommandInputSingleLine(t *testing.T) {
	in := newCommandInput()
	in.Focus()
	in, _ = in.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	in, _ = in.Update(tea.PasteMsg{Content: "ls\n-la"})
	if strings.Contains(in.Value(), "\n") {
		t.Fatalf("Befehl darf keine Zeilenumbrüche enthalten: %q", in.Value())
	}
}
