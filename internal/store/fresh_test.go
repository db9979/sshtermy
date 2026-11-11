package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/vault"
)

func openAt(t *testing.T, dir string, create bool) *Store {
	t.Helper()
	v, err := vault.Open(filepath.Join(dir, "store.vault"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{V: v, Dir: dir}
	if create {
		if err := s.Init([]byte("geheim-geheim"), false); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if err := v.UnlockPassword([]byte("geheim-geheim")); err != nil {
		t.Fatal(err)
	}
	if err := s.AfterUnlock(); err != nil {
		t.Fatal(err)
	}
	return s
}

// later: die Datei muss für die Erkennung einen anderen Zeitstempel tragen,
// auch auf Dateisystemen mit grober Auflösung.
func later(t *testing.T, dir string, d time.Duration) {
	t.Helper()
	at := time.Now().Add(d)
	if err := os.Chtimes(filepath.Join(dir, "store.vault"), at, at); err != nil {
		t.Fatal(err)
	}
}

// Zwei sshTermy auf demselben Tresor: keine Änderung geht verloren.
func TestTwoInstancesKeepBothChanges(t *testing.T) {
	dir := t.TempDir()
	a := openAt(t, dir, true)
	b := openAt(t, dir, false)

	if _, err := a.UpsertConnection(domain.Connection{Alias: "von-a", Host: "a.example", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	later(t, dir, 2*time.Second)
	if _, err := b.UpsertConnection(domain.Connection{Alias: "von-b", Host: "b.example", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	later(t, dir, 4*time.Second)
	reloaded := 0
	a.OnReload = func() { reloaded++ }
	if !a.Fresh() || reloaded != 1 {
		t.Fatalf("a hat die Änderung von b nicht bemerkt (reloaded=%d)", reloaded)
	}
	if a.Fresh() {
		t.Fatal("ohne neue Änderung darf nicht neu geladen werden")
	}
	for _, s := range []*Store{a, b, openAt(t, dir, false)} {
		got := map[string]bool{}
		for _, c := range s.Connections() {
			got[c.Alias] = true
		}
		if !got["von-a"] || !got["von-b"] {
			t.Fatalf("Verbindung verloren: %v", got)
		}
	}
}

// Ändert das eine sshTermy das Master-Passwort, dreht das andere es beim
// nächsten Speichern nicht zurück.
func TestTwoInstancesKeepPasswordChange(t *testing.T) {
	dir := t.TempDir()
	a := openAt(t, dir, true)
	b := openAt(t, dir, false)
	if err := a.ChangeMaster([]byte("geheim-geheim"), []byte("neues-passwort-1"), false); err != nil {
		t.Fatal(err)
	}
	later(t, dir, 2*time.Second)
	if _, err := b.UpsertConnection(domain.Connection{Alias: "x", Host: "x.example", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(filepath.Join(dir, "store.vault"))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.UnlockPassword([]byte("neues-passwort-1")); err != nil {
		t.Fatalf("neues Passwort gilt nicht mehr: %v", err)
	}
	if v.UnlockPassword([]byte("geheim-geheim")) == nil {
		t.Fatal("altes Passwort gilt wieder")
	}
}

// Speichern hinterlässt keine temporären Dateien.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	a := openAt(t, dir, true)
	for i := 0; i < 3; i++ {
		if err := a.UpdateSettings(func(s *domain.Settings) { s.TourDone = !s.TourDone }); err != nil {
			t.Fatal(err)
		}
	}
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("Rest: %s", e.Name())
		}
	}
}

// Scheitert das Schreiben, erfährt es der Haken, auch wenn der Aufrufer den
// Fehler verwirft.
func TestSaveErrorReported(t *testing.T) {
	if os.Getuid() == 0 || os.PathSeparator == '\\' {
		t.Skip("braucht einen schreibgeschützten Ordner")
	}
	dir := t.TempDir()
	a := openAt(t, dir, true)
	var got error
	a.OnSaveError = func(err error) { got = err }
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	_ = a.UpdateSettings(func(s *domain.Settings) { s.TourDone = true })
	if got == nil {
		t.Fatal("Fehler beim Speichern nicht gemeldet")
	}
}
