package store

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestTrashRestoresConnectionAndFolder(t *testing.T) {
	s := openTest(t)
	a := add(t, s, "a", "Web / Prod")
	add(t, s, "b", "Web")
	if _, err := s.UpsertTunnel(domain.Tunnel{ConnectionID: a.ID, Kind: domain.TunnelLocal, Name: "db", BindPort: 5432, TargetHost: "127.0.0.1", TargetPort: 5432}); err != nil {
		t.Fatal(err)
	}
	tr, err := s.DeleteConnectionTrash(a.ID)
	if err != nil || len(s.Tunnels(a.ID)) != 0 {
		t.Fatalf("löschen: %v", err)
	}
	if err := s.Restore(tr); err != nil {
		t.Fatal(err)
	}
	if c, ok := s.Connection(a.ID); !ok || c.Group != "Web / Prod" || len(s.Tunnels(a.ID)) != 1 {
		t.Fatalf("nicht wiederhergestellt: %+v %d", c, len(s.Tunnels(a.ID)))
	}
	if _, gone := s.data.Deleted[a.ID]; gone {
		t.Fatal("Löschvermerk bleibt")
	}

	tr, err = s.DeleteFolderTrash("Web")
	if err != nil {
		t.Fatal(err)
	}
	if g := groups(s); g["a"] != "Prod" || g["b"] != "" {
		t.Fatalf("aufgelöst: %v", g)
	}
	if err := s.Restore(tr); err != nil {
		t.Fatal(err)
	}
	if g := groups(s); g["a"] != "Web / Prod" || g["b"] != "Web" {
		t.Fatalf("Ordner nicht zurück: %v", g)
	}
}
