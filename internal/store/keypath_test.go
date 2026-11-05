package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/syncx"
)

// Nach dem Sync vom Windows-Rechner steht ein C:\-Pfad in der Verbindung.
// Wählt man am Mac einen anderen Key, gilt der nur hier; der synchronisierte
// Pfad (für Windows) bleibt.
func TestForeignKeyPathStaysForOtherComputer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	win := `C:\Users\db9\AppData\Local\NVIDIA Corporation\Sync\config\nvsync.key`
	added, err := s.AddConnections([]domain.Connection{{Alias: "spark", Host: "h", Username: "u", Auth: domain.AuthKeyFile, KeyPath: win}})
	if err != nil {
		t.Fatal(err)
	}
	id := added[0].ID

	// Am Mac im Formular einen lokalen Key eintragen.
	local := filepath.Join(home, "Library", "nvsync.key")
	c, _ := s.Connection(id)
	c.KeyPath = local
	saved, err := s.UpsertConnection(c)
	if err != nil {
		t.Fatal(err)
	}
	if saved.KeyPath != "~/Library/nvsync.key" {
		t.Fatalf("lokal sichtbarer Pfad = %q", saved.KeyPath)
	}
	d, _, _ := s.Snapshot()
	if d.Connections[0].KeyPath != win {
		t.Fatalf("synchronisierter Pfad überschrieben: %q", d.Connections[0].KeyPath)
	}
	p := syncx.FromData(d, nil)
	if p.Connections[0].KeyPath != win {
		t.Fatalf("Sync-Inhalt: %q", p.Connections[0].KeyPath)
	}

	// Andere Änderungen (z. B. Gerätetyp) lassen beides, wie es ist.
	c, _ = s.Connection(id)
	c.Device = domain.DeviceNetwork
	if _, err := s.UpsertConnection(c); err != nil {
		t.Fatal(err)
	}
	d, _, _ = s.Snapshot()
	if d.Connections[0].KeyPath != win || d.LocalKeys[id] != "~/Library/nvsync.key" {
		t.Fatalf("nach Bearbeiten: %q / %q", d.Connections[0].KeyPath, d.LocalKeys[id])
	}

	// Ein eingehender Sync lässt den lokalen Pfad stehen.
	if _, err := s.ApplySync(0, func(d *domain.Data) { syncx.FromData(d, nil).ApplyTo(d) }); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.Connection(id); c.KeyPath != "~/Library/nvsync.key" {
		t.Fatalf("nach Sync: %q", c.KeyPath)
	}
}

// Liegt der Key unter dem eigenen Home, wird er als ~/… gespeichert und
// passt dann auch auf den anderen Rechnern.
func TestKeyPathUnderHomeBecomesPortable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, _ := Open()
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(home, ".ssh", "id_ed25519")
	_ = os.MkdirAll(filepath.Dir(key), 0o700)
	_ = os.WriteFile(key, []byte("x"), 0o600)
	c, err := s.UpsertConnection(domain.Connection{Alias: "a", Host: "h", Username: "u", Auth: domain.AuthKeyFile, KeyPath: key})
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := s.Snapshot()
	if d.Connections[0].KeyPath != "~/.ssh/id_ed25519" || len(d.LocalKeys) != 0 {
		t.Fatalf("gespeichert: %q %v", d.Connections[0].KeyPath, d.LocalKeys)
	}
	// Pfad existiert hier: Änderung gilt für alle Rechner.
	c.KeyPath = "~/.ssh/other"
	if _, err := s.UpsertConnection(c); err != nil {
		t.Fatal(err)
	}
	d, _, _ = s.Snapshot()
	if d.Connections[0].KeyPath != "~/.ssh/other" || len(d.LocalKeys) != 0 {
		t.Fatalf("geändert: %q %v", d.Connections[0].KeyPath, d.LocalKeys)
	}
	if err := s.SetLocalKeyPath(c.ID, filepath.Join(home, "k")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Connection(c.ID); got.KeyPath != "~/k" {
		t.Fatalf("SetLocalKeyPath: %q", got.KeyPath)
	}
	_ = s.DeleteConnection(c.ID)
	if d, _, _ := s.Snapshot(); len(d.LocalKeys) != 0 {
		t.Fatal("lokaler Pfad bleibt nach Löschen")
	}
}
