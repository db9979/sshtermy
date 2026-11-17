package ui

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Menüeinträge leichter finden (Vorschläge 1, 2, 4, 5, 7 vom 2026-11-17).
//
// Die Suche der Befehlspalette verzeiht Tippfehler, ä/ae und die Reihenfolge
// der Wörter. Sie kennt jeden Eintrag auf Deutsch und auf Englisch, dazu
// Alltagswörter (palSyn, palItem.keys) und den Hilfesatz. Treffer stehen nach
// Güte sortiert und zeigen grau, wo sie liegen; → springt dorthin. Eine
// getippte Taste („alt+e“) zeigt, was sie tut. Findet sich nichts, kommen
// die nächsten Einträge („Meintest du …“) und die Frage an die KI.

// ---------------------------------------------------------------- Normalisieren

// foldText: klein, ä→ae, ö→oe, ü→ue, ß→ss, Satzzeichen als Leerzeichen.
func foldText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := true
	put := func(x string) { b.WriteString(x); space = false }
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'ä':
			put("ae")
			continue
		case 'ö':
			put("oe")
			continue
		case 'ü':
			put("ue")
			continue
		case 'ß':
			put("ss")
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// palStop: Füllwörter einer Frage („wo finde ich …“), die nichts eingrenzen.
var palStop = map[string]bool{
	"wo": true, "ich": true, "finde": true, "finden": true, "wie": true, "kann": true, "man": true, "der": true, "die": true,
	"das": true, "ein": true, "eine": true, "einen": true, "den": true, "dem": true, "des": true, "und": true, "mit": true,
	"zu": true, "zum": true, "zur": true, "ist": true, "mein": true, "meine": true, "gibt": true, "es": true, "geht": true,
	"the": true, "how": true, "where": true, "do": true, "i": true, "to": true, "is": true, "can": true, "my": true,
}

// palQueryWords: Suchwörter einer Eingabe, ohne Füllwörter (solange danach
// noch etwas übrig bleibt).
func palQueryWords(q string) []string {
	all := strings.Fields(foldText(q))
	var out []string
	for _, w := range all {
		if !palStop[w] {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// editDist: Tippfehler-Abstand (Einfügen, Löschen, Ersetzen, Vertauschen
// zweier Nachbarn), bricht ab, sobald er über max liegt.
func editDist(a, b string, limit int) int {
	ra, rb := []rune(a), []rune(b)
	if d := len(ra) - len(rb); d > limit || -d > limit {
		return limit + 1
	}
	prev2 := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		low := cur[0]
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			low = min(low, v)
		}
		if low > limit {
			return limit + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(rb)]
}

// fuzzyWord: Tippfehler-Abstand von w zu einem Wort x oder dessen Anfang
// (wer „einstelung“ tippt, meint „einstellungen“); -1 = zu weit weg.
func fuzzyWord(w, x string) int {
	n := utf8.RuneCountInString(w)
	if n < 4 {
		return -1
	}
	limit := 1
	if n >= 8 {
		limit = 2
	}
	best := editDist(w, x, limit)
	if rx := []rune(x); len(rx) > n { // Wortanfang, ein Buchstabe mehr oder weniger
		for k := n - 1; k <= n+1 && k <= len(rx); k++ {
			best = min(best, editDist(w, string(rx[:k]), limit))
		}
	}
	if best > limit {
		return -1
	}
	return best
}

// ---------------------------------------------------------------- Bewerten

// palHay: alles, was die Suche zu einem Eintrag kennt (schon gefaltet).
type palHay struct {
	label, other   []string // Wörter der Beschriftung, deutsch und englisch
	labelS, otherS string
	keys           []string // Suchwörter (palItem.keys, palSyn), gefaltet
	keysRaw        []string // dieselben, wie geschrieben (für „via“)
	help           []string // Wörter des Hilfesatzes
}

func (a *App) palHayOf(it palItem) palHay {
	base := strings.TrimSuffix(it.label, " ›") // Ordner: „Einstellungen ›“
	de := i18n.German(base)
	other := i18n.Other(base)
	h := palHay{labelS: foldText(it.label), otherS: foldText(other)}
	h.label = strings.Fields(h.labelS)
	h.other = strings.Fields(h.otherS)
	if de != it.label { // englische Oberfläche: deutsche Beschriftung zählt mit
		h.otherS += " " + foldText(de)
		h.other = append(h.other, strings.Fields(foldText(de))...)
	}
	for _, src := range []string{it.keys, palSyn[de], it.sub} {
		for _, w := range strings.Fields(src) {
			for _, f := range strings.Fields(foldText(w)) {
				h.keys = append(h.keys, f)
				h.keysRaw = append(h.keysRaw, strings.Trim(w, ".,;:()"))
			}
		}
	}
	if t, ok := palHelpText[de]; ok {
		h.help = append(strings.Fields(foldText(t)), strings.Fields(foldText(i18n.T(t)))...)
	}
	return h
}

// palScore: Güte eines Eintrags für die Eingabe (0 = passt nicht). Jedes
// Suchwort muss irgendwo passen. via: das Suchwort des Eintrags, über das
// er gefunden wurde, wenn es nicht in der Beschriftung steht.
func palScore(q string, h palHay) (int, string) {
	words := palQueryWords(q)
	if len(words) == 0 {
		return 0, ""
	}
	total, via := 0, ""
	for _, w := range words {
		s, v := palWordScore(w, h)
		if s == 0 {
			return 0, ""
		}
		total += s
		if via == "" {
			via = v
		}
	}
	fq := strings.Join(words, " ")
	switch {
	case strings.HasPrefix(h.labelS, fq):
		total += 60
	case strings.Contains(h.labelS, fq):
		total += 30
	case strings.Contains(h.otherS, fq):
		total += 20
	}
	return total, via
}

func palWordScore(w string, h palHay) (int, string) {
	best, via := 0, ""
	try := func(s int, v string) {
		if s > best {
			best, via = s, v
		}
	}
	long := utf8.RuneCountInString(w) >= 3
	for _, x := range h.label {
		switch {
		case x == w:
			try(100, "")
		case strings.HasPrefix(x, w):
			try(90, "")
		case long && strings.Contains(x, w):
			try(70, "")
		}
	}
	for _, x := range h.other {
		switch {
		case x == w:
			try(85, "")
		case strings.HasPrefix(x, w):
			try(75, "")
		case long && strings.Contains(x, w):
			try(55, "")
		}
	}
	for i, x := range h.keys {
		switch {
		case x == w:
			try(75, h.keysRaw[i])
		case strings.HasPrefix(x, w):
			try(65, h.keysRaw[i])
		case long && strings.Contains(x, w):
			try(50, h.keysRaw[i])
		}
	}
	for _, x := range h.help {
		if strings.HasPrefix(x, w) {
			try(30, "")
		}
	}
	if best > 0 {
		return best, via
	}
	// Tippfehler: nur, wenn sonst nichts passt
	for _, x := range h.label {
		if d := fuzzyWord(w, x); d >= 0 {
			try(45-10*d, "")
		}
	}
	for _, x := range h.other {
		if d := fuzzyWord(w, x); d >= 0 {
			try(38-10*d, "")
		}
	}
	for i, x := range h.keys {
		if d := fuzzyWord(w, x); d >= 0 {
			try(35-10*d, h.keysRaw[i])
		}
	}
	return best, via
}

// ---------------------------------------------------------------- Ordnen

// palRank: passende Einträge nach Güte, mit dem Weg dorthin grau dahinter.
func (a *App) palRank(q string, cands []palItem) []palItem {
	type hit struct {
		it    palItem
		score int
	}
	var hits []hit
	best := 0
	for _, it := range cands {
		s, via := palScore(q, a.palHayOf(it))
		if s == 0 {
			continue
		}
		it.sub = palWhereSub(it, via)
		hits = append(hits, hit{it, s})
		best = max(best, s)
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	var out []palItem
	for _, h := range hits {
		if h.score*4 < best { // Tippfehler-Treffer neben einem echten: weg
			continue
		}
		out = append(out, h.it)
	}
	return out
}

// palWhere: Weg zum Eintrag, z. B. „Einstellungen › Aussehen“ oder „KI“.
func palWhere(it palItem) string {
	switch {
	case it.menu != "":
		p := palMenuPath(it.menu)
		if it.menu == "settings" {
			if g := it.home; g >= pgSetBehave && g <= pgSetSetup {
				p += " › " + palTitle(i18n.T(palGroupName[g]))
			}
		}
		return p
	case it.area != "":
		return i18n.T(palMenuLabel(it.area))
	}
	return ""
}

// palWhereSub: grauer Zusatz eines Treffers: Weg und Suchwort.
func palWhereSub(it palItem, via string) string {
	parts := []string{}
	if w := palWhere(it); w != "" && it.open == "" {
		parts = append(parts, w)
	} else if it.sub != "" {
		parts = append(parts, strings.TrimLeft(it.sub, " ·"))
	}
	if via != "" && !strings.Contains(strings.ToLower(it.label), strings.ToLower(via)) {
		parts = append(parts, "„"+via+"“")
	}
	if len(parts) == 0 {
		return ""
	}
	return "· " + strings.Join(parts, " · ")
}

// palCanJump: führt → von einem Treffer an seinen Platz?
func palCanJump(it palItem) bool {
	return it.open == "" && it.cycle == nil && !it.hidden && (it.menu != "" || it.area != "") && it.menu != "help/new"
}

// palJump öffnet den Ordner eines Treffers und wählt ihn dort aus, damit
// man sieht, wo er wohnt.
func (a *App) palJump(it palItem) {
	p := a.pal
	if it.menu != "" {
		a.palEnter(it.menu)
		if it.menu == "settings" && it.home >= pgSetBehave && it.home <= pgSetSetup {
			p.tab = it.home
		}
	} else {
		a.palEnter(it.area)
	}
	for i, x := range a.paletteItems() {
		if x.label == it.label {
			p.sel = i
			break
		}
	}
}

// ---------------------------------------------------------------- Tasten (5)

// palKeyQuery: getippte Taste („alt+e“, „Strg T“, „f6“, „⇧F10“) als
// Tastenname der Keymap; "" = keine Taste gemeint.
func palKeyQuery(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	q = strings.NewReplacer("⇧", "shift+", "-", "+", " ", "+").Replace(q)
	parts := strings.FieldsFunc(q, func(r rune) bool { return r == '+' })
	if len(parts) == 0 || len(parts) > 3 {
		return ""
	}
	var mods []string
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "strg", "ctrl", "control", "ctl":
			mods = append(mods, "ctrl")
		case "alt", "option", "opt", "meta":
			mods = append(mods, "alt")
		case "shift", "umschalt", "hochstell":
			mods = append(mods, "shift")
		default:
			return ""
		}
	}
	key := parts[len(parts)-1]
	fkey := len(key) >= 2 && key[0] == 'f' && strings.Trim(key[1:], "0123456789") == ""
	named := map[string]string{"leertaste": "space", "space": "space", "enter": "enter", "tab": "tab", "entf": "delete",
		"del": "delete", "pos1": "home", "home": "home", "ende": "end", "end": "end", "bild↑": "pgup", "pgup": "pgup",
		"bild↓": "pgdown", "pgdn": "pgdown", "pgdown": "pgdown", "esc": "esc", "@": "@"}
	switch {
	case fkey:
	case named[key] != "":
		key = named[key]
	case utf8.RuneCountInString(key) == 1 && len(mods) > 0:
	default:
		return ""
	}
	// Reihenfolge wie in der Keymap: ctrl, alt, shift
	order := map[string]int{"ctrl": 0, "alt": 1, "shift": 2}
	sort.Slice(mods, func(i, j int) bool { return order[mods[i]] < order[mods[j]] })
	return strings.Join(append(mods, key), "+")
}

// palKeyItems: was die getippte Taste tut, je Bereich ein Eintrag, dazu
// freie Alt-Tasten für eigene Belegungen.
func (a *App) palKeyItems(q string) []palItem {
	k := palKeyQuery(q)
	if k == "" {
		return nil
	}
	var out []palItem
	for _, d := range actionDefs {
		if !a.keys.is(d.id, k) {
			continue
		}
		key := k
		out = append(out, palItem{group: pgKey, icon: ic("keys"), label: i18n.T(d.label), sub: "· " + i18n.T(d.area), hint: keyLabel(k),
			run: func() tea.Cmd { return pressKey(key) }})
	}
	if len(out) == 0 {
		out = append(out, palItem{group: pgKey, icon: ic("keys"), label: i18n.F("%s ist frei", keyLabel(k)), more: true,
			sub: "· " + i18n.T("Tastenkürzel anpassen"), hint: a.keys.label(actKeys), run: func() tea.Cmd { return a.openKeys() }})
	}
	if free := a.freeAltKeys(4); len(free) > 0 {
		out = append(out, palItem{group: pgKey, icon: ic("keys"), label: i18n.T("Frei für eigene Belegung: ") + strings.Join(free, ", "), more: true,
			hint: i18n.T("anpassen"), run: func() tea.Cmd { return a.openKeys() }})
	}
	return out
}

// freeAltKeys: Alt+Buchstabe, die keine Aktion belegt (höchstens n).
func (a *App) freeAltKeys(n int) []string {
	var out []string
	for r := 'a'; r <= 'z' && len(out) < n; r++ {
		k := "alt+" + string(r)
		used := false
		for _, d := range actionDefs {
			if a.keys.is(d.id, k) {
				used = true
				break
			}
		}
		if !used {
			out = append(out, keyLabel(k))
		}
	}
	return out
}

// pressKey: Kommando, das die Taste so ankommen lässt, als wäre sie gedrückt.
func pressKey(k string) tea.Cmd {
	msg := keyPressOf(k)
	return func() tea.Msg { return msg }
}

// ---------------------------------------------------------------- Meintest du (7)

// palMaybe: die drei nächsten Einträge, wenn die Suche nichts findet, und
// die Frage an die KI, wo es das gibt.
func (a *App) palMaybe(q string, cands []palItem) []palItem {
	type hit struct {
		it palItem
		s  int
	}
	var hits []hit
	words := palQueryWords(q)
	for _, it := range cands {
		if it.hidden || it.open != "" {
			continue
		}
		h := a.palHayOf(it)
		s := 0
		for _, w := range words { // jedes Wort für sich, Tippfehler großzügig
			ws, _ := palWordScore(w, h)
			if n := utf8.RuneCountInString(w); ws == 0 && n >= 5 {
				limit := 2 + n/9 // großzügiger als die Suche selbst: 2 Fehler, ab 9 Buchstaben 3
				for _, x := range append(append(h.label, h.other...), h.keys...) {
					if d := editDist(w, x, limit); d <= limit {
						ws = max(ws, 20-5*d)
					}
				}
			}
			s += ws
		}
		if s > 0 {
			it.sub = palWhereSub(it, "")
			it.group = pgMaybe
			hits = append(hits, hit{it, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].s > hits[j].s })
	var out []palItem
	for i := 0; i < len(hits) && i < 3; i++ {
		out = append(out, hits[i].it)
	}
	return out
}

// ---------------------------------------------------------------- F1 (11)

// helpGoCmd: die gewählte Zeile der Hilfe ausführen (⏎) oder in der Palette
// zeigen (Strg+Leertaste). Gehört die Taste einer Aktion, kommt sie wie
// gedrückt an; sonst öffnet die Palette mit der Beschreibung als Suche.
func (a *App) helpGoCmd(m *modal) tea.Cmd {
	if m.helpSel < 0 || m.helpSel >= len(m.helpRows) {
		return nil
	}
	row := m.helpRows[m.helpSel]
	if m.helpGo == 1 {
		for _, d := range actionDefs {
			for _, k := range a.keys.keys[d.id] {
				if keyLabel(k) == row[0] && d.id != actHelp {
					a.pal = nil
					return pressKey(k)
				}
			}
		}
	}
	var cmd tea.Cmd
	if a.pal == nil {
		cmd = a.openPalette()
	}
	if a.pal == nil {
		return cmd
	}
	a.palEnter("")
	a.pal.in.SetValue(row[1])
	a.pal.in.CursorEnd()
	if m.helpGo == 1 {
		a.pal.note = i18n.T("Diese Zeile ist keine eigene Aktion. Passende Einträge:")
	}
	return cmd
}
