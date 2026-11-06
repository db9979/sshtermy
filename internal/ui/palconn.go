package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Verbindungen in der Befehlspalette. Bei vielen Servern würde die Liste
// sonst alles andere verdrängen, deshalb:
//
//   - ohne Text: nur offene und zuletzt benutzte Verbindungen (palRecent),
//     darunter eine Zeile „alle N im Manager“,
//   - mit Text: unscharfe Suche über Name, Host, Ordner und Tags, die besten
//     Treffer zuerst (palMaxHits), Rest über „weitere im Manager“.
//
// Jede Zeile zeigt Gerätesymbol, Name, Ordner (gedimmt) und Ziel.

const (
	palRecent  = 5
	palMaxHits = 8
)

// palConnScore bewertet, wie gut q zur Verbindung passt (0 = gar nicht).
// Mehrere Wörter müssen alle passen; "#tag" sucht nur in den Tags.
func palConnScore(c domain.Connection, q string) int {
	alias, host := strings.ToLower(c.Alias), strings.ToLower(c.Host)
	group, user := strings.ToLower(c.Group), strings.ToLower(c.Username)
	total := 0
	for _, w := range strings.Fields(strings.ToLower(q)) {
		if t, ok := strings.CutPrefix(w, "#"); ok {
			best := 0
			for _, ct := range c.Tags {
				ct = strings.ToLower(ct)
				switch {
				case t == "" || ct == t:
					best = max(best, 50)
				case strings.HasPrefix(ct, t):
					best = max(best, 35)
				}
			}
			if best == 0 {
				return 0
			}
			total += best
			continue
		}
		s := 0
		switch {
		case alias == w:
			s = 120
		case strings.HasPrefix(alias, w):
			s = 100
		case wordStart(alias, w):
			s = 80
		case strings.Contains(alias, w):
			s = 60
		case strings.HasPrefix(host, w) || host == w:
			s = 55
		case strings.Contains(host, w):
			s = 40
		case wordStart(group, w):
			s = 38
		case strings.Contains(group, w):
			s = 30
		case w == user || strings.HasPrefix(user+"@"+host, w):
			s = 30
		}
		for _, ct := range c.Tags {
			ct = strings.ToLower(ct)
			if ct == w {
				s = max(s, 45)
			} else if strings.HasPrefix(ct, w) {
				s = max(s, 32)
			}
		}
		if s == 0 {
			if g := subseqGaps(alias, w); g >= 0 && utf8.RuneCountInString(w) >= 2 {
				s = max(5, 25-g) // „wb1“ findet „web-01“
			}
		}
		if s == 0 {
			return 0
		}
		total += s
	}
	return total
}

// wordStart: w beginnt an einer Wortgrenze in s („web“ in „prod-web-01“).
func wordStart(s, w string) bool {
	for i := 0; i < len(s); i++ {
		if (i == 0 || strings.ContainsRune(" -_./@:", rune(s[i-1]))) && strings.HasPrefix(s[i:], w) {
			return true
		}
	}
	return false
}

// subseqGaps: kommen die Zeichen von w der Reihe nach in s vor? Liefert die
// Zahl der übersprungenen Zeichen zwischen erstem und letztem Treffer, -1 wenn nicht.
func subseqGaps(s, w string) int {
	rs, rw := []rune(s), []rune(w)
	j, first, last := 0, -1, -1
	for i := 0; i < len(rs) && j < len(rw); i++ {
		if rs[i] == rw[j] {
			if first < 0 {
				first = i
			}
			last = i
			j++
		}
	}
	if j < len(rw) {
		return -1
	}
	return last - first + 1 - len(rw)
}

// palConnItems: die Verbindungs-Einträge der Palette für den Text q.
func (a *App) palConnItems(q string) []palItem {
	all := a.st.Connections()
	if len(all) == 0 {
		return nil
	}
	var picked []domain.Connection
	rest := 0
	if strings.TrimSpace(q) == "" {
		picked = a.palRecentConns(all)
		rest = len(all) - len(picked)
	} else {
		type hit struct {
			c     domain.Connection
			score int
		}
		var hits []hit
		for _, c := range all {
			if s := palConnScore(c, q); s > 0 {
				if a.connected(c.ID) {
					s += 15
				}
				hits = append(hits, hit{c, s})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].score != hits[j].score {
				return hits[i].score > hits[j].score
			}
			return hits[i].c.LastUsed.After(hits[j].c.LastUsed)
		})
		for i, h := range hits {
			if i >= palMaxHits {
				break
			}
			picked = append(picked, h.c)
		}
		rest = len(hits) - len(picked)
	}
	var items []palItem
	for _, c := range picked {
		c := c
		icon, col := a.devIcon(c)
		if a.connected(c.ID) {
			icon, col = "◉", a.s.T.OK
		}
		items = append(items, palItem{group: pgConns, icon: icon, iconColor: col, label: c.Alias,
			sub: c.Group, hint: shortTarget(c), run: func() tea.Cmd { return a.openOrConnect(c) }})
	}
	if rest > 0 {
		label := fmt.Sprintf(i18n.T("Alle %d Verbindungen im Manager"), len(all))
		hint := i18n.T("Name, Ordner oder #Tag tippen")
		if strings.TrimSpace(q) != "" {
			label = fmt.Sprintf(i18n.T("%d weitere Treffer im Manager zeigen"), rest)
			hint = i18n.T("genauer tippen")
		}
		items = append(items, palItem{group: pgConns, icon: "≡", label: label, hint: hint, more: true,
			run: func() tea.Cmd {
				a.scr = scrManager
				a.mgr.search.SetValue(strings.TrimSpace(q))
				return nil
			}})
	}
	return items
}

// palRecentConns: offene Verbindungen, dann zuletzt benutzte; ohne Verlauf
// die ersten aus dem Manager.
func (a *App) palRecentConns(all []domain.Connection) []domain.Connection {
	var open, used []domain.Connection
	for _, c := range all {
		switch {
		case a.connected(c.ID):
			open = append(open, c)
		case !c.LastUsed.IsZero():
			used = append(used, c)
		}
	}
	sort.SliceStable(used, func(i, j int) bool { return used[i].LastUsed.After(used[j].LastUsed) })
	out := append(open, used...)
	if len(out) == 0 {
		out = all
	}
	return out[:min(len(out), max(palRecent, len(open)))]
}

// palConnHeader: Überschrift der Verbindungs-Gruppe, je nach Zustand.
func (a *App) palConnHeader(q string) string {
	if strings.TrimSpace(q) != "" {
		return i18n.T("VERBINDUNGEN · beste Treffer")
	}
	for _, c := range a.st.Connections() {
		if !c.LastUsed.IsZero() || a.connected(c.ID) {
			return i18n.T("VERBINDUNGEN · zuletzt benutzt")
		}
	}
	return i18n.T("VERBINDUNGEN")
}
