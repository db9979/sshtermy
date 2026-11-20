package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// KI-Verbrauch je Tag (nur dieser Rechner, kein Sync): Tokens und
// geschätzte Kosten je Modell und Aufgabe. Enthält keine Inhalte, nur
// Zahlen, Modell- und Aufgabennamen, deshalb unverschlüsselt in
// <Dir>/ki-verbrauch.json. Es bleiben die letzten aiUsageDays Tage.

const aiUsageDays = 62

// AIUsageRow: Summe für einen Tag, ein Modell und eine Aufgabe.
type AIUsageRow struct {
	Day   string  `json:"day"` // 2026-11-20 (Ortszeit)
	Model string  `json:"model"`
	Task  string  `json:"task,omitempty"`
	N     int     `json:"n"`
	In    int     `json:"in"`
	Out   int     `json:"out"`
	Est   int     `json:"est,omitempty"` // davon geschätzte Anfragen
	Cost  float64 `json:"cost,omitempty"`
}

type aiUsage struct {
	mu     sync.Mutex
	loaded bool
	rows   []AIUsageRow
	dirty  bool
}

func (s *Store) aiUsagePath() string { return filepath.Join(s.Dir, "ki-verbrauch.json") }

func (s *Store) loadAIUsage() {
	if s.usage.loaded {
		return
	}
	s.usage.loaded = true
	if raw, err := os.ReadFile(s.aiUsagePath()); err == nil {
		_ = json.Unmarshal(raw, &s.usage.rows)
	}
}

// AddAIUsage zählt eine Anfrage. Gespeichert wird gebündelt (SaveAIUsage).
func (s *Store) AddAIUsage(now time.Time, model, task string, in, out int, est bool, cost float64) {
	s.usage.mu.Lock()
	defer s.usage.mu.Unlock()
	s.loadAIUsage()
	day := now.Format("2006-01-02")
	i := -1
	for j := len(s.usage.rows) - 1; j >= 0; j-- {
		r := s.usage.rows[j]
		if r.Day != day {
			break // nach Tag sortiert, ältere Tage kommen vorher
		}
		if r.Model == model && r.Task == task {
			i = j
			break
		}
	}
	if i < 0 {
		s.usage.rows = append(s.usage.rows, AIUsageRow{Day: day, Model: model, Task: task})
		i = len(s.usage.rows) - 1
		sort.SliceStable(s.usage.rows, func(a, b int) bool { return s.usage.rows[a].Day < s.usage.rows[b].Day })
		for j, r := range s.usage.rows {
			if r.Day == day && r.Model == model && r.Task == task {
				i = j
			}
		}
	}
	r := &s.usage.rows[i]
	r.N++
	r.In += in
	r.Out += out
	r.Cost += cost
	if est {
		r.Est++
	}
	s.usage.dirty = true
}

// SaveAIUsage schreibt, falls sich etwas geändert hat, und wirft alte Tage weg.
func (s *Store) SaveAIUsage(now time.Time) error {
	s.usage.mu.Lock()
	defer s.usage.mu.Unlock()
	if !s.usage.dirty {
		return nil
	}
	cut := now.AddDate(0, 0, -aiUsageDays).Format("2006-01-02")
	keep := s.usage.rows[:0]
	for _, r := range s.usage.rows {
		if r.Day >= cut {
			keep = append(keep, r)
		}
	}
	s.usage.rows = keep
	raw, err := json.Marshal(s.usage.rows)
	if err != nil {
		return err
	}
	p := s.aiUsagePath()
	if err := os.WriteFile(p+".tmp", raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(p+".tmp", p); err != nil {
		return err
	}
	s.usage.dirty = false
	return nil
}

// AIUsageSince: alle Zeilen ab dem Tag von since (einschließlich).
func (s *Store) AIUsageSince(since time.Time) []AIUsageRow {
	s.usage.mu.Lock()
	defer s.usage.mu.Unlock()
	s.loadAIUsage()
	from := since.Format("2006-01-02")
	var out []AIUsageRow
	for _, r := range s.usage.rows {
		if r.Day >= from {
			out = append(out, r)
		}
	}
	return out
}

// ResetAIUsage löscht den Verbrauch (Taste im KI-Status).
func (s *Store) ResetAIUsage() error {
	s.usage.mu.Lock()
	defer s.usage.mu.Unlock()
	s.usage.rows, s.usage.loaded, s.usage.dirty = nil, true, false
	err := os.Remove(s.aiUsagePath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
