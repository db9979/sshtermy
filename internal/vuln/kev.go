package vuln

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// KEV: Liste der US-Behörde CISA mit Lücken, die nachweislich aktiv
// ausgenutzt werden („Known Exploited Vulnerabilities“). Öffentlich, wird
// einmal am Tag geladen; beim Prüfen geht nichts nach außen.

var KEVURLs = []string{
	"https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json",
	"https://raw.githubusercontent.com/cisagov/kev-data/develop/known_exploited_vulnerabilities.json",
}

// KEVEntry: ein Eintrag der Liste.
type KEVEntry struct {
	CVE        string `json:"cveID"`
	Name       string `json:"vulnerabilityName"`
	Added      string `json:"dateAdded"`
	Due        string `json:"dueDate"`
	Action     string `json:"requiredAction"`
	Ransomware string `json:"knownRansomwareCampaignUse"`
}

// KEV: CVE → Eintrag.
type KEV map[string]KEVEntry

// LoadKEV: Liste aus dem Cache (jünger als ein Tag) oder neu laden. Schlägt
// das Laden fehl, gilt der alte Stand; ohne Stand eine leere Liste.
func LoadKEV(ctx context.Context, h *HTTP) (KEV, error) {
	p := ""
	if h.Cache != "" {
		p = filepath.Join(h.Cache, "kev.json")
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < DBMaxAge {
			if k, err := readKEV(p); err == nil {
				return k, nil
			}
		}
	}
	var lastErr error
	for _, u := range KEVURLs {
		rc, _, err := h.get(ctx, u)
		if err != nil {
			lastErr = err
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(rc, 32<<20))
		rc.Close()
		if err != nil {
			lastErr = err
			continue
		}
		k, err := parseKEV(raw)
		if err != nil {
			lastErr = err
			continue
		}
		if p != "" {
			_ = os.MkdirAll(filepath.Dir(p), 0o700)
			_ = os.WriteFile(p, raw, 0o600)
		}
		return k, nil
	}
	if p != "" {
		if k, err := readKEV(p); err == nil {
			return k, nil
		}
	}
	return KEV{}, lastErr
}

func readKEV(p string) (KEV, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return parseKEV(raw)
}

func parseKEV(raw []byte) (KEV, error) {
	var doc struct {
		Vulnerabilities []KEVEntry `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	k := KEV{}
	for _, v := range doc.Vulnerabilities {
		if v.CVE != "" {
			k[v.CVE] = v
		}
	}
	return k, nil
}
