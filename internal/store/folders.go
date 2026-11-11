package store

import (
	"errors"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vault"
)

// Ordner im Verbindungs-Manager (siehe domain/folders.go). Jede Änderung
// setzt UpdatedAt nur bei den Einträgen, die sich wirklich ändern, damit der
// Sync pro Eintrag sauber zusammenführen kann.

// Folders: alle Ordner-Einträge.
func (s *Store) Folders() []domain.Folder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	return append([]domain.Folder(nil), s.data.Folders...)
}

// Tree: Verbindungen und Ordner als Baum.
func (s *Store) Tree() *domain.TreeNode {
	return domain.BuildTree(s.Connections(), s.Folders())
}

// nextOrder: Reihenfolge für einen neuen Eintrag im Ordner (Aufrufer hält
// s.mu). Wurde der Ordner nie umsortiert, bleibt es alphabetisch (0).
func (s *Store) nextOrder(group string) float64 {
	var hi float64
	for _, c := range s.data.Connections {
		if c.Group == group && c.Order > hi {
			hi = c.Order
		}
	}
	if hi == 0 {
		return 0
	}
	return hi + 1
}

// folderIdx: Index des Ordner-Eintrags oder -1 (Aufrufer hält s.mu).
func (s *Store) folderIdx(path string) int {
	for i, f := range s.data.Folders {
		if f.Path == path {
			return i
		}
	}
	return -1
}

// ensureFolder legt einen Eintrag für path und alle Ebenen darüber an.
func (s *Store) ensureFolder(path string, now time.Time) int {
	if path == "" {
		return -1
	}
	if p := domain.PathParent(path); p != "" {
		s.ensureFolder(p, now)
	}
	if i := s.folderIdx(path); i >= 0 {
		return i
	}
	s.data.Folders = append(s.data.Folders, domain.Folder{ID: newID(), Path: path, CreatedAt: now, UpdatedAt: now})
	return len(s.data.Folders) - 1
}

func (s *Store) lockData() error {
	s.mu.Lock()
	if s.data == nil {
		s.mu.Unlock()
		return vault.ErrLocked
	}
	return nil
}

// CreateFolder legt einen (auch leeren) Ordner an.
func (s *Store) CreateFolder(path string) (string, error) {
	s.Fresh()
	path = domain.NormPath(path)
	if path == "" {
		return "", errors.New(i18n.T("Name fehlt"))
	}
	if err := s.lockData(); err != nil {
		return "", err
	}
	if s.folderIdx(path) >= 0 || s.hasConnIn(path) {
		s.mu.Unlock()
		return path, errors.New("„" + path + i18n.T("“ gibt es schon"))
	}
	s.ensureFolder(path, time.Now().UTC())
	s.mu.Unlock()
	return path, s.save()
}

func (s *Store) hasConnIn(path string) bool {
	for _, c := range s.data.Connections {
		if c.Group == path {
			return true
		}
	}
	return false
}

// MoveFolder benennt einen Ordner um bzw. hängt ihn woanders ein, samt
// Unterordnern und Verbindungen. Gibt es das Ziel schon, werden beide
// zusammengelegt.
func (s *Store) MoveFolder(old, neu string) error {
	s.Fresh()
	old, neu = domain.NormPath(old), domain.NormPath(neu)
	switch {
	case old == "":
		return errors.New(i18n.T("kein Ordner gewählt"))
	case neu == "":
		return errors.New(i18n.T("Name fehlt"))
	case old == neu:
		return nil
	case domain.InFolder(neu, old):
		return errors.New(i18n.T("ein Ordner kann nicht in sich selbst liegen"))
	}
	if err := s.lockData(); err != nil {
		return err
	}
	s.reparent(old, neu, time.Now().UTC())
	s.mu.Unlock()
	return s.save()
}

// DeleteFolder löst einen Ordner auf: Inhalt und Unterordner rücken eine
// Ebene nach oben, Verbindungen werden nicht gelöscht.
func (s *Store) DeleteFolder(path string) error {
	s.Fresh()
	path = domain.NormPath(path)
	if path == "" {
		return errors.New(i18n.T("kein Ordner gewählt"))
	}
	if err := s.lockData(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if i := s.folderIdx(path); i >= 0 {
		s.tombstone(s.data.Folders[i].ID)
		s.data.Folders = append(s.data.Folders[:i], s.data.Folders[i+1:]...)
	}
	parent := domain.PathParent(path)
	for i := range s.data.Connections {
		c := &s.data.Connections[i]
		if c.Group == path {
			c.Group, c.UpdatedAt = parent, now
			c.Order = 0
		}
	}
	// Unterordner: „A / B / C“ → „A / C“ (samt Inhalt)
	if n := domain.BuildTree(s.data.Connections, s.data.Folders).Find(path); n != nil {
		for _, sub := range n.Folders {
			s.reparent(sub.Path, domain.PathJoin(parent, sub.Name), now)
		}
	}
	s.mu.Unlock()
	return s.save()
}

// reparent verschiebt old → neu (Aufrufer hält s.mu).
func (s *Store) reparent(old, neu string, now time.Time) {
	if p := domain.PathParent(neu); p != "" {
		s.ensureFolder(p, now)
	}
	keep := s.data.Folders[:0]
	have := map[string]bool{}
	for _, f := range s.data.Folders {
		if !domain.InFolder(f.Path, old) {
			have[f.Path] = true
		}
	}
	for _, f := range s.data.Folders {
		if np, ok := domain.Reparent(f.Path, old, neu); ok {
			if have[np] { // Ziel gibt es schon: zusammenlegen
				s.tombstone(f.ID)
				continue
			}
			have[np] = true
			f.Path, f.UpdatedAt = np, now
		}
		keep = append(keep, f)
	}
	s.data.Folders = keep
	for i := range s.data.Connections {
		c := &s.data.Connections[i]
		if np, ok := domain.Reparent(c.Group, old, neu); ok {
			c.Group, c.UpdatedAt = np, now
		}
	}
}

// SetCollapsed klappt Ordner ein oder aus (paths leer = alle).
func (s *Store) SetCollapsed(collapsed bool, paths ...string) error {
	s.Fresh()
	if err := s.lockData(); err != nil {
		return err
	}
	if len(paths) == 0 {
		paths = domain.BuildTree(s.data.Connections, s.data.Folders).AllPaths()
	}
	now := time.Now().UTC()
	changed := false
	for _, p := range paths {
		i := s.folderIdx(p)
		if i < 0 {
			if !collapsed {
				continue // nie zugeklappt gewesen
			}
			i = s.ensureFolder(p, now)
		}
		if s.data.Folders[i].Collapsed != collapsed {
			s.data.Folders[i].Collapsed, s.data.Folders[i].UpdatedAt = collapsed, now
			changed = true
		}
	}
	s.mu.Unlock()
	if !changed {
		return nil
	}
	return s.save()
}

// OrderFolders setzt die Reihenfolge der Ordner paths (Geschwister).
func (s *Store) OrderFolders(paths []string) error {
	s.Fresh()
	if err := s.lockData(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for n, p := range paths {
		i := s.ensureFolder(p, now)
		if o := float64(n + 1); s.data.Folders[i].Order != o {
			s.data.Folders[i].Order, s.data.Folders[i].UpdatedAt = o, now
		}
	}
	s.mu.Unlock()
	return s.save()
}

// PlaceConns legt die Verbindungen ids in dieser Reihenfolge in den Ordner
// group (alle übrigen dort behalten ihren Platz dahinter nicht: ids sollte
// den ganzen Ordnerinhalt enthalten, wenn umsortiert wird).
func (s *Store) PlaceConns(group string, ids []string) error {
	s.Fresh()
	group = domain.NormPath(group)
	if err := s.lockData(); err != nil {
		return err
	}
	now := time.Now().UTC()
	pos := map[string]int{}
	for n, id := range ids {
		pos[id] = n + 1
	}
	for i := range s.data.Connections {
		c := &s.data.Connections[i]
		n, ok := pos[c.ID]
		if !ok {
			continue
		}
		if c.Group != group || c.Order != float64(n) {
			c.Group, c.Order, c.UpdatedAt = group, float64(n), now
		}
	}
	s.mu.Unlock()
	return s.save()
}

// MoveConns verschiebt Verbindungen in einen anderen Ordner (hinten an).
func (s *Store) MoveConns(group string, ids ...string) error {
	s.Fresh()
	group = domain.NormPath(group)
	if err := s.lockData(); err != nil {
		return err
	}
	now := time.Now().UTC()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for i := range s.data.Connections {
		c := &s.data.Connections[i]
		if want[c.ID] && c.Group != group {
			c.Order = s.nextOrder(group)
			c.Group, c.UpdatedAt = group, now
		}
	}
	s.mu.Unlock()
	return s.save()
}

// SetFolderRules setzt die KI-Hausregeln eines Ordners (leer = keine).
func (s *Store) SetFolderRules(path, rules string) error {
	s.Fresh()
	path = domain.NormPath(path)
	if path == "" {
		return errors.New(i18n.T("kein Ordner gewählt"))
	}
	if err := s.lockData(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if rules == "" && s.folderIdx(path) < 0 {
		s.mu.Unlock()
		return nil
	}
	i := s.ensureFolder(path, now)
	if s.data.Folders[i].AIRules == rules {
		s.mu.Unlock()
		return nil
	}
	s.data.Folders[i].AIRules, s.data.Folders[i].UpdatedAt = rules, now
	s.mu.Unlock()
	return s.save()
}

// FolderRules: die KI-Hausregeln, die für eine Verbindung im Ordner group
// gelten, vom obersten Ordner abwärts (Unterordner ergänzen).
func (s *Store) FolderRules(group string) []string {
	group = domain.NormPath(group)
	if group == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil
	}
	var out []string
	for p := group; p != ""; p = domain.PathParent(p) {
		if i := s.folderIdx(p); i >= 0 {
			if r := strings.TrimSpace(s.data.Folders[i].AIRules); r != "" {
				out = append([]string{p + ": " + r}, out...)
			}
		}
	}
	return out
}
