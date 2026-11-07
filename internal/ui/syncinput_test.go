package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

func syncApp(t *testing.T) (*App, []*session, []*syncBuf) {
	a := dangerApp(t)
	a.intro = nil
	var ss []*session
	var outs []*syncBuf
	for i, alias := range []string{"sw1", "sw2", "prod-sw"} {
		c := domain.Connection{Alias: alias}
		if strings.HasPrefix(alias, "prod") {
			c.Tags = []string{"prod"}
		}
		s, out := typedSession(c, "$ ", "")
		s.id = i + 1
		ss, outs = append(ss, s), append(outs, out)
	}
	a.sessions, a.cur = ss, 0
	a.split, a.panes = splitCols, []int{1, 2}
	return a, ss, outs
}

func TestSyncInputTypesEverywhere(t *testing.T) {
	a, ss, outs := syncApp(t)
	a.Update(tea.KeyPressMsg{Code: 'i', Mod: tea.ModAlt})
	if !a.syncIn {
		t.Fatal("Alt+I schaltet nicht ein")
	}
	for _, r := range "show vlan" {
		a.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for i := 0; i < 2; i++ {
		if !strings.Contains(outs[i].String(), "show vlan\r") {
			t.Fatalf("Bereich %d bekam %q", i, outs[i].String())
		}
	}
	if outs[2].String() != "" {
		t.Fatalf("verdeckter Tab tippt mit: %q", outs[2].String())
	}
	if !strings.Contains(a.paneTitle(ss[1], 80, false), "alle") {
		t.Fatal("Titelzeile ohne Hinweis")
	}
	a.Update(tea.PasteMsg{Content: "a\nb"})
	if strings.Contains(outs[0].String(), "a\nb") || strings.Contains(outs[0].String(), "a\rb") {
		t.Fatal("mehrzeiliges Einfügen an alle")
	}
	a.setSplit(splitOff)
	if a.syncIn {
		t.Fatal("bleibt nach dem Aufheben der Teilung an")
	}
}

func TestSyncInputDangerBlocksEnter(t *testing.T) {
	a, ss, outs := syncApp(t)
	a.panes = []int{1, 3} // sw1 + Produktiv-Switch
	a.syncIn = true
	// das Echo des Servers fehlt im Test: Zeile direkt in die Terminals schreiben
	for _, i := range []int{0, 2} {
		_, _ = ss[i].term.Write([]byte("reboot"))
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for _, i := range []int{0, 2} {
		if strings.Contains(outs[i].String(), "\r") {
			t.Fatalf("Enter trotz Gefahr an %s gesendet: %q", ss[i].conn.Alias, outs[i].String())
		}
	}
	if a.modal != nil {
		t.Fatal("Dialog statt Sperre")
	}
}
