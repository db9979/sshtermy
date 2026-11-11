package logscan

import (
	"math"
	"sort"
	"time"
)

// Base: Stand der letzten Prüfung eines Hosts (Grundlinie für „Was ist
// neu?“). Liegt verschlüsselt nur auf diesem Rechner.
type Base struct {
	At    time.Time           `json:"at"`
	Hours int                 `json:"h"`
	Items map[string]BaseItem `json:"items"`
}

// BaseItem: ein Muster der letzten Prüfung.
type BaseItem struct {
	N     int    `json:"n"`
	Sev   Sev    `json:"s"`
	Title string `json:"t"`
}

// Base: Grundlinie aus diesem Bericht (alle Muster, auch Rauschen).
func (r *Report) Base() Base {
	b := Base{At: r.At, Hours: r.Hours, Items: map[string]BaseItem{}}
	for _, f := range r.Findings {
		b.Items[f.Key] = BaseItem{N: f.Count, Sev: f.Sev, Title: f.Title}
	}
	return b
}

// Gone: Muster, die bei der letzten Prüfung da waren und jetzt fehlen.
type Gone struct {
	Key   string
	Title string
	N     int
	Sev   Sev
}

// Compare markiert neue und deutlich häufigere Muster gegenüber prev und
// liefert die verschwundenen (ab Warnung). prev nil = erste Prüfung.
func Compare(r *Report, prev *Base) []Gone {
	if prev == nil || len(prev.Items) == 0 {
		return nil
	}
	ph := float64(max(1, prev.Hours))
	ch := float64(max(1, r.Hours))
	seen := map[string]bool{}
	for i := range r.Findings {
		f := &r.Findings[i]
		seen[f.Key] = true
		p, ok := prev.Items[f.Key]
		switch {
		case !ok && f.Sev >= SevWarn:
			f.Status = "new"
		case ok && p.N > 0:
			cur, old := float64(f.Count)/ch, float64(p.N)/ph
			if f.Count >= 10 && cur >= 3*old {
				f.Status, f.Factor = "more", int(math.Round(cur/old))
			}
		}
	}
	var gone []Gone
	for k, p := range prev.Items {
		if !seen[k] && p.Sev >= SevWarn {
			gone = append(gone, Gone{Key: k, Title: p.Title, N: p.N, Sev: p.Sev})
		}
	}
	sort.Slice(gone, func(i, j int) bool {
		if gone[i].Sev != gone[j].Sev {
			return gone[i].Sev > gone[j].Sev
		}
		return gone[i].Title < gone[j].Title
	})
	r.Resort()
	return gone
}

// ApplyNoise blendet Muster aus, die als Rauschen markiert sind.
func ApplyNoise(r *Report, noise func(key string) bool) {
	for i := range r.Findings {
		r.Findings[i].Noise = noise(r.Findings[i].Key)
	}
	r.Resort()
}
