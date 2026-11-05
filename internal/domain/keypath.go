package domain

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Schlüsseldateien liegen auf jedem Rechner woanders (C:\Users\… unter
// Windows, /Users/… am Mac). Damit ein Sync den Pfad nicht unbrauchbar macht:
//
//   - Pfade unter dem eigenen Home werden als „~/…“ gespeichert, das passt
//     auf jedem Rechner, wenn der Key dort am gleichen Platz liegt.
//   - Wählt jemand auf einem anderen Rechner einen anderen Pfad, landet der
//     nur lokal (Data.LocalKeys), der Ursprungsrechner behält seinen.
//   - Fehlt die Datei trotzdem, probiert sshx naheliegende Orte
//     (KeyPathCandidates) und fragt sonst nach dem Key auf diesem Rechner.

// ExpandKeyPath ersetzt ein führendes „~“ durch das Home-Verzeichnis.
func ExpandKeyPath(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, filepath.FromSlash(strings.ReplaceAll(p[2:], `\`, "/")))
	}
	return p
}

// PortableKeyPath schreibt einen Pfad unter home als „~/…“ (mit „/“), damit
// er auf jedem Rechner passt. Andere Pfade bleiben, wie sie sind.
func PortableKeyPath(p, home string) string {
	p = strings.TrimSpace(p)
	if p == "" || home == "" || strings.HasPrefix(p, "~") || !filepath.IsAbs(p) {
		return p
	}
	cp, ch := filepath.Clean(p), filepath.Clean(home)
	eq := func(a, b string) bool { return a == b }
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		eq = strings.EqualFold // Dateisysteme dort ohne Groß/Klein-Unterschied
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(ch, sep) {
		ch += sep
	}
	if len(cp) <= len(ch) || !eq(cp[:len(ch)], ch) {
		return p
	}
	return "~/" + filepath.ToSlash(cp[len(ch):])
}

// foreignHome: Home-Verzeichnis eines anderen Rechners (Windows, macOS, Linux).
var foreignHome = regexp.MustCompile(`(?i)^(?:[a-z]:)?/(?:users|home)/[^/]+/(.+)$|^/root/(.+)$`)

// KeyPathCandidates: wo der Key auf diesem Rechner liegen könnte, der Reihe
// nach. Zuerst der gespeicherte Pfad, dann derselbe Pfad relativ zum eigenen
// Home (C:\Users\a\.ssh\id → ~/.ssh/id), zuletzt ~/.ssh/<Dateiname>.
func KeyPathCandidates(p, home string) []string {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil
	}
	var out []string
	add := func(x string) {
		for _, o := range out {
			if o == x {
				return
			}
		}
		out = append(out, x)
	}
	add(ExpandKeyPath(p, home))
	if home == "" {
		return out
	}
	slash := strings.ReplaceAll(p, `\`, "/")
	if m := foreignHome.FindStringSubmatch(slash); m != nil {
		rest := m[1] + m[2]
		add(filepath.Join(home, filepath.FromSlash(rest)))
	}
	if i := strings.LastIndex(slash, "/"); i >= 0 && i < len(slash)-1 {
		add(filepath.Join(home, ".ssh", slash[i+1:]))
	}
	return out
}
