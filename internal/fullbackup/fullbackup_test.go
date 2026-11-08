package fullbackup

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRoundTripAndSwap(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "alt", "sshtermy")
	write(t, filepath.Join(src, "store.vault"), "VAULT")
	write(t, filepath.Join(src, "logs", "a.log.enc"), strings.Repeat("x", 3<<20)) // mehrere Blöcke
	write(t, filepath.Join(src, "known_hosts"), "h ssh-ed25519 AAAA")
	write(t, filepath.Join(src, "crash", "c.txt"), "nicht mitnehmen")
	write(t, filepath.Join(src, "debug.log"), "nicht mitnehmen")

	var buf bytes.Buffer
	m, err := Create(src, []byte("pw"), "1.0.1", &buf)
	if err != nil || m.Files != 3 {
		t.Fatalf("Create: %+v %v", m, err)
	}
	if bytes.Contains(buf.Bytes(), []byte("VAULT")) || bytes.Contains(buf.Bytes(), []byte("ssh-ed25519")) {
		t.Fatal("Inhalt unverschlüsselt")
	}
	file := filepath.Join(root, "s"+Ext)
	write(t, file, buf.String())

	dst := filepath.Join(root, "neu", "sshtermy")
	write(t, filepath.Join(dst, "store.vault"), "ALT")
	if _, _, err := Prepare(dst, file, []byte("falsch"), nil); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("falsches Passwort: %v", err)
	}
	checked := ""
	stage, m2, err := Prepare(dst, file, []byte("pw"), func(vp string) error { checked = vp; return nil })
	if err != nil || m2.Version != "1.0.1" || checked == "" {
		t.Fatalf("Prepare: %v %+v", err, m2)
	}
	if err := MarkPending(dst, stage); err != nil {
		t.Fatal(err)
	}
	old, err := ApplyPending(dst)
	if err != nil || old == "" {
		t.Fatalf("Apply: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "store.vault")); string(b) != "VAULT" {
		t.Fatalf("Store nicht getauscht: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(old, "store.vault")); string(b) != "ALT" {
		t.Fatal("alter Stand weg")
	}
	if _, err := os.Stat(filepath.Join(dst, "crash")); !os.IsNotExist(err) {
		t.Fatal("crash mitgenommen")
	}
	if st, err := os.Stat(filepath.Join(dst, "logs", "a.log.enc")); err != nil || st.Size() != 3<<20 {
		t.Fatal("Protokoll fehlt", err)
	}
	if o, err := ApplyPending(dst); o != "" || err != nil {
		t.Fatal("zweites Apply")
	}
}

func TestTamperAndTruncate(t *testing.T) {
	src := t.TempDir()
	rnd := make([]byte, 2<<20) // zufällig: lässt sich nicht packen, also mehrere Blöcke
	_, _ = rand.Read(rnd)
	write(t, filepath.Join(src, "store.vault"), string(rnd))
	var buf bytes.Buffer
	if _, err := Create(src, []byte("pw"), "x", &buf); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	for name, bad := range map[string][]byte{
		"gekürzt":   b[:len(b)-(1<<20)],
		"verändert": append(append([]byte{}, b[:len(b)-5]...), 0, 0, 0, 0, 0),
		"fremd":     []byte("PK\x03\x04 kein Format"),
	} {
		p := filepath.Join(t.TempDir(), "x"+Ext)
		write(t, p, string(bad))
		if _, _, err := Prepare(t.TempDir(), p, []byte("pw"), nil); err == nil {
			t.Errorf("%s: kein Fehler", name)
		}
	}
}

func TestApplyPendingRejectsForeignDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sshtermy")
	write(t, filepath.Join(dir, "store.vault"), "A")
	write(t, filepath.Join(root, "woanders", "store.vault"), "B")
	if err := MarkPending(dir, filepath.Join(root, "woanders")); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPending(dir); err == nil {
		t.Fatal("fremder Ordner angenommen")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "store.vault")); string(b) != "A" {
		t.Fatal("Store verändert")
	}
}
