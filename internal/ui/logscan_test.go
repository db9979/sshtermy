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
func TestLogScanSwitch(t *testing.T) { logScanSwitch(t, false) }

// Echte ArubaOS-Switches beginnen Zeilen mit Cursor-Sprüngen statt "\r\n"
// (Fehler aus dem Praxistest: „no page“: Prompt kam nicht zurück).
func TestLogScanSwitchPositioned(t *testing.T) { logScanSwitch(t, true) }

func logScanSwitch(t *testing.T, positioned bool) {
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
	sw.Positioned = positioned

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
	// empfangenes Syslog (älter als der Puffer im Switch) kommt mit hinein
	poe, _ := logscan.SyslogPacket("<131>Nov 10 03:00:00 sw-lager 00561 chassis: Fan 1 failure detected", time.Now())
	poe.Time = time.Now().Add(-5 * time.Hour)
	a.lwatch.syslog = map[string][]logscan.Entry{c.ID: {poe}}
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
	for _, want := range []string{"Port 23 flattert", "Schleife erkannt", "Erste Prüfung", "Lüfter", "Syslog-Empfang: 1"} {
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

	// s: Syslog-Ziel am Switch einrichten (nach Bestätigung sichtbar getippt).
	a.Update(runeKey('s'))
	if a.modal == nil || a.modal.kind != mConfirm || !strings.Contains(a.modal.body, "logging 127.0.0.1 udp 5514") {
		t.Fatalf("Syslog-Dialog: %+v", a.modal)
	}
	setup := a.modal.onYes()
	a.modal = nil
	defer a.stopWatch()
	if setup == nil || a.st.Settings().SyslogPort != 5514 {
		t.Fatal("Syslog-Empfang nicht eingeschaltet")
	}
	if m, ok := setup().(syslogSetupMsg); !ok || m.err != nil {
		t.Fatalf("Einrichten: %+v %v", m, m.err)
	}
	waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })
	if !positioned && !strings.Contains(s.term.PlainLines(40), "logging 127.0.0.1 udp 5514") {
		t.Fatal("Befehl nicht sichtbar getippt")
	}
	a.scr = scrLogScan

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

	// Meldung der Logwache öffnet den Log-Blick beim Muster.
	vis := a.lsVisible()
	last := vis[len(vis)-1]
	_ = a.openWatchAlert(watchAlert{ConnID: c.ID, Key: last.Key, Host: c.Alias, Title: last.Title})
	if a.scr != scrLogScan || a.lsVisible()[a.lscan.sel].Key != last.Key {
		t.Fatalf("Log-Blick nicht beim Muster: %d", a.lscan.sel)
	}
	a.Update(keyMsg("esc"))
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
