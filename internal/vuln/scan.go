package vuln

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// CVE: eine Lücke in einem Paket.
type CVE struct {
	ID         string   `json:"id"`
	Score      float64  `json:"score,omitempty"`
	KEV        bool     `json:"kev,omitempty"`
	Ransomware bool     `json:"ransomware,omitempty"`
	Text       string   `json:"text,omitempty"`
	Fixed      string   `json:"fixed,omitempty"`
	Advisories []string `json:"advisories,omitempty"`
	Minor      bool     `json:"minor,omitempty"` // Distribution stuft als unwichtig ein
}

// Finding: ein betroffenes Paket mit allen seinen Lücken.
type Finding struct {
	Container string   `json:"container,omitempty"` // "" = Host
	Eco       string   `json:"eco"`
	Package   string   `json:"package"`            // Name wie bei OSV (oft Quellpaket)
	Binaries  []string `json:"binaries,omitempty"` // installierte Pakete dazu
	Installed string   `json:"installed"`
	Fixed     string   `json:"fixed,omitempty"` // behebt alle Lücken mit Fix
	NoFix     int      `json:"no_fix,omitempty"`
	Score     float64  `json:"score,omitempty"`
	KEV       bool     `json:"kev,omitempty"`
	Running   []string `json:"running,omitempty"`
	CVEs      []CVE    `json:"cves"`
	Minor     bool     `json:"minor,omitempty"` // alle Lücken unwichtig
	Format    Format   `json:"format,omitempty"`
}

// Level: Stufe des Pakets (höchste Lücke).
func (f Finding) Level() Level { return LevelOf(f.Score) }

// Key: eindeutig je Host (Container + Paket).
func (f Finding) Key() string { return f.Container + "|" + f.Package }

// Report: Ergebnis einer Prüfung.
type Report struct {
	At         time.Time     `json:"at"`
	Mode       string        `json:"mode"` // online, offline
	System     string        `json:"system"`
	Ecosystems []string      `json:"ecosystems,omitempty"`
	Packages   int           `json:"packages"`
	Containers int           `json:"containers,omitempty"`
	Findings   []Finding     `json:"findings,omitempty"`
	Notes      []string      `json:"notes,omitempty"`
	Took       time.Duration `json:"took,omitempty"`
	Root       bool          `json:"root,omitempty"`
	Format     Format        `json:"format,omitempty"`
}

// Counts: betroffene Pakete je Stufe (ohne unwichtige und ohne skip).
type Counts struct {
	Critical, High, Medium, Low, Unknown, KEV, Total, CVEs int
}

func (r *Report) Count(skip func(Finding) bool) Counts {
	var c Counts
	if r == nil {
		return c
	}
	for _, f := range r.Findings {
		if (f.Minor && !f.KEV) || (skip != nil && skip(f)) {
			continue
		}
		c.Total++
		c.CVEs += len(f.CVEs)
		if f.KEV {
			c.KEV++
		}
		switch f.Level() {
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
	}
	return c
}

// Options für Scan.
type Options struct {
	Step func(string) // Fortschritt
	// Enrich: Schwere fehlt (Alma, SUSE, Alpine …) → bei Debian nachsehen.
	Enrich bool
	Mode   string
}

// Scan prüft eine Paketliste.
func Scan(ctx context.Context, inv *Inventory, src Source, kev KEV, o Options) (*Report, error) {
	if o.Step == nil {
		o.Step = func(string) {}
	}
	start := time.Now()
	r := &Report{At: start, Mode: o.Mode, System: inv.OSName, Packages: len(inv.Packages), Containers: len(inv.Containers), Root: inv.Root, Format: inv.Format}
	if r.System == "" {
		r.System = strings.TrimSpace(inv.OSID + " " + inv.OSVersion)
	}
	r.Notes = append(r.Notes, inv.Notes...)
	type job struct {
		label string
		inv   *Inventory
	}
	jobs := []job{{"", inv}}
	for _, c := range inv.Containers {
		if c.Inv == nil {
			continue
		}
		label := c.Name
		if c.Image != "" {
			label += " (" + c.Image + ")"
		}
		jobs = append(jobs, job{label, c.Inv})
	}
	// Anfragen sammeln
	type qref struct {
		job   int
		key   string // Abfragename
		group string // Anzeige: Quellpaket (rpm: Quell-RPM fasst libmount, libblkid … zusammen)
		eco   string
		ver   string
		bins  []string
		insts []string
	}
	var qs []Query
	var refs []qref
	for ji, j := range jobs {
		ecos := j.inv.Ecosystems()
		if len(j.inv.Packages) == 0 {
			if ji == 0 {
				r.Notes = append(r.Notes, i18n.T("keine Paketliste lesbar (dpkg, rpm oder apk fehlt)"))
			}
			continue
		}
		if len(ecos) == 0 {
			name := j.inv.OSName
			if name == "" {
				name = j.inv.OSID
			}
			if ji == 0 {
				r.Notes = append(r.Notes, i18n.F("%s wird von OSV nicht abgedeckt", orUnknown(name)))
			} else {
				r.Notes = append(r.Notes, i18n.F("Container %s: %s wird von OSV nicht abgedeckt", j.label, orUnknown(name)))
			}
			continue
		}
		if ji == 0 {
			r.Ecosystems = ecos
		}
		for _, eco := range ecos {
			byName := map[string]int{}
			for _, p := range j.inv.Packages {
				n, v := QueryName(eco, p)
				if i, ok := byName[n]; ok {
					refs[i].bins = appendUniq(refs[i].bins, p.Name)
					continue
				}
				byName[n] = len(refs)
				g := n
				if j.inv.Format == FormatRPM && p.Source != "" {
					g = p.Source
				}
				qs = append(qs, Query{Eco: eco, Name: n, Version: v})
				refs = append(refs, qref{job: ji, key: n, group: g, eco: eco, ver: v, bins: []string{p.Name}})
			}
		}
	}
	if len(qs) == 0 {
		r.Took = time.Since(start)
		return r, nil
	}
	o.Step(i18n.F("prüfe %d Pakete gegen OSV …", len(qs)))
	res, err := src.Find(ctx, qs, o.Step)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*Finding{}
	var order []string
	for i, vs := range res {
		ref := refs[i]
		j := jobs[ref.job]
		for _, v := range vs {
			h := v.Check(ref.eco, ref.key, ref.ver, j.inv.Format)
			if !h.Affected {
				continue
			}
			k := j.label + "|" + ref.group
			f := byKey[k]
			if f == nil {
				f = &Finding{Container: j.label, Eco: ref.eco, Package: ref.group, Installed: ref.ver, Format: j.inv.Format}
				byKey[k] = f
				order = append(order, k)
			}
			for _, b := range ref.bins {
				if !contains(f.Binaries, b) {
					f.Binaries = append(f.Binaries, b)
					for _, x := range j.inv.Running[b] {
						f.Running = appendUniq(f.Running, x)
					}
				}
			}
			score, _ := v.Score(ref.eco, ref.key)
			minor := v.minor(ref.eco, ref.key)
			ids := v.CVEs()
			if len(ids) == 0 {
				ids = []string{v.ID}
			}
			for _, id := range ids {
				f.addCVE(CVE{ID: id, Score: score, Text: v.Text(), Fixed: h.Fixed, Advisories: []string{v.ID}, Minor: minor}, j.inv.Format)
			}
		}
	}
	// Schwere nachtragen, wo der Eintrag keine hat (über Debians Eintrag zur CVE)
	if o.Enrich {
		enrich(ctx, src, byKey, o.Step)
	}
	for _, k := range order {
		f := byKey[k]
		f.finish(kev)
		r.Findings = append(r.Findings, *f)
	}
	sortFindings(r.Findings)
	r.Took = time.Since(start)
	return r, nil
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return i18n.T("Das System")
	}
	return s
}

func appendUniq(xs []string, s string) []string {
	if contains(xs, s) {
		return xs
	}
	return append(xs, s)
}

// minor: stuft die Distribution die Lücke als unwichtig ein (Debian)?
func (v *Vuln) minor(eco, name string) bool {
	for _, a := range v.Affected {
		if a.Package.Ecosystem != eco || a.Package.Name != name || len(a.EcosystemSpecific) == 0 {
			continue
		}
		var es struct {
			Urgency string `json:"urgency"`
		}
		if json.Unmarshal(a.EcosystemSpecific, &es) == nil {
			switch strings.ToLower(es.Urgency) {
			case "unimportant", "end-of-life":
				return true
			}
		}
	}
	return false
}

func (f *Finding) addCVE(c CVE, fm Format) {
	for i := range f.CVEs {
		x := &f.CVEs[i]
		if x.ID != c.ID {
			continue
		}
		x.Score = max(x.Score, c.Score)
		x.Minor = x.Minor && c.Minor
		for _, a := range c.Advisories {
			x.Advisories = appendUniq(x.Advisories, a)
		}
		if c.Fixed != "" && (x.Fixed == "" || Compare(fm, c.Fixed, x.Fixed) > 0) {
			x.Fixed = c.Fixed
		}
		if x.Text == "" {
			x.Text = c.Text
		}
		return
	}
	f.CVEs = append(f.CVEs, c)
}

// finish: KEV, höchste Stufe, gemeinsame behobene Version, Sortierung.
func (f *Finding) finish(kev KEV) {
	f.Score, f.KEV, f.Fixed, f.NoFix, f.Minor = 0, false, "", 0, true
	for i := range f.CVEs {
		c := &f.CVEs[i]
		if e, ok := kev[c.ID]; ok {
			c.KEV = true
			c.Ransomware = strings.EqualFold(e.Ransomware, "Known")
			c.Minor = false
		}
		f.KEV = f.KEV || c.KEV
		if !c.Minor {
			f.Minor = false
			f.Score = max(f.Score, c.Score)
		}
		if c.Fixed == "" {
			f.NoFix++
		} else if f.Fixed == "" || Compare(f.Format, c.Fixed, f.Fixed) > 0 {
			f.Fixed = c.Fixed
		}
	}
	if f.Minor {
		for _, c := range f.CVEs {
			f.Score = max(f.Score, c.Score)
		}
	}
	sort.SliceStable(f.CVEs, func(i, j int) bool {
		a, b := f.CVEs[i], f.CVEs[j]
		if a.KEV != b.KEV {
			return a.KEV
		}
		if a.Minor != b.Minor {
			return !a.Minor
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.ID > b.ID
	})
}

// Rank: Sortierschlüssel (aktiv ausgenutzt › Stufe › Fix vorhanden › läuft).
func (f Finding) Rank() int {
	n := int(f.Level()) * 1000
	if f.KEV {
		n += 100000
	}
	if f.Minor && !f.KEV {
		n -= 10000
	}
	if f.Fixed != "" {
		n += 100
	}
	if len(f.Running) > 0 {
		n += 10
	}
	if f.Container == "" {
		n++
	}
	return n
}

func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Rank() != b.Rank() {
			return a.Rank() > b.Rank()
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Container != b.Container {
			return a.Container < b.Container
		}
		return a.Package < b.Package
	})
}

// enrich: CVEs ohne Schwere über Debians Eintrag DEBIAN-CVE-… nachtragen
// (Debian übernimmt den CVSS-Vektor von NVD). Höchstens 300 Abfragen.
func enrich(ctx context.Context, src Source, fs map[string]*Finding, step func(string)) {
	need := map[string]bool{}
	for _, f := range fs {
		for _, c := range f.CVEs {
			if c.Score == 0 && strings.HasPrefix(c.ID, "CVE-") {
				need[c.ID] = true
			}
		}
	}
	if len(need) == 0 {
		return
	}
	var ids []string
	for id := range need {
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids))) // neueste zuerst
	if len(ids) > 300 {
		ids = ids[:300]
	}
	step(i18n.F("Schwere für %d Lücken nachschlagen …", len(ids)))
	scores := map[string]float64{}
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			v := src.Lookup(ctx, "Debian", "DEBIAN-"+id)
			if v == nil {
				return
			}
			if s, _ := v.Score("", ""); s > 0 {
				mu.Lock()
				scores[id] = s
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for _, f := range fs {
		for i := range f.CVEs {
			if s := scores[f.CVEs[i].ID]; s > 0 && f.CVEs[i].Score == 0 {
				f.CVEs[i].Score = s
			}
		}
	}
}

// UpdateCommand: Befehl, der die betroffenen Pakete auf dem Host
// aktualisiert. Container werden nicht in sich aktualisiert (das Image
// käme beim nächsten Start alt zurück): dann bleibt cmd leer.
func UpdateCommand(f Finding) string {
	if f.Container != "" || len(f.Binaries) == 0 {
		return ""
	}
	bins := strings.Join(f.Binaries, " ")
	switch Base(f.Eco) {
	case "Debian", "Ubuntu":
		return "sudo apt-get update && sudo apt-get install --only-upgrade " + bins
	case "Red Hat", "Rocky Linux", "AlmaLinux":
		return "sudo dnf upgrade " + bins
	case "SUSE", "openSUSE":
		return "sudo zypper update " + bins
	case "Alpine":
		return "sudo apk upgrade " + bins
	}
	return ""
}
