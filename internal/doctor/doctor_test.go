package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, date time.Time) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", date.UTC().Format(http.TimeFormat))
	}))
	t.Cleanup(ts.Close)
	return ts.URL
}

func find(t *testing.T, rs []Result, prefix string) Result {
	t.Helper()
	for _, r := range rs {
		if strings.HasPrefix(r.Name, prefix) {
			return r
		}
	}
	t.Fatalf("keine Zeile %q", prefix)
	return Result{}
}

func TestRunFreshDir(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	rs := Run(context.Background(), Env{Dir: dir, Probe: server(t, now), Now: func() time.Time { return now }})
	if r := find(t, rs, "Datenordner"); r.Status != OK && !(runtime.GOOS != "windows" && r.Status == Warn) {
		t.Fatalf("Datenordner: %+v", r)
	}
	if r := find(t, rs, "Tresor"); r.Status != Info {
		t.Fatalf("Tresor bei erstem Start: %+v", r)
	}
	if r := find(t, rs, "Internet"); r.Status != OK {
		t.Fatalf("Internet: %+v", r)
	}
	if r := find(t, rs, "Absturz"); r.Status != OK {
		t.Fatalf("Absturzberichte: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, ".doctor-probe")); !os.IsNotExist(err) {
		t.Fatal("Probedatei liegen geblieben")
	}
	txt := Format(rs)
	if !strings.Contains(txt, "Version") || !strings.Contains(txt, "✓") {
		t.Fatalf("Format: %s", txt)
	}
}

func TestClockSkew(t *testing.T) {
	now := time.Now()
	rs := Run(context.Background(), Env{Dir: t.TempDir(), Probe: server(t, now.Add(-10*time.Minute)), Now: func() time.Time { return now }})
	r := find(t, rs, "Internet")
	if r.Status != Warn || !strings.Contains(r.Detail, "10m") {
		t.Fatalf("falsche Uhr nicht erkannt: %+v", r)
	}
}

func TestOfflineAndBrokenDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "datei")
	_ = os.WriteFile(dir, []byte("x"), 0o600)
	if r := checkDir(filepath.Join(t.TempDir(), "fehlt")); r.Status != Info {
		t.Fatalf("erster Start: %+v", r)
	}
	rs := Run(context.Background(), Env{Dir: dir, Probe: "http://127.0.0.1:1"})
	if r := find(t, rs, "Datenordner"); r.Status != Fail {
		t.Fatalf("Datei statt Ordner: %+v", r)
	}
	if r := find(t, rs, "Internet"); r.Status != Warn {
		t.Fatalf("offline: %+v", r)
	}
	if Worst(rs) != Fail {
		t.Fatal("Worst")
	}
	if !strings.Contains(Format(rs), "✗") {
		t.Fatal("Fehlerzeile fehlt")
	}
}

func TestCrashesAndExtra(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "crash"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "crash", "sshtermy-crash-1.txt"), []byte("x"), 0o600)
	extra := func(context.Context) Result { return Result{"KI", Fail, "antwortet nicht"} }
	rs := Run(context.Background(), Env{Dir: dir, Probe: server(t, time.Now()), Extra: []Check{extra}})
	if r := find(t, rs, "Absturz"); r.Status != Info || !strings.HasPrefix(r.Detail, "1 ") {
		t.Fatalf("Absturzberichte: %+v", r)
	}
	if rs[len(rs)-1].Name != "KI" {
		t.Fatal("Zusatzprüfung fehlt am Ende")
	}
}

func TestVaultGarbage(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "store.vault"), []byte("{kaputt"), 0o600)
	r := checkVault(dir)
	if r.Status != Fail {
		t.Fatalf("kaputter Tresor: %+v", r)
	}
}
