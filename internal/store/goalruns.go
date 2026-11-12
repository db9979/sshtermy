package store

import (
	"sort"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/vault"
)

// Zielverlauf: rein lokal wie Team-Ordner (persist, kein Sync, kein
// Grabstein). Je Verbindung bleiben die letzten goalRunsPer Einträge.
const goalRunsPer = 30

// GoalRuns: frühere Ziele einer Verbindung, neueste zuerst.
func (s *Store) GoalRuns(connID string) []domain.GoalRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	var out []domain.GoalRun
	for _, r := range s.data.GoalRuns {
		if r.ConnID == connID {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// AddGoalRun hängt ein Ziel an (ID wird vergeben) und kürzt den Verlauf.
func (s *Store) AddGoalRun(r domain.GoalRun) error {
	s.Fresh()
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	if r.ID == "" {
		r.ID = "g" + newID()
	}
	s.data.GoalRuns = append(s.data.GoalRuns, r)
	// je Verbindung nur die neuesten goalRunsPer behalten
	n, drop := 0, map[int]bool{}
	for i := len(s.data.GoalRuns) - 1; i >= 0; i-- {
		if s.data.GoalRuns[i].ConnID == r.ConnID {
			n++
			if n > goalRunsPer {
				drop[i] = true
			}
		}
	}
	if len(drop) > 0 {
		keep := make([]domain.GoalRun, 0, len(s.data.GoalRuns)-len(drop))
		for i, x := range s.data.GoalRuns {
			if !drop[i] {
				keep = append(keep, x)
			}
		}
		s.data.GoalRuns = keep
	}
	s.mu.Unlock()
	return s.persist()
}

// DeleteGoalRun entfernt einen Eintrag aus dem Zielverlauf.
func (s *Store) DeleteGoalRun(id string) error {
	s.Fresh()
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	rs := s.data.GoalRuns[:0]
	for _, r := range s.data.GoalRuns {
		if r.ID != id {
			rs = append(rs, r)
		}
	}
	s.data.GoalRuns = rs
	s.mu.Unlock()
	return s.persist()
}
