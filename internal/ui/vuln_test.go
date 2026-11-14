package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/vuln"
)

func vulnTestApp(t *testing.T) *App {
	t.Helper()
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
	a.w, a.h, a.intro = 150, 40, nil
	return a
}

func sampleVulnReport() *vuln.Report {
	return &vuln.Report{At: time.Now().Add(-2 * time.Hour), Mode: "online", System: "Debian GNU/Linux 12 (bookworm)", Packages: 412, Containers: 1,
		Ecosystems: []string{"Debian:12"},
		Findings: []vuln.Finding{
			{Eco: "Debian:12", Package: "openssh", Binaries: []string{"openssh-server", "openssh-client"}, Installed: "1:9.2p1-2+deb12u2", Fixed: "1:9.2p1-2+deb12u3",
				Score: 8.1, KEV: true, Running: []string{"sshd :22"}, CVEs: []vuln.CVE{{ID: "CVE-2024-6387", Score: 8.1, KEV: true, Text: "regreSSHion", Fixed: "1:9.2p1-2+deb12u3"}}},
			{Eco: "Debian:12", Package: "expat", Binaries: []string{"libexpat1"}, Installed: "2.5.0-1", Fixed: "2.5.0-1+deb12u1", Score: 9.8,
				CVEs: []vuln.CVE{{ID: "CVE-2024-45492", Score: 9.8, Fixed: "2.5.0-1+deb12u1"}}},
			{Eco: "Debian:12", Package: "glibc", Binaries: []string{"libc6"}, Installed: "2.36-9+deb12u7", Score: 7.5, NoFix: 1,
				CVEs: []vuln.CVE{{ID: "CVE-2025-0001", Score: 7.5}}},
			{Container: "web (nginx:1.25-alpine)", Eco: "Alpine:v3.20", Package: "openssl", Binaries: []string{"libssl3"}, Installed: "3.3.2-r0", Fixed: "3.3.2-r1", Score: 7.5,
				CVEs: []vuln.CVE{{ID: "CVE-2024-0003", Score: 7.5, Fixed: "3.3.2-r1"}}},
		}}
}

func TestVulnScreen(t *testing.T) {
	a := vulnTestApp(t)
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Vuln.Mode = domain.VulnOnline })
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "web01", Host: "10.0.0.1", Username: "root", Group: "Web"})
	if err != nil {
		t.Fatal(err)
	}
	a.scr = scrVuln
	a.vscan = vulnState{sid: 7, conn: c, back: scrTerminal}
	a.vscan.seq = 3
	a.vscan.busy = true
	rep := sampleVulnReport()
	a.Update(vulnStepMsg{seq: 3, step: "prüfe 12 Pakete"})
	if a.vscan.step != "prüfe 12 Pakete" {
		t.Fatalf("Schritt: %q", a.vscan.step)
	}
	a.Update(vulnMsg{seq: 3, rep: rep})
	if a.vscan.busy || a.vscan.rep == nil {
		t.Fatal("Ergebnis nicht übernommen")
	}
	out := a.View().Content
	for _, want := range []string{"Sicherheitscheck · web01", "1 aktiv ausgenutzt", "openssh", "KEV", "sshd :22", "1:9.2p1-2+deb12u3", "web (nginx", "– kein Fix", "u fügt ein"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q fehlt:\n%s", want, out)
		}
	}
	// gespeichert: F6 zeigt die Kurzfassung
	sum := a.vulnSummary(c)
	if !sum.ok || sum.c.KEV != 1 || sum.c.Critical != 1 || sum.fixes != 2 {
		t.Fatalf("Kurzfassung: %+v", sum)
	}
	// ⏎ zeigt alle Lücken
	a.Update(keyMsg("enter"))
	if a.modal == nil || !strings.Contains(strings.Join(a.modal.lines, "\n"), "AKTIV AUSGENUTZT") {
		t.Fatal("Detail fehlt")
	}
	a.modal = nil
	// x blendet aus (nur dieser Host), n zeigt Ausgeblendetes, x holt zurück
	first := a.vulnVisible()[0]
	a.Update(runeKey('x'))
	ign := a.st.Settings().Vuln.Ignore
	if len(ign) != 1 || ign[0].Conn != c.ID || ign[0].Pkg != first.Key() || len(a.vulnVisible()) != 3 {
		t.Fatalf("x: %+v %d", ign, len(a.vulnVisible()))
	}
	if a.vulnSummary(c).c.KEV != 0 {
		t.Fatal("Ausgeblendetes zählt noch mit")
	}
	a.Update(runeKey('n'))
	if len(a.vulnVisible()) != 1 || !strings.Contains(a.View().Content, "Ausgeblendet") {
		t.Fatal("n zeigt Ausgeblendetes nicht")
	}
	a.Update(runeKey('x'))
	if len(a.st.Settings().Vuln.Ignore) != 0 {
		t.Fatal("x holt nicht zurück")
	}
	a.Update(keyMsg("esc"))
	if a.vscan.hidden || a.scr != scrVuln {
		t.Fatal("Esc aus der Ausgeblendet-Ansicht")
	}
	// neue Lücke im selben Paket: Ausblendung gilt nicht mehr
	a.vulnIgnore(a.vulnVisible()[0], true)
	if len(a.vulnVisible()) != 3 {
		t.Fatal("X blendet nicht aus")
	}
	rep2 := sampleVulnReport()
	rep2.Findings[0].CVEs = append(rep2.Findings[0].CVEs, vuln.CVE{ID: "CVE-2026-1", Score: 9})
	a.vscan.prev, a.vscan.rep = a.vscan.rep, rep2
	if len(a.vulnVisible()) != 4 || !strings.Contains(a.View().Content, "1 Pakete neu betroffen") {
		t.Fatalf("neue Lücke: %d\n%s", len(a.vulnVisible()), a.View().Content)
	}
	// f: nur mit Update
	a.Update(runeKey('f'))
	for _, f := range a.vulnVisible() {
		if f.Fixed == "" {
			t.Fatal("f filtert nicht")
		}
	}
	// c kopiert Markdown
	if _, cmd := a.Update(runeKey('c')); cmd == nil || !strings.Contains(a.vulnMarkdown(), "| HOCH KEV | openssh |") {
		t.Fatalf("Markdown:\n%s", a.vulnMarkdown())
	}
	// Container: u fügt nichts ein, sondern erklärt
	for i, f := range a.vulnVisible() {
		if f.Container != "" {
			a.vscan.sel = i
		}
	}
	a.toasts = nil
	a.Update(runeKey('u'))
	if len(a.toasts) == 0 || !strings.Contains(a.toasts[0].text, "Image") {
		t.Fatalf("Container-Hinweis fehlt: %+v", a.toasts)
	}
	a.Update(keyMsg("esc"))
	if a.scr != scrTerminal {
		t.Fatal("Esc geht nicht zurück")
	}
}

func TestVulnFolderAndSchedule(t *testing.T) {
	a := vulnTestApp(t)
	var conns []domain.Connection
	for _, n := range []string{"web01", "web02", "sw01"} {
		c := domain.Connection{Alias: n, Host: n, Username: "root", Group: "Web"}
		if n == "sw01" {
			c.Device = domain.DeviceNetwork
		}
		c, err := a.st.UpsertConnection(c)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	// Quelle noch nicht gewählt: erst fragen
	a.scr = scrManager
	a.openVulnFolder()
	if a.pick == nil || !strings.Contains(a.pick.title, "woher") {
		t.Fatal("fragt nicht nach der Quelle")
	}
	a.pick.sel = 0
	a.pick = nil
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Vuln.Mode = domain.VulnOffline })

	old := vulnScanHost
	defer func() { vulnScanHost = old }()
	calls := 0
	vulnScanHost = func(*App) func(context.Context, domain.Connection, vuln.Source, *vuln.HTTP, domain.VulnConfig) (*vuln.Report, error) {
		return func(_ context.Context, c domain.Connection, _ vuln.Source, _ *vuln.HTTP, cfg domain.VulnConfig) (*vuln.Report, error) {
			calls++
			if cfg.Mode != domain.VulnOffline {
				return nil, errors.New("Modus fehlt")
			}
			if c.Alias == "web02" {
				return nil, errors.New("braucht eine Eingabe (Passwort)")
			}
			return sampleVulnReport(), nil
		}
	}
	if got := a.vulnFolderConns("Web"); len(got) != 2 {
		t.Fatalf("Switch zählt mit: %d", len(got))
	}
	a.runVulnFolder("Web")
	deadline := time.After(10 * time.Second)
	for !a.vfold.allDone() {
		select {
		case m := <-a.events:
			a.Update(m)
		case <-deadline:
			t.Fatal("keine Antwort")
		}
	}
	if !a.vfold.allDone() || a.modal == nil || !a.modal.vuln {
		t.Fatal("Ergebnisfenster fehlt")
	}
	txt := strings.Join(a.modal.lines, "\n")
	if !strings.Contains(txt, "web01") || !strings.Contains(txt, "× braucht eine Eingabe") || !strings.Contains(a.modal.copy, "| web01 |") {
		t.Fatalf("Ordner:\n%s", txt)
	}
	if a.vulnSummary(conns[0]).c.KEV != 1 {
		t.Fatal("nicht gespeichert")
	}

	// Zeitplan: zweiter Lauf mit einer neuen kritischen Lücke meldet sie
	vulnScanHost = func(*App) func(context.Context, domain.Connection, vuln.Source, *vuln.HTTP, domain.VulnConfig) (*vuln.Report, error) {
		return func(_ context.Context, c domain.Connection, _ vuln.Source, _ *vuln.HTTP, _ domain.VulnConfig) (*vuln.Report, error) {
			r := sampleVulnReport()
			r.Findings = append(r.Findings, vuln.Finding{Eco: "Debian:12", Package: "curl", Installed: "1", Fixed: "2", Score: 9.1, CVEs: []vuln.CVE{{ID: "CVE-2026-9", Score: 9.1}}})
			return r, nil
		}
	}
	sc := domain.Schedule{ID: "s1", Name: "Nachts", At: "03:00", Folder: "Web", Kind: domain.ScheduleVuln}
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Schedules = append(s.Schedules, sc) }); err != nil {
		t.Fatal(err)
	}
	if got := a.schedConns(sc); len(got) != 2 {
		t.Fatalf("Zeitplan-Ziel: %d", len(got))
	}
	if steps, err := a.schedSteps(sc); err != nil || steps[0] != "osv-scan" {
		t.Fatalf("Schritte: %v %v", steps, err)
	}
	a.toasts = nil
	m := a.startScheduleVuln(sc, a.schedConns(sc))()
	a.Update(m)
	if len(a.toasts) == 0 || !strings.Contains(a.toasts[len(a.toasts)-1].text, "1 neu betroffene Pakete") {
		t.Fatalf("Meldung: %+v", a.toasts)
	}
	got, _ := a.scheduleByID("s1")
	if got.LastOK != 2 || !strings.Contains(got.LastText, "web01") {
		t.Fatalf("Zeitplan-Ergebnis: %+v", got)
	}
}

func TestVulnNotForSwitches(t *testing.T) {
	a := vulnTestApp(t)
	s := &session{id: 1, conn: domain.Connection{Alias: "sw", Device: domain.DeviceNetwork}, state: stConnected}
	if err := a.vulnOK(s); err == nil || !strings.Contains(err.Error(), "Firmware") {
		t.Fatalf("Switch: %v", err)
	}
	s = &session{id: 2, conn: domain.Connection{Alias: "win"}, state: stConnected}
	s.prof.Family = domain.FamilyWindows
	if err := a.vulnOK(s); err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("Windows: %v", err)
	}
}
