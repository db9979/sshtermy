package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Gesicherte Konfigurationen von Netzwerkgeräten (show running-config). Sie
// enthalten oft Passwort-Hashes und SNMP-Communities, deshalb liegen sie wie
// der Store verschlüsselt auf der Platte, eine Datei je Stand:
//
//	<Dir>/backups/<Verbindungs-ID>/<UTC-Zeit>_<Zeilen>.cfg.enc
//
// Mit dem zuschaltbaren Sync-Archiv (syncx/archive.go) kommen sie zusätzlich
// verschlüsselt auf die anderen Rechner (ImportBackup).

// Backup beschreibt einen gesicherten Stand.
type Backup struct {
	ID    string // Dateiname ohne Endung
	Taken time.Time
	Lines int
}

const backupTime = "20060102T150405Z"

func (s *Store) backupDir(connID string) (string, error) {
	id := filepath.Base(connID)
	if id != connID || id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", errors.New(i18n.T("ungültige Verbindung"))
	}
	return filepath.Join(s.Dir, "backups", id), nil
}

func backupAAD(connID, id string) []byte { return []byte("sshtermy-backup:" + connID + "/" + id) }

// SaveBackup speichert einen neuen Stand verschlüsselt.
func (s *Store) SaveBackup(connID, cfg string) (Backup, error) {
	dir, err := s.backupDir(connID)
	if err != nil {
		return Backup{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Backup{}, err
	}
	now := time.Now().UTC()
	lines := strings.Count(strings.TrimRight(cfg, "\n"), "\n") + 1
	b := Backup{ID: fmt.Sprintf("%s_%d", now.Format(backupTime), lines), Taken: now.Truncate(time.Second), Lines: lines}
	blob, err := s.V.SealBlob([]byte(cfg), backupAAD(connID, b.ID))
	if err != nil {
		return Backup{}, err
	}
	p := filepath.Join(dir, b.ID+".cfg.enc")
	if err := os.WriteFile(p+".tmp", blob, 0o600); err != nil {
		return Backup{}, err
	}
	return b, os.Rename(p+".tmp", p)
}

// ImportBackup legt einen Stand von einem anderen Rechner ab (gleiche ID wie
// dort). Gibt es ihn schon, bleibt er unverändert (added = false).
func (s *Store) ImportBackup(connID, id, cfg string) (added bool, err error) {
	dir, err := s.backupDir(connID)
	if err != nil {
		return false, err
	}
	ts, _, ok := strings.Cut(id, "_")
	if _, perr := time.Parse(backupTime, ts); !ok || perr != nil || filepath.Base(id) != id {
		return false, errors.New(i18n.T("ungültige Sicherung"))
	}
	p := filepath.Join(dir, id+".cfg.enc")
	if _, err := os.Stat(p); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	blob, err := s.V.SealBlob([]byte(cfg), backupAAD(connID, id))
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(p+".tmp", blob, 0o600); err != nil {
		return false, err
	}
	return true, os.Rename(p+".tmp", p)
}

// DeleteBackup entfernt einen Stand (fehlt er schon, ist das kein Fehler).
func (s *Store) DeleteBackup(connID, id string) error {
	dir, err := s.backupDir(connID)
	if err != nil {
		return err
	}
	if filepath.Base(id) != id || id == "" || id == "." || id == ".." {
		return errors.New(i18n.T("ungültige Sicherung"))
	}
	err = os.Remove(filepath.Join(dir, id+".cfg.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// BackupConns: Verbindungs-IDs, für die es Sicherungen gibt.
func (s *Store) BackupConns() []string {
	ents, _ := os.ReadDir(filepath.Join(s.Dir, "backups"))
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// Backups: alle Stände einer Verbindung, neuester zuerst.
func (s *Store) Backups(connID string) []Backup {
	dir, err := s.backupDir(connID)
	if err != nil {
		return nil
	}
	ents, _ := os.ReadDir(dir)
	var out []Backup
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".cfg.enc")
		if !ok {
			continue
		}
		ts, n, _ := strings.Cut(name, "_")
		t, err := time.Parse(backupTime, ts)
		if err != nil {
			continue
		}
		lines, _ := strconv.Atoi(n)
		out = append(out, Backup{ID: name, Taken: t, Lines: lines})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// LoadBackup entschlüsselt einen Stand.
func (s *Store) LoadBackup(connID, id string) (string, error) {
	dir, err := s.backupDir(connID)
	if err != nil {
		return "", err
	}
	if filepath.Base(id) != id {
		return "", errors.New(i18n.T("ungültige Sicherung"))
	}
	blob, err := os.ReadFile(filepath.Join(dir, id+".cfg.enc"))
	if err != nil {
		return "", err
	}
	plain, err := s.V.OpenBlob(blob, backupAAD(connID, id))
	if err != nil {
		return "", errors.New(i18n.T("Sicherung lässt sich nicht entschlüsseln"))
	}
	return string(plain), nil
}

// BackupDir: Ordner der Sicherungen einer Verbindung (für die Anzeige).
func (s *Store) BackupDir(connID string) string {
	d, _ := s.backupDir(connID)
	return d
}
