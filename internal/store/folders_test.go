package store

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	return s
}

func add(t *testing.T, s *Store, alias, group string) domain.Connection {
	t.Helper()
	c, err := s.UpsertConnection(domain.Connection{Alias: alias, Host: "h", Username: "u", Auth: domain.AuthAgent, Group: group})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func groups(s *Store) map[string]string {
	out := map[string]string{}
	for _, c := range s.Connections() {
		out[c.Alias] = c.Group
	}
	return out
}

func TestFolderLifecycle(t *testing.T) {
	s := openTest(t)
	add(t, s, "web1", "Prod/Web")
	add(t, s, "db1", "Prod")
	add(t, s, "pi", "")
	if g := groups(s); g["web1"] != "Prod / Web" {
		t.Fatalf("nicht normalisiert: %q", g["web1"])
	}
	if _, err := s.CreateFolder("Prod"); err == nil {
		t.Fatal("doppelter Ordner erlaubt")
	}
	if _, err := s.CreateFolder("Leer / Unter"); err != nil {
		t.Fatal(err)
	}
	if s.Tree().Find("Leer / Unter") == nil {
		t.Fatal("leerer Ordner fehlt")
	}
	// Umbenennen nimmt Unterordner und Verbindungen mit
	if err := s.MoveFolder("Prod", "Kunde / Prod"); err != nil {
		t.Fatal(err)
	}
	if g := groups(s); g["web1"] != "Kunde / Prod / Web" || g["db1"] != "Kunde / Prod" {
		t.Fatalf("nach Umbenennen: %v", g)
	}
	if err := s.MoveFolder("Kunde", "Kunde / Prod / x"); err == nil {
		t.Fatal("Ordner in sich selbst verschoben")
	}
	// Auflösen: Inhalt rückt eine Ebene hoch, nichts wird gelöscht
	if err := s.DeleteFolder("Kunde / Prod"); err != nil {
		t.Fatal(err)
	}
	if g := groups(s); g["web1"] != "Kunde / Web" || g["db1"] != "Kunde" || len(g) != 3 {
		t.Fatalf("nach Auflösen: %v", g)
	}
	if err := s.SetCollapsed(true, "Kunde"); err != nil || !s.Tree().Find("Kunde").Collapsed {
		t.Fatal("zuklappen")
	}
	if err := s.SetCollapsed(false); err != nil || s.Tree().Find("Kunde").Collapsed {
		t.Fatal("alle aufklappen")
	}
}

func TestReorder(t *testing.T) {
	s := openTest(t)
	a := add(t, s, "a", "G")
	b := add(t, s, "b", "G")
	c := add(t, s, "c", "G")
	if err := s.PlaceConns("G", []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	n := s.Tree().Find("G")
	if n.Conns[0].Alias != "c" || n.Conns[1].Alias != "a" || n.Conns[2].Alias != "b" {
		t.Fatalf("Reihenfolge: %v", n.Conns)
	}
	// neue Verbindung im umsortierten Ordner kommt ans Ende
	add(t, s, "0-neu", "G")
	if n := s.Tree().Find("G"); n.Conns[3].Alias != "0-neu" {
		t.Fatalf("neu nicht hinten: %v", n.Conns[3].Alias)
	}
	if err := s.MoveConns("H", a.ID); err != nil || groups(s)["a"] != "H" {
		t.Fatal("verschieben")
	}
	if err := s.OrderFolders([]string{"H", "G"}); err != nil {
		t.Fatal(err)
	}
	if tr := s.Tree(); tr.Folders[0].Path != "H" {
		t.Fatalf("Ordner-Reihenfolge: %s", tr.Folders[0].Path)
	}
	d, _, _ := s.Snapshot()
	if len(d.Folders) != 2 {
		t.Fatalf("Ordner-Einträge im Snapshot: %d", len(d.Folders))
	}
}
