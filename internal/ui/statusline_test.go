package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

// Schmale Fenster (uConsole mit großer Schrift): Die Kopfzeile lässt Teile
// weg, statt den KI-Modellnamen mitten im Wort abzuschneiden.
func TestSessionBarNarrow(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: 1, conn: domain.Connection{Alias: "web-01", Host: "10.0.0.1", Port: 22, Username: "dominik"}, chipArmed: -1}
	model := a.st.LLM().Model
	if model == "" {
		t.Skip("kein Standardmodell")
	}
	for _, w := range []int{140, 100, 80, 64, 50} {
		a.w = w
		bar := ansi.Strip(a.sessionBar(s))
		if lipgloss.Width(bar) > w {
			t.Errorf("w=%d: Kopfzeile %d breit", w, lipgloss.Width(bar))
		}
		if !strings.Contains(bar, "✦") {
			t.Errorf("w=%d: KI-Anzeige fehlt: %q", w, bar)
		}
		if strings.Contains(bar, model[:3]) && !strings.Contains(bar, trunc(model, 22)) {
			t.Errorf("w=%d: Modellname abgeschnitten: %q", w, bar)
		}
	}
	a.w = 140
	if bar := ansi.Strip(a.sessionBar(s)); !strings.Contains(bar, trunc(model, 22)) {
		t.Errorf("breit: Modellname fehlt: %q", bar)
	}
}
