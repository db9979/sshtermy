package ui

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/devfake"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

// Ende-zu-Ende gegen einen nachgebauten ProCurve: Übersicht und Sicherung
// laufen über die interaktive Shell (keine Exec-Kanäle), "-- MORE --" wird
// weitergeblättert, ein zweiter Stand ergibt einen Diff, ein dritter
// unveränderter wird nicht gespeichert.
func TestHostPanelSwitchOverShell(t *testing.T) { hostPanelSwitch(t, false) }

// wie ein echter ArubaOS-Switch: Zeilen per Cursor-Sprung
func TestHostPanelSwitchPositioned(t *testing.T) { hostPanelSwitch(t, true) }

func hostPanelSwitch(t *testing.T, positioned bool) {
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
	sw.PressAnyKey = true
	sw.Positioned = positioned

	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	a.w, a.h, a.scr = 140, 40, scrTerminal
	go func() { // Rückfragen beantworten, Ausgabe-Meldungen verwerfen
		for m := range a.events {
			if hk, ok := m.(hostKeyAskMsg); ok {
				hk.reply <- true
			}
		}
	}()
	host, port, _ := net.SplitHostPort(sw.Addr)
	p, _ := strconv.Atoi(port)
	c, err := st.UpsertConnection(domain.Connection{Alias: "sw-buero", Host: host, Port: p, Username: "manager",
		Auth: domain.AuthPassword, Password: "x"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: 1, conn: c, chipArmed: -1}
	a.sessions = []*session{s}
	msg := a.connectCmd(s)()
	if e, ok := msg.(connectErrMsg); ok {
		t.Fatal(e.err)
	}
	a.Update(msg)
	if !s.client.NetDevice {
		t.Fatal("Mocana-Banner nicht als Netzwerkgerät erkannt")
	}

	// Vor "Press any key" darf nichts getippt werden.
	waitFor(t, func() bool { return strings.Contains(s.term.PlainLines(5), "Press any key") })
	if cmd := a.hostNetCmd(s, true, false); cmd != nil || s.host.err == "" {
		t.Fatal("Abfrage ohne Prompt gestartet")
	}
	_, _ = s.shell.Write([]byte(" "))
	waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })

	// Ohne "no page" blättert der Switch; shellRun muss weiterblättern.
	lines, err := shellRun(context.Background(), s, "sw-buero#", "show interfaces brief", 20*time.Second)
	if err != nil || len(lines) != 32 || strings.Contains(strings.Join(lines, "\n"), "MORE") {
		t.Fatalf("Blättern: %v %d\n%s", err, len(lines), strings.Join(lines, "\n"))
	}
	waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })

	run := func(info, backup bool) {
		t.Helper()
		cmd := a.hostNetCmd(s, info, backup)
		if cmd == nil {
			t.Fatalf("keine Abfrage: %s", s.host.err)
		}
		a.Update(cmd())
		if s.host.err != "" {
			t.Fatalf("Fehler: %s", s.host.err)
		}
		waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })
	}

	run(true, true)
	h := s.host
	if h.sw == nil || h.sw.Firmware != "YA.16.10.0009" || h.sw.Name != "sw-buero" || h.sw.Model != "J9773A" {
		t.Fatalf("Übersicht: %+v", h.sw)
	}
	if h.sw.PortsUp != 10 || h.sw.PortsDown != 18 || len(h.sw.VLANs) != 3 {
		t.Fatalf("Ports/VLANs: %d/%d %v", h.sw.PortsUp, h.sw.PortsDown, h.sw.VLANs)
	}
	if !h.pagingOff || len(h.backups) != 1 || !strings.Contains(h.note, "Erste Sicherung") {
		t.Fatalf("Sicherung: %+v", h)
	}
	first, _ := st.LoadBackup(c.ID, h.backups[0].ID)
	if strings.Contains(first, "MORE") || !strings.Contains(first, "password manager") || strings.Contains(first, "sw-buero#") {
		t.Fatalf("Konfiguration unsauber:\n%s", first)
	}

	time.Sleep(1100 * time.Millisecond) // neuer Dateiname (Sekunden)
	sw.AddVLAN(30)
	run(false, true)
	if len(s.host.backups) != 2 || !strings.Contains(s.host.note, "+5 −0 Zeilen") {
		t.Fatalf("Diff: %q (%d)", s.host.note, len(s.host.backups))
	}
	s.host.sel = 0
	a.showBackupDiff(s)
	if a.modal == nil || a.modal.kind != mText || !strings.Contains(strings.Join(a.modal.lines, "\n"), "vlan 30") {
		t.Fatal("Diff-Ansicht fehlt")
	}
	if out := a.modal.view(a.s, a.w, a.h); !strings.Contains(out, "+5") {
		t.Fatal("Diff-Kopf ohne Zähler")
	}
	a.modalKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	time.Sleep(1100 * time.Millisecond)
	run(false, true)
	if len(s.host.backups) != 2 || !strings.Contains(s.host.note, "Unverändert") {
		t.Fatalf("unverändert: %q (%d)", s.host.note, len(s.host.backups))
	}
	if n := sw.Execs.Load(); n != 0 {
		t.Fatalf("%d Exec-Anfragen an den Switch", n)
	}
	// Panel zeichnen geht
	s.panel = panelHost
	if v, _ := a.viewTerminal(); !strings.Contains(v, "Config-Sicherung") {
		t.Fatal("Panel fehlt")
	}
	s.close()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatal("Zeitüberschreitung")
		case <-time.After(30 * time.Millisecond):
		}
	}
}
