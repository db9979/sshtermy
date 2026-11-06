// Package store ist das Repository über dem verschlüsselten Vault: CRUD für
// Verbindungen, Tunnel und LLM-Konfiguration. Jede Änderung wird sofort
// verschlüsselt gespeichert (kein Klartext-Cache auf der Platte).
package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
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

	// OnChange wird nach jeder lokalen Änderung aufgerufen (nicht nach dem
	// Übernehmen eines Sync-Stands), z. B. um einen Abgleich anzustoßen.
	OnChange func()
	rev      uint64 // zählt lokale Änderungen

	// Zwischenspeicher für Connections(): bei 500 Verbindungen kostet das
	// Sortieren pro Bild mehrere Millisekunden, und der Manager fragt pro
	// Bild mehrmals. gen zählt jede Änderung (persist, Laden, Sperren); ein
	// Eintrag gilt nur, solange seine gen aktuell ist.
	gen   atomic.Uint64
	cmu   sync.Mutex
	conns connCache

	stats cmdStats // Befehlsstatistik (cmdstats.go), nur lokal
}

type connCache struct {
	gen uint64
	ok  bool
	cs  []domain.Connection
}

// changed macht den Zwischenspeicher ungültig.
func (s *Store) changed() { s.gen.Add(1) }

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
	s.changed()
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
	s.changed()
	return nil
}

// Lock sperrt und verwirft die entschlüsselten Daten.
func (s *Store) Lock() {
	s.mu.Lock()
	s.data = nil
	s.mu.Unlock()
	s.changed()
	s.forgetStats()
	s.V.Lock()
}

func (s *Store) save() error {
	if err := s.persist(); err != nil {
		return err
	}
	s.mu.Lock()
	s.rev++
	s.mu.Unlock()
	if s.OnChange != nil {
		s.OnChange()
	}
	return nil
}

func (s *Store) persist() error {
	s.changed()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return vault.ErrLocked
	}
	return s.V.Save(s.data)
}

// tombstone merkt eine gelöschte ID für den Sync (Aufrufer hält s.mu).
func (s *Store) tombstone(id string) {
	if s.data.Deleted == nil {
		s.data.Deleted = map[string]time.Time{}
	}
	s.data.Deleted[id] = time.Now().UTC()
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Connections: sortiert nach Ordner, dann Reihenfolge im Ordner, dann Alias.
// Der Aufrufer bekommt eine eigene Kopie und darf sie verändern.
func (s *Store) Connections() []domain.Connection {
	g := s.gen.Load()
	s.cmu.Lock()
	if c := s.conns; c.ok && c.gen == g {
		s.cmu.Unlock()
		return append([]domain.Connection(nil), c.cs...)
	}
	s.cmu.Unlock()

	s.mu.RLock()
	if s.data == nil {
		s.mu.RUnlock()
		return nil
	}
	out := s.sortedConns()
	s.mu.RUnlock()

	s.cmu.Lock()
	s.conns = connCache{gen: g, ok: true, cs: out}
	s.cmu.Unlock()
	return append([]domain.Connection(nil), out...)
}

// sortedConns (Aufrufer hält s.mu).
func (s *Store) sortedConns() []domain.Connection {
	out := append([]domain.Connection(nil), s.data.Connections...)
	for i := range out {
		out[i] = s.withLocalKey(out[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return false
	})
	// innerhalb eines Ordners wie im Baum
	for i := 0; i < len(out); {
		j := i
		for j < len(out) && out[j].Group == out[i].Group {
			j++
		}
		domain.SortConns(out[i:j])
		i = j
	}
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
			return s.withLocalKey(c), true
		}
	}
	return domain.Connection{}, false
}

// withLocalKey: Verbindung, wie dieser Rechner sie benutzt (eigener Pfad zur
// Schlüsseldatei statt des synchronisierten). Aufrufer hält s.mu.
func (s *Store) withLocalKey(c domain.Connection) domain.Connection {
	if p := s.data.LocalKeys[c.ID]; p != "" && c.Auth == domain.AuthKeyFile {
		c.KeyPath = p
	}
	return c
}

// SetLocalKeyPath legt fest, wo die Schlüsseldatei einer Verbindung auf
// diesem Rechner liegt. Der synchronisierte Pfad (für die anderen Rechner)
// bleibt unverändert. Leer = wieder den synchronisierten Pfad nehmen.
func (s *Store) SetLocalKeyPath(id, path string) error {
	home, _ := os.UserHomeDir()
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	s.setLocalKey(id, domain.PortableKeyPath(path, home))
	s.mu.Unlock()
	return s.save()
}

// setLocalKey (Aufrufer hält s.mu).
func (s *Store) setLocalKey(id, path string) {
	if path == "" {
		delete(s.data.LocalKeys, id)
		return
	}
	if s.data.LocalKeys == nil {
		s.data.LocalKeys = map[string]string{}
	}
	s.data.LocalKeys[id] = path
}

// keyPathFor: Schlüsselpfad beim Speichern aufteilen. Der synchronisierte
// Pfad gehört dem Rechner, auf dem er gewählt wurde. Gibt es ihn hier nicht
// (z. B. C:\Users\… am Mac) und wird ein anderer gewählt, gilt der neue nur
// lokal, sonst bräche der Sync die Verbindung auf dem anderen Rechner.
// Aufrufer hält s.mu.
func (s *Store) keyPathFor(c *domain.Connection, old domain.Connection, found bool, home string) {
	if c.Auth != domain.AuthKeyFile || c.KeyPath == "" {
		s.setLocalKey(c.ID, "")
		return
	}
	c.KeyPath = domain.PortableKeyPath(c.KeyPath, home)
	if !found {
		return
	}
	local := s.data.LocalKeys[c.ID]
	eff := old.KeyPath
	if local != "" {
		eff = local
	}
	switch {
	case c.KeyPath == domain.PortableKeyPath(eff, home): // Pfad nicht geändert
		c.KeyPath = old.KeyPath
	case old.Auth == domain.AuthKeyFile && old.KeyPath != "" && (local != "" || !fileExists(domain.ExpandKeyPath(old.KeyPath, home))):
		s.setLocalKey(c.ID, c.KeyPath)
		c.KeyPath = old.KeyPath
	default:
		s.setLocalKey(c.ID, "")
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Validate prüft Pflichtfelder (die Oberfläche zeigt den Text an).
func Validate(c domain.Connection) error {
	switch {
	case strings.TrimSpace(c.Alias) == "":
		return errors.New(i18n.T("Alias fehlt"))
	case strings.TrimSpace(c.Host) == "" && c.Protocol != domain.ProtoSerial:
		return errors.New(i18n.T("Host fehlt"))
	case c.Port < 0 || c.Port > 65535:
		return errors.New(i18n.T("Port ungültig"))
	case c.Protocol == domain.ProtoSerial && (c.Baud < 0 || c.Baud > 4000000):
		return errors.New(i18n.T("Baudrate ungültig"))
	case c.Protocol.Raw(): // Telnet/Konsole: Anmeldung optional, am Gerät
	case strings.TrimSpace(c.Username) == "":
		return errors.New(i18n.T("Benutzer fehlt"))
	case c.Auth == domain.AuthKeyFile && c.KeyPath == "" && !c.KeyPEM.IsSet():
		return errors.New(i18n.T("Keyfile: Pfad oder eingebetteten Key angeben"))
	}
	return nil
}

// UpsertConnection legt an (ID leer) oder ändert.
func (s *Store) UpsertConnection(c domain.Connection) (domain.Connection, error) {
	if err := Validate(c); err != nil {
		return c, err
	}
	c.Group = domain.NormPath(c.Group)
	home, _ := os.UserHomeDir()
	s.mu.Lock()
	c.UpdatedAt = time.Now().UTC()
	if c.ID == "" {
		c.ID, c.CreatedAt = newID(), time.Now()
		c.Order = s.nextOrder(c.Group)
		s.keyPathFor(&c, domain.Connection{}, false, home)
		s.data.Connections = append(s.data.Connections, c)
	} else {
		var old domain.Connection
		found := false
		for _, o := range s.data.Connections {
			if o.ID == c.ID {
				old, found = o, true
			}
		}
		s.keyPathFor(&c, old, found, home)
		for i := range s.data.Connections {
			if s.data.Connections[i].ID == c.ID {
				if s.data.Connections[i].Group != c.Group { // anderer Ordner: hinten anstellen
					c.Order = s.nextOrder(c.Group)
				}
				s.data.Connections[i] = c
			}
		}
		if !found {
			s.data.Connections = append(s.data.Connections, c)
		}
	}
	c = s.withLocalKey(c)
	s.mu.Unlock()
	return c, s.save()
}

// SetProfile speichert nur das Systemprofil einer Verbindung (die übrigen
// Felder bleiben, wie sie im Store stehen, auch wenn sie inzwischen jemand
// bearbeitet hat).
func (s *Store) SetProfile(id string, p domain.SystemProfile) error {
	s.mu.Lock()
	found := false
	for i := range s.data.Connections {
		if s.data.Connections[i].ID == id {
			cp := p
			cp.Failed = append([]string(nil), p.Failed...)
			cp.Learned = append([]domain.Correction(nil), p.Learned...)
			s.data.Connections[i].Profile = &cp
			s.data.Connections[i].UpdatedAt = time.Now().UTC()
			found = true
		}
	}
	s.mu.Unlock()
	if !found {
		return nil
	}
	return s.save()
}

// AddConnections legt mehrere Verbindungen auf einmal an (Import) und
// speichert nur einmal. Vorbelegte IDs bleiben erhalten (damit Jump-Hosts
// innerhalb des Imports aufeinander zeigen können).
func (s *Store) AddConnections(cs []domain.Connection) ([]domain.Connection, error) {
	for _, c := range cs {
		if err := Validate(c); err != nil {
			return nil, errors.New(c.Alias + ": " + err.Error())
		}
	}
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return nil, vault.ErrLocked
	}
	now := time.Now().UTC()
	home, _ := os.UserHomeDir()
	out := make([]domain.Connection, len(cs))
	for i, c := range cs {
		if c.ID == "" {
			c.ID = newID()
		}
		if c.Auth == domain.AuthKeyFile {
			c.KeyPath = domain.PortableKeyPath(c.KeyPath, home)
		}
		c.Group = domain.NormPath(c.Group)
		c.Order = s.nextOrder(c.Group)
		c.CreatedAt, c.UpdatedAt = now, now
		s.data.Connections = append(s.data.Connections, c)
		out[i] = c
	}
	s.mu.Unlock()
	return out, s.save()
}

// NewID: neue, zufällige ID für einen Datensatz.
func NewID() string { return newID() }

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
	delete(s.data.LocalKeys, id)
	s.tombstone(id)
	ts := s.data.Tunnels[:0]
	for _, t := range s.data.Tunnels {
		if t.ConnectionID != id {
			ts = append(ts, t)
		} else {
			s.tombstone(t.ID)
		}
	}
	s.data.Tunnels = ts
	s.mu.Unlock()
	s.dropStats(id)
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
	t.UpdatedAt = time.Now().UTC()
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
	s.tombstone(id)
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
	s.data.LLMUpdatedAt = time.Now().UTC()
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

// ---------------------------------------------------------------- Sync

// SyncConfig: lokale Sync-Einstellungen.
func (s *Store) SyncConfig() domain.SyncConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return domain.SyncConfig{}
	}
	return s.data.Sync
}

// SetSyncConfig speichert die Sync-Einstellungen (löst keinen Abgleich aus).
func (s *Store) SetSyncConfig(c domain.SyncConfig) error {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	s.data.Sync = c
	s.mu.Unlock()
	return s.persist()
}

// ForgetArchived merkt sich, dass ein Archiv-Eintrag (Sicherung, Protokoll)
// hier gelöscht wurde, damit er auch im Sync-Archiv und auf den anderen
// Rechnern verschwindet. key: syncx.ArchiveTombstone(...).
func (s *Store) ForgetArchived(key string) error {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	s.tombstone(key)
	s.mu.Unlock()
	return s.persist()
}

// Snapshot liefert eine Kopie der Daten und den Änderungszähler.
func (s *Store) Snapshot() (*domain.Data, uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, 0, vault.ErrLocked
	}
	d := *s.data
	d.Connections = append([]domain.Connection(nil), s.data.Connections...)
	d.Tunnels = append([]domain.Tunnel(nil), s.data.Tunnels...)
	d.Runbooks = append([]domain.Runbook(nil), s.data.Runbooks...)
	d.Folders = append([]domain.Folder(nil), s.data.Folders...)
	d.Deleted = map[string]time.Time{}
	for k, v := range s.data.Deleted {
		d.Deleted[k] = v
	}
	return &d, s.rev, nil
}

// ApplySync übernimmt einen Sync-Stand. Gab es seit dem Snapshot (rev)
// lokale Änderungen, wird stattdessen merge auf den aktuellen Stand
// angewendet und changed = true gemeldet (dann bitte erneut abgleichen).
func (s *Store) ApplySync(rev uint64, apply func(d *domain.Data)) (changed bool, err error) {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return false, vault.ErrLocked
	}
	apply(s.data)
	changed = s.rev != rev
	s.mu.Unlock()
	return changed, s.persist()
}

// Update ändert den Stand in einem Schritt und speichert einmal (z. B. bei
// der Übernahme eines Team-Pakets).
func (s *Store) Update(f func(d *domain.Data)) error {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	f(s.data)
	s.mu.Unlock()
	return s.save()
}

// UpdateSettings ändert die Einstellungen und speichert.
func (s *Store) UpdateSettings(f func(*domain.Settings)) error {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	before, _ := json.Marshal(s.data.Settings)
	f(&s.data.Settings)
	if after, _ := json.Marshal(s.data.Settings); !bytes.Equal(before, after) {
		s.data.SettingsUpdatedAt = time.Now().UTC() // Sync: neuere Einstellungen gewinnen
	}
	s.mu.Unlock()
	return s.save()
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
		return r, errors.New(i18n.T("Ziel fehlt"))
	}
	if r.Name == "" {
		r.Name = r.Goal
	}
	s.mu.Lock()
	r.UpdatedAt = time.Now().UTC()
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
	s.tombstone(id)
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

// LogDir: Ordner der verschlüsselten Sitzungsprotokolle (im Sync nur über das
// zuschaltbare Archiv, siehe syncx/archive.go).
func (s *Store) LogDir() string { return filepath.Join(s.Dir, "logs") }

// SessionLogKey: eigener Schlüssel für die Sitzungsprotokolle, abgeleitet aus
// dem Store-Schlüssel (nur bei entsperrtem Store).
func (s *Store) SessionLogKey() ([]byte, error) { return s.V.DeriveKey("sessionlog") }
