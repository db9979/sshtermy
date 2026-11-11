package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Grundlinie des Log-Blicks: Stand der letzten Prüfung je Verbindung, damit
// „Was ist neu?“ neue, häufigere und verschwundene Meldungen zeigen kann.
// Nur auf diesem Rechner (nie im Sync) und verschlüsselt:
//
//	<Dir>/logbase.enc
type logBase struct {
	mu     sync.Mutex
	loaded bool
	m      map[string]json.RawMessage // Verbindungs-ID → logscan.Base
}

const logBaseAAD = "sshtermy-logbase/v1"

func (s *Store) logBasePath() string { return filepath.Join(s.Dir, "logbase.enc") }

func (s *Store) loadLogBase() bool {
	if s.lbase.loaded {
		return true
	}
	if s.V == nil || !s.V.Unlocked() {
		return false
	}
	s.lbase.m = map[string]json.RawMessage{}
	if blob, err := os.ReadFile(s.logBasePath()); err == nil {
		if plain, err := s.V.OpenBlob(blob, []byte(logBaseAAD)); err == nil {
			_ = json.Unmarshal(plain, &s.lbase.m)
		}
	}
	if s.lbase.m == nil {
		s.lbase.m = map[string]json.RawMessage{}
	}
	s.lbase.loaded = true
	return true
}

// LogBase: gespeicherte Grundlinie einer Verbindung (nil = keine).
func (s *Store) LogBase(connID string) json.RawMessage {
	s.lbase.mu.Lock()
	defer s.lbase.mu.Unlock()
	if !s.loadLogBase() {
		return nil
	}
	return s.lbase.m[connID]
}

// SetLogBase speichert die Grundlinie einer Verbindung.
func (s *Store) SetLogBase(connID string, v json.RawMessage) error {
	s.lbase.mu.Lock()
	defer s.lbase.mu.Unlock()
	if !s.loadLogBase() {
		return nil
	}
	s.lbase.m[connID] = v
	plain, err := json.Marshal(s.lbase.m)
	if err != nil {
		return err
	}
	blob, err := s.V.SealBlob(plain, []byte(logBaseAAD))
	if err != nil {
		return err
	}
	p := s.logBasePath()
	if err := os.WriteFile(p+".tmp", blob, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}
