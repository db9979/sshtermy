package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// Kleine Änderungen gelten sofort, schreiben aber erst gebündelt.
func TestSaveSoonBundlesWrites(t *testing.T) {
	old := saveDelay
	saveDelay = time.Hour // nur Flush schreibt
	defer func() { saveDelay = old }()

	dir := t.TempDir()
	s := openAt(t, dir, true)
	c, err := s.UpsertConnection(domain.Connection{Alias: "web", Host: "web.example", Username: "u"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "store.vault")
	before, _ := os.ReadFile(file)
	for i := 0; i < 50; i++ {
		s.Touch(c.ID)
	}
	if got, _ := s.Connection(c.ID); got.LastUsed.IsZero() {
		t.Fatal("Touch gilt nicht sofort im Speicher")
	}
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Fatal("Touch hat sofort geschrieben statt zu bündeln")
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(file); string(after) == string(before) {
		t.Fatal("Flush hat nichts geschrieben")
	}
	if got, _ := openAt(t, dir, false).Connection(c.ID); got.LastUsed.IsZero() {
		t.Fatal("LastUsed fehlt nach dem Neuladen")
	}
	if err := s.Flush(); err != nil { // nichts mehr vorgemerkt
		t.Fatal(err)
	}
}

// Sperren schreibt Vorgemerktes, und der Zeitgeber schreibt von selbst.
func TestSaveSoonOnLockAndTimer(t *testing.T) {
	old := saveDelay
	defer func() { saveDelay = old }()

	saveDelay = time.Hour
	dir := t.TempDir()
	s := openAt(t, dir, true)
	c, _ := s.UpsertConnection(domain.Connection{Alias: "web", Host: "web.example", Username: "u"})
	s.Touch(c.ID)
	s.Lock()
	if got, _ := openAt(t, dir, false).Connection(c.ID); got.LastUsed.IsZero() {
		t.Fatal("Lock hat die vorgemerkte Änderung nicht geschrieben")
	}

	saveDelay = 20 * time.Millisecond
	dir = t.TempDir()
	s = openAt(t, dir, true)
	c, _ = s.UpsertConnection(domain.Connection{Alias: "web", Host: "web.example", Username: "u"})
	s.Touch(c.ID)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got, _ := openAt(t, dir, false).Connection(c.ID); !got.LastUsed.IsZero() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Zeitgeber hat nicht gespeichert")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Eine Sammeländerung fasst nur die genannten Verbindungen an.
func TestChangeConnsAndBulkDelete(t *testing.T) {
	s := openAt(t, t.TempDir(), true)
	var ids []string
	for _, n := range []string{"a", "b", "c"} {
		c, err := s.UpsertConnection(domain.Connection{Alias: n, Host: n + ".example", Username: "u"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	n, err := s.ChangeConns(ids[:2], func(c *domain.Connection) bool {
		if c.Alias == "b" {
			return false
		}
		c.Tags = append(c.Tags, "neu")
		return true
	})
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	if a, _ := s.Connection(ids[0]); len(a.Tags) != 1 {
		t.Fatalf("Tags a = %v", a.Tags)
	}
	if c, _ := s.Connection(ids[2]); len(c.Tags) != 0 {
		t.Fatalf("Tags c = %v", c.Tags)
	}
	tr, err := s.DeleteConnectionsTrash(ids[:2])
	if err != nil || len(tr.Conns()) != 2 || len(s.Connections()) != 1 {
		t.Fatalf("gelöscht %d, übrig %d, err %v", len(tr.Conns()), len(s.Connections()), err)
	}
	if err := s.Restore(tr); err != nil || len(s.Connections()) != 3 {
		t.Fatalf("nach Rückgängig %d, err %v", len(s.Connections()), err)
	}
}
