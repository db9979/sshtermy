// Package webwatch liefert die Seite zum Zusehen im Browser aus (ohne
// sshTermy). Der eingebaute Vermittler im lokalen Netz und `sshtermy
// vermittler` hängen sie unter /zusehen/ ein. Die Seite enthält keine
// Geheimnisse: Der Code steht im Link hinter „#“ (geht nie an den Server),
// Anmeldung und Entschlüsseln laufen im Browser (zusehen.wasm, gebaut aus
// cmd/zusehen-wasm mit `make zusehen`).
package webwatch

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed assets
var assets embed.FS

// Path: hier liegt die Seite (mit Schrägstrich am Ende).
const Path = "/zusehen/"

// Link: Adresse zum Zusehen im Browser für einen Code.
func Link(base, code string) string {
	return strings.TrimRight(base, "/") + Path + "#" + code
}

const csp = "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; img-src 'self' data:; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

var types = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".wasm": "application/wasm",
	".txt":  "text/plain; charset=utf-8",
}

// Register hängt die Seite in m ein (GET /zusehen/…).
func Register(m *http.ServeMux) {
	m.HandleFunc("GET /zusehen", func(w http.ResponseWriter, _ *http.Request) {
		// relativ, damit es auch hinter einem Proxy mit Pfad (/vermittler/zusehen) passt
		w.Header().Set("Location", "zusehen/")
		w.WriteHeader(http.StatusMovedPermanently)
	})
	m.HandleFunc("GET "+Path+"{file...}", serve)
}

func serve(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if name == "" {
		name = "index.html"
	}
	if strings.Contains(name, "/") || strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	ct, ok := types[path.Ext(name)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Security-Policy", csp)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Type", ct)
	if name == "zusehen.wasm" {
		gz, err := fs.ReadFile(assets, "assets/zusehen.wasm.gz")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.Set("Content-Encoding", "gzip")
			h.Set("Vary", "Accept-Encoding")
			_, _ = w.Write(gz)
			return
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			http.Error(w, "wasm", http.StatusInternalServerError)
			return
		}
		_, _ = io.Copy(w, zr)
		return
	}
	b, err := fs.ReadFile(assets, "assets/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(b)
}
