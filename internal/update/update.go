// Package update: Auto-Update aus GitHub Releases.
//
// Ablauf:
//  1. GET https://api.github.com/repos/<Repo>/releases/latest (ohne Token,
//     öffentliches Repo; 60 Anfragen/h reichen, geprüft wird höchstens 1×/Tag).
//  2. Ist tag_name (vX.Y.Z) neuer als die laufende Version, wird das passende
//     Asset sshtermy_<os>_<arch>[.exe] und checksums.txt geladen.
//  3. Sicherheit:
//     - nur HTTPS zu github.com / objects.githubusercontent.com (Redirects
//       werden geprüft),
//     - SHA-256 des Downloads muss zu checksums.txt passen,
//     - checksums.txt muss mit dem eingebauten Ed25519-Schlüssel signiert sein
//       (checksums.txt.sig), sobald UpdatePubKey gesetzt ist. So kann auch ein
//       kompromittiertes GitHub-Konto ohne den privaten Signierschlüssel kein
//       Update unterschieben.
//  4. Austausch: die laufende Datei wird zu .old umbenannt (geht auch unter
//     Windows bei laufender .exe), die neue an ihren Platz geschrieben; beim
//     nächsten Start räumt Cleanup() die .old weg. Neustart durch den Nutzer.
package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/version"
)

// Release: was die Oberfläche anzeigt.
type Release struct {
	Version string // ohne "v"
	Notes   string
	URL     string
	assets  map[string]string // Name → Download-URL
}

var allowedHosts = map[string]bool{
	"api.github.com": true, "github.com": true,
	"objects.githubusercontent.com": true, "release-assets.githubusercontent.com": true,
}

func client() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || !allowedHosts[req.URL.Hostname()] || len(via) > 5 {
				return fmt.Errorf("update: Weiterleitung nach %s abgelehnt", req.URL.Host)
			}
			return nil
		},
	}
}

func get(ctx context.Context, u string, limit int64) ([]byte, error) {
	pu, err := url.Parse(u)
	if err != nil || pu.Scheme != "https" || !allowedHosts[pu.Hostname()] {
		return nil, fmt.Errorf("update: URL nicht erlaubt: %s", u)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", version.Short())
	req.Header.Set("Accept", "application/vnd.github+json, application/octet-stream")
	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("update: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// AssetName: sshtermy_windows_arm64.exe, sshtermy_darwin_arm64, …
func AssetName() string {
	n := fmt.Sprintf("sshtermy_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

// Check fragt das neueste Release ab. nil, nil = schon aktuell.
func Check(ctx context.Context) (*Release, error) {
	raw, err := get(ctx, "https://api.github.com/repos/"+version.Repo+"/releases/latest", 1<<20)
	if err != nil {
		return nil, err
	}
	var r struct {
		Tag    string `json:"tag_name"`
		Body   string `json:"body"`
		HTML   string `json:"html_url"`
		Draft  bool   `json:"draft"`
		Pre    bool   `json:"prerelease"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Draft || r.Pre {
		return nil, nil
	}
	v := strings.TrimPrefix(r.Tag, "v")
	if !Newer(v, version.Version) {
		return nil, nil
	}
	rel := &Release{Version: v, Notes: r.Body, URL: r.HTML, assets: map[string]string{}}
	for _, a := range r.Assets {
		rel.assets[a.Name] = a.URL
	}
	return rel, nil
}

// Newer: ist a > b? (semver MAJOR.MINOR.PATCH; "dev" ist immer älter)
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA {
		return false
	}
	if !okB {
		return true
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.SplitN(v, "-", 2)[0], ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Apply lädt, prüft und tauscht die laufende Datei aus.
func Apply(ctx context.Context, rel *Release) error {
	name := AssetName()
	binURL, ok1 := rel.assets[name]
	sumURL, ok2 := rel.assets["checksums.txt"]
	if !ok1 || !ok2 {
		return fmt.Errorf("update: Release enthält %s oder checksums.txt nicht", name)
	}
	sums, err := get(ctx, sumURL, 64<<10)
	if err != nil {
		return err
	}
	if version.UpdatePubKey != "" {
		sigURL, ok := rel.assets["checksums.txt.sig"]
		if !ok {
			return errors.New("update: Signatur fehlt")
		}
		sig, err := get(ctx, sigURL, 1024)
		if err != nil {
			return err
		}
		if err := VerifySignature(sums, sig, version.UpdatePubKey); err != nil {
			return err
		}
	}
	want := ""
	for _, l := range strings.Split(string(sums), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return errors.New("update: keine Prüfsumme für " + name)
	}
	bin, err := get(ctx, binURL, 200<<20)
	if err != nil {
		return err
	}
	got := sha256.Sum256(bin)
	if hex.EncodeToString(got[:]) != want {
		return errors.New("update: Prüfsumme stimmt nicht, Download verworfen")
	}
	return replaceSelf(bin)
}

// VerifySignature prüft eine Ed25519-Signatur (Rohbytes oder hex) über data.
func VerifySignature(data, sig []byte, pubHex string) error {
	pub, err := hex.DecodeString(strings.TrimSpace(pubHex))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("update: eingebauter Schlüssel ungültig")
	}
	s := sig
	if d, err := hex.DecodeString(strings.TrimSpace(string(sig))); err == nil {
		s = d
	}
	if !ed25519.Verify(pub, data, s) {
		return errors.New("update: Signatur ungültig, Update abgelehnt")
	}
	return nil
}

func replaceSelf(bin []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf("update: keine Schreibrechte im Programmordner: %w", err)
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // zurückrollen
		return err
	}
	return nil
}

// Cleanup entfernt eine .old-Datei vom letzten Update.
func Cleanup() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}
