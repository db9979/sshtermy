package offlineai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetSum(t *testing.T) {
	h := strings.Repeat("ab", 32)
	for d, want := range map[string]string{"sha256:" + h: h, "SHA256:" + strings.ToUpper(h): h, "": "", "sha1:" + h: "", "sha256:xyz": "", "sha256:" + strings.Repeat("zz", 32): ""} {
		if got := (asset{Digest: d}).Sum(); got != want {
			t.Errorf("%q → %q, want %q", d, got, want)
		}
	}
}

// Ohne passende Prüfsumme bleibt nichts liegen.
func TestDownloadNeedsSum(t *testing.T) {
	body := "llama-server"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer srv.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	if err := download(context.Background(), srv.URL, dest, "", "x", 0, nil); err == nil {
		t.Fatal("ohne Prüfsumme geladen")
	}
	if err := download(context.Background(), srv.URL, dest, strings.Repeat("0", 64), "x", 0, nil); err == nil {
		t.Fatal("falsche Prüfsumme angenommen")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("Datei trotz Fehler da")
	}
	s := sha256.Sum256([]byte(body))
	if err := download(context.Background(), srv.URL, dest, hex.EncodeToString(s[:]), "x", 0, nil); err != nil {
		t.Fatal(err)
	}
}
