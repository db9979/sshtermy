package store

import (
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/vault"
)

// Team-Ordner sind rein lokal wie Vermittler: kein Grabstein, nie in der
// Sync-Datei. persist statt save, damit ein Abgleich keinen Sync auslöst.

func (s *Store) TeamLinks() []domain.TeamLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	return append([]domain.TeamLink(nil), s.data.TeamLinks...)
}

// UpsertTeamLink legt an (ID leer) oder ersetzt.
func (s *Store) UpsertTeamLink(l domain.TeamLink) (domain.TeamLink, error) {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return l, vault.ErrLocked
	}
	if l.ID == "" {
		l.ID = "t" + newID()
		s.data.TeamLinks = append(s.data.TeamLinks, l)
	} else {
		for i := range s.data.TeamLinks {
			if s.data.TeamLinks[i].ID == l.ID {
				s.data.TeamLinks[i] = l
			}
		}
	}
	s.mu.Unlock()
	return l, s.persist()
}

func (s *Store) DeleteTeamLink(id string) error {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	ls := s.data.TeamLinks[:0]
	for _, l := range s.data.TeamLinks {
		if l.ID != id {
			ls = append(ls, l)
		}
	}
	s.data.TeamLinks = ls
	s.mu.Unlock()
	return s.persist()
}
