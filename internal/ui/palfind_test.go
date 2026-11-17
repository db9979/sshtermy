package ui

import (
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Auffindbarkeit (Vorschlag 12 vom 2026-11-17): Jede Aktion und jede
// Einstellung braucht Suchwörter, die Standardwerte stehen fest, und typische
// Suchanfragen landen beim richtigen Eintrag.

func palTestApp(t *testing.T, lang string) *gold {
	t.Helper()
	g := newGold(t, lang, 140, 50, true)
	g.session("web-01", "deploy@web-01:~$ ")
	g.keys("ctrl+space")
	if g.a.pal == nil {
		t.Fatal("Palette geht nicht auf")
	}
	return g
}

func palSearch(a *App, q string) []palItem {
	a.palEnter("")
	a.pal.in.SetValue(q)
	return a.paletteItems()
}

func TestPalSearchWords(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	check := func(where string, items []palItem) {
		for _, it := range items {
			if it.hidden || it.open != "" || it.label == "" {
				continue
			}
			if strings.TrimSpace(it.keys) == "" && palSyn[i18n.German(it.label)] == "" {
				t.Errorf("%s: %q hat keine Suchwörter (palItem.keys oder palSyn)", where, it.label)
			}
		}
	}
	check("Terminal", a.palActions(a.current(), true, false))
	check("Einstellungen", a.palMenuItems())
	a.scr = scrManager
	check("Manager", a.palActions(nil, false, true))
}

func TestPalSearchProbes(t *testing.T) {
	for _, lang := range []string{"de", "en"} {
		g := palTestApp(t, lang)
		a := g.a
		probes := []struct{ q, want string }{
			{"farbshema", "Farbschema wählen …"}, // Tippfehler
			{"dark mode", "Farbschema wählen …"}, // Alltagswort, englisch
			{"theme", "Farbschema wählen …"},     // englisch in deutscher Oberfläche
			{"schrift", "Symbole"},               // Alltagswort
			{"passwort ändern", "Master-Passwort ändern …"},
			{"aendern passwort", "Master-Passwort ändern …"}, // ae statt ä, Reihenfolge egal
			{"einstelungen", "Einstellungen"},                // Tippfehler im Ordner
			{"port weiterleiten", "Tunnel-Panel"},
			{"upload", "Dateien (SFTP)"},
			{"text markieren", "Markieren per Tastatur (v, y kopiert)"},
			{"wo finde ich die logs", "Log-Blick: Auffälligkeiten in den Logs"},
			{"screenshot", "Terminal-Verlauf als Text exportieren"},
			{"sprache", "Sprache / Language"},
			{"language", "Sprache / Language"},
			{"updates", "Updates anbieten"},
			{"backup", "Gesamtsicherung speichern (für Umzug/Sicherung) …"},
			{"key", "SSH-Schlüssel erzeugen / auf den Server übertragen"},
			{"neuer tab", "Neuer Tab"},
			{"vollbild", "Fenster beim Start"},
			{"animation", "Bewegung"},
		}
		for _, p := range probes {
			items := palSearch(a, p.q)
			want := i18n.T(p.want)
			found := -1
			n := 0
			for _, it := range items {
				if it.group != pgHits {
					continue
				}
				if strings.HasPrefix(it.label, want) {
					found = n
					break
				}
				n++
			}
			if found < 0 || found > 2 {
				var got []string
				for _, it := range items {
					if it.group == pgHits && len(got) < 5 {
						got = append(got, it.label)
					}
				}
				t.Errorf("%s: „%s“ findet %q nicht unter den ersten drei: %q", lang, p.q, want, got)
			}
		}
	}
}

func TestPalWhereAndJump(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	items := palSearch(a, "fensterstil")
	it, ok := hasLabel(items, "Fensterstil")
	if !ok || !strings.Contains(it.sub, "Einstellungen › Aussehen") {
		t.Fatalf("Weg fehlt: %+v", it)
	}
	for i, x := range items {
		if x.label == it.label {
			a.pal.sel = i
		}
	}
	// → auf einem Durchschalt-Eintrag ändert den Wert; der Sprung gilt für
	// alles andere. Darum ein Eintrag ohne Durchschalten:
	items = palSearch(a, "master passwort")
	it, ok = hasLabel(items, "Master-Passwort ändern")
	if !ok || !palCanJump(it) {
		t.Fatalf("Master-Passwort: %+v", it)
	}
	for i, x := range items {
		if x.label == it.label {
			a.pal.sel = i
		}
	}
	g.keys("right")
	if a.pal.menu != "settings" || a.pal.tab != pgSetSafe || a.pal.in.Value() != "" {
		t.Fatalf("→ springt nicht in Einstellungen › Sicherheit: menu=%q tab=%v", a.pal.menu, a.pal.tab)
	}
	if cur := a.paletteItems()[a.pal.sel]; cur.label != it.label {
		t.Fatalf("nach dem Sprung ist %q gewählt", cur.label)
	}
}

func TestPalKeyQuery(t *testing.T) {
	for in, want := range map[string]string{
		"alt+e": "alt+e", "Alt E": "alt+e", "strg+t": "ctrl+t", "ctrl t": "ctrl+t", "F6": "f6",
		"⇧F10": "shift+f10", "shift+f10": "shift+f10", "alt shift up": "", "e": "", "farbe": "", "f": "",
		"strg alt x": "ctrl+alt+x",
	} {
		if got := palKeyQuery(in); got != want {
			t.Errorf("palKeyQuery(%q) = %q, wollte %q", in, got, want)
		}
	}
	g := palTestApp(t, "de")
	items := palSearch(g.a, "alt+e")
	if len(items) == 0 || items[0].group != pgKey || items[0].label != i18n.T("Log-Blick (Auffälligkeiten in den Logs)") {
		t.Fatalf("Taste nachschlagen: %+v", items)
	}
	if msg := items[0].run()(); msg != keyPressOf("alt+e") {
		t.Fatalf("⏎ löst %v aus", msg)
	}
}

func TestPalFold(t *testing.T) {
	if got := foldText("Schlüssel-Größe: ÄÖÜ"); got != "schluessel groesse aeoeue" {
		t.Errorf("foldText: %q", got)
	}
	if d := editDist("farbshema", "farbschema", 2); d != 1 {
		t.Errorf("editDist = %d", d)
	}
	if d := fuzzyWord("einstelung", "einstellungen"); d != 1 {
		t.Errorf("fuzzyWord = %d", d)
	}
	if d := fuzzyWord("abc", "abd"); d != -1 {
		t.Errorf("kurze Wörter ohne Tippfehler-Suche: %d", d)
	}
}

func TestPalMaybeAndAskWhere(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	a.scr = scrManager
	items := palSearch(a, "fxrbschemaxx")
	var maybe, ask bool
	for _, it := range items {
		if it.group == pgMaybe && strings.HasPrefix(it.label, "Farb") {
			maybe = true
		}
		if it.tabRun {
			ask = true
		}
	}
	if !maybe || !ask {
		t.Fatalf("Meintest du / KI fragen fehlt: %+v", items)
	}
	// Antwort der KI: der Eintrag erscheint als Treffer.
	cands := []palItem{{label: "Fensterstil", menu: "settings", run: func() tea.Cmd { return nil }}}
	a.palFindDone(palFindMsg{q: "ecken", cand: cands, pick: agentPick(0, "Ecken stellt man beim Fensterstil ein.")})
	if a.pal == nil || a.pal.in.Value() != "Fensterstil" || !strings.Contains(a.pal.note, "Fensterstil") {
		t.Fatalf("KI-Antwort nicht übernommen: %+v", a.pal)
	}
}

func TestPalPinAndMine(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	// zweimal benutzt → steht unter „Häufig“ ganz oben
	find := func(label string) palItem {
		it, ok := hasLabel(palSearch(a, label), label)
		if !ok {
			t.Fatalf("%q nicht gefunden", label)
		}
		return it
	}
	exp := find("Terminal-Verlauf als Text exportieren")
	a.menuUsed(exp)
	a.menuUsed(exp)
	a.palEnter("")
	var top []string
	for _, it := range a.paletteItems() {
		if it.group == pgTop {
			top = append(top, it.label)
		}
	}
	if len(top) == 0 || top[0] != exp.label || len(top) > menuTopMax {
		t.Fatalf("Häufig lernt nicht mit: %q", top)
	}
	// Anheften mit *
	items := a.paletteItems()
	for i, it := range items {
		if it.label == top[1] {
			a.pal.sel = i
		}
	}
	g.keys("*")
	items = a.paletteItems()
	if len(items) == 0 || items[0].group != pgPinned && !(len(items) > 0) {
		t.Fatalf("Anheften: %+v", items)
	}
	pinned := false
	for _, it := range items {
		if it.group == pgPinned && it.label == top[1] {
			pinned = true
		}
	}
	if !pinned {
		t.Fatalf("%q nicht angeheftet", top[1])
	}
	// gemerkt in menu.json
	a.mst = nil
	if !a.palPinned(top[1]) {
		t.Fatal("Anheften nicht gespeichert")
	}
}

func TestPalTipOnce(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	it, _ := hasLabel(palSearch(a, "log-blick"), "Log-Blick")
	for range 3 {
		a.menuUsed(it)
	}
	if len(a.toasts) == 0 || !strings.Contains(a.toasts[len(a.toasts)-1].text, "Alt+E") {
		t.Fatalf("kein Tipp zur Taste: %+v", a.toasts)
	}
	a.toasts = nil
	other, _ := hasLabel(palSearch(a, "alle tabs zeigen"), "Alle Tabs zeigen")
	for range 3 {
		a.menuUsed(other)
	}
	if len(a.toasts) != 0 {
		t.Fatalf("zweiter Tipp am selben Tag: %+v", a.toasts)
	}
	a.menuState().TipDay = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	a.menuState().TipsOff = true
	a.menuUsed(other)
	if len(a.toasts) != 0 {
		t.Fatal("Tipp trotz „Tipps zu Tasten: aus“")
	}
}

func TestPalDefaultsComplete(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	for _, it := range a.palMenuItems() {
		if it.hidden || it.label == "Sprache / Language" {
			continue
		}
		if it.cycle == nil && it.state == palNoState {
			continue
		}
		if _, ok := palDefaults[it.label]; !ok {
			t.Errorf("Standardwert fehlt in palDefaults: %q (jetzt %q)", it.label, palValue(it))
		}
	}
	if runtime.GOOS != "linux" {
		return // manche Standards hängen vom System ab (Meldungen, Fenster)
	}
	if ch := a.palChangedItems(); len(ch) > 0 {
		var l []string
		for _, it := range ch {
			l = append(l, it.label+" = "+palValue(it))
		}
		t.Errorf("frische Installation zeigt geänderte Einstellungen: %q", l)
	}
}

func TestPalChangedAndReset(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	a.toggleMouse()
	_ = a.cycleMotion(1)
	items := palSearch(a, "!")
	if len(items) < 2 {
		t.Fatalf("„!“ zeigt %d geänderte", len(items))
	}
	for _, it := range items {
		if it.group != pgChanged || !it.changed {
			t.Fatalf("falsche Gruppe: %+v", it)
		}
	}
	for i := range items {
		a.pal.sel = 0
		_ = i
		g.keys("delete")
		if len(a.paletteItems()) == 0 {
			break
		}
	}
	if left := a.palChangedItems(); len(left) != 0 {
		t.Fatalf("Entf setzt nicht zurück: %+v", left)
	}
	if !a.mouseEnabled() {
		t.Fatal("Maus nicht zurück auf an")
	}
}

func TestMenuFresh(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	defer func() { menuFresh = map[string]bool{} }()
	// erster Start: nichts ist neu
	a.menuFreshCheck()
	if len(menuFresh) != 0 {
		t.Fatalf("erster Start markiert %d Texte als neu", len(menuFresh))
	}
	// Update: ein Text war vorher unbekannt
	ms := a.menuState()
	drop := textSum("Log-Blick: Auffälligkeiten in den Logs")
	var seen []uint32
	for _, h := range ms.Seen {
		if h != drop {
			seen = append(seen, h)
		}
	}
	ms.Seen, ms.Ver = seen, "0.0.1"
	a.menuFreshCheck()
	if !menuFresh["Log-Blick: Auffälligkeiten in den Logs"] || len(menuFresh) != 1 {
		t.Fatalf("NEU nach Update: %v", menuFresh)
	}
	a.palEnter("help")
	if it, ok := hasLabel(a.paletteItems(), "Neu seit dem letzten Update"); !ok || it.hint != "1" {
		t.Fatalf("Ordner „Neu seit dem letzten Update“: %+v", it)
	}
	a.palEnter("help/new")
	items := a.paletteItems()
	if len(items) != 1 || !items[0].fresh || !strings.Contains(items[0].sub, "Suchen & Logs") {
		t.Fatalf("NEU-Liste: %+v", items)
	}
	// nach 14 Tagen ist die Marke weg
	for k := range ms.Fresh {
		ms.Fresh[k] = time.Now().Add(-15 * 24 * time.Hour).Unix()
	}
	a.menuFreshCheck()
	if len(menuFresh) != 0 {
		t.Fatal("NEU-Marke bleibt länger als 14 Tage")
	}
}

func TestPalHere(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	a.pal = nil
	a.scr = scrManager
	g.keys("ctrl+space")
	items := a.paletteItems()
	if len(items) == 0 || items[0].group != pgHere || len(items) < 2 {
		t.Fatalf("Manager: „Hier“ fehlt oben: %+v", items[:min(3, len(items))])
	}
	n := 0
	for _, it := range items {
		if it.group == pgHere {
			n++
		}
	}
	if n > palHereMax {
		t.Fatalf("„Hier“ zu lang: %d", n)
	}
}

func TestHelpRunsRow(t *testing.T) {
	g := palTestApp(t, "de")
	a := g.a
	a.pal = nil
	g.keys("f1")
	if a.modal == nil || a.modal.kind != mHelp {
		t.Fatal("F1 öffnet die Hilfe nicht")
	}
	g.typ("log-blick")
	a.View()
	if len(a.modal.helpRows) == 0 {
		t.Fatal("Filter findet nichts")
	}
	if a.modal.helpSel != 0 {
		t.Fatalf("Auswahl nach dem Tippen: %d", a.modal.helpSel)
	}
	cmd := a.modalKey(keyMsg("enter"))
	if a.modal != nil || cmd == nil {
		t.Fatal("⏎ führt die Zeile nicht aus")
	}
	if msg := cmd(); msg != keyPressOf("alt+e") {
		t.Fatalf("⏎ löst %v aus", msg)
	}
	// Taste drücken zeigt, was sie tut
	g.keys("f1", "alt+e")
	if a.modal == nil || a.modal.filter != "Alt+E" {
		t.Fatalf("Taste in der Hilfe: %+v", a.modal)
	}
}

func agentPick(i int, why string) agent.MenuPick { return agent.MenuPick{Index: i, Why: why} }
