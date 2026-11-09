package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Laufende Antwort: Esc bricht ab, späte Brocken fallen weg, danach geht
// sofort eine neue Frage oder eine Folgefrage.
func TestAskStopAndFollowUp(t *testing.T) {
	a := splitApp(1)
	a.w = 140
	s := withPanel(a.sessions[0], panelAsk)
	a.panelOpened(s)
	q := &s.ask
	q.begin("Warum startet nginx nicht?")
	q.answer.WriteString("Port 80 ist belegt.\n\n```sh\nss -tlnp\n```")
	key := func(l string) {
		t.Helper()
		k, ok := keyFromLabel(l)
		if !ok {
			t.Fatalf("Taste %q", l)
		}
		a.panelMsg(s, k)
	}
	view := func() string {
		var b strings.Builder
		a.viewAsk(&b, s, 60)
		return ansi.Strip(b.String())
	}
	if !strings.Contains(view(), "Esc Antwort abbrechen") {
		t.Fatalf("Abbrechen-Knopf fehlt:\n%s", view())
	}
	seq := q.seq
	key("Esc")
	if q.busy || !q.stopped || q.seq == seq || !s.panelFocus {
		t.Fatalf("Esc muss die Antwort abbrechen und im Panel bleiben: busy=%v stopped=%v", q.busy, q.stopped)
	}
	a.sessionMsg(askDeltaMsg{s.id, seq, " zu spät"})
	if strings.Contains(q.answer.String(), "zu spät") {
		t.Fatal("Brocken der abgebrochenen Anfrage darf nicht ankommen")
	}
	v := view()
	if !strings.Contains(v, "abgebrochen") || !strings.Contains(v, "n neue Frage") || !strings.Contains(v, "f Folgefrage") {
		t.Fatalf("nach dem Abbruch fehlen Hinweise:\n%s", v)
	}

	key("f")
	if !q.follow || !q.input.Focused() {
		t.Fatal("f öffnet die Folgefrage")
	}
	if v := view(); !strings.Contains(v, "Port 80") || !strings.Contains(v, "Folgefrage") {
		t.Fatalf("Folgefrage zeigt die alte Antwort und darunter die Eingabe:\n%s", v)
	}
	key("Esc")
	if q.input.Focused() || q.follow || !s.panelFocus {
		t.Fatal("Esc in der Eingabe führt zurück zur Antwort")
	}
	key("n")
	if q.follow || !q.input.Focused() || strings.Contains(view(), "Port 80") {
		t.Fatal("n: neue Frage ohne alte Antwort")
	}

	// zweites Esc nach dem Abbruch geht ins Terminal
	key("Esc")
	key("Esc")
	if s.panelFocus {
		t.Fatal("ohne laufende Antwort führt Esc ins Terminal")
	}
}
