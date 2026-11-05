package vault

import (
	"os"
	"path/filepath"
	"testing"
)

type payload struct{ Secret string }

func TestRoundTripAndWrongPassword(t *testing.T) {
	p := filepath.Join(t.TempDir(), "store.json")
	v, _ := Open(p)
	if err := v.Create([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(payload{"geheim"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if contains(raw, "geheim") {
		t.Fatal("Klartext in der Datei")
	}
	v2, _ := Open(p)
	if err := v2.UnlockPassword([]byte("falsch!!")); err != ErrWrongPassword {
		t.Fatalf("falsches Passwort akzeptiert: %v", err)
	}
	if err := v2.UnlockPassword([]byte("correct horse")); err != nil {
		t.Fatal(err)
	}
	var out payload
	if err := v2.Load(&out); err != nil || out.Secret != "geheim" {
		t.Fatalf("Load: %v %q", err, out.Secret)
	}
}

func TestTamperedHeaderIsRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "store.json")
	v, _ := Open(p)
	_ = v.Create([]byte("correct horse"), false)
	_ = v.Save(payload{"x"})
	// KDF-Parameter im Header herabsetzen (Downgrade-Angriff): Entsperren oder
	// spätestens Load muss scheitern, weil der Header als AAD mitgeprüft wird.
	raw, _ := os.ReadFile(p)
	raw = replace(raw, `"time": 3`, `"time": 1`)
	_ = os.WriteFile(p, raw, 0o600)
	v2, _ := Open(p)
	if err := v2.UnlockPassword([]byte("correct horse")); err == nil {
		if err := v2.Load(&payload{}); err == nil {
			t.Fatal("manipulierter Header wurde akzeptiert")
		}
	}
}

func contains(b []byte, s string) bool { return len(replace(b, s, "")) != len(b) }
func replace(b []byte, old, new string) []byte {
	out := []byte{}
	for i := 0; i < len(b); {
		if i+len(old) <= len(b) && string(b[i:i+len(old)]) == old {
			out = append(out, new...)
			i += len(old)
			continue
		}
		out = append(out, b[i])
		i++
	}
	return out
}

func TestDeriveKey(t *testing.T) {
	v, err := Open(filepath.Join(t.TempDir(), "s.vault"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.DeriveKey("x"); err == nil {
		t.Fatal("gesperrt darf kein Schlüssel kommen")
	}
	if err := v.Create([]byte("pw-1234567890"), false); err != nil {
		t.Fatal(err)
	}
	a, _ := v.DeriveKey("sessionlog")
	b, _ := v.DeriveKey("sessionlog")
	c, _ := v.DeriveKey("other")
	if len(a) != 32 || string(a) != string(b) || string(a) == string(c) {
		t.Fatal("DeriveKey nicht deterministisch oder ohne Domänentrennung")
	}
}
