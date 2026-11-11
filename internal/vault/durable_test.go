package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type doc struct {
	Text string `json:"text"`
}

func newVault(t *testing.T) (*Vault, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "store.vault")
	v, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Create([]byte("geheim-geheim"), false); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(doc{"eins"}); err != nil {
		t.Fatal(err)
	}
	return v, p
}

// Speichern: Rechte 0600, keine temporären Reste, Inhalt vollständig.
func TestSaveDurable(t *testing.T) {
	v, p := newVault(t)
	for i := 0; i < 3; i++ {
		if err := v.Save(doc{"zwei"}); err != nil {
			t.Fatal(err)
		}
	}
	if fi, err := os.Stat(p); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("Rechte %v", fi.Mode().Perm())
	}
	es, _ := os.ReadDir(filepath.Dir(p))
	if len(es) != 1 {
		t.Fatalf("Reste: %v", es)
	}
	var d doc
	if err := v.Load(&d); err != nil || d.Text != "zwei" {
		t.Fatalf("%+v %v", d, err)
	}
}

// Scheitert das Schreiben, bleibt die alte Datei unversehrt.
func TestSaveFailureKeepsOldFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("braucht einen schreibgeschützten Ordner")
	}
	v, p := newVault(t)
	dir := filepath.Dir(p)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if err := v.Save(doc{"zwei"}); err == nil {
		t.Fatal("Schreiben in schreibgeschützten Ordner gelang")
	}
	var d doc
	if err := v.Load(&d); err != nil || d.Text != "eins" {
		t.Fatalf("alte Datei beschädigt: %+v %v", d, err)
	}
}

// ChangedOnDisk meldet nur fremde Änderungen, nie das eigene Speichern.
func TestChangedOnDisk(t *testing.T) {
	v, p := newVault(t)
	if v.ChangedOnDisk() {
		t.Fatal("eigenes Speichern gilt als fremde Änderung")
	}
	other, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.UnlockPassword([]byte("geheim-geheim")); err != nil {
		t.Fatal(err)
	}
	if other.ChangedOnDisk() {
		t.Fatal("vor dem ersten Laden gibt es keinen Vergleichsstand")
	}
	var d doc
	if err := other.Load(&d); err != nil {
		t.Fatal(err)
	}
	if err := other.Save(doc{"vom anderen"}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(3 * time.Second) // auch bei grober Zeitauflösung erkennbar
	_ = os.Chtimes(p, at, at)
	if !v.ChangedOnDisk() {
		t.Fatal("fremde Änderung nicht bemerkt")
	}
	if err := v.Load(&d); err != nil || d.Text != "vom anderen" {
		t.Fatalf("%+v %v", d, err)
	}
	if v.ChangedOnDisk() {
		t.Fatal("nach dem Laden noch als geändert gemeldet")
	}
	v.Lock()
	if v.ChangedOnDisk() {
		t.Fatal("gesperrt: nichts zu vergleichen")
	}
}
