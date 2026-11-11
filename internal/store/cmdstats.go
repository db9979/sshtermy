package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// Befehlsstatistik für Schnellbefehl-Vorschläge: wie oft welcher Befehl auf
// einer Verbindung ohne Fehler lief. Jeder Rechner zählt für sich
// (verschlüsselt, <Dir>/cmdstats.enc). Mit dem Sync kommen die Zählungen
// der anderen Rechner dazu (<Dir>/cmdstats-sync.enc, je Rechnername); die
// Vorschläge rechnen alle zusammen. Jeder Rechner schreibt nur seine eigene
// Zählung in die Sync-Datei, so zählt nichts doppelt.

// CmdUse: ein Befehl einer Verbindung (siehe domain.CmdUse).
type CmdUse = domain.CmdUse

// MaxCmdStats: so viele verschiedene Befehle merkt sich eine Verbindung.
const MaxCmdStats = 150

type cmdStats struct {
	mu     sync.Mutex
	loaded bool
	m      map[string][]CmdUse // Verbindungs-ID → Befehle (dieser Rechner)
	at     time.Time           // letzte Änderung von m
	remote map[string]domain.CmdStatsSet
}

const (
	statsAAD       = "sshtermy-cmdstats/v1"
	statsRemoteAAD = "sshtermy-cmdstats-sync/v1"
)

func (s *Store) statsPath() string       { return filepath.Join(s.Dir, "cmdstats.enc") }
func (s *Store) statsRemotePath() string { return filepath.Join(s.Dir, "cmdstats-sync.enc") }

// loadStats (Aufrufer hält s.stats.mu).
func (s *Store) loadStats() {
	if s.stats.loaded {
		return
	}
	s.stats.m = map[string][]CmdUse{}
	s.stats.remote = map[string]domain.CmdStatsSet{}
	if s.V == nil || !s.V.Unlocked() {
		return // gesperrt: nichts laden und vor allem nichts überschreiben
	}
	if blob, err := os.ReadFile(s.statsPath()); err == nil {
		if plain, err := s.V.OpenBlob(blob, []byte(statsAAD)); err == nil {
			_ = json.Unmarshal(plain, &s.stats.m)
		}
		if fi, err := os.Stat(s.statsPath()); err == nil {
			s.stats.at = fi.ModTime().UTC().Truncate(time.Second)
		}
	}
	if blob, err := os.ReadFile(s.statsRemotePath()); err == nil {
		if plain, err := s.V.OpenBlob(blob, []byte(statsRemoteAAD)); err == nil {
			_ = json.Unmarshal(plain, &s.stats.remote)
		}
	}
	if s.stats.m == nil {
		s.stats.m = map[string][]CmdUse{}
	}
	if s.stats.remote == nil {
		s.stats.remote = map[string]domain.CmdStatsSet{}
	}
	s.stats.loaded = true
}

func (s *Store) writeSealed(p, aad string, v any) error {
	plain, err := json.Marshal(v)
	if err != nil {
		return err
	}
	blob, err := s.V.SealBlob(plain, []byte(aad))
	if err != nil {
		return err
	}
	if err := os.WriteFile(p+".tmp", blob, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// saveStats (Aufrufer hält s.stats.mu).
func (s *Store) saveStats() error {
	s.stats.at = time.Now().UTC().Truncate(time.Second)
	// kein OnChange: die Zählung fährt beim nächsten Abgleich mit (sonst
	// liefe nach jedem Befehl ein Upload)
	return s.writeSealed(s.statsPath(), statsAAD, s.stats.m)
}

// NoteCommand zählt einen erfolgreich gelaufenen Befehl und liefert den
// neuen Stand (zusammen mit den anderen Rechnern).
func (s *Store) NoteCommand(connID, cmd string) (CmdUse, error) {
	cmd = strings.Join(strings.Fields(cmd), " ")
	if connID == "" || cmd == "" {
		return CmdUse{}, errors.New("leer")
	}
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	if !s.stats.loaded {
		return CmdUse{}, errors.New("gesperrt")
	}
	list := s.stats.m[connID]
	now := time.Now().UTC().Truncate(time.Second)
	found := false
	for i := range list {
		if list[i].Cmd == cmd {
			list[i].N++
			list[i].Last = now
			found = true
			break
		}
	}
	if !found {
		list = append(list, CmdUse{Cmd: cmd, N: 1, Last: now})
		if len(list) > MaxCmdStats {
			// seltenste und älteste zuerst raus
			sort.SliceStable(list, func(i, j int) bool { return less(list[j], list[i]) })
			list = list[:MaxCmdStats]
		}
	}
	s.stats.m[connID] = list
	err := s.saveStats()
	return s.combined(connID)[cmd], err
}

func less(a, b CmdUse) bool {
	if a.N != b.N {
		return a.N < b.N
	}
	return a.Last.Before(b.Last)
}

// combined: Zählung aller Rechner für eine Verbindung (Aufrufer hält
// s.stats.mu). Anzahl addiert, letzter Lauf und Markierungen vereinigt.
func (s *Store) combined(connID string) map[string]CmdUse {
	out := map[string]CmdUse{}
	add := func(list []CmdUse) {
		for _, u := range list {
			c, ok := out[u.Cmd]
			if !ok {
				out[u.Cmd] = u
				continue
			}
			c.N += u.N
			if u.Last.After(c.Last) {
				c.Last = u.Last
			}
			c.Offered = c.Offered || u.Offered
			c.Hidden = c.Hidden || u.Hidden
			out[u.Cmd] = c
		}
	}
	add(s.stats.m[connID])
	for _, set := range s.stats.remote {
		add(set.Conns[connID])
	}
	return out
}

// TopCommands: die häufigsten Befehle einer Verbindung (ohne „nicht
// vorschlagen“), häufigste zuerst, über alle Rechner im Sync.
func (s *Store) TopCommands(connID string, n int) []CmdUse {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	var out []CmdUse
	for _, u := range s.combined(connID) {
		if !u.Hidden && u.N > 0 {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].N == out[j].N && out[i].Last.Equal(out[j].Last) {
			return out[i].Cmd < out[j].Cmd
		}
		return less(out[j], out[i])
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// CommandUse: Stand eines einzelnen Befehls (N = 0, wenn unbekannt).
func (s *Store) CommandUse(connID, cmd string) CmdUse {
	cmd = strings.Join(strings.Fields(cmd), " ")
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	if u, ok := s.combined(connID)[cmd]; ok {
		return u
	}
	return CmdUse{Cmd: cmd}
}

// MarkCommand setzt Offered bzw. Hidden für einen Befehl. Kennt ihn nur ein
// anderer Rechner, merkt sich dieser die Markierung (N = 0), damit sie mit
// dem Sync überall gilt.
func (s *Store) MarkCommand(connID, cmd string, offered, hidden bool) error {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	if !s.stats.loaded {
		return nil
	}
	list := s.stats.m[connID]
	for i := range list {
		if list[i].Cmd == cmd {
			list[i].Offered = list[i].Offered || offered
			list[i].Hidden = list[i].Hidden || hidden
			return s.saveStats()
		}
	}
	if _, ok := s.combined(connID)[cmd]; !ok || (!offered && !hidden) {
		return nil
	}
	s.stats.m[connID] = append(list, CmdUse{Cmd: cmd, Offered: offered, Hidden: hidden})
	return s.saveStats()
}

// LocalCmdStats: die eigene Zählung für den Sync (ohne gelöschte
// Verbindungen). ok = false, wenn gesperrt.
func (s *Store) LocalCmdStats() (domain.CmdStatsSet, bool) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	if !s.stats.loaded {
		return domain.CmdStatsSet{}, false
	}
	set := domain.CmdStatsSet{At: s.stats.at, Conns: map[string][]CmdUse{}}
	for id, list := range s.stats.m {
		if len(list) > 0 {
			set.Conns[id] = append([]CmdUse(nil), list...)
		}
	}
	return set, true
}

// SetRemoteCmdStats übernimmt die Zählungen der anderen Rechner (ohne den
// eigenen, den der Aufrufer herausnimmt).
func (s *Store) SetRemoteCmdStats(m map[string]domain.CmdStatsSet) error {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	if !s.stats.loaded {
		return nil
	}
	if m == nil {
		m = map[string]domain.CmdStatsSet{}
	}
	if reflect.DeepEqual(m, s.stats.remote) {
		return nil
	}
	s.stats.remote = m
	return s.writeSealed(s.statsRemotePath(), statsRemoteAAD, m)
}

// dropStats entfernt die Statistik einer gelöschten Verbindung.
func (s *Store) dropStats(connID string) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	if _, ok := s.stats.m[connID]; ok && s.stats.loaded {
		delete(s.stats.m, connID)
		_ = s.saveStats()
	}
}

// forgetStats verwirft die entschlüsselte Statistik (beim Sperren).
func (s *Store) forgetStats() {
	s.stats.mu.Lock()
	s.stats.m, s.stats.remote, s.stats.loaded = nil, nil, false
	s.stats.mu.Unlock()
}
