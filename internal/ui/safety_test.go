package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/safe"
)

func safetyApp(t *testing.T) *App {
	t.Helper()
	a := newPaletteTestApp(t)
	safe.Reset()
	a.CatchPanics()
	t.Cleanup(safe.Reset)
	return a
}

// waitCaught holt die Meldung über die abgefangene Panik aus a.events.
func waitCaught(t *testing.T, a *App) caughtMsg {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m := <-a.events:
			if c, ok := m.(caughtMsg); ok {
				return c
			}
		case <-deadline:
			t.Fatal("keine Meldung über den abgefangenen Fehler")
		}
	}
}

func TestUpdatePanicIsCaught(t *testing.T) {
	a := safetyApp(t)
	before := safe.Count()
	_, cmd := a.Update(crashTestMsg{}) // panic in update0
	if cmd == nil {
		t.Fatal("nach dem Fehler muss der Empfang neu angestoßen werden")
	}
	if safe.Count() != before+1 {
		t.Fatal("Panik nicht gezählt")
	}
	c := waitCaught(t, a)
	if c.where != "ui.Update" || c.path == "" {
		t.Fatalf("Meldung unvollständig: %+v", c)
	}
	raw, err := os.ReadFile(c.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "hunter2") {
		t.Fatal("Bericht enthält das Passwort")
	}
	if !strings.Contains(string(raw), "sshTermy crash test (ui)") {
		t.Fatal("Bericht ohne Meldung")
	}
	a.Update(c)
	if len(a.toasts) == 0 || a.toasts[len(a.toasts)-1].kind != toastErr {
		t.Fatal("keine Fehlermeldung oben rechts")
	}
}

func TestTickPanicKeepsTicking(t *testing.T) {
	a := safetyApp(t)
	// Panik mitten im Takt: rearm muss den Takt neu anstoßen.
	cmd := a.rearm(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("Takt nicht neu angestoßen")
	}
}

func TestGuardCmdNested(t *testing.T) {
	_ = safetyApp(t)
	boom := func() tea.Msg { panic("cmd boom") }
	ok := func() tea.Msg { return "fein" }
	c := guardCmd(tea.Batch(boom, ok))
	m := c()
	batch, isBatch := m.(tea.BatchMsg)
	if !isBatch || len(batch) != 2 {
		t.Fatalf("Batch verloren: %T", m)
	}
	if got := batch[0](); got != nil {
		t.Fatalf("abgefangene Panik muss nil liefern, nicht %v", got)
	}
	if got := batch[1](); got != "fein" {
		t.Fatalf("zweites Kommando: %v", got)
	}
	// Sequence (nicht exportierter Typ) bleibt dieselbe Art Liste
	s := guardCmd(tea.Sequence(boom, ok))()
	if s == nil {
		t.Fatal("Sequence verloren")
	}
	if _, isBatch := s.(tea.BatchMsg); isBatch {
		t.Fatal("Sequence darf kein Batch werden (Reihenfolge)")
	}
}

func TestOutputPanicDropsBlock(t *testing.T) {
	a := safetyApp(t)
	w := outWriter{s: nil, ev: nil} // nil-Sitzung: write0 stürzt ab
	n, err := w.Write([]byte("hallo"))
	if n != 5 || err != nil {
		t.Fatalf("Block muss verworfen werden, Verbindung bleibt: %d %v", n, err)
	}
	waitCaught(t, a) // Bericht fertig, bevor der Testordner verschwindet
}

func TestWithoutHandlerPanicsStillSurface(t *testing.T) {
	a := newPaletteTestApp(t)
	safe.SetHandler(nil)
	defer func() {
		if recover() == nil {
			t.Fatal("ohne Absturzschutz (Tests) muss die Panik durchlaufen")
		}
	}()
	a.Update(crashTestMsg{})
}
