package store

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestDeleteLeavesTombstonesAndBumpsRev(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	changes := 0
	s.OnChange = func() { changes++ }
	c, err := s.UpsertConnection(domain.Connection{Alias: "a", Host: "h", Username: "u", Auth: domain.AuthAgent})
	if err != nil || c.UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt fehlt: %v", err)
	}
	tun, _ := s.UpsertTunnel(domain.Tunnel{ConnectionID: c.ID, Kind: domain.TunnelDynamic, BindPort: 1080})
	_, rev, _ := s.Snapshot()
	if err := s.DeleteConnection(c.ID); err != nil {
		t.Fatal(err)
	}
	d, rev2, _ := s.Snapshot()
	if d.Deleted[c.ID].IsZero() || d.Deleted[tun.ID].IsZero() {
		t.Fatal("Grabsteine fehlen")
	}
	if rev2 == rev || changes != 3 {
		t.Fatalf("Änderungen nicht gezählt: %d", changes)
	}
	// Sync-Stand übernehmen löst keinen neuen Abgleich aus
	if changed, err := s.ApplySync(rev2, func(d *domain.Data) {}); err != nil || changed || changes != 3 {
		t.Fatalf("ApplySync: %v %v %d", changed, err, changes)
	}
}

func TestChangeMaster(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeMaster([]byte("falsch!!"), []byte("battery staple"), false); err == nil {
		t.Fatal("falsches altes Passwort angenommen")
	}
	if err := s.ChangeMaster([]byte("correct horse"), []byte("kurz"), false); err == nil {
		t.Fatal("zu kurzes Passwort angenommen")
	}
	if err := s.ChangeMaster([]byte("correct horse"), []byte("battery staple"), false); err != nil {
		t.Fatal(err)
	}
	s2, _ := Open()
	if err := s2.V.UnlockPassword([]byte("correct horse")); err == nil {
		t.Fatal("altes Passwort geht noch")
	}
	if err := s2.V.UnlockPassword([]byte("battery staple")); err != nil {
		t.Fatal(err)
	}
	if err := s2.AfterUnlock(); err != nil {
		t.Fatal(err)
	}
}
