package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

func cmKey(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "alt+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func TestCopyModeKeyboard(t *testing.T) {
	a := dangerApp(t)
	s, out := typedSession(domain.Connection{Alias: "web1"}, "", "")
	s.term.Write([]byte("eth0 10.0.0.5 up\r\neth1 192.168.1.7 down\r\n$ "))
	a.sessions, a.cur = []*session{s}, 0

	a.updateTerminal(cmKey("alt+c"))
	if s.copy == nil {
		t.Fatal("Markiermodus nicht an")
	}
	// zwei Zeilen hoch, zum zweiten Wort, Wort markieren, kopieren
	a.updateTerminal(cmKey("k"))
	a.updateTerminal(cmKey("0"))
	a.updateTerminal(cmKey("w"))
	a.updateTerminal(cmKey("v"))
	a.updateTerminal(cmKey("e"))
	if got := s.term.Text(*s.sel); got != "192.168.1.7" {
		t.Fatalf("Markierung = %q", got)
	}
	a.updateTerminal(cmKey("y"))
	if s.copy != nil || s.sel != nil {
		t.Fatal("y beendet nicht")
	}
	if out.String() != "" {
		t.Fatalf("Tasten gingen an den Server: %q", out.String())
	}

	// Zeilen markieren
	a.updateTerminal(cmKey("alt+c"))
	a.updateTerminal(cmKey("k"))
	a.updateTerminal(cmKey("k"))
	a.updateTerminal(cmKey("V"))
	a.updateTerminal(cmKey("j"))
	if got := s.term.Text(*s.sel); got != "eth0 10.0.0.5 up\neth1 192.168.1.7 down" {
		t.Fatalf("Zeilen = %q", got)
	}
	a.updateTerminal(cmKey("esc"))
	if s.copy != nil {
		t.Fatal("Esc beendet nicht")
	}
}
