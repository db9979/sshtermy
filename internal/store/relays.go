package store

import (
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/vault"
)

// Vermittler sind rein lokal (eigener Geräteschlüssel je Rechner): kein
// Grabstein, nie in der Sync-Datei. persist statt save, damit eine Kopplung
// keinen Sync-Lauf auslöst.

func (s *Store) Relays() []domain.Relay {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	return append([]domain.Relay(nil), s.data.Relays...)
}

func (s *Store) Relay(id string) (domain.Relay, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return domain.Relay{}, false
	}
	for _, r := range s.data.Relays {
		if r.ID == id {
			return r, true
		}
	}
	return domain.Relay{}, false
}

// UpsertRelay legt an (ID leer) oder ersetzt.
func (s *Store) UpsertRelay(r domain.Relay) (domain.Relay, error) {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return r, vault.ErrLocked
	}
	if r.ID == "" {
		r.ID = "r" + newID()
		if r.Since.IsZero() {
			r.Since = time.Now().UTC()
		}
		s.data.Relays = append(s.data.Relays, r)
	} else {
		for i := range s.data.Relays {
			if s.data.Relays[i].ID == r.ID {
				s.data.Relays[i] = r
			}
		}
	}
	s.mu.Unlock()
	return r, s.persist()
}

func (s *Store) DeleteRelay(id string) error {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	rs := s.data.Relays[:0]
	for _, r := range s.data.Relays {
		if r.ID != id {
			rs = append(rs, r)
		}
	}
	s.data.Relays = rs
	s.mu.Unlock()
	return s.persist()
}
