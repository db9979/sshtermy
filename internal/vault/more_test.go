package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func freshVault(t *testing.T, pw string) (*Vault, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "store.json")
	v, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if v.Exists() {
		t.Fatal("neuer Store gilt als vorhanden")
	}
	if err := v.Create([]byte(pw), false); err != nil {
		t.Fatal(err)
	}
	if !v.Exists() {
		t.Fatal("Store nach Create nicht vorhanden")
	}
	return v, p
}

func TestCreateRejectsShortPassword(t *testing.T) {
	v, _ := Open(filepath.Join(t.TempDir(), "s.json"))
	if err := v.Create([]byte("kurz"), false); err == nil {
		t.Fatal("zu kurzes Master-Passwort angenommen")
	}
}

// Passwort wechseln: altes geht nicht mehr, neues schon, Daten bleiben.
func TestChangePassword(t *testing.T) {
	v, p := freshVault(t, "altes passwort")
	if err := v.Save(payload{"geheim"}); err != nil {
		t.Fatal(err)
	}
	if err := v.ChangePassword([]byte("falsches pw"), []byte("neues passwort")); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("falsches altes Passwort: %v", err)
	}
	if err := v.ChangePassword([]byte("altes passwort"), []byte("kurz")); err == nil {
		t.Fatal("zu kurzes neues Passwort angenommen")
	}
	if err := v.ChangePassword([]byte("altes passwort"), []byte("neues passwort")); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(payload{"geheim"}); err != nil {
		t.Fatal(err)
	}
	v2, _ := Open(p)
	if err := v2.UnlockPassword([]byte("altes passwort")); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("altes Passwort geht noch: %v", err)
	}
	if err := v2.CheckPassword([]byte("neues passwort")); err != nil {
		t.Fatal(err)
	}
	if v2.Unlocked() {
		t.Fatal("CheckPassword darf nicht entsperren")
	}
	if err := v2.UnlockPassword([]byte("neues passwort")); err != nil {
		t.Fatal(err)
	}
	var out payload
	if err := v2.Load(&out); err != nil || out.Secret != "geheim" {
		t.Fatalf("Daten nach Passwortwechsel: %v %q", err, out.Secret)
	}
}

func TestLockedRefuses(t *testing.T) {
	v, _ := freshVault(t, "correct horse")
	_ = v.Save(payload{"x"})
	v.Lock()
	if v.Unlocked() {
		t.Fatal("nach Lock noch offen")
	}
	var out payload
	if err := v.Load(&out); !errors.Is(err, ErrLocked) {
		t.Fatalf("Load gesperrt: %v", err)
	}
	if _, err := v.SealBlob([]byte("a"), nil); !errors.Is(err, ErrLocked) {
		t.Fatalf("SealBlob gesperrt: %v", err)
	}
	if _, err := v.OpenBlob(make([]byte, 40), nil); !errors.Is(err, ErrLocked) {
		t.Fatalf("OpenBlob gesperrt: %v", err)
	}
	if _, err := v.DeriveKey("x"); !errors.Is(err, ErrLocked) {
		t.Fatalf("DeriveKey gesperrt: %v", err)
	}
	if err := v.SetDPAPI(false); !errors.Is(err, ErrLocked) {
		t.Fatalf("SetDPAPI gesperrt: %v", err)
	}
}

// Blobs (z. B. Switch-Sicherungen): an den Ablageort gebunden.
func TestBlobBoundToPlace(t *testing.T) {
	v, _ := freshVault(t, "correct horse")
	blob, err := v.SealBlob([]byte("running-config"), []byte("sw1/2026-11-20"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.OpenBlob(blob, []byte("sw1/2026-11-20"))
	if err != nil || string(got) != "running-config" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := v.OpenBlob(blob, []byte("sw2/2026-11-20")); err == nil {
		t.Fatal("vertauschte Datei akzeptiert")
	}
	blob[len(blob)-1] ^= 1
	if _, err := v.OpenBlob(blob, []byte("sw1/2026-11-20")); err == nil {
		t.Fatal("veränderter Blob akzeptiert")
	}
	if _, err := v.OpenBlob([]byte{1, 2, 3}, nil); err == nil {
		t.Fatal("zu kurzer Blob akzeptiert")
	}
	k1, _ := v.DeriveKey("logs")
	k2, _ := v.DeriveKey("audit")
	if bytes.Equal(k1, k2) || len(k1) != 32 {
		t.Fatal("abgeleitete Schlüssel nicht getrennt")
	}
}

// Manipulierte KDF-Werte: zu schwach (Downgrade) oder absurd (Speicher-DoS).
func TestKDFCheck(t *testing.T) {
	good, err := DefaultKDF()
	if err != nil {
		t.Fatal(err)
	}
	if err := good.Check(); err != nil {
		t.Fatalf("Vorgabe abgelehnt: %v", err)
	}
	bad := []func(*KDFParams){
		func(p *KDFParams) { p.Alg = "pbkdf2" },
		func(p *KDFParams) { p.Salt = p.Salt[:4] },
		func(p *KDFParams) { p.Time = 0 },
		func(p *KDFParams) { p.Time = 1000 },
		func(p *KDFParams) { p.MemoryKiB = 1024 },
		func(p *KDFParams) { p.MemoryKiB = 64 * 1024 * 1024 },
		func(p *KDFParams) { p.Threads = 0 },
	}
	for i, mod := range bad {
		p := good
		p.Salt = append([]byte(nil), good.Salt...)
		mod(&p)
		if p.Check() == nil {
			t.Errorf("Fall %d: schwache/absurde KDF angenommen", i)
		}
	}
	a, b := good.Derive([]byte("pw")), good.Derive([]byte("pw"))
	if !bytes.Equal(a, b) || bytes.Equal(a, good.Derive([]byte("pW"))) {
		t.Fatal("Derive nicht stabil oder nicht passwortabhängig")
	}
	Wipe(a)
	if !bytes.Equal(a, make([]byte, len(a))) {
		t.Fatal("Wipe hat nicht genullt")
	}
}

func TestGCMHelpers(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	n, ct, err := SealGCM(key, []byte("hallo"), []byte("ort"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := OpenGCM(key, n, ct, []byte("ort")); err != nil || string(got) != "hallo" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := OpenGCM(key, n, ct, []byte("anderer")); err == nil {
		t.Fatal("falsche AAD angenommen")
	}
}

func TestOpenRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"kaputt.json": "{nicht json",
		"fremd.json":  `{"format":"anderes","version":1}`,
		"zu-neu.json": `{"format":"sshtermy-vault","version":99}`,
	}
	for name, body := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(p); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
}

// Ohne Windows gibt es kein DPAPI: Anfragen werden klar abgelehnt, der
// Passwort-Weg bleibt.
func TestDPAPIOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echtes DPAPI: siehe Plattform-Job")
	}
	v, p := freshVault(t, "correct horse")
	if err := v.SetDPAPI(true); !errors.Is(err, ErrDPAPI) {
		t.Fatalf("SetDPAPI ohne Windows: %v", err)
	}
	if err := v.SetDPAPI(false); err != nil {
		t.Fatal(err)
	}
	if v.HasDPAPI() {
		t.Fatal("DPAPI-Wrap ohne Windows")
	}
	_ = v.Save(payload{"x"})
	v2, _ := Open(p)
	if err := v2.UnlockDPAPI(); !errors.Is(err, ErrNoUnlockPath) {
		t.Fatalf("UnlockDPAPI ohne Wrap: %v", err)
	}
}
