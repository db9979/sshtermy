// Package changelog: „Was ist neu“ – die Änderungen je Version, eingebaut in
// die Programmdatei.
//
// CHANGELOG.md erzeugt die Release-Pipeline bei jedem Release neu aus den
// gemergten Pull Requests (tools/changelog). Die Datei im Repo ist nur der
// Stand für lokale Builds.
package changelog

import (
	_ "embed"
	"strings"
)

//go:embed CHANGELOG.md
var raw string

// Entry: eine Version mit ihren Neuerungen.
type Entry struct {
	Version string // ohne "v"
	Date    string // z. B. "05.11.2026"
	Items   []string
}

// All: alle Einträge, neueste zuerst.
func All() []Entry { return Parse(raw) }

// Parse liest das Format, das tools/changelog schreibt:
//
//	## v1.0.42 · 05.11.2026
//	- Neuerung
func Parse(s string) []Entry {
	var out []Entry
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "## "):
			h := strings.TrimSpace(t[3:])
			e := Entry{}
			v, d, _ := strings.Cut(h, "·")
			e.Version = strings.TrimPrefix(strings.TrimSpace(v), "v")
			e.Date = strings.TrimSpace(d)
			out = append(out, e)
		case len(out) > 0 && (strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ")):
			out[len(out)-1].Items = append(out[len(out)-1].Items, strings.TrimSpace(t[2:]))
		case len(out) > 0 && t != "" && len(out[len(out)-1].Items) > 0 && strings.HasPrefix(l, "  "):
			// Fortsetzungszeile eines Punkts
			last := &out[len(out)-1].Items[len(out[len(out)-1].Items)-1]
			*last += " " + t
		}
	}
	return out
}

// Since: Einträge, die neuer als „seen“ sind (bis einschließlich „upto“),
// neueste zuerst. newer(a, b) vergleicht Versionen (a > b).
func Since(all []Entry, seen, upto string, newer func(a, b string) bool) []Entry {
	var out []Entry
	for _, e := range all {
		if newer(e.Version, upto) {
			continue
		}
		if seen != "" && !newer(e.Version, seen) {
			continue
		}
		out = append(out, e)
	}
	return out
}
