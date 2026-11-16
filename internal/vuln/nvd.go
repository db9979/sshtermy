package vuln

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Firmware von Switches, Firewalls und Routern kennt OSV nicht. Dafür
// fragt sshTermy die NVD (National Vulnerability Database, NIST) nach allen
// CVEs eines Produkts (CPE „cpe:2.3:o:fortinet:fortios“) und prüft die
// Versionsangaben selbst. Gesendet wird nur der Produktname, nie die
// Firmware-Version, ein Gerätename oder eine Adresse. Welche CPE zu welchem
// Befehlssatz gehört, steht in dessen YAML (advisory.cpe).

// DefaultNVD: Adresse der NVD-API (CVE 2.0).
const DefaultNVD = "https://services.nvd.nist.gov/rest/json/cves/2.0"

// NVD fragt die NVD-API. Ohne Schlüssel erlaubt sie 5 Anfragen in 30
// Sekunden, mit Schlüssel 50.
type NVD struct {
	HTTP
	URL string // leer = DefaultNVD (oder eigener Spiegel)
	Key string // optionaler API-Schlüssel
	// Wait: Pause zwischen zwei Seiten; 0 = je nach Schlüssel 6 s bzw. 0,7 s.
	Wait time.Duration
}

// NVDCVE: eine CVE, auf das Produkt verkleinert.
type NVDCVE struct {
	ID      string     `json:"id"`
	Score   float64    `json:"score,omitempty"`
	Summary string     `json:"sum,omitempty"`
	KEV     bool       `json:"kev,omitempty"`
	Matches []NVDMatch `json:"m"`
}

// NVDMatch: eine betroffene Version oder ein Bereich.
type NVDMatch struct {
	Version   string `json:"v,omitempty"` // genaue Version ("" = Bereich)
	StartIncl string `json:"si,omitempty"`
	StartExcl string `json:"se,omitempty"`
	EndIncl   string `json:"ei,omitempty"`
	EndExcl   string `json:"ee,omitempty"`
	HW        bool   `json:"hw,omitempty"` // gilt nur für bestimmte Modelle
}

// FirmwareHit: eine CVE, die die Firmware betrifft.
type FirmwareHit struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score,omitempty"`
	Summary string  `json:"sum,omitempty"`
	KEV     bool    `json:"kev,omitempty"`
	FixedIn string  `json:"fixed,omitempty"` // erste Version ohne die Lücke ("" = unbekannt)
	HW      bool    `json:"hw,omitempty"`
}

// Level: Stufe der Lücke.
func (h FirmwareHit) Level() Level { return LevelOf(h.Score) }

// FirmwareReport: Ergebnis für ein Gerät.
type FirmwareReport struct {
	Product  string        `json:"product"` // CPE-Präfix
	Version  string        `json:"version"` // wie verglichen
	Raw      string        `json:"raw"`     // wie das Gerät sie nennt
	Hits     []FirmwareHit `json:"hits"`
	Total    int           `json:"total"` // CVEs zum Produkt in der NVD
	At       time.Time     `json:"at"`
	Stale    bool          `json:"stale,omitempty"`     // NVD nicht erreichbar, älterer Stand
	MinFixed string        `json:"min_fixed,omitempty"` // höchste „behoben ab“ aller Treffer
	URL      string        `json:"url,omitempty"`       // Seite mit Sicherheitsmeldungen des Herstellers
}

// Counts: Treffer je Stufe und aktiv ausgenutzte.
func (r *FirmwareReport) Counts() (c Counts) {
	for _, h := range r.Hits {
		c.Total++
		c.CVEs++
		switch h.Level() {
		case LevelCritical:
			c.Critical++
		case LevelHigh:
			c.High++
		case LevelMedium:
			c.Medium++
		case LevelLow:
			c.Low++
		default:
			c.Unknown++
		}
		if h.KEV {
			c.KEV++
		}
	}
	return c
}

func (n *NVD) url() string {
	if n.URL != "" {
		return n.URL
	}
	return DefaultNVD
}

func (n *NVD) cachePath(product string) string {
	if n.Cache == "" {
		return ""
	}
	return filepath.Join(n.Cache, "nvd", safeName(strings.TrimPrefix(product, "cpe:2.3:"))+".json.gz")
}

// Product: alle CVEs zu einem Produkt (aus dem Cache, wenn jünger als
// DBMaxAge). stale = NVD nicht erreichbar, Stand aus dem Cache.
func (n *NVD) Product(ctx context.Context, product string, step func(string)) (cves []NVDCVE, stale bool, err error) {
	p := n.cachePath(product)
	if p != "" {
		if st, e := os.Stat(p); e == nil && time.Since(st.ModTime()) < DBMaxAge {
			if c, e := readNVDCache(p); e == nil {
				return c, false, nil
			}
		}
	}
	cves, err = n.fetchProduct(ctx, product, step)
	if err != nil {
		if p != "" {
			if c, e := readNVDCache(p); e == nil {
				return c, true, nil
			}
		}
		return nil, false, err
	}
	if p != "" {
		_ = writeNVDCache(p, cves)
	}
	return cves, false, nil
}

type nvdPage struct {
	ResultsPerPage  int `json:"resultsPerPage"`
	StartIndex      int `json:"startIndex"`
	TotalResults    int `json:"totalResults"`
	Vulnerabilities []struct {
		CVE nvdCVE `json:"cve"`
	} `json:"vulnerabilities"`
}

type nvdCVE struct {
	ID             string `json:"id"`
	VulnStatus     string `json:"vulnStatus"`
	CISAExploitAdd string `json:"cisaExploitAdd"`
	Descriptions   []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	} `json:"descriptions"`
	Metrics map[string][]struct {
		Type     string `json:"type"`
		CVSSData struct {
			BaseScore float64 `json:"baseScore"`
		} `json:"cvssData"`
	} `json:"metrics"`
	Configurations []struct {
		Operator string    `json:"operator"`
		Nodes    []nvdNode `json:"nodes"`
	} `json:"configurations"`
}

type nvdNode struct {
	Operator string `json:"operator"`
	Negate   bool   `json:"negate"`
	CPEMatch []struct {
		Vulnerable bool   `json:"vulnerable"`
		Criteria   string `json:"criteria"`
		StartIncl  string `json:"versionStartIncluding"`
		StartExcl  string `json:"versionStartExcluding"`
		EndIncl    string `json:"versionEndIncluding"`
		EndExcl    string `json:"versionEndExcluding"`
	} `json:"cpeMatch"`
}

func (n *NVD) fetchProduct(ctx context.Context, product string, step func(string)) ([]NVDCVE, error) {
	wait := n.Wait
	if wait == 0 {
		wait = 6 * time.Second
		if n.Key != "" {
			wait = 700 * time.Millisecond
		}
	}
	const per = 2000
	var out []NVDCVE
	for start, total := 0, -1; total < 0 || start < total; {
		if start > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		q := url.Values{}
		q.Set("virtualMatchString", product)
		q.Set("resultsPerPage", fmt.Sprint(per))
		q.Set("startIndex", fmt.Sprint(start))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.url()+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "sshtermy")
		if n.Key != "" {
			req.Header.Set("apiKey", n.Key)
		}
		resp, err := n.client().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, &statusError{resp.StatusCode, n.url()}
		}
		var pg nvdPage
		err = json.NewDecoder(io.LimitReader(resp.Body, 512<<20)).Decode(&pg)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		total = pg.TotalResults
		for _, v := range pg.Vulnerabilities {
			if c, ok := reduceNVD(v.CVE, product); ok {
				out = append(out, c)
			}
		}
		if len(pg.Vulnerabilities) == 0 {
			break
		}
		start += len(pg.Vulnerabilities)
		if step != nil && total > 0 {
			step(fmt.Sprintf("NVD %d/%d", min(start, total), total))
		}
	}
	return out, nil
}

// reduceNVD: nur die Angaben zum Produkt behalten.
func reduceNVD(v nvdCVE, product string) (NVDCVE, bool) {
	if strings.EqualFold(v.VulnStatus, "Rejected") {
		return NVDCVE{}, false
	}
	c := NVDCVE{ID: v.ID, KEV: v.CISAExploitAdd != ""}
	for _, d := range v.Descriptions {
		if d.Lang == "en" {
			c.Summary = shorten(d.Value, 300)
			break
		}
	}
	for _, k := range []string{"cvssMetricV40", "cvssMetricV31", "cvssMetricV30", "cvssMetricV2"} {
		ms := v.Metrics[k]
		for _, m := range ms {
			if m.Type == "Primary" || len(ms) == 1 {
				c.Score = m.CVSSData.BaseScore
				break
			}
		}
		if c.Score == 0 && len(ms) > 0 {
			c.Score = ms[0].CVSSData.BaseScore
		}
		if c.Score > 0 {
			break
		}
	}
	pre := strings.ToLower(product) + ":"
	for _, cfg := range v.Configurations {
		hw := strings.EqualFold(cfg.Operator, "AND") && len(cfg.Nodes) > 1
		for _, nd := range cfg.Nodes {
			if nd.Negate {
				continue
			}
			for _, m := range nd.CPEMatch {
				if !m.Vulnerable || !strings.HasPrefix(strings.ToLower(m.Criteria), pre) {
					continue
				}
				f := splitCPE(m.Criteria)
				x := NVDMatch{StartIncl: m.StartIncl, StartExcl: m.StartExcl, EndIncl: m.EndIncl, EndExcl: m.EndExcl, HW: hw}
				if len(f) > 5 && f[5] != "*" && f[5] != "-" && f[5] != "" {
					x.Version = f[5]
					if len(f) > 6 && f[6] != "*" && f[6] != "-" && f[6] != "" {
						x.Version += f[6]
					}
				}
				c.Matches = append(c.Matches, x)
			}
		}
	}
	return c, len(c.Matches) > 0
}

// splitCPE zerlegt eine CPE 2.3 an „:“ (mit \-Maskierung).
func splitCPE(s string) []string {
	var out []string
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case s[i] == ':':
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteByte(s[i])
		}
	}
	return append(out, b.String())
}

func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

var defaultFW = regexp.MustCompile(`(?i)\d+(?:[.\-()a-z0-9]*[0-9a-z)])?`)

// NormFirmware: Firmware-Angabe des Geräts in die Schreibweise der NVD
// bringen. pattern (aus advisory.version) mit einer Gruppe; leer = erste
// Zahlenfolge mit Punkten/Buchstaben.
func NormFirmware(raw, pattern string) string {
	raw = strings.TrimSpace(raw)
	if pattern != "" {
		if re, err := regexp.Compile(pattern); err == nil {
			if m := re.FindStringSubmatch(raw); len(m) > 1 {
				return strings.ToLower(m[1])
			} else if len(m) == 1 {
				return strings.ToLower(m[0])
			}
		}
	}
	return strings.ToLower(defaultFW.FindString(raw))
}

// fwCmp vergleicht Firmware-Versionen (Ziffern- und Buchstabengruppen wie
// rpm, Klammern und Bindestriche trennen nur).
func fwCmp(a, b string) int {
	return CompareRPM(strings.ToLower(a), strings.ToLower(b))
}

// CheckFirmware: welche CVEs die Version betreffen.
func CheckFirmware(cves []NVDCVE, version string, kev KEV) []FirmwareHit {
	var hits []FirmwareHit
	if version == "" {
		return nil
	}
	for _, c := range cves {
		var hit bool
		var fixed string
		hw := true
		for _, m := range c.Matches {
			ok, fx := matchFW(m, version)
			if !ok {
				continue
			}
			hit = true
			hw = hw && m.HW
			if fx != "" && (fixed == "" || fwCmp(fx, fixed) > 0) {
				fixed = fx
			}
		}
		if !hit {
			continue
		}
		h := FirmwareHit{ID: c.ID, Score: c.Score, Summary: c.Summary, KEV: c.KEV, FixedIn: fixed, HW: hw}
		if _, ok := kev[c.ID]; ok {
			h.KEV = true
		}
		hits = append(hits, h)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].KEV != hits[j].KEV {
			return hits[i].KEV
		}
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID > hits[j].ID
	})
	return hits
}

func matchFW(m NVDMatch, v string) (bool, string) {
	if m.Version != "" {
		return fwCmp(m.Version, v) == 0, ""
	}
	if m.StartIncl == "" && m.StartExcl == "" && m.EndIncl == "" && m.EndExcl == "" {
		return false, "" // „alle Versionen“: zu ungenau, nicht melden
	}
	if m.StartIncl != "" && fwCmp(v, m.StartIncl) < 0 {
		return false, ""
	}
	if m.StartExcl != "" && fwCmp(v, m.StartExcl) <= 0 {
		return false, ""
	}
	if m.EndIncl != "" && fwCmp(v, m.EndIncl) > 0 {
		return false, ""
	}
	if m.EndExcl != "" {
		if fwCmp(v, m.EndExcl) >= 0 {
			return false, ""
		}
		return true, m.EndExcl
	}
	return true, ""
}

// FirmwareReportFor: Bericht für ein Gerät aus den CVEs des Produkts.
func FirmwareReportFor(product, raw, pattern string, cves []NVDCVE, kev KEV) *FirmwareReport {
	v := NormFirmware(raw, pattern)
	r := &FirmwareReport{Product: product, Version: v, Raw: raw, Total: len(cves), At: time.Now()}
	r.Hits = CheckFirmware(cves, v, kev)
	for _, h := range r.Hits {
		if h.FixedIn != "" && (r.MinFixed == "" || fwCmp(h.FixedIn, r.MinFixed) > 0) {
			r.MinFixed = h.FixedIn
		}
	}
	return r
}

func readNVDCache(p string) ([]NVDCVE, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var c []NVDCVE
	return c, json.NewDecoder(zr).Decode(&c)
}

func writeNVDCache(p string, c []NVDCVE) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	if err := json.NewEncoder(zw).Encode(c); err != nil {
		zw.Close()
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
