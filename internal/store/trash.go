package store

import (
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// Trash hält fest, was ein Löschen entfernt hat, damit die Oberfläche es ein
// paar Sekunden lang rückgängig machen kann (Hinweis „Rückgängig“ statt
// Rückfrage). Wiederhergestellte Einträge bekommen einen frischen
// Änderungszeitpunkt, damit sie im Sync gegen den Löschvermerk gewinnen.
type Trash struct {
	conns     []domain.Connection
	tunnels   []domain.Tunnel
	localKeys map[string]string
	runbooks  []domain.Runbook

	folders     []domain.Folder // Ordner vor dem Auflösen (nil = unverändert)
	groups      map[string]connPos
	foldersSnap bool
}

type connPos struct {
	group string
	order float64
}

// Conns: die gelöschten Verbindungen (für den Hinweis).
func (t *Trash) Conns() []domain.Connection { return t.conns }

// DeleteConnectionTrash löscht wie DeleteConnection und liefert, was zum
// Wiederherstellen nötig ist.
func (s *Store) DeleteConnectionTrash(id string) (*Trash, error) {
	t := &Trash{localKeys: map[string]string{}}
	s.mu.RLock()
	if s.data != nil {
		for _, c := range s.data.Connections {
			if c.ID == id {
				t.conns = append(t.conns, c)
			}
		}
		for _, tu := range s.data.Tunnels {
			if tu.ConnectionID == id {
				t.tunnels = append(t.tunnels, tu)
			}
		}
		if p, ok := s.data.LocalKeys[id]; ok {
			t.localKeys[id] = p
		}
	}
	s.mu.RUnlock()
	return t, s.DeleteConnection(id)
}

// DeleteRunbookTrash löscht ein Runbook und merkt es sich.
func (s *Store) DeleteRunbookTrash(id string) (*Trash, error) {
	t := &Trash{}
	s.mu.RLock()
	if s.data != nil {
		for _, r := range s.data.Runbooks {
			if r.ID == id {
				t.runbooks = append(t.runbooks, r)
			}
		}
	}
	s.mu.RUnlock()
	return t, s.DeleteRunbook(id)
}

// DeleteFolderTrash löst einen Ordner auf und merkt sich Ordner und die
// Plätze aller Verbindungen.
func (s *Store) DeleteFolderTrash(path string) (*Trash, error) {
	t := &Trash{groups: map[string]connPos{}, foldersSnap: true}
	s.mu.RLock()
	if s.data != nil {
		t.folders = append([]domain.Folder(nil), s.data.Folders...)
		for _, c := range s.data.Connections {
			t.groups[c.ID] = connPos{c.Group, c.Order}
		}
	}
	s.mu.RUnlock()
	return t, s.DeleteFolder(path)
}

// Restore macht ein Löschen rückgängig.
func (s *Store) Restore(t *Trash) error {
	s.Fresh()
	if t == nil {
		return nil
	}
	if err := s.lockData(); err != nil {
		return err
	}
	now := time.Now().UTC()
	has := func(id string) bool {
		for _, c := range s.data.Connections {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	for _, c := range t.conns {
		delete(s.data.Deleted, c.ID)
		if !has(c.ID) {
			c.UpdatedAt = now
			s.data.Connections = append(s.data.Connections, c)
		}
	}
	for _, tu := range t.tunnels {
		delete(s.data.Deleted, tu.ID)
		tu.UpdatedAt = now
		s.data.Tunnels = append(s.data.Tunnels, tu)
	}
	if len(t.localKeys) > 0 {
		if s.data.LocalKeys == nil {
			s.data.LocalKeys = map[string]string{}
		}
		for k, v := range t.localKeys {
			s.data.LocalKeys[k] = v
		}
	}
	for _, r := range t.runbooks {
		delete(s.data.Deleted, r.ID)
		r.UpdatedAt = now
		s.data.Runbooks = append(s.data.Runbooks, r)
	}
	if t.foldersSnap {
		old := map[string]bool{}
		for _, f := range t.folders {
			old[f.ID] = true
		}
		// Ordner, die es vorher nicht gab (beim Auflösen zusammengelegt
		// oder neu entstanden), bleiben; die alten kommen zurück.
		keep := append([]domain.Folder(nil), t.folders...)
		for _, f := range s.data.Folders {
			if !old[f.ID] && !folderPathIn(keep, f.Path) {
				keep = append(keep, f)
			}
		}
		for i := range keep {
			if old[keep[i].ID] {
				delete(s.data.Deleted, keep[i].ID)
				keep[i].UpdatedAt = now
			}
		}
		s.data.Folders = keep
		for i := range s.data.Connections {
			c := &s.data.Connections[i]
			if p, ok := t.groups[c.ID]; ok && (c.Group != p.group || c.Order != p.order) {
				c.Group, c.Order, c.UpdatedAt = p.group, p.order, now
			}
		}
	}
	s.mu.Unlock()
	return s.save()
}

func folderPathIn(fs []domain.Folder, path string) bool {
	for _, f := range fs {
		if f.Path == path {
			return true
		}
	}
	return false
}

// DeleteConnectionsTrash löscht mehrere Verbindungen (Mehrfachauswahl im
// Manager) und liefert einen gemeinsamen Papierkorb für „Rückgängig“.
func (s *Store) DeleteConnectionsTrash(ids []string) (*Trash, error) {
	all := &Trash{localKeys: map[string]string{}}
	for _, id := range ids {
		t, err := s.DeleteConnectionTrash(id)
		if err != nil {
			return all, err
		}
		all.conns = append(all.conns, t.conns...)
		all.tunnels = append(all.tunnels, t.tunnels...)
		for k, v := range t.localKeys {
			all.localKeys[k] = v
		}
	}
	return all, nil
}
