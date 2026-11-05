package syncx

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// tombstoneTTL: so lange merkt sich der Sync gelöschte IDs. Ein Rechner, der
// länger offline war, kann danach gelöschte Einträge wieder einbringen.
const tombstoneTTL = 180 * 24 * time.Hour

// SharedSettings: der Teil der Einstellungen, der zwischen Rechnern gleich
// sein soll. UseDPAPI ist bewusst nicht dabei (gehört zum jeweiligen Rechner).
type SharedSettings struct {
	Theme        string `json:"theme"`
	LockAfterMin int    `json:"lock_after_min"`
	Language     string `json:"language"`
}

// Payload ist der entschlüsselte Inhalt der Sync-Datei.
type Payload struct {
	Format      int                  `json:"format"`
	Connections []domain.Connection  `json:"connections"`
	Tunnels     []domain.Tunnel      `json:"tunnels"`
	LLM         domain.LLMConfig     `json:"llm"`
	LLMAt       time.Time            `json:"llm_at"`
	Settings    SharedSettings       `json:"settings"`
	SettingsAt  time.Time            `json:"settings_at"`
	Deleted     map[string]time.Time `json:"deleted,omitempty"`
	KnownHosts  []string             `json:"known_hosts,omitempty"`
	Devices     map[string]time.Time `json:"devices,omitempty"` // Rechnername → letzter Abgleich
}

// FromData baut den Sync-Inhalt aus dem lokalen Store.
func FromData(d *domain.Data, knownHosts []string) *Payload {
	p := &Payload{
		Format:      payloadFormat,
		Connections: append([]domain.Connection(nil), d.Connections...),
		Tunnels:     append([]domain.Tunnel(nil), d.Tunnels...),
		LLM:         d.LLM, LLMAt: d.LLMUpdatedAt,
		Settings:   SharedSettings{d.Settings.Theme, d.Settings.LockAfterMin, d.Settings.Language},
		SettingsAt: d.SettingsUpdatedAt,
		Deleted:    map[string]time.Time{},
		KnownHosts: append([]string(nil), knownHosts...),
	}
	for k, v := range d.Deleted {
		p.Deleted[k] = v
	}
	return p
}

// ApplyTo schreibt den zusammengeführten Inhalt in den lokalen Store. Lokale
// Felder (Sync-Konfiguration, DPAPI) bleiben unberührt.
func (p *Payload) ApplyTo(d *domain.Data) {
	d.Connections = append([]domain.Connection(nil), p.Connections...)
	d.Tunnels = append([]domain.Tunnel(nil), p.Tunnels...)
	d.LLM, d.LLMUpdatedAt = p.LLM, p.LLMAt
	d.Settings.Theme, d.Settings.LockAfterMin, d.Settings.Language = p.Settings.Theme, p.Settings.LockAfterMin, p.Settings.Language
	d.SettingsUpdatedAt = p.SettingsAt
	d.Deleted = map[string]time.Time{}
	for k, v := range p.Deleted {
		d.Deleted[k] = v
	}
}

// Merge führt zwei Stände pro Eintrag zusammen. Die Funktion ist
// kommutativ und idempotent: egal in welcher Reihenfolge Rechner abgleichen,
// am Ende haben alle denselben Stand.
//
//   - Verbindungen/Tunnel: pro ID gewinnt die neuere Änderung (UpdatedAt).
//   - Löschen: ein Grabstein gewinnt, wenn er nicht älter ist als die letzte
//     Änderung des Eintrags. Wer nach dem Löschen woanders noch bearbeitet,
//     behält den Eintrag.
//   - Zuletzt benutzt: das spätere Datum (zählt nicht als Änderung).
//   - KI- und Oberflächen-Einstellungen: jeweils als Ganzes, neuere gewinnt.
//   - known_hosts: Vereinigung.
func Merge(a, b *Payload, now time.Time) *Payload {
	out := &Payload{Format: payloadFormat, Deleted: map[string]time.Time{}, Devices: map[string]time.Time{}}
	for _, src := range []*Payload{a, b} {
		for id, t := range src.Deleted {
			if now.Sub(t) > tombstoneTTL {
				continue
			}
			if t.After(out.Deleted[id]) {
				out.Deleted[id] = t
			}
		}
		for n, t := range src.Devices {
			if t.After(out.Devices[n]) {
				out.Devices[n] = t
			}
		}
	}

	out.Connections = mergeByID(a.Connections, b.Connections, out.Deleted,
		func(c domain.Connection) string { return c.ID },
		func(c domain.Connection) time.Time { return c.Rev() },
		func(win, lose domain.Connection) domain.Connection {
			if lose.LastUsed.After(win.LastUsed) {
				win.LastUsed = lose.LastUsed
			}
			return win
		})
	sort.SliceStable(out.Connections, func(i, j int) bool { return out.Connections[i].ID < out.Connections[j].ID })

	alive := map[string]bool{}
	for _, c := range out.Connections {
		alive[c.ID] = true
	}
	tuns := mergeByID(a.Tunnels, b.Tunnels, out.Deleted,
		func(t domain.Tunnel) string { return t.ID },
		func(t domain.Tunnel) time.Time { return t.UpdatedAt }, nil)
	for _, t := range tuns {
		if alive[t.ConnectionID] { // Tunnel gelöschter Verbindungen fallen weg
			out.Tunnels = append(out.Tunnels, t)
		}
	}
	sort.SliceStable(out.Tunnels, func(i, j int) bool { return out.Tunnels[i].ID < out.Tunnels[j].ID })

	out.LLM, out.LLMAt = pick(a.LLM, a.LLMAt, b.LLM, b.LLMAt)
	out.Settings, out.SettingsAt = pick(a.Settings, a.SettingsAt, b.Settings, b.SettingsAt)

	seen := map[string]bool{}
	for _, l := range append(append([]string(nil), a.KnownHosts...), b.KnownHosts...) {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || seen[l] {
			continue
		}
		seen[l] = true
		out.KnownHosts = append(out.KnownHosts, l)
	}
	sort.Strings(out.KnownHosts)
	if len(out.Devices) == 0 {
		out.Devices = nil
	}
	return out
}

func mergeByID[T any](a, b []T, deleted map[string]time.Time, id func(T) string, rev func(T) time.Time, combine func(win, lose T) T) []T {
	byID := map[string]T{}
	var order []string
	for _, list := range [][]T{a, b} {
		for _, x := range list {
			k := id(x)
			old, ok := byID[k]
			if !ok {
				byID[k] = x
				order = append(order, k)
				continue
			}
			win, lose := old, x
			if newer(x, rev(x), old, rev(old)) {
				win, lose = x, old
			}
			if combine != nil {
				win = combine(win, lose)
			}
			byID[k] = win
		}
	}
	var out []T
	for _, k := range order {
		x := byID[k]
		if t, gone := deleted[k]; gone && !t.Before(rev(x)) {
			continue
		}
		out = append(out, x)
	}
	return out
}

// newer: x ist neuer als y; bei Gleichstand entscheidet der Inhalt, damit
// jeder Rechner dasselbe Ergebnis bekommt.
func newer[T any](x T, xt time.Time, y T, yt time.Time) bool {
	if !xt.Equal(yt) {
		return xt.After(yt)
	}
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Compare(bx, by) > 0
}

func pick[T any](a T, at time.Time, b T, bt time.Time) (T, time.Time) {
	if newer(b, bt, a, at) {
		return b, bt
	}
	return a, at
}

// Stats beschreibt, was ein Abgleich verändert hat (für die Anzeige).
type Stats struct {
	Connections, Tunnels int  // Anzahl nach dem Abgleich
	Added, Changed, Gone int  // lokal hinzugekommen / geändert / entfernt
	Uploaded             bool // Sync-Datei wurde neu geschrieben
	Merged               int  // eingearbeitete Konfliktkopien des Cloud-Dienstes
}

func diff(local, merged *Payload) (added, changed, gone int) {
	l := map[string][]byte{}
	for _, c := range local.Connections {
		l[c.ID], _ = json.Marshal(c)
	}
	for _, t := range local.Tunnels {
		l[t.ID], _ = json.Marshal(t)
	}
	m := map[string]bool{}
	check := func(id string, v any) {
		m[id] = true
		b, _ := json.Marshal(v)
		if old, ok := l[id]; !ok {
			added++
		} else if !bytes.Equal(old, b) {
			changed++
		}
	}
	for _, c := range merged.Connections {
		check(c.ID, c)
	}
	for _, t := range merged.Tunnels {
		check(t.ID, t)
	}
	for id := range l {
		if !m[id] {
			gone++
		}
	}
	return
}

// equalContent: gleicher Inhalt (ohne Geräte-Liste)?
func equalContent(a, b *Payload) bool {
	x, y := *a, *b
	x.Devices, y.Devices = nil, nil
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(bx, by)
}
