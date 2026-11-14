package ui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/patchrun"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/vuln"
)

// fakeHost spielt einen Server: Zustand vor/nach dem Update, Neustart.
type fakeHost struct {
	mu       sync.Mutex
	boot     string
	updated  bool
	rebooted bool
	breaks   bool // nach dem Update läuft nginx nicht mehr
	root     bool
	sudoPw   string
	cmds     []string
}

type fakeConn struct{ h *fakeHost }

func (f fakeConn) Close() error { return nil }

func (f fakeConn) ExecInput(_ context.Context, cmd string, in []byte) (sshx.RunResult, error) {
	h := f.h
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cmds = append(h.cmds, cmd)
	switch {
	case strings.Contains(cmd, "sudo -n true"):
		if h.sudoPw == "" {
			return sshx.RunResult{}, nil
		}
		return sshx.RunResult{Exit: 1, Output: []byte("sudo: a password is required")}, nil
	case strings.Contains(cmd, "sudo -S -p '' true"):
		if string(in) == h.sudoPw+"\n" {
			return sshx.RunResult{}, nil
		}
		return sshx.RunResult{Exit: 1}, nil
	case strings.Contains(cmd, "pm=apt"): // Zustand
		var b strings.Builder
		if h.root {
			b.WriteString("root=1\n")
		}
		b.WriteString("pm=apt\nboot=" + h.boot + "\nsvc=ssh.service\n")
		if !(h.updated && h.breaks) {
			b.WriteString("svc=nginx.service\nport=0.0.0.0:80\n")
		}
		b.WriteString("port=0.0.0.0:22\n")
		if h.updated && !h.rebooted {
			b.WriteString("reboot=1\n")
		}
		return sshx.RunResult{Output: []byte(b.String())}, nil
	case strings.Contains(cmd, "apt-get upgrade"):
		h.updated = true
		return sshx.RunResult{Output: []byte("Unpacking openssl (3) over (2) ...\nUnpacking curl (8) over (7) ...\n")}, nil
	case strings.Contains(cmd, "apt-get install --only-upgrade"):
		h.updated = true
		return sshx.RunResult{Output: []byte("Unpacking openssl (3) over (2) ...\n")}, nil
	case strings.Contains(cmd, "systemctl reboot"):
		h.rebooted, h.boot = true, h.boot+"-neu"
		return sshx.RunResult{}, nil
	}
	return sshx.RunResult{}, nil // Logs: leer
}

func TestRunPatchHost(t *testing.T) {
	defer func(a, b, c, d time.Duration) { patchRebootWait, patchPoll, patchBackMax, patchSettle = a, b, c, d }(patchRebootWait, patchPoll, patchBackMax, patchSettle)
	patchRebootWait, patchPoll, patchBackMax, patchSettle = time.Millisecond, time.Millisecond, time.Second, time.Millisecond
	h := &fakeHost{boot: "b1", sudoPw: "geheim"}
	dial := func(context.Context, domain.Connection) (patchConn, error) { return fakeConn{h}, nil }
	var phases []string
	j := patchJob{conn: domain.Connection{Alias: "web01", Username: "u", Host: "web01"}, scope: patchrun.ScopeAll, reboot: true, pws: []string{"falsch", "geheim"}}
	got := runPatchHost(context.Background(), j, dial, nil, func(x patchHost) { phases = append(phases, x.phase) })
	if got.err != "" || !got.done || !got.rebooted || got.upgraded != 2 || got.problems() != 0 {
		t.Fatalf("%+v", got)
	}
	if len(phases) < 5 {
		t.Fatalf("Phasen: %v", phases)
	}
	upd := ""
	for _, c := range h.cmds {
		if strings.Contains(c, "apt-get upgrade") {
			upd = c
		}
	}
	if !strings.HasPrefix(upd, "LC_ALL=C sudo -S -p '' sh -c") {
		t.Fatalf("Update ohne sudo-Passwort: %s", upd)
	}

	// Dienst fällt aus, kein Neustart gewünscht
	h2 := &fakeHost{boot: "b1", root: true, breaks: true}
	j2 := patchJob{conn: domain.Connection{Alias: "web02"}, scope: patchrun.ScopeSecurity, pkgs: []string{"openssl"}}
	got = runPatchHost(context.Background(), j2, func(context.Context, domain.Connection) (patchConn, error) { return fakeConn{h2}, nil }, nil, func(patchHost) {})
	if got.rebooted || !got.reboot || got.problems() != 2 || got.diff.StoppedServices[0] != "nginx.service" {
		t.Fatalf("%+v", got)
	}
	// ohne Pakete aus dem Sicherheitscheck: übersprungen
	got = runPatchHost(context.Background(), patchJob{scope: patchrun.ScopeSecurity}, nil, nil, func(patchHost) {})
	if !got.skipped {
		t.Fatal("nicht übersprungen")
	}
	// sudo-Passwort fehlt
	h3 := &fakeHost{boot: "b", sudoPw: "x"}
	got = runPatchHost(context.Background(), patchJob{scope: patchrun.ScopeAll}, func(context.Context, domain.Connection) (patchConn, error) { return fakeConn{h3}, nil }, func(string) string { return "" }, func(patchHost) {})
	if !strings.Contains(got.err, "sudo") {
		t.Fatalf("%+v", got)
	}
}

func TestPatchRoundFlow(t *testing.T) {
	a := vulnTestApp(t)
	for _, n := range []string{"app-test", "app01", "app02"} {
		if _, err := a.st.UpsertConnection(domain.Connection{Alias: n, Host: n, Username: "root", Group: "Apps"}); err != nil {
			t.Fatal(err)
		}
	}
	old := patchHostRun
	defer func() { patchHostRun = old }()
	patchHostRun = func(*App) func(context.Context, patchJob, func(patchHost)) patchHost {
		return func(_ context.Context, j patchJob, upd func(patchHost)) patchHost {
			upd(patchHost{conn: j.conn, phase: "spielt Updates ein …"})
			h := patchHost{conn: j.conn, phase: "fertig", done: true, upgraded: 3}
			if j.conn.Alias == "app01" {
				h.diff.ClosedPorts = []string{"*:443"}
			}
			return h
		}
	}
	a.scr = scrManager
	a.openPatch()
	if a.pick == nil {
		t.Fatal("Ordnerwahl fehlt")
	}
	a.pick.onPick(pickItem{value: "Apps"})
	if a.pick == nil || len(a.pick.items) != 1 {
		t.Fatal("Umfang: ohne Sicherheitscheck nur „alle Updates“")
	}
	a.pick.onPick(a.pick.items[0])
	a.pick.onPick(pickItem{value: "yes"})
	if a.pick.items[0].label != "app-test" {
		t.Fatalf("Test-Host nicht zuerst vorgeschlagen: %v", a.pick.items[0].label)
	}
	a.pick.onPick(a.pick.items[0])
	if a.modal == nil || a.modal.kind != mConfirm || !a.modal.twice || !strings.Contains(a.modal.body, "geschützt") {
		t.Fatalf("Bestätigung: %+v", a.modal)
	}
	a.Update(keyMsg("j"))
	if a.patch.running {
		t.Fatal("startet nach einmal j")
	}
	a.Update(keyMsg("j"))
	if !a.patch.running || a.modal == nil || !a.modal.patch {
		t.Fatal("Runde läuft nicht")
	}
	pump := func(cond func() bool) {
		deadline := time.After(5 * time.Second)
		for !cond() {
			select {
			case m := <-a.events:
				a.Update(m)
			case <-deadline:
				t.Fatal("keine Antwort")
			}
		}
	}
	pump(func() bool { return a.patch.paused })
	if a.patch.hosts[0].conn.Alias != "app-test" || !strings.Contains(strings.Join(a.modal.lines, "\n"), "w = ") {
		t.Fatalf("Pause nach dem ersten Host fehlt:\n%s", strings.Join(a.modal.lines, "\n"))
	}
	a.Update(keyMsg("w"))
	pump(func() bool { return a.patch.paused || a.patch.done })
	if !a.patch.paused || a.patch.hosts[1].conn.Alias != "app01" {
		t.Fatal("hält bei Auffälligkeiten nicht an")
	}
	txt := strings.Join(a.modal.lines, "\n")
	if !strings.Contains(txt, "Ports nicht mehr offen: *:443") || !strings.Contains(a.modal.copy, "| app01 |") {
		t.Fatalf("Bericht:\n%s", txt)
	}
	a.Update(keyMsg("x"))
	if !a.patch.done || !a.patch.hosts[2].skipped {
		t.Fatal("x beendet nicht")
	}
}

func TestPatchPkgsFromVulnCheck(t *testing.T) {
	a := vulnTestApp(t)
	c, _ := a.st.UpsertConnection(domain.Connection{Alias: "db", Host: "db", Username: "root"})
	r := sampleVulnReport()
	r.Findings = append(r.Findings, vuln.Finding{Package: "nofix", Binaries: []string{"nofix"}}, vuln.Finding{Container: "web", Package: "x", Fixed: "2", Binaries: []string{"x"}})
	a.saveVuln(c.ID, r)
	got := a.patchPkgs(c.ID)
	for _, p := range got {
		if p == "nofix" || p == "x" {
			t.Fatalf("falsche Pakete: %v", got)
		}
	}
	if len(got) == 0 {
		t.Fatal("keine Pakete")
	}
}
