package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
)

// Lange Abschlusszusammenfassung des Agenten: beginnt oben im Panel, der Rest
// ist per Bild↓/Mausrad erreichbar, c kopiert, l zeigt sie ganz.
func TestAgentDoneSummaryScrolls(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{Alias: "lab-01"}, lvlNormal)
	var parts []string
	for i := 1; i <= 30; i++ {
		parts = append(parts, "Befund "+string(rune('A'+i%26))+": alles in Ordnung, geprüft.")
	}
	sum := strings.Join(parts, "\n") + "\nServer ist sauber und ENDE."
	a.sessionMsg(stepMsg(s, agent.Step{Done: true, Suggestion: agent.Suggestion{Reason: sum}}))
	if !s.goal.done || s.goal.doneReason != sum {
		t.Fatalf("Zusammenfassung nicht übernommen: %q", s.goal.doneReason)
	}
	s.panelFocus = true
	view := func() string { return ansi.Strip(a.viewPanel(s, 60, 20)) }
	v := view()
	if !strings.Contains(v, "✓ Ziel erreicht") {
		t.Fatalf("Anfang der Zusammenfassung muss sichtbar sein:\n%s", v)
	}
	if strings.Contains(v, "ENDE") || !strings.Contains(v, "weitere Zeilen") {
		t.Fatalf("Rest sollte per Blättern kommen, mit Hinweis:\n%s", v)
	}
	for i := 0; i < 10 && !strings.Contains(view(), "ENDE"); i++ {
		a.panelMsg(s, tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if !strings.Contains(view(), "ENDE") {
		t.Fatalf("Ende nicht erreichbar:\n%s", view())
	}
	if !s.scrollPanel(-1000) || !strings.Contains(view(), "Agent-Modus") {
		t.Fatalf("zurück nach oben geht nicht:\n%s", view())
	}
	a.panelMsg(s, runeKey('l'))
	if a.modal == nil || a.modal.kind != mText || !strings.Contains(strings.Join(a.modal.lines, "\n"), "ENDE") {
		t.Fatal("l muss die ganze Zusammenfassung zeigen")
	}
	a.modal = nil
	if cmd := a.panelMsg(s, runeKey('c')); cmd == nil {
		t.Error("c muss kopieren")
	}
}

// Kurzer Inhalt: nichts zu blättern, das Mausrad bleibt bei der Auswahl.
func TestPanelScrollShort(t *testing.T) {
	a, s, _ := agentSession(t, domain.Connection{Alias: "lab-01"}, lvlNormal)
	_ = a.viewPanel(s, 60, 80)
	if s.scrollPanel(3) {
		t.Error("kurzer Inhalt darf nicht blättern")
	}
}
