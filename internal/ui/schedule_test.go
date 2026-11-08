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
