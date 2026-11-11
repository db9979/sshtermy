package domain

import (
	"testing"
	"time"
)

func TestScheduleDue(t *testing.T) {
	loc := time.Local
	mon3 := time.Date(2026, 11, 9, 3, 0, 0, 0, loc) // Montag
	s := Schedule{At: "3:00"}
	if !s.Due(mon3) || s.Due(mon3.Add(-time.Minute)) {
		t.Fatal("pünktlich")
	}
	if !s.Due(mon3.Add(5 * time.Hour)) {
		t.Fatal("nachholen innerhalb 12 h")
	}
	if s.Due(mon3.Add(13 * time.Hour)) {
		t.Fatal("zu spät")
	}
	s.LastRun = mon3.Add(time.Second)
	if s.Due(mon3.Add(time.Hour)) {
		t.Fatal("schon gelaufen")
	}
	s = Schedule{At: "0300", Days: []int{6, 0}} // nur Wochenende
	if s.Due(mon3) {
		t.Fatal("Montag ist kein Lauftag")
	}
	if !s.Due(mon3.AddDate(0, 0, -1)) {
		t.Fatal("Sonntag")
	}
	for _, bad := range []string{"25:00", "3", "3:7", "ab:cd"} {
		if _, _, ok := ParseClock(bad); ok {
			t.Errorf("%q", bad)
		}
	}
	s.Off = true
	if s.Due(mon3.AddDate(0, 0, -1)) {
		t.Fatal("aus")
	}
}

func TestScheduleRunsOnlyOnItsMachine(t *testing.T) {
	s := Schedule{Machine: "PC-Buero"}
	if !s.RunsOn("pc-buero") || s.RunsOn("macbook") {
		t.Fatal("falscher Rechner")
	}
	if !(Schedule{}).RunsOn("x") {
		t.Fatal("ohne Rechner: hier (wird vor dem Sync zugeordnet)")
	}
}

func TestKeysHereWin(t *testing.T) {
	s := Settings{Keys: map[string]string{"goal": "ctrl+g", "ask": "f3"}, KeysHere: map[string]string{"goal": "alt+g"}}
	k := s.KeysNow()
	if k["goal"] != "alt+g" || k["ask"] != "f3" || s.Keys["goal"] != "ctrl+g" {
		t.Fatalf("%v", k)
	}
}
