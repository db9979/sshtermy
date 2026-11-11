package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestShellLock(t *testing.T) {
	if !configPrompt("sw-buero(config)#") || !configPrompt("[edit]") || configPrompt("sw-buero#") {
		t.Fatal("configPrompt")
	}
	if cliError([]string{"Invalid input: configure"}) == "" || cliError([]string{"logging 10.0.0.1 udp 5514"}) != "" {
		t.Fatal("cliError")
	}
	a := newPaletteTestApp(t)
	s := &session{}
	if a.shellBusyKey(s, tea.KeyPressMsg{Code: 'n', Text: "n"}) {
		t.Fatal("ohne laufende Folge gesperrt")
	}
	s.cap.hold()
	if !a.shellBusyKey(s, tea.KeyPressMsg{Code: 'n', Text: "n"}) {
		t.Fatal("Taste während sshTermy tippt nicht abgefangen")
	}
	if !a.shellBusyKey(s, tea.KeyPressMsg{Code: tea.KeyEscape}) || !s.cap.aborted() {
		t.Fatal("Esc bricht nicht ab")
	}
	s.cap.release()
	if s.cap.active() {
		t.Fatal("bleibt gesperrt")
	}
}
