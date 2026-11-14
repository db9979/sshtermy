package vuln

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Woher die Einträge kommen:
//
//   - Online: OSV-API (api.osv.dev/v1/querybatch). Gesendet werden nur
//     Ökosystem, Paketname und Version – keine Hostnamen, keine Adressen.
//     Die Einträge selbst kommen aus dem öffentlichen OSV-Speicher und werden
//     im Cache-Ordner abgelegt.
//   - Offline: die ganze Datenbank des Ökosystems (all.zip) wird geladen und
//     auf die gebrauchten Systeme verkleinert. Danach verlässt beim Prüfen
//     nichts den Rechner. Statt osv.dev geht auch ein eigener Spiegel
//     (Firmennetz ohne Internet).

const (
	DefaultAPI    = "https://api.osv.dev"
	DefaultBucket = "https://osv-vulnerabilities.storage.googleapis.com"
	// DBMaxAge: so alt darf die Offline-Datenbank werden, dann wird neu geladen.
	DBMaxAge = 24 * time.Hour
)

// Query: ein Paket in einem Ökosystem.
type Query struct {
	Eco     string
	Name    string
	Version string
}

// Source liefert zu jeder Anfrage die OSV-Einträge, die das Paket nennen
// (ob die Version betroffen ist, entscheidet Vuln.Check).
type Source interface {
	Find(ctx context.Context, qs []Query, step func(string)) ([][]*Vuln, error)
	// Lookup: einzelner Eintrag (für die Schwere über DEBIAN-CVE-…), nil = unbekannt.
	Lookup(ctx context.Context, base, id string) *Vuln
}

// HTTP: gemeinsamer Teil beider Quellen.
type HTTP struct {
	Client *http.Client
	API    string // leer = DefaultAPI
	Bucket string // leer = DefaultBucket (oder eigener Spiegel)
	Cache  string // Cache-Ordner (…/sshtermy/osv)
}

func (h *HTTP) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (h *HTTP) bucket() string {
	if h.Bucket != "" {
		return strings.TrimRight(h.Bucket, "/")
	}
	return DefaultBucket
}

func (h *HTTP) api() string {
	if h.API != "" {
		return strings.TrimRight(h.API, "/")
	}
	return DefaultAPI
}

func (h *HTTP) get(ctx context.Context, u string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "sshtermy")
	resp, err := h.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, &statusError{resp.StatusCode, u}
	}
	return resp.Body, resp.ContentLength, nil
}

type statusError struct {
	code int
	url  string
}

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.code, e.url) }

// IsNotFound: Eintrag gibt es nicht (404).
func IsNotFound(err error) bool {
	var se *statusError
	return errors.As(err, &se) && se.code == http.StatusNotFound
}

// baseOfID: Ordner im OSV-Speicher nach dem Präfix der Kennung.
func baseOfID(id, fallback string) string {
	switch {
	case strings.HasPrefix(id, "openSUSE-"):
		return "openSUSE"
	case strings.HasPrefix(id, "SUSE-"):
		return "SUSE"
	case strings.HasPrefix(id, "DEBIAN-"), strings.HasPrefix(id, "DSA-"), strings.HasPrefix(id, "DLA-"), strings.HasPrefix(id, "DTSA-"):
		return "Debian"
	case strings.HasPrefix(id, "UBUNTU-"), strings.HasPrefix(id, "USN-"), strings.HasPrefix(id, "LSN-"):
		return "Ubuntu"
	case strings.HasPrefix(id, "ALPINE-"):
		return "Alpine"
	case strings.HasPrefix(id, "RLSA-"):
		return "Rocky Linux"
	case strings.HasPrefix(id, "ALSA-"), strings.HasPrefix(id, "ALBA-"), strings.HasPrefix(id, "ALEA-"):
		return "AlmaLinux"
	case strings.HasPrefix(id, "RHSA-"), strings.HasPrefix(id, "RHBA-"), strings.HasPrefix(id, "RHEA-"):
		return "Red Hat"
	}
	return fallback
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == ' ' || r == '*' || r == '?' {
			return '_'
		}
		return r
	}, s)
}

// ---------------------------------------------------------------- online

// Online fragt die OSV-API.
type Online struct{ HTTP }

func (o *Online) Find(ctx context.Context, qs []Query, step func(string)) ([][]*Vuln, error) {
	type ref struct{ id, base string }
	ids := make([][]ref, len(qs))
	for start := 0; start < len(qs); start += 500 {
		end := min(len(qs), start+500)
		step(i18n.F("osv.dev: %d–%d von %d Paketen", start+1, end, len(qs)))
		res, err := o.batch(ctx, qs[start:end], nil)
		if err != nil {
			return nil, err
		}
		for i, r := range res {
			for _, v := range r.Vulns {
				ids[start+i] = append(ids[start+i], ref{v.ID, baseOfID(v.ID, Base(qs[start+i].Eco))})
			}
			// weitere Seiten einzeln holen
			tok := r.Next
			for tok != "" {
				more, err := o.batch(ctx, qs[start+i:start+i+1], []string{tok})
				if err != nil || len(more) == 0 {
					break
				}
				for _, v := range more[0].Vulns {
					ids[start+i] = append(ids[start+i], ref{v.ID, baseOfID(v.ID, Base(qs[start+i].Eco))})
				}
				tok = more[0].Next
			}
		}
	}
	// Einträge holen (aus dem Cache oder dem OSV-Speicher), 8 gleichzeitig
	uniq := map[string]ref{}
	for _, rs := range ids {
		for _, r := range rs {
			uniq[r.id] = r
		}
	}
	got := map[string]*Vuln{}
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	n := 0
	for _, r := range uniq {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			v, err := o.vuln(ctx, r.base, r.id)
			mu.Lock()
			defer mu.Unlock()
			n++
			if n%25 == 0 {
				step(i18n.F("osv.dev: %d von %d Einträgen", n, len(uniq)))
			}
			if err != nil {
				if firstErr == nil && ctx.Err() == nil {
					firstErr = err
				}
				return
			}
			got[r.id] = v
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(got) == 0 && firstErr != nil {
		return nil, firstErr
	}
	out := make([][]*Vuln, len(qs))
	for i, rs := range ids {
		for _, r := range rs {
			if v := got[r.id]; v != nil {
				out[i] = append(out[i], v)
			}
		}
	}
	return out, nil
}

type batchResult struct {
	Vulns []struct {
		ID       string    `json:"id"`
		Modified time.Time `json:"modified"`
	} `json:"vulns"`
	Next string `json:"next_page_token"`
}

func (o *Online) batch(ctx context.Context, qs []Query, tokens []string) ([]batchResult, error) {
	type pkg struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	}
	type q struct {
		Package   pkg    `json:"package"`
		Version   string `json:"version"`
		PageToken string `json:"page_token,omitempty"`
	}
	body := struct {
		Queries []q `json:"queries"`
	}{}
	for i, x := range qs {
		e := q{Package: pkg{x.Name, x.Eco}, Version: x.Version}
		if i < len(tokens) {
			e.PageToken = tokens[i]
		}
		body.Queries = append(body.Queries, e)
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.api()+"/v1/querybatch", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sshtermy")
	resp, err := o.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("osv.dev: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Results []batchResult `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("osv.dev: %w", err)
	}
	return out.Results, nil
}

// vuln: Eintrag aus dem Cache oder aus dem OSV-Speicher (Rückfall: API).
func (o *Online) vuln(ctx context.Context, base, id string) (*Vuln, error) {
	p := ""
	if o.Cache != "" {
		p = filepath.Join(o.Cache, "vulns", safeName(base), safeName(id)+".json")
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < DBMaxAge {
			if raw, err := os.ReadFile(p); err == nil {
				var v Vuln
				if json.Unmarshal(raw, &v) == nil {
					return &v, nil
				}
			}
		}
	}
	raw, err := o.fetch(ctx, o.bucket()+"/"+url.PathEscape(base)+"/"+url.PathEscape(id)+".json")
	if err != nil {
		raw, err = o.fetch(ctx, o.api()+"/v1/vulns/"+url.PathEscape(id))
	}
	if err != nil {
		return nil, err
	}
	var v Vuln
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	if p != "" {
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		_ = os.WriteFile(p, raw, 0o600)
	}
	return &v, nil
}

func (o *Online) fetch(ctx context.Context, u string) ([]byte, error) {
	rc, _, err := o.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 16<<20))
}

func (o *Online) Lookup(ctx context.Context, base, id string) *Vuln {
	v, err := o.vuln(ctx, base, id)
	if err != nil {
		return nil
	}
	return v
}

// ---------------------------------------------------------------- offline

// Offline: lokal geladene Datenbank je Ökosystem.
type Offline struct {
	HTTP
	mu  sync.Mutex
	idx map[string]*ecoIndex // Ökosystem → Index
	// Stale: Datenbank war zu alt und ließ sich nicht neu laden (Hinweis).
	Stale []string
}

// ecoIndex: alle Einträge eines Ökosystems, nach Paketname.
type ecoIndex struct {
	Eco   string             `json:"eco"`
	Built time.Time          `json:"built"`
	Pkgs  map[string][]*Vuln `json:"pkgs"`
}

func (o *Offline) indexPath(eco string) string {
	return filepath.Join(o.Cache, "db", safeName(eco)+".json.gz")
}

// DBInfo: Stand der Offline-Datenbank je Ökosystem (für die Einstellungen).
func DBInfo(cache string) map[string]time.Time {
	out := map[string]time.Time{}
	es, _ := os.ReadDir(filepath.Join(cache, "db"))
	for _, e := range es {
		if fi, err := e.Info(); err == nil && strings.HasSuffix(e.Name(), ".json.gz") {
			out[strings.TrimSuffix(e.Name(), ".json.gz")] = fi.ModTime()
		}
	}
	return out
}

func (o *Offline) load(eco string) *ecoIndex {
	f, err := os.Open(o.indexPath(eco))
	if err != nil {
		return nil
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	var ix ecoIndex
	if json.NewDecoder(zr).Decode(&ix) != nil || ix.Eco != eco {
		return nil
	}
	return &ix
}

// ensure: Indizes der Ökosysteme laden, wenn nötig neu bauen.
func (o *Offline) ensure(ctx context.Context, ecos []string, step func(string)) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.idx == nil {
		o.idx = map[string]*ecoIndex{}
	}
	need := map[string][]string{} // Basis → Ökosysteme
	var stale []*ecoIndex
	for _, e := range ecos {
		if ix := o.idx[e]; ix != nil && time.Since(ix.Built) < DBMaxAge {
			continue
		}
		ix := o.load(e)
		if ix != nil && time.Since(ix.Built) < DBMaxAge {
			o.idx[e] = ix
			continue
		}
		if ix != nil {
			stale = append(stale, ix)
		}
		need[Base(e)] = append(need[Base(e)], e)
	}
	var errs []error
	for base, es := range need {
		if err := o.build(ctx, base, es, step); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, fmt.Errorf("%s: %w", base, err))
		}
	}
	for _, ix := range stale {
		if o.idx[ix.Eco] == nil || o.idx[ix.Eco].Built.Before(ix.Built) {
			o.idx[ix.Eco] = ix
			o.Stale = append(o.Stale, ix.Eco)
		}
	}
	if len(errs) > 0 {
		missing := false
		for _, e := range ecos {
			if o.idx[e] == nil {
				missing = true
			}
		}
		if missing {
			return errors.Join(errs...)
		}
	}
	return nil
}

// build lädt <Basis>/all.zip und behält nur die gebrauchten Ökosysteme.
func (o *Offline) build(ctx context.Context, base string, ecos []string, step func(string)) error {
	if o.Cache == "" {
		return errors.New(i18n.T("kein Cache-Ordner"))
	}
	dir := filepath.Join(o.Cache, "db")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	rc, size, err := o.get(ctx, o.bucket()+"/"+url.PathEscape(base)+"/all.zip")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "dl-*.zip")
	if err != nil {
		rc.Close()
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	pr := &progressReader{r: rc, total: size, step: step, label: base}
	_, err = io.Copy(tmp, pr)
	rc.Close()
	if err != nil {
		return err
	}
	st, err := tmp.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(tmp, st.Size())
	if err != nil {
		return err
	}
	want := map[string]*ecoIndex{}
	for _, e := range ecos {
		want[e] = &ecoIndex{Eco: e, Built: time.Now(), Pkgs: map[string][]*Vuln{}}
	}
	step(i18n.F("%s: Datenbank auswerten (%d Einträge) …", base, len(zr.File)))
	for i, f := range zr.File {
		if i%2000 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		if !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		r, err := f.Open()
		if err != nil {
			continue
		}
		var v Vuln
		err = json.NewDecoder(io.LimitReader(r, 16<<20)).Decode(&v)
		r.Close()
		if err != nil || v.Withdrawn != nil {
			continue
		}
		for _, a := range v.Affected {
			ix := want[a.Package.Ecosystem]
			if ix == nil {
				continue
			}
			// nur die Angaben dieses Ökosystems behalten (spart Platz)
			lean := v
			lean.Affected = nil
			for _, b := range v.Affected {
				if b.Package.Ecosystem == a.Package.Ecosystem && b.Package.Name == a.Package.Name {
					if len(b.EcosystemSpecific) > 200 {
						b.EcosystemSpecific = nil // z. B. Ubuntus Binärliste
					}
					lean.Affected = append(lean.Affected, b)
				}
			}
			lean.References = nil
			if len(lean.Details) > 1200 {
				lean.Details = lean.Details[:1200] + "…"
			}
			list := ix.Pkgs[a.Package.Name]
			dup := false
			for _, x := range list {
				if x.ID == v.ID {
					dup = true
					break
				}
			}
			if !dup {
				ix.Pkgs[a.Package.Name] = append(list, &lean)
			}
		}
	}
	for e, ix := range want {
		if err := o.save(ix); err != nil {
			return err
		}
		o.idx[e] = ix
	}
	return nil
}

func (o *Offline) save(ix *ecoIndex) error {
	p := o.indexPath(ix.Eco)
	f, err := os.CreateTemp(filepath.Dir(p), "ix-*.tmp")
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	err = json.NewEncoder(zw).Encode(ix)
	if e := zw.Close(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), p)
}

func (o *Offline) Find(ctx context.Context, qs []Query, step func(string)) ([][]*Vuln, error) {
	ecos := map[string]bool{}
	for _, q := range qs {
		ecos[q.Eco] = true
	}
	var list []string
	for e := range ecos {
		list = append(list, e)
	}
	sort.Strings(list)
	if err := o.ensure(ctx, list, step); err != nil {
		return nil, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([][]*Vuln, len(qs))
	for i, q := range qs {
		if ix := o.idx[q.Eco]; ix != nil {
			out[i] = ix.Pkgs[q.Name]
		}
	}
	return out, nil
}

// Lookup: offline nur aus geladenen Indizes (keine Anfrage nach außen).
func (o *Offline) Lookup(_ context.Context, base, id string) *Vuln {
	o.mu.Lock()
	defer o.mu.Unlock()
	for eco, ix := range o.idx {
		if Base(eco) != base {
			continue
		}
		for _, vs := range ix.Pkgs {
			for _, v := range vs {
				if v.ID == id {
					return v
				}
			}
		}
	}
	return nil
}

type progressReader struct {
	r     io.Reader
	total int64
	n     int64
	last  time.Time
	step  func(string)
	label string
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if time.Since(p.last) > 500*time.Millisecond {
		p.last = time.Now()
		if p.total > 0 {
			p.step(i18n.F("%s: Datenbank laden %d von %d MB", p.label, p.n>>20, p.total>>20))
		} else {
			p.step(i18n.F("%s: Datenbank laden %d MB", p.label, p.n>>20))
		}
	}
	return n, err
}
