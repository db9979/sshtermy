package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestScrollKeys(t *testing.T) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{ID: "c1", Alias: "lab"}, "$ ", "")
	a.sessions = []*session{s}
	a.scr = scrTerminal
	for i := 0; i < 200; i++ {
		_, _ = s.term.Write([]byte(fmt.Sprintf("zeile %d\r\n", i)))
	}
	sb := s.term.ScrollbackLen()
	_, h := s.term.Size()
	if sb == 0 {
		t.Fatal("kein Verlauf")
	}
	a.updateTerminal(tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModShift})
	if s.scroll != min(h-1, sb) {
		t.Fatalf("⇧Bild↑: scroll=%d, erwartet %d", s.scroll, h-1)
	}
	a.updateTerminal(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModShift})
	if s.scroll != sb {
		t.Fatalf("⇧Pos1: scroll=%d, erwartet %d", s.scroll, sb)
	}
	a.updateTerminal(tea.KeyPressMsg{Code: tea.KeyPgDown, Mod: tea.ModShift})
	if s.scroll != sb-(h-1) {
		t.Fatalf("⇧Bild↓: scroll=%d", s.scroll)
	}
	a.updateTerminal(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModShift})
	if s.scroll != 0 {
		t.Fatalf("⇧Ende: scroll=%d", s.scroll)
	}
}
