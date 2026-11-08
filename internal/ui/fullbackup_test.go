package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/db9979/sshtermy/internal/fullbackup"
)

func TestFullBackupForms(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal = nil
	out := filepath.Join(t.TempDir(), "sich")
	open := func(label string) {
		t.Helper()
		a.openPalette()
		a.palEnter("settings/io")
		it, ok := hasLabel(a.paletteItems(), label)
		if !ok {
			t.Fatalf("%q fehlt: %v", label, labels(a.paletteItems()))
		}
		a.pal = nil
		it.run()
		if a.scr != scrBackupForm || a.form == nil {
			t.Fatal("Formular nicht offen")
		}
	}
	set := func(k, v string) { a.form.get(k).in.SetValue(v) }

	open("Gesamtsicherung speichern")
	set("file", out)
	set("pw", "falsch!!")
	if _, err := a.saveBackupForm(); err == nil {
		t.Fatal("falsches Passwort angenommen")
	}
	set("pw", "correct horse")
	if _, err := a.saveBackupForm(); err != nil {
		t.Fatal(err)
	}
	file := out + fullbackup.Ext
	if st, err := os.Stat(file); err != nil || st.Size() == 0 {
		t.Fatalf("Sicherung fehlt: %v", err)
	}

	open("Gesamtsicherung einspielen")
	set("file", file)
	set("pw", "falsch!!")
	if _, err := a.saveBackupForm(); err == nil {
		t.Fatal("falsches Passwort beim Einspielen angenommen")
	}
	set("pw", "correct horse")
	if _, err := a.saveBackupForm(); err != nil {
		t.Fatal(err)
	}
	if a.modal == nil || a.modal.onYes == nil {
		t.Fatal("keine Rückfrage vor dem Ersetzen")
	}
	dir := a.st.Dir
	a.modal.onYes()
	if !a.Restart() {
		t.Fatal("kein Neustart nach dem Einspielen")
	}
	old, err := fullbackup.ApplyPending(dir)
	if err != nil || old == "" {
		t.Fatalf("ApplyPending: %q %v", old, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "store.vault")); err != nil {
		t.Fatal("Store fehlt nach dem Einspielen")
	}
}
