package store

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

// Shell „nur dieser Rechner“: der Sync-Stand behält die alte Shell.
func TestConnHereKeepsSyncedShell(t *testing.T) {
	s := openTest(t)
	c, err := s.UpsertConnection(domain.Connection{Alias: "l", Protocol: domain.ProtoLocal, LocalShell: "zsh"})
	if err != nil {
		t.Fatal(err)
	}
	c.LocalShell = "pwsh"
	got, err := s.UpsertConnectionHere(c, &domain.ConnHere{LocalShell: "pwsh"})
	if err != nil || got.LocalShell != "pwsh" {
		t.Fatalf("%v %q", err, got.LocalShell)
	}
	d, _, _ := s.Snapshot()
	if d.Connections[0].LocalShell != "zsh" {
		t.Fatalf("Sync-Stand geändert: %q", d.Connections[0].LocalShell)
	}
	if c2, _ := s.Connection(c.ID); c2.LocalShell != "pwsh" {
		t.Fatalf("hier: %q", c2.LocalShell)
	}
	// wieder für alle Rechner
	c.LocalShell = "bash"
	if _, err := s.UpsertConnectionHere(c, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ConnHere(c.ID); ok {
		t.Fatal("lokale Abweichung nicht entfernt")
	}
	if c2, _ := s.Connection(c.ID); c2.LocalShell != "bash" {
		t.Fatalf("%q", c2.LocalShell)
	}
}

// Zeitpläne: Änderung setzt UpdatedAt, Löschen einen Grabstein.
func TestUpdateSchedulesStampsAndTombstones(t *testing.T) {
	s := openTest(t)
	_ = s.UpdateSchedules(func(l []domain.Schedule) []domain.Schedule {
		return append(l, domain.Schedule{ID: "s1", At: "03:00"})
	})
	d, _, _ := s.Snapshot()
	if d.Settings.Schedules[0].UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt fehlt")
	}
	_ = s.UpdateSchedules(func([]domain.Schedule) []domain.Schedule { return nil })
	if d, _, _ = s.Snapshot(); d.Deleted["s1"].IsZero() || len(d.Settings.Schedules) != 0 {
		t.Fatalf("%+v", d)
	}
}

// Befehlsstatistik: andere Rechner zählen mit, eigene Zählung bleibt getrennt.
func TestCmdStatsCombined(t *testing.T) {
	s := openTest(t)
	_, _ = s.NoteCommand("c", "uptime")
	_ = s.SetRemoteCmdStats(map[string]domain.CmdStatsSet{
		"mac": {Conns: map[string][]domain.CmdUse{"c": {{Cmd: "uptime", N: 2}, {Cmd: "df -h", N: 1, Hidden: true}}}},
	})
	if u := s.CommandUse("c", "uptime"); u.N != 3 {
		t.Fatalf("zusammen: %d", u.N)
	}
	if top := s.TopCommands("c", 0); len(top) != 1 || top[0].Cmd != "uptime" {
		t.Fatalf("%+v", top)
	}
	own, ok := s.LocalCmdStats()
	if !ok || own.Conns["c"][0].N != 1 || own.At.IsZero() {
		t.Fatalf("eigene: %+v", own)
	}
}
