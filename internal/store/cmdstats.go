package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Befehlsstatistik für Schnellbefehl-Vorschläge: wie oft welcher Befehl auf
// einer Verbindung ohne Fehler lief. Sie liegt wie die Sicherungen nur auf
// diesem Rechner (nie im Sync) und verschlüsselt:
//
//	<Dir>/cmdstats.enc

// CmdUse: ein Befehl einer Verbindung.
type CmdUse struct {
	Cmd     string    `json:"c"`
	N       int       `json:"n"`
	Last    time.Time `json:"t"`
	Offered bool      `json:"o,omitempty"` // Vorschlag schon einmal gezeigt
	Hidden  bool      `json:"h,omitempty"` // „nicht vorschlagen“
}

// MaxCmdStats: so viele verschiedene Befehle merkt sich eine Verbindung.
const MaxCmdStats = 150

type cmdStats struct {
	mu     sync.Mutex
	loaded bool
	m      map[string][]CmdUse // Verbindungs-ID → Befehle
}

const statsAAD = "sshtermy-cmdstats/v1"

func (s *Store) statsPath() string { return filepath.Join(s.Dir, "cmdstats.enc") }

// loadStats (Aufrufer hält s.stats.mu).
func (s *Store) loadStats() {
	if s.stats.loaded {
		return
	}
	s.stats.m = map[string][]CmdUse{}
	if s.V == nil || !s.V.Unlocked() {
		return // gesperrt: nichts laden und vor allem nichts überschreiben
	}
	blob, err := os.ReadFile(s.statsPath())
	if err == nil {
		if plain, err := s.V.OpenBlob(blob, []byte(statsAAD)); err == nil {
			_ = json.Unmarshal(plain, &s.stats.m)
		}
	}
	if s.stats.m == nil {
		s.stats.m = map[string][]CmdUse{}
	}
	s.stats.loaded = true
}

// saveStats (Aufrufer hält s.stats.mu).
func (s *Store) saveStats() error {
	plain, err := json.Marshal(s.stats.m)
	if err != nil {
		return err
	}
	blob, err := s.V.SealBlob(plain, []byte(statsAAD))
	if err != nil {
		return err
	}
	p := s.statsPath()
	if err := os.WriteFile(p+".tmp", blob, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// NoteCommand zählt einen erfolgreich gelaufenen Befehl und liefert den
// neuen Stand.
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
	var u CmdUse
	found := false
	for i := range list {
		if list[i].Cmd == cmd {
			list[i].N++
			list[i].Last = now
			u, found = list[i], true
			break
		}
	}
	if !found {
		u = CmdUse{Cmd: cmd, N: 1, Last: now}
		list = append(list, u)
		if len(list) > MaxCmdStats {
			// seltenste und älteste zuerst raus
			sort.SliceStable(list, func(i, j int) bool { return less(list[j], list[i]) })
			list = list[:MaxCmdStats]
		}
	}
	s.stats.m[connID] = list
	return u, s.saveStats()
}

func less(a, b CmdUse) bool {
	if a.N != b.N {
		return a.N < b.N
	}
	return a.Last.Before(b.Last)
}

// TopCommands: die häufigsten Befehle einer Verbindung (ohne „nicht
// vorschlagen“), häufigste zuerst.
func (s *Store) TopCommands(connID string, n int) []CmdUse {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.loadStats()
	var out []CmdUse
	for _, u := range s.stats.m[connID] {
		if !u.Hidden {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return less(out[j], out[i]) })
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
	for _, u := range s.stats.m[connID] {
		if u.Cmd == cmd {
			return u
		}
	}
	return CmdUse{Cmd: cmd}
}

// MarkCommand setzt Offered bzw. Hidden für einen Befehl.
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
	return nil
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
	s.stats.m, s.stats.loaded = nil, false
	s.stats.mu.Unlock()
}
