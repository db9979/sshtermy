package ui

import (
	"encoding/json"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// Was sich die Menüs pro Rechner merken (Vorschläge 3, 6, 8 vom 2026-11-17):
// wie oft und wann ein Eintrag über Palette oder Maus lief („Häufig“ nach
// eigener Nutzung), angeheftete Einträge, gezeigte Tipps zu Tasten und welche
// Texte mit einem Update neu kamen (NEU-Marke). Die Datei menu.json liegt im
// Konfigurationsordner, geht nicht in den Sync und enthält nur Beschriftungen.

const (
	menuFile     = "menu.json"
	menuFreshFor = 14 * 24 * time.Hour // so lange trägt ein neuer Eintrag „NEU“
	menuTipAfter = 3                   // ab so vielen Starts über das Menü kommt der Tipp zur Taste
	menuTopMax   = 7                   // so viele Einträge hat „Häufig“ höchstens
	menuMineMax  = 5                   // davon höchstens so viele aus der eigenen Nutzung
)

type menuUse struct {
	N    int   `json:"n"`
	Last int64 `json:"last"` // Unix-Sekunden
}

type menuState struct {
	Use     map[string]menuUse `json:"use,omitempty"`  // deutsche Beschriftung → Nutzung
	Pins    []string           `json:"pins,omitempty"` // angeheftet, deutsche Beschriftung
	TipsOff bool               `json:"tips_off,omitempty"`
	TipDay  string             `json:"tip_day,omitempty"` // höchstens ein Tipp am Tag
	Tipped  []string           `json:"tipped,omitempty"`  // diese Tipps kamen schon
	Ver     string             `json:"ver,omitempty"`     // Version beim letzten Abgleich der Texte
	Seen    []uint32           `json:"seen,omitempty"`    // Prüfsummen aller bekannten Texte
	Fresh   map[string]int64   `json:"fresh,omitempty"`   // neue Texte → seit (Unix-Sekunden)
}

// menuFresh: deutsche Texte, die mit einem Update der letzten 14 Tage neu
// kamen. Paketweit, damit auch die Hilfe (F1) ohne App-Zugriff markiert.
var menuFresh = map[string]bool{}

func (a *App) menuState() *menuState {
	if a.mst != nil {
		return a.mst
	}
	a.mst = &menuState{}
	if a.st != nil {
		if b, err := os.ReadFile(filepath.Join(a.st.Dir, menuFile)); err == nil {
			_ = json.Unmarshal(b, a.mst)
		}
	}
	if a.mst.Use == nil {
		a.mst.Use = map[string]menuUse{}
	}
	return a.mst
}

func (a *App) menuSave() {
	if a.st == nil || a.mst == nil {
		return
	}
	b, err := json.Marshal(a.mst)
	if err != nil {
		return
	}
	p := filepath.Join(a.st.Dir, menuFile)
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, p)
	}
}

// ---------------------------------------------------------------- Nutzung (3)

// palTrackable: zählt der Eintrag für „Häufig“ und Tipps? Nur feste
// Aktionen und Einstellungen, keine Verbindungen, Vorschläge oder Ordner.
func palTrackable(it palItem) bool {
	if it.open != "" || it.run == nil {
		return false
	}
	switch it.group {
	case pgAction, pgHits, pgTop, pgPinned, pgAreas, pgNew, pgChanged, pgMaybe,
		pgSetBehave, pgSetLook, pgSetSafe, pgSetSetup, pgHelp:
		return true
	}
	return false
}

// menuUsed: ein Eintrag lief über Palette oder Maus.
func (a *App) menuUsed(it palItem) {
	if !palTrackable(it) {
		return
	}
	ms := a.menuState()
	key := i18n.German(it.label)
	u := ms.Use[key]
	u.N++
	u.Last = time.Now().Unix()
	ms.Use[key] = u
	a.menuTip(it, u.N)
	a.menuSave()
}

// palMine: die eigenen meistgenutzten Aktionen, die es gerade gibt, nach
// Häufigkeit und wie lange es her ist.
func (a *App) palMine(actions []palItem, skip map[string]bool) []palItem {
	ms := a.menuState()
	now := time.Now().Unix()
	type cand struct {
		it    palItem
		score float64
	}
	var cs []cand
	for _, it := range actions {
		if it.hidden || it.open != "" {
			continue
		}
		key := i18n.German(it.label)
		u, ok := ms.Use[key]
		if !ok || u.N < 2 || skip[key] {
			continue
		}
		age := float64(now-u.Last) / 86400
		w := 1.0
		switch {
		case age > 30:
			w = 0.2
		case age > 7:
			w = 0.5
		}
		cs = append(cs, cand{it, float64(u.N) * w})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].score > cs[j].score })
	var out []palItem
	for i := 0; i < len(cs) && i < menuMineMax; i++ {
		out = append(out, cs[i].it)
	}
	return out
}

// ---------------------------------------------------------------- Anheften

func (a *App) palPinned(label string) bool {
	key := i18n.German(label)
	for _, p := range a.menuState().Pins {
		if p == key {
			return true
		}
	}
	return false
}

// palTogglePin heftet einen Eintrag an oder löst ihn.
func (a *App) palTogglePin(it palItem) {
	if !palTrackable(it) && it.group != pgHere {
		a.flash(i18n.T("Diesen Eintrag kann man nicht anheften"))
		return
	}
	ms := a.menuState()
	key := i18n.German(it.label)
	for i, p := range ms.Pins {
		if p == key {
			ms.Pins = append(ms.Pins[:i], ms.Pins[i+1:]...)
			a.menuSave()
			a.flash(i18n.T("Gelöst: ") + it.label)
			return
		}
	}
	ms.Pins = append(ms.Pins, key)
	a.menuSave()
	a.flash(i18n.T("Angeheftet: ") + it.label)
}

// palPins: angeheftete Einträge, die es gerade gibt, in der Reihenfolge des
// Anheftens. Gesucht wird in den Aktionen und in Einstellungen/Hilfe.
func (a *App) palPins(actions []palItem) []palItem {
	ms := a.menuState()
	if len(ms.Pins) == 0 {
		return nil
	}
	all := append(append([]palItem(nil), actions...), a.palMenuItems()...)
	var out []palItem
	for _, p := range ms.Pins {
		for _, it := range all {
			if it.open == "" && i18n.German(it.label) == p {
				it.home = it.group
				it.group = pgPinned
				it.top = false
				out = append(out, it)
				break
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Tipp zur Taste (6)

// menuTip: wer eine Aktion zum dritten Mal über das Menü startet, erfährt
// einmal ihre Taste. Höchstens ein Tipp am Tag, abschaltbar, im einfachen
// Modus aus.
func (a *App) menuTip(it palItem, n int) {
	ms := a.menuState()
	if n < menuTipAfter || ms.TipsOff || a.simple() || it.hint == "" {
		return
	}
	today := time.Now().Format("2006-01-02")
	if ms.TipDay == today {
		return
	}
	key := i18n.German(it.label)
	for _, t := range ms.Tipped {
		if t == key {
			return
		}
	}
	var act action
	for _, d := range actionDefs {
		if a.keys.label(d.id) == it.hint {
			act = d.id
			break
		}
	}
	if act == "" {
		return
	}
	ms.TipDay = today
	ms.Tipped = append(ms.Tipped, key)
	a.flash(i18n.F("Tipp: „%s“ geht direkt mit %s. Abschalten: in der Palette „Tipps“ suchen.", it.label, a.keys.label(act)))
}

func (a *App) tipsOn() bool { return !a.menuState().TipsOff }

func (a *App) toggleTips() tea.Cmd {
	ms := a.menuState()
	ms.TipsOff = !ms.TipsOff
	a.menuSave()
	return nil
}

// ---------------------------------------------------------------- NEU (8)

func textSum(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// menuFreshCheck: nach einem Update die neuen Texte merken. Beim ersten
// Start gilt alles als bekannt, damit nicht jeder Eintrag „NEU“ trägt.
func (a *App) menuFreshCheck() {
	ms := a.menuState()
	cat := i18n.Catalog()
	now := time.Now()
	changed := false
	if ms.Ver != version.Version || len(ms.Seen) == 0 {
		seen := make(map[uint32]bool, len(ms.Seen))
		for _, h := range ms.Seen {
			seen[h] = true
		}
		first := len(ms.Seen) == 0
		if ms.Fresh == nil {
			ms.Fresh = map[string]int64{}
		}
		sums := make([]uint32, 0, len(cat))
		for _, de := range cat {
			h := textSum(de)
			sums = append(sums, h)
			if !first && !seen[h] {
				ms.Fresh[de] = now.Unix()
			}
		}
		sort.Slice(sums, func(i, j int) bool { return sums[i] < sums[j] })
		ms.Seen, ms.Ver, changed = sums, version.Version, true
	}
	fresh := map[string]bool{}
	for de, at := range ms.Fresh {
		if now.Sub(time.Unix(at, 0)) > menuFreshFor {
			delete(ms.Fresh, de)
			changed = true
			continue
		}
		fresh[de] = true
	}
	menuFresh = fresh
	if changed {
		a.menuSave()
	}
}

// isFresh: trägt der (angezeigte) Text die NEU-Marke?
func isFresh(shown string) bool {
	return len(menuFresh) > 0 && menuFresh[i18n.German(shown)]
}
