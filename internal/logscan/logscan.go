// Package logscan liest die Logs des Systems, auf dem ein Tab gerade ist
// (Linux/BSD-Server, Windows über WinRM, Switches und Firewalls über die
// sichtbare Shell), fasst gleiche Meldungen zu Mustern zusammen und ordnet
// sie nach Schwere. Alles läuft lokal und ohne KI; die KI bekommt auf Wunsch
// nur die verdichteten Muster (Brief).
package logscan

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Sev: Schwere einer Meldung.
type Sev int

const (
	SevInfo Sev = iota
	SevWarn
	SevErr
	SevCrit
)

// Label: kurze Anzeige (KRIT, FEHL, WARN, INFO).
func (s Sev) Label() string {
	switch s {
	case SevCrit:
		return i18n.T("KRIT")
	case SevErr:
		return i18n.T("FEHL")
	case SevWarn:
		return "WARN"
	}
	return "INFO"
}

// Kind: woher die Logs kommen.
type Kind string

const (
	KindUnix    Kind = "unix"    // Linux/BSD per Exec
	KindWindows Kind = "windows" // PowerShell über WinRM
	KindNetwork Kind = "network" // Switch/Firewall über die sichtbare Shell
)

// Entry: eine Logzeile.
type Entry struct {
	Time   time.Time // Null = unbekannt (Gerät ohne Uhrzeit)
	Sev    Sev
	Source string // Dienst/Prozess/Ereignisquelle
	Msg    string // Text ohne Zeitstempel und Quelle
	ID     string // Ereignis-ID (Windows) oder Kennung (Cisco %LINK-3-UPDOWN)
	key    string
}

// MarkKind: besondere Zeitpunkte für die Zeitleiste.
type MarkKind string

const (
	MarkBoot   MarkKind = "boot"   // Neustart
	MarkPkg    MarkKind = "pkg"    // Pakete/Updates installiert
	MarkConfig MarkKind = "config" // Konfiguration geändert (Switch, Firewall)
	MarkCmd    MarkKind = "cmd"    // eigener Befehl (Sitzungsprotokoll)
)

// Mark: ein besonderer Zeitpunkt.
type Mark struct {
	Time time.Time
	Kind MarkKind
	Text string
}

// Finding: ein Muster (gleiche Meldungen zusammengefasst).
type Finding struct {
	Key      string
	Sev      Sev
	Source   string
	Title    string // Anzeige
	Sample   string // eine echte Zeile
	Count    int
	First    time.Time
	Last     time.Time
	Buckets  []int
	Hint     string
	Checks   []string
	Trend    string // "up" steigt, "always" immer da
	Status   string // "new", "more" (siehe Compare)
	Factor   int    // bei "more": so viel häufiger
	Noise    bool   // als Rauschen ausgeblendet
	Ports    []string
	rule     *Rule
	nonZero  int
	firstSet bool
}

// Port: Zustand eines Switch-Ports (für die Portleiste).
type Port struct {
	Name string
	Up   bool
	Off  bool // abgeschaltet
}

// Report: Ergebnis einer Prüfung.
type Report struct {
	Kind     Kind
	Host     string
	System   string
	At       time.Time
	Hours    int
	Lines    int
	Took     time.Duration
	Read     []string // was gelesen wurde (Anzeige)
	Entries  []Entry  // für Zeitleiste und Zeilenansicht (neueste zuletzt)
	Findings []Finding
	Marks    []Mark
	Notes    []string
	Ports    []Port
	Boot     time.Time
}

// MaxEntries: so viele Zeilen hebt ein Bericht auf.
const MaxEntries = 8000

// Buckets: Säulen im Verlauf je Muster.
const Buckets = 24

// Since: Beginn des Zeitraums.
func (r *Report) Since() time.Time { return r.At.Add(-time.Duration(r.Hours) * time.Hour) }

// Counts: Muster je Schwere (ohne ausgeblendete).
func (r *Report) Counts() (crit, err, warn int) {
	for _, f := range r.Findings {
		if f.Noise {
			continue
		}
		switch f.Sev {
		case SevCrit:
			crit++
		case SevErr:
			err++
		case SevWarn:
			warn++
		}
	}
	return
}

// Worst: höchste Schwere (ohne ausgeblendete), -1 ohne Muster.
func (r *Report) Worst() Sev {
	w := Sev(-1)
	for _, f := range r.Findings {
		if !f.Noise && f.Sev > w {
			w = f.Sev
		}
	}
	return w
}

// Lines einer Gruppe (für die Zeilenansicht).
func (r *Report) LinesOf(key string) []Entry {
	var out []Entry
	for _, e := range r.Entries {
		if e.key == key {
			out = append(out, e)
		}
	}
	return out
}

// EntriesBetween: Zeilen im Zeitfenster [from, to).
func (r *Report) EntriesBetween(from, to time.Time) []Entry {
	var out []Entry
	for _, e := range r.Entries {
		if !e.Time.IsZero() && !e.Time.Before(from) && e.Time.Before(to) {
			out = append(out, e)
		}
	}
	return out
}

// ---------------------------------------------------------------- Muster bilden

var (
	reIPv4  = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?\b`)
	reIPv6  = regexp.MustCompile(`\b[0-9a-fA-F]{1,4}(:[0-9a-fA-F]{0,4}){3,7}\b`)
	reMAC   = regexp.MustCompile(`\b[0-9a-fA-F]{2}([:-][0-9a-fA-F]{2}){5}\b|\b[0-9a-fA-F]{4}\.[0-9a-fA-F]{4}\.[0-9a-fA-F]{4}\b|\b[0-9a-fA-F]{6}-[0-9a-fA-F]{6}\b`)
	reUUID  = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	reHex   = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b|\b[0-9a-fA-F]*\d[0-9a-fA-F]*[a-fA-F][0-9a-fA-F]{6,}\b|\b[0-9a-fA-F]*[a-fA-F][0-9a-fA-F]*\d[0-9a-fA-F]{6,}\b`)
	reNum   = regexp.MustCompile(`\b\d+([.,]\d+)?\b`)
	reUser  = regexp.MustCompile(`(?i)\b(for (?:invalid user )?|user[= ]|benutzer[= ])("?)[^\s"]+`)
	reSpace = regexp.MustCompile(`\s+`)
	reKTime = regexp.MustCompile(`^\[\s*\d+\.\d+\]\s*`)
)

// Template: Meldung ohne wechselnde Teile (Zahlen, Adressen, Benutzer).
func Template(msg string) string {
	t := reKTime.ReplaceAllString(msg, "")
	t = reUUID.ReplaceAllString(t, "‹id›")
	t = reMAC.ReplaceAllString(t, "‹mac›")
	t = reIPv4.ReplaceAllString(t, "‹ip›")
	t = reIPv6.ReplaceAllString(t, "‹ip›")
	t = reHex.ReplaceAllString(t, "‹hex›")
	t = reUser.ReplaceAllString(t, "${1}${2}‹user›")
	t = reNum.ReplaceAllString(t, "‹n›")
	t = strings.TrimSpace(reSpace.ReplaceAllString(t, " "))
	if r := []rune(t); len(r) > 160 {
		t = string(r[:159]) + "…"
	}
	return t
}

// Group fasst Zeilen zu Mustern zusammen: Regeln aus dem Katalog zuerst,
// sonst gleiche Vorlage aus gleicher Quelle. Zeilen unter Warnung zählen
// nur, wenn eine Regel sie kennt.
func Group(r *Report) {
	since, at := r.Since(), r.At
	span := at.Sub(since)
	byKey := map[string]*Finding{}
	var order []string
	for i := range r.Entries {
		e := &r.Entries[i]
		f := classify(r.Kind, e)
		if f == nil {
			continue
		}
		e.key = f.Key
		g := byKey[f.Key]
		if g == nil {
			g = f
			g.Buckets = make([]int, Buckets)
			byKey[f.Key] = g
			order = append(order, f.Key)
		}
		g.Count++
		if e.Sev > g.Sev && (g.rule == nil || g.rule.Sev < 0) {
			g.Sev = e.Sev
		}
		if !e.Time.IsZero() {
			if !g.firstSet || e.Time.Before(g.First) {
				g.First, g.firstSet = e.Time, true
			}
			if e.Time.After(g.Last) {
				g.Last = e.Time
				g.Sample = e.raw()
			}
			if span > 0 && !e.Time.Before(since) {
				b := int(float64(e.Time.Sub(since)) / float64(span) * Buckets)
				g.Buckets[clampInt(b, 0, Buckets-1)]++
			}
		} else if g.Sample == "" || g.Last.IsZero() {
			g.Sample = e.raw()
		}
		for _, p := range portsIn(e.Msg) {
			if !contains(g.Ports, p) && len(g.Ports) < 8 {
				g.Ports = append(g.Ports, p)
			}
		}
	}
	r.Findings = r.Findings[:0]
	for _, k := range order {
		f := byKey[k]
		finish(f)
		r.Findings = append(r.Findings, *f)
	}
	sortFindings(r.Findings)
}

func (e Entry) raw() string {
	t := e.Msg
	if e.Source != "" {
		t = e.Source + ": " + t
	}
	if !e.Time.IsZero() {
		t = e.Time.Local().Format("02.01. 15:04:05") + "  " + t
	}
	return t
}

// Line: Zeile für die Anzeige (Zeit, Quelle, Text).
func (e Entry) Line() string { return e.raw() }

// Key: zu welchem Muster die Zeile gehört.
func (e Entry) Key() string { return e.key }

func finish(f *Finding) {
	for _, n := range f.Buckets {
		if n > 0 {
			f.nonZero++
		}
	}
	half := Buckets / 2
	var a, b int
	for i, n := range f.Buckets {
		if i < half {
			a += n
		} else {
			b += n
		}
	}
	switch {
	case f.nonZero >= Buckets*3/4:
		f.Trend = "always"
	case f.nonZero >= 4 && b >= 2*a+2 && a > 0:
		f.Trend = "up"
	}
	if f.rule != nil && f.rule.after != nil {
		f.rule.after(f)
	}
}

func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Noise != b.Noise {
			return !a.Noise
		}
		if a.Sev != b.Sev {
			return a.Sev > b.Sev
		}
		if (a.Status == "new") != (b.Status == "new") {
			return a.Status == "new"
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Last.After(b.Last)
	})
}

// Resort: nach Rauschen/Neu-Markierung neu ordnen.
func (r *Report) Resort() { sortFindings(r.Findings) }

func classify(k Kind, e *Entry) *Finding {
	if rl, m := match(k, e); rl != nil {
		f := &Finding{rule: rl, Source: e.Source, Sev: e.Sev}
		if rl.Sev >= 0 {
			f.Sev = rl.Sev
		} else if rl.Down != nil && f.Sev < SevWarn && rl.Down.MatchString(e.Msg) {
			f.Sev = SevWarn
			e.Sev = SevWarn
		}
		f.Key = k.prefix() + expand(rl.Key, m)
		f.Title = expand(i18n.T(rl.Title), m)
		f.Hint = i18n.T(rl.Hint)
		for _, c := range rl.Checks {
			f.Checks = append(f.Checks, expand(c, m))
		}
		return f
	}
	if e.Sev < SevWarn {
		return nil
	}
	t := Template(e.Msg)
	if e.ID != "" && k == KindWindows {
		t = e.ID + " " + t
	}
	src := e.Source
	f := &Finding{Source: src, Sev: e.Sev, Title: Template(e.Msg)}
	f.Key = k.prefix() + strings.ToLower(src) + "|" + t
	f.Checks = defaultChecks(k, e)
	return f
}

func (k Kind) prefix() string { return string(k) + ":" }

// expand ersetzt $1 … $9 durch Gruppen der Regel.
func expand(s string, m []string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	for i := len(m) - 1; i >= 0; i-- {
		s = strings.ReplaceAll(s, fmt.Sprintf("$%d", i), m[i])
	}
	return s
}

var rePort = regexp.MustCompile(`(?i)\b(?:port|interface|ethernet|ether|gi(?:gabitethernet)?|te(?:ngigabitethernet)?|fa(?:stethernet)?|ge-|xe-)\s*([0-9]+(?:[/:][0-9]+){0,3}|[A-Z]?\d+)\b`)

func portsIn(msg string) []string {
	var out []string
	for _, m := range rePort.FindAllStringSubmatch(msg, 4) {
		out = append(out, m[1])
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------- Für die KI

// Brief: verdichtete Fassung für die KI (Muster mit Anzahl und Zeit statt
// tausender Zeilen). Ausgeblendetes Rauschen bleibt draußen.
func (r *Report) Brief(max int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "System: %s (%s), Zeitraum: letzte %d h, %d Zeilen gelesen.\n", r.Host, r.System, r.Hours, r.Lines)
	if !r.Boot.IsZero() {
		fmt.Fprintf(&b, "Letzter Start: %s\n", r.Boot.Local().Format("02.01. 15:04"))
	}
	for _, m := range r.Marks {
		if m.Kind == MarkCmd {
			continue
		}
		fmt.Fprintf(&b, "Ereignis %s: %s\n", m.Time.Local().Format("02.01. 15:04"), m.Text)
	}
	b.WriteString("Muster (gleiche Meldungen zusammengefasst, wichtigste zuerst):\n")
	n := 0
	for _, f := range r.Findings {
		if f.Noise {
			continue
		}
		if n >= max {
			break
		}
		n++
		when := ""
		if !f.First.IsZero() {
			when = fmt.Sprintf(", erstmals %s, zuletzt %s", f.First.Local().Format("02.01. 15:04"), f.Last.Local().Format("02.01. 15:04"))
		}
		extra := ""
		switch {
		case f.Status == "new":
			extra = ", neu seit der letzten Prüfung"
		case f.Status == "more":
			extra = fmt.Sprintf(", %d× häufiger als bei der letzten Prüfung", f.Factor)
		}
		if f.Trend == "up" {
			extra += ", steigend"
		}
		fmt.Fprintf(&b, "- [%s] %d× %s (Quelle %s%s%s)\n  Beispiel: %s\n", f.Sev.Label(), f.Count, f.Title, orDash(f.Source), when, extra, f.Sample)
	}
	if n == 0 {
		b.WriteString("- keine Auffälligkeiten\n")
	}
	for _, x := range r.Notes {
		b.WriteString("Hinweis: " + x + "\n")
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
