package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/logscan"
)

func newWatchApp(t *testing.T) *App {
	t.Helper()
	a := newPaletteTestApp(t)
	a.pal, a.intro = nil, nil
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.LogScan.Watch = true })
	return a
}

func syslogItem(addr, pkt string) watchItem {
	e, _ := logscan.SyslogPacket(pkt, time.Now())
	return watchItem{addr: addr, kind: logscan.KindNetwork, e: e}
}

// Logwache meldet neue Muster einmal, zählt Wiederholungen, lässt
// Bekanntes und Rauschen weg.
func TestLogWatchAlerts(t *testing.T) {
	a := newWatchApp(t)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "sw-lager", Host: "10.0.0.9", Username: "manager", Group: "Lager"})
	if err != nil {
		t.Fatal(err)
	}
	loop := "<130>Nov 10 08:00:00 sw-lager 00921 loop-protect: port 17 disabled - loop detected on VLAN 20"
	a.Update(watchBatchMsg{[]watchItem{syslogItem("10.0.0.9", loop), syslogItem("10.0.0.9", loop)}})
	w := &a.lwatch
	if len(w.alerts) != 1 || w.alerts[0].Host != "sw-lager" || w.alerts[0].Count != 2 || !w.alerts[0].New || w.unread != 1 {
		t.Fatalf("Meldungen: %+v", w.alerts)
	}
	if len(a.toasts) == 0 || !strings.Contains(a.toasts[len(a.toasts)-1].text, "Logwache sw-lager") {
		t.Fatalf("kein Toast: %+v", a.toasts)
	}

	// Info-Zeilen und Warnungen (Stufe „Fehler“) melden nichts.
	a.Update(watchBatchMsg{[]watchItem{syslogItem("10.0.0.9", "<134>Nov 10 08:00:00 sw-lager 00422 mgr: SME SSH from 10.0.0.50 - MANAGER Mode"),
		syslogItem("10.0.0.9", "<132>Nov 10 08:00:00 sw-lager 00077 ports: port 3 is now off-line")}})
	if len(w.alerts) != 1 {
		t.Fatalf("Info/Warnung gemeldet: %+v", w.alerts)
	}

	// Muster aus der Grundlinie: nur kritische melden.
	cpu := "<131>Nov 10 08:00:00 sw-lager 00100 chassis: Fan 2 failure detected"
	f, ok := logscan.Classify(logscan.KindNetwork, syslogItem("", cpu).e)
	if !ok {
		t.Fatal("Lüfter nicht erkannt")
	}
	raw, _ := json.Marshal(logscan.Base{Items: map[string]logscan.BaseItem{f.Key: {N: 3, Sev: f.Sev}}})
	_ = a.st.SetLogBase(c.ID, raw)
	delete(w.base, c.ID)
	a.Update(watchBatchMsg{[]watchItem{syslogItem("10.0.0.9", cpu)}})
	if f.Sev < logscan.SevCrit && len(w.alerts) != 1 {
		t.Fatalf("bekanntes Muster gemeldet: %+v", w.alerts)
	}

	// Rauschen bleibt still, unbekannte Absender werden mit IP gemeldet.
	a.lscan.conn = c
	a.addNoise(logscan.Finding{Key: "network:x", Title: "x"}, true)
	n := len(w.alerts)
	a.Update(watchBatchMsg{[]watchItem{syslogItem("192.0.2.7", "<11>sshd[99]: error: kex_exchange_identification: Connection closed")}})
	if len(w.alerts) != n+1 || w.alerts[n].Host != "192.0.2.7" {
		t.Fatalf("fremder Absender: %+v", w.alerts)
	}
	_ = a.openWatchList()
	if a.pick == nil || len(a.pick.items) != len(w.alerts) || w.unread != 0 {
		t.Fatal("Liste der Logwache fehlt")
	}
	// neueste oben: der fremde Absender; ⏎ ohne offenen Tab zeigt die Zeile
	if !strings.Contains(a.pick.items[0].label, "192.0.2.7") {
		t.Fatalf("Reihenfolge: %+v", a.pick.items[0])
	}
	// x blendet das Muster des Switches aus und nimmt die Meldung weg
	a.pick.sel = len(a.pick.items) - 1
	a.Update(runeKey('x'))
	if len(w.alerts) != n || len(a.st.Settings().LogScan.Noise) != 2 {
		t.Fatalf("x in der Liste: %d Meldungen, Rauschen %+v", len(w.alerts), a.st.Settings().LogScan.Noise)
	}
	a.Update(watchBatchMsg{[]watchItem{syslogItem("10.0.0.9", loop)}})
	if len(w.alerts) != n {
		t.Fatal("ausgeblendetes Muster wieder gemeldet")
	}
	a.pick.sel = 0
	a.Update(keyMsg("enter"))
	if a.modal == nil || !strings.Contains(strings.Join(a.modal.lines, "\n"), "kex_exchange") {
		t.Fatal("⏎ ohne Tab zeigt die Zeile nicht")
	}

	// Aus: nichts mehr aus Sitzungen.
	_ = a.cycleWatch(1) // Fehler → Fehler + Warnungen
	if a.watchLevel() != 2 {
		t.Fatalf("Stufe %d", a.watchLevel())
	}
	_ = a.cycleWatch(1)
	if a.watchLevel() != 0 {
		t.Fatal("Logwache nicht aus")
	}
}

// Syslog-Empfang über echtes UDP.
func TestSyslogReceiver(t *testing.T) {
	a := newWatchApp(t)
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	_ = pc.Close()
	if _, err := a.st.UpsertConnection(domain.Connection{Alias: "fw", Host: "127.0.0.1", Username: "admin"}); err != nil {
		t.Fatal(err)
	}
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.SyslogPort = port })
	a.watchTick()
	defer a.stopWatch()
	if a.lwatch.sysErr != "" {
		t.Skip(a.lwatch.sysErr)
	}
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline := time.After(5 * time.Second)
	send := time.NewTicker(300 * time.Millisecond)
	defer send.Stop()
	for len(a.lwatch.alerts) == 0 {
		select {
		case <-send.C:
			_, _ = conn.Write([]byte("<130>Nov 10 08:00:00 fw 00921 loop-protect: port 17 disabled - loop detected on VLAN 20"))
		case m := <-a.events:
			a.Update(m)
		case <-deadline:
			t.Fatal("keine Meldung über Syslog")
		}
	}
	if al := a.lwatch.alerts[0]; al.Host != "fw" || al.Src != "syslog" {
		t.Fatalf("Meldung: %+v", al)
	}
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.SyslogPort = 0 })
	a.watchTick()
	if a.lwatch.sysStop != nil {
		t.Fatal("Empfänger läuft weiter")
	}
}

// Ordner-Scan: alle Hosts eines Ordners, Fehler pro Host, gemeinsame Muster.
func TestFolderScan(t *testing.T) {
	a := newWatchApp(t)
	for _, c := range []domain.Connection{
		{Alias: "web1", Host: "w1", Group: "Prod"}, {Alias: "web2", Host: "w2", Group: "Prod / Web"},
		{Alias: "kaputt", Host: "k", Group: "Prod"}, {Alias: "test", Host: "t", Group: "Test"},
	} {
		c.Username = "root"
		if _, err := a.st.UpsertConnection(c); err != nil {
			t.Fatal(err)
		}
	}
	old := folderScanDial
	defer func() { folderScanDial = old }()
	folderScanDial = func(*App) func(context.Context, domain.Connection, int) (*logscan.Report, error) {
		return func(_ context.Context, c domain.Connection, hours int) (*logscan.Report, error) {
			if c.Alias == "kaputt" {
				return nil, errors.New("connection refused")
			}
			out := "##S meta\nuid=0\n##S journal crit\n" +
				fmt.Sprintf("%d.000000 %s kernel: Out of memory: Killed process 1234 (java) total-vm:1kB\n", time.Now().Add(-time.Hour).Unix(), c.Alias) +
				"##S end\n"
			return logscan.ParseUnix(out, hours, time.Now()), nil
		}
	}
	_ = a.openFolderScan()
	if a.pick == nil || len(a.pick.items) < 3 {
		t.Fatal("Ordnerwahl fehlt")
	}
	a.pick = nil
	_ = a.runFolderScan("Prod")
	if len(a.lfold.hosts) != 3 {
		t.Fatalf("Hosts: %d", len(a.lfold.hosts))
	}
	for !a.lfold.allDone() {
		select {
		case m := <-a.events:
			a.Update(m)
		case <-time.After(5 * time.Second):
			t.Fatal("Ordner-Scan wird nicht fertig")
		}
	}
	if a.modal == nil || a.modal.copy == "" {
		t.Fatal("Ergebnis fehlt")
	}
	out := strings.Join(a.modal.lines, "\n")
	for _, want := range []string{"web1", "web2", "× connection refused", "Gemeinsam", "2 Hosts", "java"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q fehlt:\n%s", want, out)
		}
	}
	if strings.Contains(out, "test") || !strings.Contains(a.modal.copy, "| web1 |") {
		t.Fatalf("Ordner falsch oder Markdown fehlt:\n%s", a.modal.copy)
	}
	web1 := a.lfold.hosts[0]
	for _, h := range a.lfold.hosts {
		if h.conn.Alias == "web1" {
			web1 = h
		}
	}
	if len(a.st.LogBase(web1.conn.ID)) == 0 {
		t.Fatal("Grundlinie nicht gespeichert")
	}

	// Als Zeitplan: gleicher Scan ohne Fenster, Ergebnis im Zeitplan.
	sc := domain.Schedule{ID: "sch-logs", Name: "Logs Prod", At: "03:00", Folder: "Prod", Kind: domain.ScheduleLogs}
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Schedules = append(s.Schedules, sc) })
	if n := len(a.schedConns(sc)); n != 3 {
		t.Fatalf("Ziele: %d", n)
	}
	a.modal = nil
	cmd := a.startSchedule(sc)
	if cmd == nil {
		t.Fatal("Zeitplan startet nicht")
	}
	a.Update(cmd())
	got, _ := a.scheduleByID("sch-logs")
	if got.LastOK != 2 || got.LastFail != 1 || !strings.Contains(got.LastText, "Gemeinsam") || a.schedRun["sch-logs"] {
		t.Fatalf("Zeitplan-Ergebnis: %+v", got)
	}
}

// Fehlanmeldungen: einzeln Info, ab 20 in 10 Minuten eine Fehler-Meldung.
func TestLogWatchAuthBurst(t *testing.T) {
	a := newWatchApp(t)
	if _, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "10.0.0.5", Username: "root"}); err != nil {
		t.Fatal(err)
	}
	var items []watchItem
	for i := 0; i < 25; i++ {
		e := logscan.Entry{Time: time.Now(), Sev: logscan.SevInfo, Source: "sshd", Msg: fmt.Sprintf("Failed password for root from 203.0.113.%d port 4%d ssh2", i, i)}
		items = append(items, watchItem{addr: "10.0.0.5", kind: logscan.KindUnix, e: e})
		if i == 10 {
			a.Update(watchBatchMsg{items})
			items = nil
			if len(a.lwatch.alerts) != 0 {
				t.Fatalf("zu früh gemeldet: %+v", a.lwatch.alerts)
			}
		}
	}
	a.Update(watchBatchMsg{items})
	if len(a.lwatch.alerts) != 1 || a.lwatch.alerts[0].Sev != logscan.SevErr || a.lwatch.alerts[0].Count != 6 {
		t.Fatalf("Meldungen: %+v", a.lwatch.alerts)
	}
}
