// Package update: Auto-Update aus GitHub Releases.
//
// Ablauf:
//  1. GET https://api.github.com/repos/<Repo>/releases/latest (ohne Token,
//     öffentliches Repo; 60 Anfragen/h reichen, geprüft wird höchstens 1×/Tag).
//  2. Ist tag_name (vX.Y.Z) neuer als die laufende Version, wird das passende
//     Asset sshtermy_<os>_<arch>[.exe] und checksums.txt geladen.
//  3. Sicherheit:
//     - nur HTTPS zu github.com / objects.githubusercontent.com (Redirects
//     werden geprüft),
//     - SHA-256 des Downloads muss zu checksums.txt passen,
//     - checksums.txt muss mit dem eingebauten Ed25519-Schlüssel signiert sein
//     (checksums.txt.sig), sobald UpdatePubKey gesetzt ist. So kann auch ein
//     kompromittiertes GitHub-Konto ohne den privaten Signierschlüssel kein
//     Update unterschieben.
//  4. Austausch: die laufende Datei wird zu .old umbenannt (geht auch unter
//     Windows bei laufender .exe), die neue an ihren Platz geschrieben; beim
//     nächsten Start räumt Cleanup() die .old weg.
//  5. Neustart: Restart() startet die neue Datei (Unix: exec im selben
//     Prozess; Windows: Kindprozess im selben Konsolenfenster, der alte
//     wartet nur noch und beendet sich mit dessen Exit-Code).
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

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// Release: was die Oberfläche anzeigt.
type Release struct {
	Version string // ohne "v"
	Notes   string
	URL     string
	assets  map[string][]string // Name → Download-URLs (API zuerst, dann github.com/…/releases/download)
}

var allowedHosts = map[string]bool{
	"api.github.com": true, "github.com": true,
	"objects.githubusercontent.com": true, "release-assets.githubusercontent.com": true,
}

var errRedirect = errors.New("redirect")

// httpClient: Tests setzen einen eigenen ein.
var httpClient = client

func client() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || !allowedHosts[req.URL.Hostname()] || len(via) > 5 {
				return fmt.Errorf("%w: "+i18n.T("update: Weiterleitung nach %s abgelehnt"), errRedirect, req.URL.Host)
			}
			return nil
		},
	}
}

// retryWait: Pausen zwischen den Versuchen (Tests verkürzen sie).
var retryWait = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// get lädt u. Vorübergehende Fehler (Netz, HTTP 429/5xx wie 502/503/504 von
// GitHubs Download-Servern) werden bis zu dreimal wiederholt.
func get(ctx context.Context, u, accept string, limit int64) ([]byte, error) {
	pu, err := url.Parse(u)
	if err != nil || pu.Scheme != "https" || !allowedHosts[pu.Hostname()] {
		return nil, fmt.Errorf(i18n.T("update: URL nicht erlaubt: %s"), u)
	}
	for i := 0; ; i++ {
		b, retry, err := getOnce(ctx, u, accept, limit)
		if err == nil || !retry || i >= len(retryWait) {
			return b, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(retryWait[i]):
		}
	}
}

func getOnce(ctx context.Context, u, accept string, limit int64) ([]byte, bool, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", version.Short())
	req.Header.Set("Accept", accept)
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, ctx.Err() == nil && !errors.Is(err, errRedirect), err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return nil, true, fmt.Errorf(i18n.T("update: GitHub antwortet gerade nicht (HTTP %d), bitte später noch einmal versuchen"), resp.StatusCode)
		}
		return nil, false, fmt.Errorf(i18n.T("update: HTTP %d"), resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return b, err != nil && ctx.Err() == nil, err
}

const (
	acceptJSON = "application/vnd.github+json"
	acceptBin  = "application/octet-stream" // API-Asset-URL liefert damit die Datei statt JSON
)

// download probiert die Adressen eines Assets der Reihe nach.
func download(ctx context.Context, urls []string, limit int64) ([]byte, error) {
	err := errors.New(i18n.T("update: keine Download-Adresse"))
	for _, u := range urls {
		var b []byte
		if b, err = get(ctx, u, acceptBin, limit); err == nil {
			return b, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, err
}

// AssetName: sshtermy_windows_arm64.exe, sshtermy_darwin_arm64, …
func AssetName() string {
	n := fmt.Sprintf("sshtermy_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

// apiBase: GitHub-API. Variable für Tests.
var apiBase = "https://api.github.com"

type ghRelease struct {
	Tag       string    `json:"tag_name"`
	Body      string    `json:"body"`
	HTML      string    `json:"html_url"`
	Draft     bool      `json:"draft"`
	Pre       bool      `json:"prerelease"`
	Published time.Time `json:"published_at"`
	Assets    []struct {
		Name string `json:"name"`
		API  string `json:"url"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// usable: ein fertiges Release, neuer als dieses Programm und nicht das
// zurückgenommene (guard.go).
func (r ghRelease) usable() bool {
	v := strings.TrimPrefix(r.Tag, "v")
	return !r.Draft && !r.Pre && Newer(v, version.Version) && v != skipped()
}

func (r ghRelease) release() *Release {
	rel := &Release{Version: strings.TrimPrefix(r.Tag, "v"), Notes: r.Body, URL: r.HTML, assets: map[string][]string{}}
	for _, a := range r.Assets {
		// Der API-Weg ist verlässlicher; github.com/…/releases/download
		// antwortet zeitweise mit 504.
		for _, u := range []string{a.API, a.URL} {
			if u != "" {
				rel.assets[a.Name] = append(rel.assets[a.Name], u)
			}
		}
	}
	return rel
}

// Check fragt das neueste Release ab. nil, nil = schon aktuell.
func Check(ctx context.Context) (*Release, error) {
	raw, err := get(ctx, apiBase+"/repos/"+version.Repo+"/releases/latest", acceptJSON, 1<<20)
	if err != nil {
		return nil, err
	}
	var r ghRelease
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if !r.usable() {
		return nil, nil
	}
	return r.release(), nil
}

// CheckAged ist der Kanal „stabil“: das neueste Release, das seit
// mindestens minAge veröffentlicht ist. Jede Änderung wird sofort ein
// Release; wer wartet, bekommt nur Versionen, die andere schon eine Weile
// benutzen und die nicht gleich wieder ersetzt wurden. minAge <= 0 = Check.
func CheckAged(ctx context.Context, minAge time.Duration) (*Release, error) {
	if minAge <= 0 {
		return Check(ctx)
	}
	raw, err := get(ctx, apiBase+"/repos/"+version.Repo+"/releases?per_page=50", acceptJSON, 8<<20)
	if err != nil {
		return nil, err
	}
	var list []ghRelease
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	var best *ghRelease
	for i := range list {
		r := &list[i]
		if !r.usable() || r.Published.IsZero() || now().Sub(r.Published) < minAge {
			continue
		}
		if best == nil || Newer(strings.TrimPrefix(r.Tag, "v"), strings.TrimPrefix(best.Tag, "v")) {
			best = r
		}
	}
	if best == nil {
		return nil, nil
	}
	return best.release(), nil
}

// now: Variable für Tests.
var now = time.Now

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
	if cmd := Managed(); cmd != "" {
		return ManagedError{cmd}
	}
	name := AssetName()
	binURL, ok1 := rel.assets[name]
	sumURL, ok2 := rel.assets["checksums.txt"]
	if !ok1 || !ok2 {
		return fmt.Errorf(i18n.T("update: Release enthält %s oder checksums.txt nicht"), name)
	}
	sums, err := download(ctx, sumURL, 64<<10)
	if err != nil {
		return err
	}
	if version.UpdatePubKey != "" {
		sigURL, ok := rel.assets["checksums.txt.sig"]
		if !ok {
			return errors.New(i18n.T("update: Signatur fehlt"))
		}
		sig, err := download(ctx, sigURL, 1024)
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
		return errors.New(i18n.T("update: keine Prüfsumme für ") + name)
	}
	bin, err := download(ctx, binURL, 200<<20)
	if err != nil {
		return err
	}
	got := sha256.Sum256(bin)
	if hex.EncodeToString(got[:]) != want {
		return errors.New(i18n.T("update: Prüfsumme stimmt nicht, Download verworfen"))
	}
	return replaceSelf(bin, rel.Version)
}

// VerifySignature prüft eine Ed25519-Signatur (Rohbytes oder hex) über data.
func VerifySignature(data, sig []byte, pubHex string) error {
	pub, err := hex.DecodeString(strings.TrimSpace(pubHex))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New(i18n.T("update: eingebauter Schlüssel ungültig"))
	}
	s := sig
	if d, err := hex.DecodeString(strings.TrimSpace(string(sig))); err == nil {
		s = d
	}
	if !ed25519.Verify(pub, data, s) {
		return errors.New(i18n.T("update: Signatur ungültig, Update abgelehnt"))
	}
	return nil
}

// exePath merkt sich den Pfad der Programmdatei beim Start. Später geht das
// nicht mehr zuverlässig: unter Linux zeigt os.Executable() nach dem
// Umbenennen auf die .old-Datei.
var exePath string

// Executable: Pfad der Programmdatei, wie er beim Start war.
func Executable() (string, error) {
	if exePath != "" {
		return exePath, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	exePath = exe
	return exe, nil
}

func replaceSelf(bin []byte, ver string) error {
	exe, err := Executable()
	if err != nil {
		return err
	}
	tmp := newPath(exe)
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf(i18n.T("update: keine Schreibrechte im Programmordner: %w"), err)
	}
	// Erst prüfen, dann tauschen: startet die neue Datei nicht oder ist sie
	// nicht die angekündigte Version, bleibt alles beim Alten (guard.go).
	if err := selfTest(tmp, ver); err != nil {
		os.Remove(tmp)
		return err
	}
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Unter Windows läuft die .old evtl. noch (der Elternprozess eines
		// früheren Neustarts wartet darin). Dann eben ein eigener Name.
		old = fmt.Sprintf("%s.%d.old", exe, time.Now().UnixNano())
	}
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // zurückrollen
		return err
	}
	// Die alte Datei bleibt, bis die neue einmal sauber gelaufen ist.
	writeJSON(pendingPath(exe), pendingInfo{Version: ver, From: version.Version, Old: old})
	_ = os.Remove(skipPath(exe))
	return nil
}

// Cleanup merkt sich den Programmpfad und entfernt .old-Dateien früherer
// Updates (was noch läuft, bleibt liegen und fällt beim nächsten Mal weg).
func Cleanup() {
	exe, err := Executable()
	if err != nil {
		return
	}
	if _, err := os.Stat(pendingPath(exe)); err == nil {
		return // unbestätigtes Update: die alte Datei wird für den Rückfall gebraucht
	}
	_ = os.Remove(newPath(exe)) // Rest eines abgebrochenen Updates
	dir, base := filepath.Dir(exe), filepath.Base(exe)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, base+".") && strings.HasSuffix(n, ".old") && !e.IsDir() {
			_ = os.Remove(filepath.Join(dir, n))
		}
	}
}

// Signed: prüft dieser Build die Signatur von Updates? Offizielle Releases
// tragen den Schlüssel (release.yml, UPDATE_PUBKEY); wer selbst baut, ohne
// ihn zu setzen, bekommt Updates nur mit Prüfsumme. Die Oberfläche warnt
// dann und fragt vor dem Austausch nach.
func Signed() bool { return version.UpdatePubKey != "" }
