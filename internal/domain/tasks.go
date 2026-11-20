package domain

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Aufgaben je Verbindung oder Ordner: offene Arbeiten („Zertifikat bis
// 12.12. erneuern“). Eine Aufgabe läuft als Ziel im Ziel- oder Agent-Modus,
// danach steht sie „zur Abnahme“, bis du sie abhakst. Gehen mit dem Sync
// (Teil der Verbindung bzw. des Ordners), nie in ein Team-Paket und nie an
// die KI außer als Ziel, das du selbst startest.

// Zustände einer Aufgabe.
const (
	TaskOpen    = ""        // offen
	TaskRunning = "running" // läuft gerade als Ziel
	TaskReview  = "review"  // Ziel erreicht, wartet auf deine Abnahme
	TaskDone    = "done"    // abgehakt
)

// Task: eine Aufgabe.
type Task struct {
	ID    string    `json:"id"`
	Text  string    `json:"text"`
	Due   time.Time `json:"due,omitempty"` // Fälligkeit (Datum, 0 = keine)
	State string    `json:"state,omitempty"`
	// Note: Ergebnis des Ziels (Meldung der KI, Gegenprüfung).
	Note    string    `json:"note,omitempty"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated,omitempty"`
}

// Overdue: fällig und noch nicht erledigt.
func (t Task) Overdue(now time.Time) bool {
	return !t.Due.IsZero() && t.State != TaskDone && !now.Before(t.Due)
}

// OpenTasks: Aufgaben, die noch nicht abgehakt sind.
func OpenTasks(ts []Task) int {
	n := 0
	for _, t := range ts {
		if t.State != TaskDone {
			n++
		}
	}
	return n
}

var dueRe = regexp.MustCompile(`(?i)\s*\b(?:bis|by|due|fällig)?\s*(\d{1,2})\.(\d{1,2})\.(\d{2,4})?\s*$|\s*\b(?:bis|by|due|fällig)\s+(\d{4})-(\d{2})-(\d{2})\s*$`)

// ParseTaskText trennt eine Fälligkeit am Ende ab: „… bis 12.12.“,
// „… 12.12.2026“, „… by 2026-12-12“. Ohne Jahr gilt das nächste Vorkommen.
func ParseTaskText(s string, now time.Time) (string, time.Time) {
	s = strings.TrimSpace(s)
	m := dueRe.FindStringSubmatchIndex(s)
	if m == nil {
		return s, time.Time{}
	}
	sub := func(i int) string {
		if m[2*i] < 0 {
			return ""
		}
		return s[m[2*i]:m[2*i+1]]
	}
	var y, mo, d int
	if sub(1) != "" {
		d, _ = strconv.Atoi(sub(1))
		mo, _ = strconv.Atoi(sub(2))
		if ys := sub(3); ys != "" {
			y, _ = strconv.Atoi(ys)
			if y < 100 {
				y += 2000
			}
		}
	} else {
		y, _ = strconv.Atoi(sub(4))
		mo, _ = strconv.Atoi(sub(5))
		d, _ = strconv.Atoi(sub(6))
	}
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return s, time.Time{}
	}
	if y == 0 {
		y = now.Year()
		if time.Date(y, time.Month(mo), d, 0, 0, 0, 0, now.Location()).Before(now.AddDate(0, 0, -1)) {
			y++
		}
	}
	due := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, now.Location())
	text := strings.TrimSpace(s[:m[0]])
	if text == "" {
		return s, time.Time{}
	}
	return text, due
}
