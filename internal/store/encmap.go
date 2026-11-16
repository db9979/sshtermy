package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// encMap: kleine, verschlüsselte Zuordnung Verbindungs-ID → JSON in einer
// eigenen Datei (nur dieser Rechner, nicht im Sync). Für Messverläufe und
// Inventar-Stände, die zu groß für vulnbase.enc wären.
type encMap struct {
	mu     sync.Mutex
	loaded bool
	m      map[string]json.RawMessage
}

func (s *Store) encLoad(e *encMap, file, aad string) bool {
	if e.loaded {
		return true
	}
	if s.V == nil || !s.V.Unlocked() {
		return false
	}
	e.m = map[string]json.RawMessage{}
	if blob, err := os.ReadFile(filepath.Join(s.Dir, file)); err == nil {
		if plain, err := s.V.OpenBlob(blob, []byte(aad)); err == nil {
			_ = json.Unmarshal(plain, &e.m)
		}
	}
	if e.m == nil {
		e.m = map[string]json.RawMessage{}
	}
	e.loaded = true
	return true
}

func (s *Store) encGet(e *encMap, file, aad, key string) json.RawMessage {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !s.encLoad(e, file, aad) {
		return nil
	}
	return e.m[key]
}

func (s *Store) encSet(e *encMap, file, aad, key string, v json.RawMessage) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !s.encLoad(e, file, aad) {
		return nil
	}
	if v == nil {
		delete(e.m, key)
	} else {
		e.m[key] = v
	}
	plain, err := json.Marshal(e.m)
	if err != nil {
		return err
	}
	blob, err := s.V.SealBlob(plain, []byte(aad))
	if err != nil {
		return err
	}
	p := filepath.Join(s.Dir, file)
	if err := os.WriteFile(p+".tmp", blob, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

const (
	metricsFile = "metrics.enc"
	metricsAAD  = "sshtermy-metrics/v1"
	invFile     = "inventory.enc"
	invAAD      = "sshtermy-inventory/v1"
)

// Metrics: Messverlauf einer Verbindung (CPU, RAM, Platten), nil = keiner.
func (s *Store) Metrics(connID string) json.RawMessage {
	return s.encGet(&s.metrics, metricsFile, metricsAAD, connID)
}

// SetMetrics speichert den Messverlauf (nil löscht).
func (s *Store) SetMetrics(connID string, v json.RawMessage) error {
	return s.encSet(&s.metrics, metricsFile, metricsAAD, connID, v)
}

// Inventory: letzter Inventar-Stand und Änderungen einer Verbindung.
func (s *Store) Inventory(connID string) json.RawMessage {
	return s.encGet(&s.inv, invFile, invAAD, connID)
}

// SetInventory speichert Inventar-Stand und Änderungen (nil löscht).
func (s *Store) SetInventory(connID string, v json.RawMessage) error {
	return s.encSet(&s.inv, invFile, invAAD, connID, v)
}
