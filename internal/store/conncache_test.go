package store

import (
	"fmt"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

// Connections() wird zwischengespeichert: jede Änderung muss trotzdem
// sofort sichtbar sein, und Aufrufer dürfen ihre Kopie verändern.
func TestConnectionsCacheFollowsChanges(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	c, _ := s.UpsertConnection(domain.Connection{Alias: "b", Host: "h", Username: "u", Auth: domain.AuthAgent})
	cs := s.Connections()
	cs[0].Alias = "verändert"
	if s.Connections()[0].Alias != "b" {
		t.Fatal("Aufrufer hat den Zwischenspeicher verändert")
	}
	s.UpsertConnection(domain.Connection{Alias: "a", Host: "h2", Username: "u", Auth: domain.AuthAgent})
	if cs := s.Connections(); len(cs) != 2 || cs[0].Alias != "a" {
		t.Fatalf("neue Verbindung fehlt oder falsch sortiert: %v", cs)
	}
	s.Touch(c.ID)
	if got, _ := s.Connection(c.ID); s.Connections()[1].LastUsed != got.LastUsed {
		t.Fatal("Touch nicht sichtbar")
	}
	if _, err := s.ApplySync(0, func(d *domain.Data) { d.Connections[0].Host = "neu" }); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range s.Connections() {
		found = found || x.Host == "neu"
	}
	if !found {
		t.Fatal("Sync-Stand nicht sichtbar")
	}
	s.DeleteConnection(c.ID)
	if len(s.Connections()) != 1 {
		t.Fatal("Löschen nicht sichtbar")
	}
	s.Lock()
	if s.Connections() != nil {
		t.Fatal("nach Sperren noch Verbindungen")
	}
}

func BenchmarkConnections500(b *testing.B) {
	b.Setenv("SSHTERMY_HOME", b.TempDir())
	s, _ := Open()
	s.Init([]byte("correct horse"), false)
	cs := make([]domain.Connection, 500)
	for i := range cs {
		cs[i] = domain.Connection{Alias: fmt.Sprintf("srv-%03d", i), Host: "h", Username: "u", Auth: domain.AuthAgent, Group: fmt.Sprintf("G%d", i%10)}
	}
	s.AddConnections(cs)
	b.ReportAllocs()
	for b.Loop() {
		_ = s.Connections()
	}
}
