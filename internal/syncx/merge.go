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

// SharedSettings: die Einstellungen, die auf allen Rechnern gleich sind.
// Rein lokal bleiben nur Dinge, die vom Rechner abhängen: Windows-DPAPI,
// ob das Terminal hier eine Nerd Font hat, ob die Maus eingefangen wird
// (hängt vom Terminalprogramm ab) und die zuletzt gestartete Version.
type SharedSettings struct {
	Theme        string               `json:"theme"`
	LockAfterMin int                  `json:"lock_after_min"`
	Language     string               `json:"language"`
	Keys         map[string]string    `json:"keys,omitempty"`   // eigene Tastenkürzel
	Colors       *domain.CustomColors `json:"colors,omitempty"` // eigene Farben
	// Prefs: alle übrigen Schalter. Zeiger, damit ein Stand von einer
	// älteren Version (ohne Prefs) die lokalen Schalter nicht zurücksetzt.
	Prefs *SharedPrefs `json:"prefs,omitempty"`
}

// SharedPrefs: Verhalten, Meldungen, Überwachung und Aussehen.
type SharedPrefs struct {
	NoSessionLog    bool                 `json:"no_session_log,omitempty"`
	NoPasteGuard    bool                 `json:"no_paste_guard,omitempty"`
	DangerAll       bool                 `json:"danger_all,omitempty"`
	NoAutoReconnect bool                 `json:"no_auto_reconnect,omitempty"`
	TourDone        bool                 `json:"tour_done,omitempty"`
	ExplainKeys     bool                 `json:"explain_keys,omitempty"`
	NoReachCheck    bool                 `json:"no_reach_check,omitempty"`
	NoNotify        bool                 `json:"no_notify,omitempty"`
	NoDesktopNotify bool                 `json:"no_desktop_notify,omitempty"`
	NoOSC52         bool                 `json:"no_osc52,omitempty"`
	Monitor         domain.MonitorConfig `json:"monitor,omitempty"`
	Icons           string               `json:"icons,omitempty"`
	Intro           string               `json:"intro,omitempty"`
	DimPanes        bool                 `json:"dim_panes,omitempty"`
	Workspaces      []domain.Workspace   `json:"workspaces,omitempty"`
}

func sharedSettings(s domain.Settings) SharedSettings {
	return SharedSettings{
		Theme: s.Theme, LockAfterMin: s.LockAfterMin, Language: s.Language,
		Keys: copyKeys(s.Keys), Colors: copyColors(s.Colors),
		Prefs: &SharedPrefs{
			NoSessionLog: s.NoSessionLog, NoPasteGuard: s.NoPasteGuard, DangerAll: s.DangerAll,
			NoAutoReconnect: s.NoAutoReconnect, TourDone: s.TourDone, ExplainKeys: s.ExplainKeys,
			NoReachCheck: s.NoReachCheck, NoNotify: s.NoNotify, NoDesktopNotify: s.NoDesktopNotify,
			NoOSC52: s.NoOSC52, Monitor: s.Monitor, Icons: s.Icons, Intro: s.Intro,
			DimPanes: s.DimPanes, Workspaces: append([]domain.Workspace(nil), s.Workspaces...),
		},
	}
}

// applyShared übernimmt den gemeinsamen Teil in die lokalen Einstellungen.
func applyShared(p SharedSettings, d *domain.Settings) {
	d.Theme, d.LockAfterMin, d.Language = p.Theme, p.LockAfterMin, p.Language
	d.Keys = copyKeys(p.Keys)
	// Ältere Versionen kennen keine eigenen Farben: dann die lokalen behalten.
	if p.Colors != nil || p.Theme != "eigene" {
		d.Colors = copyColors(p.Colors)
	}
	x := p.Prefs
	if x == nil {
		return // Stand einer älteren Version: lokale Schalter behalten
	}
	d.NoSessionLog, d.NoPasteGuard, d.DangerAll = x.NoSessionLog, x.NoPasteGuard, x.DangerAll
	d.NoAutoReconnect, d.ExplainKeys, d.NoReachCheck = x.NoAutoReconnect, x.ExplainKeys, x.NoReachCheck
	d.NoNotify, d.NoDesktopNotify, d.NoOSC52 = x.NoNotify, x.NoDesktopNotify, x.NoOSC52
	d.TourDone = d.TourDone || x.TourDone
	d.Monitor, d.Intro = x.Monitor, x.Intro
	d.DimPanes, d.Workspaces = x.DimPanes, append([]domain.Workspace(nil), x.Workspaces...)
	// Nerd-Font-Symbole von einem anderen Rechner: hier erst nach der Probe,
	// sonst stünden in jedem Menü Kästchen.
	if x.Icons == "nerd" && d.Icons != "nerd" && d.NerdFont != "ja" {
		d.NerdFont = "nein"
	}
	d.Icons = x.Icons
}

// Payload ist der entschlüsselte Inhalt der Sync-Datei.
type Payload struct {
	Format      int                  `json:"format"`
	Connections []domain.Connection  `json:"connections"`
	Tunnels     []domain.Tunnel      `json:"tunnels"`
	Runbooks    []domain.Runbook     `json:"runbooks,omitempty"`
	Folders     []domain.Folder      `json:"folders,omitempty"` // Ordner: Reihenfolge, zugeklappt, leere Ordner
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
		Runbooks:    append([]domain.Runbook(nil), d.Runbooks...),
		Folders:     append([]domain.Folder(nil), d.Folders...),
		LLM:         d.LLM, LLMAt: d.LLMUpdatedAt,
		Settings:   sharedSettings(d.Settings),
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
// Felder (Sync-Konfiguration, Schlüsselpfade, DPAPI, Nerd Font, Maus)
// bleiben unberührt.
func (p *Payload) ApplyTo(d *domain.Data) {
	d.Connections = append([]domain.Connection(nil), p.Connections...)
	d.Tunnels = append([]domain.Tunnel(nil), p.Tunnels...)
	d.Runbooks = append([]domain.Runbook(nil), p.Runbooks...)
	d.Folders = append([]domain.Folder(nil), p.Folders...)
	d.LLM, d.LLMUpdatedAt = p.LLM, p.LLMAt
	applyShared(p.Settings, &d.Settings)
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

	out.Runbooks = mergeByID(a.Runbooks, b.Runbooks, out.Deleted,
		func(r domain.Runbook) string { return r.ID },
		func(r domain.Runbook) time.Time { return r.Rev() },
		func(win, lose domain.Runbook) domain.Runbook {
			if lose.LastUsed.After(win.LastUsed) {
				win.LastUsed = lose.LastUsed
			}
			win.Uses = max(win.Uses, lose.Uses)
			return win
		})
	sort.SliceStable(out.Runbooks, func(i, j int) bool { return out.Runbooks[i].ID < out.Runbooks[j].ID })

	// Ordner: pro ID wie Verbindungen; legen zwei Rechner denselben Pfad
	// unabhängig an, bleibt nur der neuere Eintrag (gleiches Ergebnis überall).
	folders := mergeByID(a.Folders, b.Folders, out.Deleted,
		func(f domain.Folder) string { return f.ID },
		func(f domain.Folder) time.Time { return f.Rev() }, nil)
	byPath := map[string]domain.Folder{}
	for _, f := range folders {
		if o, ok := byPath[f.Path]; !ok || newer(f, f.Rev(), o, o.Rev()) {
			byPath[f.Path] = f
		}
	}
	for _, f := range byPath {
		out.Folders = append(out.Folders, f)
	}
	sort.SliceStable(out.Folders, func(i, j int) bool { return out.Folders[i].ID < out.Folders[j].ID })

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
	for _, r := range local.Runbooks {
		l[r.ID], _ = json.Marshal(r)
	}
	for _, f := range local.Folders {
		l[f.ID], _ = json.Marshal(f)
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
	for _, r := range merged.Runbooks {
		check(r.ID, r)
	}
	for _, f := range merged.Folders {
		check(f.ID, f)
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

func copyKeys(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyColors(c *domain.CustomColors) *domain.CustomColors {
	if c == nil {
		return nil
	}
	x := *c
	return &x
}
