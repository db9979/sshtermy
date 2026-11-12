package webwatch

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// zusehen.wasm ist gebaut und eingecheckt, damit der Build von sshTermy ohne
// diesen Schritt geht. Dieser Test merkt sich einen Fingerabdruck der
// Quellen. Ändern sie sich, schlägt er fehl, bis `make zusehen` die Datei neu
// gebaut hat.
const wasmStamp = "wasm.sha256"

func wasmSources(t *testing.T) []string {
	var out []string
	for _, pat := range []string{"../sharewire/*.go", "../../cmd/zusehen-wasm/*.go"} {
		m, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range m {
			if !strings.HasSuffix(f, "_test.go") {
				out = append(out, f)
			}
		}
	}
	sort.Strings(out)
	return out
}

func wasmFingerprint(t *testing.T) string {
	h := sha256.New()
	for _, f := range wasmSources(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(h, "%s\n", filepath.ToSlash(f))
		h.Write([]byte(strings.ReplaceAll(string(b), "\r", "")))
	}
	// Bibliotheken, die im WebAssembly stecken
	mod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.ReplaceAll(string(mod), "\r", ""), "\n") {
		for _, p := range []string{"github.com/coder/websocket ", "filippo.io/cpace ", "golang.org/x/crypto ", "github.com/gtank/ristretto255 "} {
			if strings.HasPrefix(strings.TrimSpace(l), p) {
				fmt.Fprintf(h, "%s\n", strings.TrimSpace(l))
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestWasmCurrent(t *testing.T) {
	got := wasmFingerprint(t)
	if os.Getenv("ZUSEHEN_STAMP") != "" {
		if err := os.WriteFile(wasmStamp, []byte(got+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(wasmStamp)
	if err != nil {
		t.Fatalf("Fingerabdruck fehlt: %v (make zusehen ausführen)", err)
	}
	if strings.TrimSpace(string(want)) != got {
		t.Fatal("internal/sharewire oder cmd/zusehen-wasm haben sich geändert: `make zusehen` baut zusehen.wasm.gz und den Fingerabdruck neu")
	}
}

func TestServe(t *testing.T) {
	m := http.NewServeMux()
	Register(m)
	srv := httptest.NewServer(m)
	defer srv.Close()
	cl := &http.Client{Transport: &http.Transport{DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(p string, gz bool) *http.Response {
		req, _ := http.NewRequest("GET", srv.URL+p, nil)
		if gz {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	r := get("/zusehen", false)
	if r.StatusCode != http.StatusMovedPermanently || r.Header.Get("Location") != "zusehen/" {
		t.Fatalf("Umleitung: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
	r = get("/zusehen/", false)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), "app.js") || !strings.Contains(r.Header.Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatalf("Seite: %d %s", r.StatusCode, r.Header)
	}
	for _, f := range []string{"app.js", "app.css", "xterm.js", "xterm.css", "wasm_exec.js"} {
		if r := get("/zusehen/"+f, false); r.StatusCode != 200 {
			t.Fatalf("%s: %d", f, r.StatusCode)
		}
	}
	r = get("/zusehen/zusehen.wasm", true)
	if r.StatusCode != 200 || r.Header.Get("Content-Encoding") != "gzip" || r.Header.Get("Content-Type") != "application/wasm" {
		t.Fatalf("wasm gz: %d %s", r.StatusCode, r.Header)
	}
	r = get("/zusehen/zusehen.wasm", false)
	b, _ = io.ReadAll(r.Body)
	if r.StatusCode != 200 || len(b) < 4 || string(b[:4]) != "\x00asm" {
		t.Fatalf("wasm: %d %d", r.StatusCode, len(b))
	}
	for _, bad := range []string{"/zusehen/zusehen.wasm.gz", "/zusehen/../webwatch.go", "/zusehen/LICENSE", "/zusehen/.x.js"} {
		if r := get(bad, false); r.StatusCode == 200 {
			t.Fatalf("%s sollte nicht gehen", bad)
		}
	}
	if l := Link("https://10.0.0.5:4711", "7-taube-kompass-ofen"); l != "https://10.0.0.5:4711/zusehen/#7-taube-kompass-ofen" {
		t.Fatal(l)
	}
}
