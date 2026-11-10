package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
)

type schedExec struct{ alias string }

func (e schedExec) NetDevice() bool { return false }
func (e schedExec) Learned() bool   { return false }
func (e schedExec) Close() error    { return nil }
func (e schedExec) Exec(_ context.Context, cmd string) (sshx.RunResult, error) {
	return sshx.RunResult{Output: []byte(e.alias + ": " + cmd + "\n")}, nil
}
func (e schedExec) RunInShell(_ context.Context, cmds []string) (sshx.RunResult, error) {
	return sshx.RunResult{Output: []byte(strings.Join(cmds, "\n"))}, nil
}

func TestScheduleFormAndRun(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal = nil
	for _, alias := range []string{"sw-1", "sw-2"} {
		if _, err := a.st.UpsertConnection(domain.Connection{Alias: alias, Host: alias, Username: "admin", Auth: domain.AuthAgent, Group: "Netz"}); err != nil {
			t.Fatal(err)
		}
	}
	old := schedDialer
	t.Cleanup(func() { schedDialer = old })
	schedDialer = func(*App) broadcast.Dialer {
		return func(_ context.Context, c domain.Connection) (broadcast.Executor, error) {
			if c.Alias == "sw-2" {
				return nil, errors.New("host key unbekannt")
			}
			return schedExec{c.Alias}, nil
		}
	}
	a.openScheduleForm(domain.Schedule{At: "03:00"})
	f := a.form
	f.get("name").in.SetValue("Sicherung")
	f.get("at").in.SetValue("3:5")
	if err := a.saveScheduleForm(); err == nil {
		t.Fatal("Uhrzeit 3:5 muss abgelehnt werden")
	}
	f.get("at").in.SetValue("0300")
	setChoice(t, f, "folder", "Netz")
	f.get("steps").in.SetValue("reboot")
	if err := a.saveScheduleForm(); err == nil || !strings.Contains(err.Error(), "Gefährlich") {
		t.Fatalf("Gefahr muss abgelehnt werden: %v", err)
	}
	f.get("steps").in.SetValue("uptime ;; hostname {{alias}}")
	if err := a.saveScheduleForm(); err != nil {
		t.Fatal(err)
	}
	scs := a.schedules()
	if len(scs) != 1 || scs[0].At != "03:00" || scs[0].Folder != "Netz" || len(scs[0].Steps) != 2 {
		t.Fatalf("%+v", scs)
	}
	// frisch angelegt: kein „verpasster“ Lauf von heute
	if scs[0].Due(time.Now()) {
		t.Fatal("darf nicht sofort laufen")
	}
	cmd := a.startSchedule(scs[0])
	if !a.schedRun[scs[0].ID] {
		t.Fatal("läuft nicht")
	}
	a.scheduleMsg(cmd())
	sc, _ := a.scheduleByID(scs[0].ID)
	if sc.LastOK != 1 || sc.LastFail != 1 || !strings.Contains(sc.LastText, "sw-1: hostname sw-1") || !strings.Contains(sc.LastText, "× sw-2") {
		t.Fatalf("Ergebnis: %d/%d\n%s", sc.LastOK, sc.LastFail, sc.LastText)
	}
	if a.schedRun[sc.ID] {
		t.Fatal("muss als fertig gelten")
	}
}

// Sicherheitsprüfung 2026-11-10: neue Hosts im Ordner laufen erst nach
// Bestätigung, Änderndes wird nicht stundenlang nachgeholt, ein vom Team
// geändertes Runbook läuft nicht nach Zeitplan.
func TestScheduleGuards(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal = nil
	a.st.UpsertConnection(domain.Connection{Alias: "web-1", Host: "w1", Username: "u", Auth: domain.AuthAgent, Group: "Web"})
	old := schedDialer
	t.Cleanup(func() { schedDialer = old })
	schedDialer = func(*App) broadcast.Dialer {
		return func(_ context.Context, c domain.Connection) (broadcast.Executor, error) {
			return schedExec{c.Alias}, nil
		}
	}
	a.openScheduleForm(domain.Schedule{At: "03:00"})
	f := a.form
	f.get("name").in.SetValue("Neustart App")
	setChoice(t, f, "folder", "Web")
	f.get("steps").in.SetValue("systemctl restart app")
	if err := a.saveScheduleForm(); err != nil {
		t.Fatal(err)
	}
	// später kommt ein Host dazu (z. B. aus einem Team-Ordner)
	a.st.UpsertConnection(domain.Connection{Alias: "fremd", Host: "evil", Username: "u", Auth: domain.AuthAgent, Group: "Web"})
	sc := a.schedules()[0]
	run, fresh := a.schedTargets(sc)
	if len(run) != 1 || len(fresh) != 1 || fresh[0].Alias != "fremd" {
		t.Fatalf("run %v fresh %v", run, fresh)
	}
	res := a.startSchedule(sc)()
	a.scheduleMsg(res)
	sc, _ = a.scheduleByID(sc.ID)
	if strings.Contains(sc.LastText, "fremd") || sc.LastOK != 1 {
		t.Fatalf("neuer Host lief ohne Bestätigung mit:\n%s", sc.LastText)
	}

	// verpasst: 3 Uhr, jetzt 3 Stunden später, Schritte ändern etwas
	now := time.Now()
	a.updateSchedule(sc.ID, func(x *domain.Schedule) {
		x.At = now.Add(-3 * time.Hour).Format("15:04")
		x.Days, x.LastRun = nil, now.Add(-30*time.Hour)
	})
	if now.Add(-3*time.Hour).Day() != now.Day() {
		t.Skip("um Mitternacht nicht prüfbar")
	}
	a.scheduleTick()
	sc, _ = a.scheduleByID(sc.ID)
	if a.schedRun[sc.ID] || !strings.Contains(sc.LastText, "nicht nachgeholt") {
		t.Fatalf("Änderndes nachgeholt: %q", sc.LastText)
	}

	// Runbook aus dem Team, geändert
	rb, _ := a.st.UpsertRunbook(domain.Runbook{Name: "Team", Goal: "x", Steps: []string{"uptime"}})
	a.st.Update(func(d *domain.Data) {
		for i := range d.Runbooks {
			d.Runbooks[i].TeamChanged = true
		}
	})
	if _, err := a.schedSteps(domain.Schedule{RunbookID: rb.ID}); err == nil || !strings.Contains(err.Error(), "Team") {
		t.Fatalf("Team-Runbook läuft ungesehen: %v", err)
	}
	a.st.UseRunbook(rb.ID)
	if _, err := a.schedSteps(domain.Schedule{RunbookID: rb.ID}); err != nil {
		t.Fatalf("nach dem Ansehen: %v", err)
	}
}
