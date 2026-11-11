package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/version"
)

// fakeInstall: Programmdatei in einem eigenen Ordner, Version und Schlüssel
// wie in einem Release-Build. Alles wird nach dem Test zurückgesetzt.
func fakeInstall(t *testing.T, ver string) (exe string) {
	t.Helper()
	exe = filepath.Join(t.TempDir(), "sshtermy")
	if err := os.WriteFile(exe, []byte("alt"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe, oldVer, oldKey, oldTest, oldRestart := exePath, version.Version, version.UpdatePubKey, selfTest, restart
	exePath, version.Version = exe, ver
	selfTest = func(string, string) error { return nil }
	t.Cleanup(func() {
		exePath, version.Version, version.UpdatePubKey, selfTest, restart = oldExe, oldVer, oldKey, oldTest, oldRestart
	})
	return exe
}

// fakeRelease: Server mit Programmdatei, Prüfsummen und Signatur.
func fakeRelease(t *testing.T, ver string, bin []byte, sums string, sign bool) *Release {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	if sums == "" {
		h := sha256.Sum256(bin)
		sums = hex.EncodeToString(h[:]) + "  " + AssetName() + "\n"
	}
	files := map[string][]byte{"/bin": bin, "/sums": []byte(sums)}
	rel := &Release{Version: ver, assets: map[string][]string{}}
	if sign {
		version.UpdatePubKey = hex.EncodeToString(pub)
		files["/sig"] = []byte(hex.EncodeToString(ed25519.Sign(priv, []byte(sums))))
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := files[r.URL.Path]; ok {
			_, _ = w.Write(b)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)
	u, _ := url.Parse(ts.URL)
	allowedHosts[u.Hostname()] = true
	oldClient := httpClient
	httpClient = func() *http.Client { return ts.Client() }
	t.Cleanup(func() { delete(allowedHosts, u.Hostname()); httpClient = oldClient })
	rel.assets[AssetName()] = []string{ts.URL + "/bin"}
	rel.assets["checksums.txt"] = []string{ts.URL + "/sums"}
	if sign {
		rel.assets["checksums.txt.sig"] = []string{ts.URL + "/sig"}
	}
	return rel
}

func content(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyReplacesAndKeepsOld(t *testing.T) {
	exe := fakeInstall(t, "1.0.1")
	rel := fakeRelease(t, "1.0.2", []byte("neu"), "", true)
	if err := Apply(context.Background(), rel); err != nil {
		t.Fatal(err)
	}
	if content(t, exe) != "neu" || content(t, exe+".old") != "alt" {
		t.Fatal("Programmdatei nicht getauscht oder alte fehlt")
	}
	var p pendingInfo
	if !readJSON(pendingPath(exe), &p) || p.Version != "1.0.2" || p.From != "1.0.1" || p.Old != exe+".old" {
		t.Fatalf("Merker: %+v", p)
	}
	if _, err := os.Stat(newPath(exe)); err == nil {
		t.Fatal("temporäre Datei bleibt liegen")
	}
}

func TestApplyRejects(t *testing.T) {
	bad := strings.Repeat("0", 64) + "  " + AssetName() + "\n"
	cases := []struct {
		name string
		prep func(t *testing.T) *Release
		want string
	}{
		{"falsche Prüfsumme", func(t *testing.T) *Release { return fakeRelease(t, "1.0.2", []byte("neu"), bad, true) }, "Prüfsumme"},
		{"Signatur fehlt", func(t *testing.T) *Release {
			r := fakeRelease(t, "1.0.2", []byte("neu"), "", true)
			delete(r.assets, "checksums.txt.sig")
			return r
		}, "Signatur"},
		{"fremder Schlüssel", func(t *testing.T) *Release {
			r := fakeRelease(t, "1.0.2", []byte("neu"), "", true)
			pub, _, _ := ed25519.GenerateKey(rand.Reader)
			version.UpdatePubKey = hex.EncodeToString(pub)
			return r
		}, "Signatur"},
		{"Datei fehlt im Release", func(t *testing.T) *Release {
			r := fakeRelease(t, "1.0.2", []byte("neu"), "", true)
			delete(r.assets, AssetName())
			return r
		}, "checksums.txt"},
		{"Selbsttest scheitert", func(t *testing.T) *Release {
			selfTest = func(string, string) error { return errors.New("startet nicht") }
			return fakeRelease(t, "1.0.2", []byte("neu"), "", true)
		}, "startet nicht"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exe := fakeInstall(t, "1.0.1")
			err := Apply(context.Background(), c.prep(t))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Fehler: %v", err)
			}
			if content(t, exe) != "alt" {
				t.Fatal("Programmdatei trotz Fehler ersetzt")
			}
			es, _ := os.ReadDir(filepath.Dir(exe))
			if len(es) != 1 {
				t.Fatalf("Reste im Ordner: %v", es)
			}
		})
	}
}

// Der echte Selbsttest: startet die Datei und vergleicht die Version.
func TestSelfTestRunsBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skript braucht sh")
	}
	dir := t.TempDir()
	script := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ok := script("ok", `echo "sshTermy v1.0.2 (abc, heute) linux/amd64"`)
	if err := selfTest(ok, "1.0.2"); err != nil {
		t.Fatal(err)
	}
	if err := selfTest(ok, "1.0.3"); err == nil || !strings.Contains(err.Error(), "1.0.2") {
		t.Fatalf("falsche Version angenommen: %v", err)
	}
	if err := selfTest(script("kaputt", "exit 3"), "1.0.2"); err == nil {
		t.Fatal("kaputte Datei angenommen")
	}
	if err := selfTest(filepath.Join(dir, "fehlt"), "1.0.2"); err == nil {
		t.Fatal("fehlende Datei angenommen")
	}
}

func TestGuardRollsBackAfterFailedStarts(t *testing.T) {
	exe := fakeInstall(t, "1.0.1")
	if err := replaceSelf([]byte("neu"), "1.0.2"); err != nil {
		t.Fatal(err)
	}
	version.Version = "1.0.2" // ab jetzt läuft die neue Version
	restarted := 0
	restart = func() error { restarted++; return nil }

	Guard() // 1. Start
	Guard() // 2. Start, beide ohne Confirm
	if content(t, exe) != "neu" || restarted != 0 {
		t.Fatal("zu früh zurückgenommen")
	}
	if content(t, exe+".old") != "alt" {
		t.Fatal("alte Datei vor der Bestätigung gelöscht")
	}
	Guard() // 3. Start: zurück zur alten
	if content(t, exe) != "alt" || restarted != 1 {
		t.Fatalf("nicht zurückgenommen (Neustarts: %d)", restarted)
	}
	if _, err := os.Stat(pendingPath(exe)); err == nil {
		t.Fatal("Merker bleibt")
	}
	version.Version = "1.0.1"
	if skipped() != "1.0.2" {
		t.Fatal("zurückgenommene Version nicht gemerkt")
	}
	if TakeRollbackNote() != "1.0.2" || TakeRollbackNote() != "" {
		t.Fatal("Hinweis muss genau einmal kommen")
	}
	Guard() // alte Version startet: räumt die kaputte weg
	es, _ := os.ReadDir(filepath.Dir(exe))
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".old") {
			t.Fatalf("Rest: %s", e.Name())
		}
	}
}

func TestConfirmKeepsNewVersion(t *testing.T) {
	exe := fakeInstall(t, "1.0.1")
	if err := replaceSelf([]byte("neu"), "1.0.2"); err != nil {
		t.Fatal(err)
	}
	version.Version = "1.0.2"
	restart = func() error { t.Fatal("Neustart trotz Bestätigung"); return nil }
	Guard()
	Confirm()
	for i := 0; i < 5; i++ {
		Guard()
	}
	if content(t, exe) != "neu" {
		t.Fatal("bestätigte Version zurückgenommen")
	}
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Fatal("alte Datei bleibt nach der Bestätigung liegen")
	}
}

// Ein Merker für eine andere Version (von Hand ersetzt) löst nichts aus.
func TestGuardIgnoresForeignMarker(t *testing.T) {
	exe := fakeInstall(t, "1.0.5")
	writeJSON(pendingPath(exe), pendingInfo{Version: "1.0.2", From: "1.0.1", Old: exe + ".old", Starts: 9})
	restart = func() error { t.Fatal("Neustart"); return nil }
	Guard()
	if content(t, exe) != "alt" {
		t.Fatal("Programmdatei angefasst")
	}
	if _, err := os.Stat(pendingPath(exe)); err == nil {
		t.Fatal("fremder Merker bleibt")
	}
}

func TestReportedVersion(t *testing.T) {
	for in, want := range map[string]string{
		"sshTermy v1.0.42 (abc, 2026) linux/amd64\nx": "1.0.42",
		"sshTermy vdev (none, unknown)":               "?",
		"":                                            "?",
	} {
		if got := reportedVersion(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
