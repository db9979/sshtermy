package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Letzter Sicherheitscheck (OSV) je Verbindung: Grundlage für die Zeile in
// F6, den Punkt im Manager und „neu seit dem letzten Mal“. Nur auf diesem
// Rechner und verschlüsselt:
//
//	<Dir>/vulnbase.enc
const vulnBaseAAD = "sshtermy-vulnbase/v1"

func (s *Store) vulnBasePath() string { return filepath.Join(s.Dir, "vulnbase.enc") }

func (s *Store) loadVulnBase() bool {
	b := &s.vbase
	if b.loaded {
		return true
	}
	if s.V == nil || !s.V.Unlocked() {
		return false
	}
	b.m = map[string]json.RawMessage{}
	if blob, err := os.ReadFile(s.vulnBasePath()); err == nil {
		if plain, err := s.V.OpenBlob(blob, []byte(vulnBaseAAD)); err == nil {
			_ = json.Unmarshal(plain, &b.m)
		}
	}
	if b.m == nil {
		b.m = map[string]json.RawMessage{}
	}
	b.loaded = true
	return true
}

// VulnReport: letzter gespeicherter Sicherheitscheck (nil = keiner).
func (s *Store) VulnReport(connID string) json.RawMessage {
	s.vbase.mu.Lock()
	defer s.vbase.mu.Unlock()
	if !s.loadVulnBase() {
		return nil
	}
	return s.vbase.m[connID]
}

// VulnReports: alle gespeicherten Prüfungen (Kopie der Zuordnung).
func (s *Store) VulnReports() map[string]json.RawMessage {
	s.vbase.mu.Lock()
	defer s.vbase.mu.Unlock()
	out := map[string]json.RawMessage{}
	if !s.loadVulnBase() {
		return out
	}
	for k, v := range s.vbase.m {
		if !strings.Contains(k, "|") { // „fw|…“, „hd|…“: andere Prüfungen
			out[k] = v
		}
	}
	return out
}

// SetVulnReport speichert den Sicherheitscheck einer Verbindung (nil löscht).
func (s *Store) SetVulnReport(connID string, v json.RawMessage) error {
	s.vbase.mu.Lock()
	defer s.vbase.mu.Unlock()
	if !s.loadVulnBase() {
		return nil
	}
	if v == nil {
		delete(s.vbase.m, connID)
	} else {
		s.vbase.m[connID] = v
	}
	plain, err := json.Marshal(s.vbase.m)
	if err != nil {
		return err
	}
	blob, err := s.V.SealBlob(plain, []byte(vulnBaseAAD))
	if err != nil {
		return err
	}
	p := s.vulnBasePath()
	if err := os.WriteFile(p+".tmp", blob, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// Firmware-Hinweise (NVD) für Netzwerkgeräte liegen in derselben Datei,
// unter „fw|<Verbindung>“.
const fwPrefix = "fw|"

// FirmwareReport: letzte Firmware-Prüfung eines Netzwerkgeräts (nil = keine).
func (s *Store) FirmwareReport(connID string) json.RawMessage {
	return s.VulnReport(fwPrefix + connID)
}

// SetFirmwareReport speichert die Firmware-Prüfung (nil löscht).
func (s *Store) SetFirmwareReport(connID string, v json.RawMessage) error {
	return s.SetVulnReport(fwPrefix+connID, v)
}

// ExtraReport: weitere Prüfungen je Verbindung in derselben Datei
// (kind „hd“ Härtungs-Check, „ki“ Schlüssel-Inventar).
func (s *Store) ExtraReport(kind, connID string) json.RawMessage {
	return s.VulnReport(kind + "|" + connID)
}

// SetExtraReport speichert eine weitere Prüfung (nil löscht).
func (s *Store) SetExtraReport(kind, connID string, v json.RawMessage) error {
	return s.SetVulnReport(kind+"|"+connID, v)
}
