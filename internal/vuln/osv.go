// Package vuln prüft installierte Pakete eines Servers gegen die offene
// Schwachstellen-Datenbank OSV (osv.dev): Paketliste lesen (inventory.go),
// passende Einträge holen – online über die OSV-API oder aus einer lokal
// geladenen Datenbank (source.go) –, Versionen nach den Regeln von dpkg, rpm
// und apk vergleichen (version.go), mit der CISA-Liste aktiv ausgenutzter
// Lücken (kev.go) gewichten und je Paket zusammenfassen (scan.go).
// Das Paket kennt weder SSH noch die Oberfläche.
package vuln

import (
	"encoding/json"
	"strings"
	"time"
)

// Format: Paketformat und damit die Regeln für den Versionsvergleich.
type Format string

const (
	FormatDeb Format = "deb"
	FormatRPM Format = "rpm"
	FormatAPK Format = "apk"
)

// Vuln: der Teil eines OSV-Eintrags, den sshTermy braucht.
type Vuln struct {
	ID         string     `json:"id"`
	Modified   time.Time  `json:"modified"`
	Published  time.Time  `json:"published,omitempty"`
	Withdrawn  *time.Time `json:"withdrawn,omitempty"`
	Aliases    []string   `json:"aliases,omitempty"`
	Upstream   []string   `json:"upstream,omitempty"`
	Related    []string   `json:"related,omitempty"`
	Summary    string     `json:"summary,omitempty"`
	Details    string     `json:"details,omitempty"`
	Affected   []Affected `json:"affected,omitempty"`
	Severity   []Severity `json:"severity,omitempty"`
	References []Ref      `json:"references,omitempty"`
}

// Affected: ein betroffenes Paket in einem Ökosystem.
type Affected struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Ranges            []Range         `json:"ranges,omitempty"`
	Versions          []string        `json:"versions,omitempty"`
	Severity          []Severity      `json:"severity,omitempty"`
	EcosystemSpecific json.RawMessage `json:"ecosystem_specific,omitempty"`
}

// Range: Versionsbereich aus Ereignissen (introduced, fixed, last_affected).
type Range struct {
	Type   string  `json:"type"`
	Events []Event `json:"events"`
}

type Event struct {
	Introduced   string `json:"introduced,omitempty"`
	Fixed        string `json:"fixed,omitempty"`
	LastAffected string `json:"last_affected,omitempty"`
	Limit        string `json:"limit,omitempty"`
}

// Severity: CVSS-Vektor oder Einstufung der Distribution (Ubuntu).
type Severity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type Ref struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Hit: Ergebnis der Prüfung eines Eintrags gegen ein installiertes Paket.
type Hit struct {
	Affected bool
	Fixed    string // kleinste behobene Version über der installierten ("" = kein Fix bekannt)
}

// Check: ist das Paket name in Version ver im Ökosystem eco betroffen?
func (v *Vuln) Check(eco, name, ver string, f Format) Hit {
	if v.Withdrawn != nil {
		return Hit{}
	}
	var h Hit
	for _, a := range v.Affected {
		if a.Package.Ecosystem != eco || a.Package.Name != name {
			continue
		}
		hit := false
		for _, x := range a.Versions {
			if x == ver || Compare(f, x, ver) == 0 {
				hit = true
				break
			}
		}
		for _, r := range a.Ranges {
			if r.Type != "ECOSYSTEM" {
				continue
			}
			if in, fixed := inRange(r.Events, ver, f); in {
				hit = true
				if fixed != "" && (h.Fixed == "" || Compare(f, fixed, h.Fixed) > 0) {
					h.Fixed = fixed
				}
			}
		}
		if hit {
			h.Affected = true
			if h.Fixed == "" {
				h.Fixed = fixedAbove(a.Ranges, ver, f)
			}
		}
	}
	return h
}

// inRange: liegt ver in einem der Abschnitte? Die Ereignisse werden wie in
// der OSV-Spezifikation nach Version sortiert abgelaufen.
func inRange(evs []Event, ver string, f Format) (bool, string) {
	type pt struct {
		v    string
		kind int // 0 introduced, 1 fixed, 2 last_affected
	}
	var pts []pt
	for _, e := range evs {
		switch {
		case e.Introduced != "":
			pts = append(pts, pt{e.Introduced, 0})
		case e.Fixed != "":
			pts = append(pts, pt{e.Fixed, 1})
		case e.LastAffected != "":
			pts = append(pts, pt{e.LastAffected, 2})
		}
	}
	less := func(a, b string) bool {
		if a == "0" {
			return b != "0"
		}
		if b == "0" {
			return false
		}
		return Compare(f, a, b) < 0
	}
	// Einfügesortierung: Bereiche haben nur wenige Ereignisse
	for i := 1; i < len(pts); i++ {
		for j := i; j > 0 && less(pts[j].v, pts[j-1].v); j-- {
			pts[j], pts[j-1] = pts[j-1], pts[j]
		}
	}
	in, fixed := false, ""
	for _, p := range pts {
		switch p.kind {
		case 0:
			if p.v == "0" || Compare(f, ver, p.v) >= 0 {
				in, fixed = true, ""
			}
		case 1:
			if Compare(f, ver, p.v) >= 0 {
				in = false
			} else if in && fixed == "" {
				fixed = p.v
			}
		case 2:
			if Compare(f, ver, p.v) > 0 {
				in = false
			}
		}
	}
	return in, fixed
}

func fixedAbove(rs []Range, ver string, f Format) string {
	best := ""
	for _, r := range rs {
		for _, e := range r.Events {
			if e.Fixed != "" && Compare(f, e.Fixed, ver) > 0 && (best == "" || Compare(f, e.Fixed, best) < 0) {
				best = e.Fixed
			}
		}
	}
	return best
}

// CVEs: CVE-Kennungen des Eintrags (Kennung selbst, upstream, aliases).
func (v *Vuln) CVEs() []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if i := strings.Index(s, "CVE-"); i >= 0 {
			s = s[i:]
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	if strings.HasPrefix(v.ID, "CVE-") || strings.Contains(v.ID, "-CVE-") {
		add(v.ID)
	}
	for _, s := range v.Upstream {
		add(s)
	}
	for _, s := range v.Aliases {
		add(s)
	}
	return out
}

// Text: kurze Beschreibung (Summary, sonst erster Satz der Details).
func (v *Vuln) Text() string {
	if s := strings.TrimSpace(v.Summary); s != "" {
		return s
	}
	d := strings.Join(strings.Fields(v.Details), " ")
	if i := strings.Index(d, ". "); i > 0 && i < 300 {
		return d[:i+1]
	}
	if len(d) > 300 {
		return d[:297] + "…"
	}
	return d
}

// Score: höchste Schwere aus dem Eintrag (0 = unbekannt).
func (v *Vuln) Score(eco, name string) (float64, string) {
	best, src := 0.0, ""
	consider := func(ss []Severity) {
		for _, s := range ss {
			if sc, ok := ScoreOf(s); ok && sc > best {
				best, src = sc, s.Type
			}
		}
	}
	consider(v.Severity)
	for _, a := range v.Affected {
		if a.Package.Ecosystem == eco && a.Package.Name == name {
			consider(a.Severity)
		}
	}
	return best, src
}
