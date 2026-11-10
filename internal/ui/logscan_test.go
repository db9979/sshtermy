package ui

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/devfake"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/logscan"
	"github.com/db9979/sshtermy/internal/store"
)

// Log-Blick an einem nachgebauten ProCurve-Switch: Befehle sichtbar an der
// Shell, Muster gruppiert, Rauschen ausblenden, Was ist neu?, Zeitleiste.
func TestLogScanSwitch(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	sw, err := devfake.StartSwitch("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sw.Close()

	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	a.w, a.h, a.scr, a.intro = 140, 40, scrTerminal, nil
	go func() {
		for m := range a.events {
			if hk, ok := m.(hostKeyAskMsg); ok {
				hk.reply <- true
			}
		}
	}()
	host, port, _ := net.SplitHostPort(sw.Addr)
	p, _ := strconv.Atoi(port)
	c, err := st.UpsertConnection(domain.Connection{Alias: "sw-lager", Host: host, Port: p, Username: "manager",
		Auth: domain.AuthPassword, Password: "x", Group: "Lager"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: 1, conn: c, chipArmed: -1}
	a.sessions = []*session{s}
	a.cur = 0
	msg := a.connectCmd(s)()
	if e, ok := msg.(connectErrMsg); ok {
		t.Fatal(e.err)
	}
	a.Update(msg)
	waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })

	scan := func(cmd tea.Cmd) {
		t.Helper()
		if cmd == nil {
			t.Fatalf("kein Lesen gestartet: %s", a.lscan.err)
		}
		m := cmd()
		a.Update(m)
		if a.lscan.err != "" || a.lscan.rep == nil {
			t.Fatalf("Fehler: %s", a.lscan.err)
		}
		waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })
	}
	_, cmd := a.Update(keyMsg("alt+e"))
	scan(cmd)
	if a.scr != scrLogScan {
		t.Fatal("Alt+E öffnet den Log-Blick nicht")
	}
	r := a.lscan.rep
	if r.Kind != logscan.KindNetwork || r.Host != "sw-lager" || !s.host.pagingOff {
		t.Fatalf("Bericht: %+v", r)
	}
	out := a.View().Content
	for _, want := range []string{"Port 23 flattert", "Schleife erkannt", "Erste Prüfung"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q fehlt:\n%s", want, out)
		}
	}
	if len(a.lsVisible()) < 2 {
		t.Fatalf("Muster: %d", len(a.lsVisible()))
	}

	// x blendet das gewählte Muster aus, u in "Was ist neu?" holt es zurück.
	first := a.lsVisible()[0]
	a.Update(runeKey('x'))
	if len(a.st.Settings().LogScan.Noise) != 1 || a.lsVisible()[0].Key == first.Key {
		t.Fatal("x blendet nicht aus")
	}
	if n := a.st.Settings().LogScan.Noise[0]; n.Conn != c.ID || n.Folder != "" {
		t.Fatalf("Rauschen: %+v", n)
	}
	a.Update(runeKey('n'))
	if a.lscan.view != lsNews || !strings.Contains(a.View().Content, "Ausgeblendet als Rauschen") {
		t.Fatal("Was ist neu? fehlt")
	}
	a.Update(runeKey('u'))
	if len(a.st.Settings().LogScan.Noise) != 0 || a.lsVisible()[0].Key != first.Key {
		t.Fatal("u holt nicht zurück")
	}
	a.Update(keyMsg("esc"))
	if a.lscan.view != lsList || a.scr != scrLogScan {
		t.Fatal("Esc aus der Unteransicht")
	}

	// Zeitleiste: Fehler und Warnungen je Abschnitt, ←→ wählt.
	a.Update(runeKey('t'))
	out = a.View().Content
	if a.lscan.view != lsTime || !strings.Contains(out, "Marken") || !strings.Contains(out, "Abschnitt") {
		t.Fatalf("Zeitleiste:\n%s", out)
	}
	sel := a.lscan.tl.sel
	a.Update(keyMsg("left"))
	if a.lscan.tl.sel != max(0, sel-1) {
		t.Fatal("← wählt keinen Abschnitt")
	}
	a.Update(keyMsg("end"))
	a.Update(keyMsg("enter"))
	if a.modal == nil {
		t.Fatal("⏎ zeigt die Zeilen nicht")
	}
	a.modalKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	a.Update(keyMsg("esc"))

	// Zweiter Blick: gleiche Muster, also nichts Neues.
	_, cmd = a.Update(runeKey('r'))
	scan(cmd)
	if len(a.lscan.gone) != 0 {
		t.Fatalf("Veränderungen beim zweiten Blick: %+v", a.lscan.gone)
	}
	for _, f := range a.lscan.rep.Findings {
		if f.Status == "new" {
			t.Fatalf("neu beim zweiten Blick: %s", f.Title)
		}
	}
	a.Update(keyMsg("esc"))
	if a.scr != scrTerminal {
		t.Fatal("Esc führt nicht zurück")
	}
}

// Ordner-Rauschen gilt für alle Hosts des Ordners, nicht für andere.
func TestLogNoiseFolder(t *testing.T) {
	a := newPaletteTestApp(t)
	c1 := domain.Connection{ID: "a", Alias: "a", Group: "Lager"}
	c2 := domain.Connection{ID: "b", Alias: "b", Group: "Lager"}
	c3 := domain.Connection{ID: "c", Alias: "c", Group: "Büro"}
	r := &logscan.Report{Kind: logscan.KindUnix, At: time.Now(), Hours: 24}
	r.Findings = []logscan.Finding{{Key: "unix:x", Title: "x", Sev: logscan.SevWarn, Count: 3}}
	a.lscan = logScanState{conn: c1, rep: r}
	a.addNoise(r.Findings[0], true)
	if !a.noiseFor(c2)("unix:x") || a.noiseFor(c3)("unix:x") {
		t.Fatal("Ordner-Rauschen falsch zugeordnet")
	}
	if len(a.lsVisible()) != 0 || len(a.lsNoise()) != 1 {
		t.Fatal("Ausgeblendetes noch in der Liste")
	}
}
