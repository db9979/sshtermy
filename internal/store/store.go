// Package store ist das Repository über dem verschlüsselten Vault: CRUD für
// Verbindungen, Tunnel und LLM-Konfiguration. Jede Änderung wird sofort
// verschlüsselt gespeichert (kein Klartext-Cache auf der Platte).
package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/vault"
)

// Dir liefert den Konfigurationsordner:
//
//	Windows: %APPDATA%\sshtermy
//	macOS:   ~/Library/Application Support/sshtermy
//	Linux:   $XDG_CONFIG_HOME/sshtermy bzw. ~/.config/sshtermy
//
// SSHTERMY_HOME überschreibt das (z. B. für eine portable Installation auf USB).
func Dir() (string, error) {
	if d := os.Getenv("SSHTERMY_HOME"); d != "" {
		return d, nil
	}
	if runtime.GOOS == "windows" {
		if a := os.Getenv("APPDATA"); a != "" {
			return filepath.Join(a, "sshtermy"), nil
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "sshtermy"), nil
}

// Store bündelt Vault und entschlüsselte Daten.
type Store struct {
	mu   sync.RWMutex
	V    *vault.Vault
	data *domain.Data
	Dir  string
}

// Open öffnet (oder bereitet vor) den Store im Standardordner.
func Open() (*Store, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	v, err := vault.Open(filepath.Join(dir, "store.vault"))
	if err != nil {
		return nil, err
	}
	return &Store{V: v, Dir: dir}, nil
}

// KnownHostsPath: eigene known_hosts-Datei (OpenSSH-Format, öffentliche Daten).
func (s *Store) KnownHostsPath() string { return filepath.Join(s.Dir, "known_hosts") }

// Init legt einen neuen Store an.
func (s *Store) Init(password []byte, useDPAPI bool) error {
	if err := s.V.Create(password, useDPAPI); err != nil {
		return err
	}
	s.mu.Lock()
	s.data = domain.NewData()
	s.data.Settings.UseDPAPI = s.V.HasDPAPI()
	s.mu.Unlock()
	return s.save()
}

// AfterUnlock lädt die Daten, nachdem der Vault entsperrt wurde.
func (s *Store) AfterUnlock() error {
	d := domain.NewData()
	if err := s.V.Load(d); err != nil {
		return err
	}
	s.mu.Lock()
	s.data = d
	s.mu.Unlock()
	return nil
}

// Lock sperrt und verwirft die entschlüsselten Daten.
func (s *Store) Lock() {
	s.mu.Lock()
	s.data = nil
	s.mu.Unlock()
	s.V.Lock()
}

func (s *Store) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return vault.ErrLocked
	}
	return s.V.Save(s.data)
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Connections: sortiert nach Gruppe, dann Alias.
func (s *Store) Connections() []domain.Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	out := append([]domain.Connection(nil), s.data.Connections...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return strings.ToLower(out[i].Alias) < strings.ToLower(out[j].Alias)
	})
	return out
}

func (s *Store) Connection(id string) (domain.Connection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return domain.Connection{}, false
	}
	for _, c := range s.data.Connections {
		if c.ID == id {
			return c, true
		}
	}
	return domain.Connection{}, false
}

// Validate prüft Pflichtfelder (die Oberfläche zeigt den Text an).
func Validate(c domain.Connection) error {
	switch {
	case strings.TrimSpace(c.Alias) == "":
		return errors.New("Alias fehlt")
	case strings.TrimSpace(c.Host) == "":
		return errors.New("Host fehlt")
	case c.Port < 0 || c.Port > 65535:
		return errors.New("Port ungültig")
	case strings.TrimSpace(c.Username) == "":
		return errors.New("Benutzer fehlt")
	case c.Auth == domain.AuthKeyFile && c.KeyPath == "" && !c.KeyPEM.IsSet():
		return errors.New("Keyfile: Pfad oder eingebetteten Key angeben")
	}
	return nil
}

// UpsertConnection legt an (ID leer) oder ändert.
func (s *Store) UpsertConnection(c domain.Connection) (domain.Connection, error) {
	if err := Validate(c); err != nil {
		return c, err
	}
	s.mu.Lock()
	if c.ID == "" {
		c.ID, c.CreatedAt = newID(), time.Now()
		s.data.Connections = append(s.data.Connections, c)
	} else {
		for i := range s.data.Connections {
			if s.data.Connections[i].ID == c.ID {
				s.data.Connections[i] = c
			}
		}
	}
	s.mu.Unlock()
	return c, s.save()
}

// DeleteConnection entfernt auch die Tunnel dieser Verbindung.
func (s *Store) DeleteConnection(id string) error {
	s.mu.Lock()
	cs := s.data.Connections[:0]
	for _, c := range s.data.Connections {
		if c.ID != id {
			cs = append(cs, c)
		}
	}
	s.data.Connections = cs
	ts := s.data.Tunnels[:0]
	for _, t := range s.data.Tunnels {
		if t.ConnectionID != id {
			ts = append(ts, t)
		}
	}
	s.data.Tunnels = ts
	s.mu.Unlock()
	return s.save()
}

// Touch merkt die letzte Nutzung.
func (s *Store) Touch(id string) {
	s.mu.Lock()
	for i := range s.data.Connections {
		if s.data.Connections[i].ID == id {
			s.data.Connections[i].LastUsed = time.Now()
		}
	}
	s.mu.Unlock()
	_ = s.save()
}

// Tunnels einer Verbindung ("" = alle).
func (s *Store) Tunnels(connID string) []domain.Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	var out []domain.Tunnel
	for _, t := range s.data.Tunnels {
		if connID == "" || t.ConnectionID == connID {
			out = append(out, t)
		}
	}
	return out
}

func (s *Store) UpsertTunnel(t domain.Tunnel) (domain.Tunnel, error) {
	if t.BindHost == "" {
		t.BindHost = "127.0.0.1"
	}
	s.mu.Lock()
	if t.ID == "" {
		t.ID = newID()
		s.data.Tunnels = append(s.data.Tunnels, t)
	} else {
		for i := range s.data.Tunnels {
			if s.data.Tunnels[i].ID == t.ID {
				s.data.Tunnels[i] = t
			}
		}
	}
	s.mu.Unlock()
	return t, s.save()
}

func (s *Store) DeleteTunnel(id string) error {
	s.mu.Lock()
	ts := s.data.Tunnels[:0]
	for _, t := range s.data.Tunnels {
		if t.ID != id {
			ts = append(ts, t)
		}
	}
	s.data.Tunnels = ts
	s.mu.Unlock()
	return s.save()
}

func (s *Store) LLM() domain.LLMConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return domain.DefaultLLM()
	}
	return s.data.LLM
}

func (s *Store) SetLLM(c domain.LLMConfig) error {
	s.mu.Lock()
	s.data.LLM = c
	s.mu.Unlock()
	return s.save()
}

func (s *Store) Settings() domain.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return domain.Settings{}
	}
	return s.data.Settings
}

// Runbooks liefert alle Runbooks, die für connID gelten (eigene und globale),
// zuletzt benutzte zuerst.
func (s *Store) Runbooks(connID string) []domain.Runbook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	var out []domain.Runbook
	for _, r := range s.data.Runbooks {
		if connID == "" || r.ConnectionID == "" || r.ConnectionID == connID {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].LastUsed, out[j].LastUsed
		if a.IsZero() {
			a = out[i].CreatedAt
		}
		if b.IsZero() {
			b = out[j].CreatedAt
		}
		return a.After(b)
	})
	return out
}

func (s *Store) UpsertRunbook(r domain.Runbook) (domain.Runbook, error) {
	r.Name, r.Goal = strings.TrimSpace(r.Name), strings.TrimSpace(r.Goal)
	if r.Goal == "" {
		return r, errors.New("Ziel fehlt")
	}
	if r.Name == "" {
		r.Name = r.Goal
	}
	s.mu.Lock()
	if r.ID == "" {
		r.ID, r.CreatedAt = newID(), time.Now()
		s.data.Runbooks = append(s.data.Runbooks, r)
	} else {
		for i := range s.data.Runbooks {
			if s.data.Runbooks[i].ID == r.ID {
				s.data.Runbooks[i] = r
			}
		}
	}
	s.mu.Unlock()
	return r, s.save()
}

func (s *Store) DeleteRunbook(id string) error {
	s.mu.Lock()
	rs := s.data.Runbooks[:0]
	for _, r := range s.data.Runbooks {
		if r.ID != id {
			rs = append(rs, r)
		}
	}
	s.data.Runbooks = rs
	s.mu.Unlock()
	return s.save()
}

// UseRunbook zählt eine Benutzung (für die Sortierung).
func (s *Store) UseRunbook(id string) {
	s.mu.Lock()
	for i := range s.data.Runbooks {
		if s.data.Runbooks[i].ID == id {
			s.data.Runbooks[i].Uses++
			s.data.Runbooks[i].LastUsed = time.Now()
		}
	}
	s.mu.Unlock()
	_ = s.save()
}
