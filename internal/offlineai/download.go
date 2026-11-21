package offlineai

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Quellen (je die neueste Veröffentlichung).
var (
	ServerRelease  = "https://api.github.com/repos/ggml-org/llama.cpp/releases/latest"
	ServerReleases = "https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=10"
	ModelRelease   = "https://api.github.com/repos/arxhr007/lxask/releases/latest"
)

// Progress: Stand des Herunterladens.
type Progress struct {
	Step        string // was gerade geladen wird
	Done, Total int64
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type release struct {
	Tag    string  `json:"tag_name"`
	Assets []asset `json:"assets"`
}

var client = &http.Client{Timeout: 0} // große Dateien: kein Gesamtlimit, Abbruch per ctx

func getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sshTermy")
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := client.Do(req.WithContext(cctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

// Ausschlüsse: Builds für Grafikkarten, wir nehmen den CPU-Build.
var gpuBuild = regexp.MustCompile(`(?i)cuda|vulkan|hip|sycl|opencl|kompute|musa|rocm|cann|openvino|radeon|kleidiai|xcframework`)

// PickServer wählt den llama.cpp-Build für dieses System. Der Name wird in
// Teile zerlegt (llama-b7000-bin-macos-arm64.zip → llama, b7000, bin, macos,
// arm64), damit auch umbenannte Pakete (ohne „bin“, .tgz, osx/darwin) passen.
func PickServer(goos, goarch string, assets []asset) (asset, bool) {
	osTok := map[string][]string{"windows": {"win", "windows"}, "darwin": {"macos", "mac", "osx", "darwin", "apple"}, "linux": {"ubuntu", "linux"}}[goos]
	archTok := map[string][]string{"amd64": {"x64", "x86_64", "amd64"}, "arm64": {"arm64", "aarch64"}}[goarch]
	if osTok == nil || archTok == nil {
		return asset{}, false
	}
	allArch := []string{"x64", "x86_64", "amd64", "arm64", "aarch64", "x86", "i686", "armv7", "riscv64", "s390x"}
	best, bestScore := asset{}, -1
	for _, a := range assets {
		n := strings.ToLower(a.Name)
		isZip, isTar := strings.HasSuffix(n, ".zip"), strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz")
		if !strings.HasPrefix(n, "llama") || gpuBuild.MatchString(n) || !(isZip || isTar) {
			continue
		}
		parts := strings.FieldsFunc(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(n, ".zip"), ".gz"), ".tar"),
			func(r rune) bool { return r == '-' || r == '_' || r == '.' })
		has := func(toks []string) int {
			for i, t := range toks {
				if slices.Contains(parts, t) {
					return i
				}
			}
			return -1
		}
		oi := has(osTok)
		if oi < 0 {
			continue
		}
		// x86_64 zerfällt in x86 und 64: als Ganzes prüfen
		archOK := has(archTok) >= 0 || (goarch == "amd64" && strings.Contains(n, "x86_64"))
		if !archOK {
			// ohne Architektur im Namen nur auf dem Mac (Universal-Build)
			if goos != "darwin" || has(allArch) >= 0 || strings.Contains(n, "x86_64") {
				continue
			}
		}
		score := 10 - oi
		if archOK {
			score += 3
		}
		if slices.Contains(parts, "cpu") {
			score += 5
		}
		if isZip == (goos == "windows") {
			score++
		}
		if score > bestScore {
			best, bestScore = a, score
		}
	}
	return best, bestScore >= 0
}

var modelName = regexp.MustCompile(`(?i)^lxask.*q4_k_m\.gguf$`)

// PickModel wählt die Modelldatei und die Prüfsummen aus dem lxask-Release.
func PickModel(assets []asset) (model, sums asset, ok bool) {
	for _, a := range assets {
		switch {
		case modelName.MatchString(a.Name):
			model, ok = a, true
		case strings.EqualFold(a.Name, "SHA256SUMS"):
			sums = a
		}
	}
	return model, sums, ok
}

// ParseSums liest „<sha256>  <name>“-Zeilen.
func ParseSums(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && len(f[0]) == 64 {
			out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	return out
}

// download lädt url nach dest (über dest.part) und prüft die Prüfsumme.
func download(ctx context.Context, url, dest, sum, step string, total int64, progress func(Progress)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "sshTermy")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	if total <= 0 {
		total = resp.ContentLength
	}
	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	h := sha256.New()
	var done int64
	buf := make([]byte, 256<<10)
	last := time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return err
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil && time.Since(last) > 200*time.Millisecond {
				progress(Progress{Step: step, Done: done, Total: total})
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if sum != "" && hex.EncodeToString(h.Sum(nil)) != sum {
		os.Remove(part)
		return fmt.Errorf(i18n.T("Prüfsumme von %s stimmt nicht"), filepath.Base(dest))
	}
	return os.Rename(part, dest)
}

// Download holt Server (falls keiner auf dem Rechner ist) und Modell.
func (e *Engine) Download(ctx context.Context, progress func(Progress)) error {
	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		return err
	}
	if e.ServerPath() == "" {
		if err := e.downloadServer(ctx, progress); err != nil {
			return err
		}
	}
	if st, err := os.Stat(e.ModelPath()); err != nil || st.Size() < 100<<20 {
		var rel release
		if err := getJSON(ctx, ModelRelease, &rel); err != nil {
			return fmt.Errorf(i18n.T("lxask-Veröffentlichung nicht erreichbar: %w"), err)
		}
		model, sums, ok := PickModel(rel.Assets)
		if !ok {
			return errors.New(i18n.T("im lxask-Release fehlt die Modelldatei"))
		}
		sum := ""
		if sums.URL != "" {
			if m, err := fetchSums(ctx, sums.URL); err == nil {
				sum = m[model.Name]
			}
		}
		if err := download(ctx, model.URL, e.ModelPath(), sum, i18n.T("Modell (lxask)"), model.Size, progress); err != nil {
			return err
		}
	}
	return nil
}

func fetchSums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sshTermy")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ParseSums(io.LimitReader(resp.Body, 1<<20)), nil
}

func (e *Engine) downloadServer(ctx context.Context, progress func(Progress)) error {
	var rel release
	if err := getJSON(ctx, ServerRelease, &rel); err != nil {
		return fmt.Errorf(i18n.T("llama.cpp-Veröffentlichung nicht erreichbar: %w"), err)
	}
	a, ok := PickServer(runtime.GOOS, runtime.GOARCH, rel.Assets)
	if !ok {
		// Der neueste Build hat manchmal (noch) nicht alle Pakete: die letzten zehn durchsehen.
		var rels []release
		if err := getJSON(ctx, ServerReleases, &rels); err == nil {
			for _, r := range rels {
				if a, ok = PickServer(runtime.GOOS, runtime.GOARCH, r.Assets); ok {
					break
				}
			}
		}
	}
	if !ok && runtime.GOOS == "darwin" {
		return errors.New(i18n.T("kein fertiger llama-server für den Mac gefunden; im Terminal „brew install llama.cpp“ ausführen, dann erneut herunterladen"))
	}
	if !ok {
		return fmt.Errorf(i18n.T("für %s/%s gibt es keinen fertigen llama-server; installiere llama.cpp selbst (llama-server im PATH)"), runtime.GOOS, runtime.GOARCH)
	}
	arch := filepath.Join(e.dir, a.Name)
	if err := download(ctx, a.URL, arch, "", i18n.T("llama-server (llama.cpp)"), a.Size, progress); err != nil {
		return err
	}
	defer os.Remove(arch)
	dst := filepath.Join(e.dir, "llama")
	os.RemoveAll(dst)
	if err := Extract(arch, dst); err != nil {
		return err
	}
	srv := findFile(dst, exeName("llama-server"))
	if srv == "" {
		return errors.New(i18n.T("llama-server fehlt im heruntergeladenen Paket"))
	}
	return os.WriteFile(filepath.Join(e.dir, "server.path"), []byte(srv), 0o644)
}

func findFile(root, name string) string {
	found := ""
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && fi.Name() == name && found == "" {
			found = p
		}
		return nil
	})
	return found
}

// Extract packt ein .zip oder .tar.gz nach dst aus (ohne Pfade nach außen).
func Extract(archive, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	safe := func(name string) (string, error) {
		p := filepath.Join(dst, filepath.FromSlash(name))
		if !strings.HasPrefix(p, filepath.Clean(dst)+string(os.PathSeparator)) {
			return "", fmt.Errorf("unsicherer Pfad im Archiv: %s", name)
		}
		return p, nil
	}
	if strings.HasSuffix(strings.ToLower(archive), ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			p, err := safe(f.Name)
			if err != nil {
				return err
			}
			if f.FileInfo().IsDir() {
				os.MkdirAll(p, 0o755)
				continue
			}
			if f.Mode()&os.ModeSymlink != 0 {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				target, _ := io.ReadAll(io.LimitReader(rc, 4096))
				rc.Close()
				if err := symlinkInside(dst, p, string(target)); err != nil {
					return err
				}
				continue
			}
			if err := writeFrom(p, f.Mode()|0o755, func() (io.ReadCloser, error) { return f.Open() }); err != nil {
				return err
			}
		}
		return nil
	}
	fh, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := safe(h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(p, 0o755)
		case tar.TypeSymlink:
			if err := symlinkInside(dst, p, h.Linkname); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFrom(p, os.FileMode(h.Mode)|0o755, func() (io.ReadCloser, error) { return io.NopCloser(tr), nil }); err != nil {
				return err
			}
		}
	}
}

// symlinkInside: Bibliotheks-Links (libllama.so → libllama.so.1) nur innerhalb von dst.
func symlinkInside(dst, p, target string) error {
	if filepath.IsAbs(target) {
		return nil
	}
	full := filepath.Join(filepath.Dir(p), target)
	if !strings.HasPrefix(full, filepath.Clean(dst)+string(os.PathSeparator)) {
		return nil
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.Remove(p)
	return os.Symlink(target, p)
}

func writeFrom(p string, mode os.FileMode, open func() (io.ReadCloser, error)) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	rc, err := open()
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode&0o777)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(rc, 2<<30)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
