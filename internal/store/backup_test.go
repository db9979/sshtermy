package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupEncryptedRoundTrip(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	cfg := "hostname sw\nsnmp-server community \"geheim\"\n"
	b, err := s.SaveBackup("abc123", cfg)
	if err != nil || b.Lines != 2 {
		t.Fatalf("save: %v %+v", err, b)
	}
	raw, _ := os.ReadFile(filepath.Join(s.BackupDir("abc123"), b.ID+".cfg.enc"))
	if len(raw) == 0 || bytes.Contains(raw, []byte("geheim")) {
		t.Fatal("Sicherung liegt im Klartext")
	}
	list := s.Backups("abc123")
	if len(list) != 1 || list[0].ID != b.ID || list[0].Lines != 2 {
		t.Fatalf("list %+v", list)
	}
	got, err := s.LoadBackup("abc123", b.ID)
	if err != nil || got != cfg {
		t.Fatalf("load %v %q", err, got)
	}
	// Datei einer anderen Verbindung unterschieben → abgelehnt (AAD)
	os.MkdirAll(s.BackupDir("other"), 0o700)
	os.WriteFile(filepath.Join(s.BackupDir("other"), b.ID+".cfg.enc"), raw, 0o600)
	if _, err := s.LoadBackup("other", b.ID); err == nil {
		t.Fatal("vertauschte Sicherung akzeptiert")
	}
	if _, err := s.SaveBackup("../x", cfg); err == nil {
		t.Fatal("Pfad aus dem Ordner heraus akzeptiert")
	}
}
