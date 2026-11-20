package domain

import (
	"testing"
	"time"
)

func TestParseTaskText(t *testing.T) {
	now := time.Date(2026, 11, 20, 20, 0, 0, 0, time.Local)
	cases := []struct {
		in, text, due string
	}{
		{"Zertifikat erneuern bis 12.12.", "Zertifikat erneuern", "2026-12-12"},
		{"Backup prüfen 3.1.", "Backup prüfen", "2027-01-03"}, // ohne Jahr: nächstes Vorkommen
		{"Kernel-Update 24.12.2026", "Kernel-Update", "2026-12-24"},
		{"Patch by 2027-02-01", "Patch", "2027-02-01"},
		{"Version 1.2.3 einspielen", "Version 1.2.3 einspielen", ""},
		{"bis 12.12.", "bis 12.12.", ""},
		{"nginx neu laden", "nginx neu laden", ""},
	}
	for _, c := range cases {
		text, due := ParseTaskText(c.in, now)
		got := ""
		if !due.IsZero() {
			got = due.Format("2006-01-02")
		}
		if text != c.text || got != c.due {
			t.Errorf("%q → %q %q, erwartet %q %q", c.in, text, got, c.text, c.due)
		}
	}
	tk := Task{Due: now.AddDate(0, 0, -1)}
	if !tk.Overdue(now) {
		t.Error("überfällig")
	}
	tk.State = TaskDone
	if tk.Overdue(now) || OpenTasks([]Task{tk, {}}) != 1 {
		t.Error("erledigt zählt nicht")
	}
}
