package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// „Hier“ zuerst, NEU-Liste und geänderte Einstellungen (Vorschläge 8, 9, 10
// vom 2026-11-17).

// ---------------------------------------------------------------- Hier (10)

// palHereMax: so viele Aktionen zeigt „Hier“ ohne Suchtext; die Suche
// findet alle.
const palHereMax = 4

// palHere: Aktionen des Bildschirms, auf dem die Palette aufging: dieselben
// wie im Rechtsklick-Menü der Dateiansicht oder des Managers.
func (a *App) palHere() (string, []palItem) {
	return a.palHereN(palHereMax)
}

// palHereAll: alle Aktionen des Bildschirms (für die Suche).
func (a *App) palHereAll() []palItem {
	_, out := a.palHereN(-1)
	return out
}

func (a *App) palHereN(limit int) (string, []palItem) {
	var title string
	var items []menuItem
	icon := ic("command")
	switch {
	case a.scr == scrTerminal && a.current() != nil && a.current().filesFocus():
		s := a.current()
		f := &s.files
		if p := f.panes[f.side]; p != nil {
			title = i18n.T("DATEIEN") + "  " + trunc(p.path, 30)
			items = a.fileMenuItems(s, f.side, p.sel)
			icon = ic("open")
		}
	case a.scr == scrManager:
		if r, ok := a.mgrCur(); ok {
			if m := a.mgrMenu(r, 0, 0); m != nil {
				title = trunc(strings.ToUpper(r.name()), 30)
				items = m.items
				icon = ic("manager")
			}
		}
	}
	if len(items) == 0 {
		return "", nil
	}
	var out []palItem
	for _, mi := range items {
		if limit >= 0 && len(out) >= limit {
			break
		}
		if limit >= 0 && mi.hint == "⏎" { // Verbinden, Aufklappen: macht ⏎ ohnehin
			continue
		}
		mi := mi
		out = append(out, palItem{group: pgHere, icon: icon, label: mi.label, hint: mi.hint, run: mi.run})
	}
	return i18n.T("HIER") + " · " + title, out
}

// ---------------------------------------------------------------- NEU (8)

// palNewItems: alle Einträge mit NEU-Marke (Aktionen und Einstellungen).
func (a *App) palNewItems(q string) []palItem {
	s := a.current()
	inTerm := a.scr == scrTerminal && s != nil && s.state == stConnected
	all := a.palActions(s, inTerm, a.scr == scrManager)
	all = append(all, a.palMenuItems()...)
	var out []palItem
	seen := map[string]bool{}
	for _, it := range all {
		if it.open != "" || !isFresh(it.label) || seen[it.label] {
			continue
		}
		seen[it.label] = true
		it.sub = palWhereSub(it, "")
		it.home, it.group = it.group, pgNew
		out = append(out, it)
	}
	if q != "" {
		return a.palRank(q, out)
	}
	return out
}

// palNewCount: wie viele Einträge gerade NEU tragen (für den Ordner).
func (a *App) palNewCount() int {
	if len(menuFresh) == 0 {
		return 0
	}
	return len(a.palNewItems(""))
}

// ---------------------------------------------------------------- Geändert (9)

// palDefaults: Standardwert jeder sichtbaren Einstellung (deutsch, wie er
// rechts in der Zeile steht; Schalter „an“/„aus“). Was davon abweicht,
// trägt einen gelben Punkt; Entf stellt den Standard wieder her.
// TestPalDefaultsComplete prüft, dass jede Einstellung hier steht und eine
// frische Installation nichts als geändert zeigt.
var palDefaults = map[string]string{
	"Einfacher Modus: Profi-Funktionen ausblenden":            "aus",
	"Maus: klicken, markieren, scrollen":                      "an",
	"Auto-Reconnect: nach Abbruch neu verbinden":              "an",
	"Online-Punkt: Erreichbarkeit der Hosts prüfen":           "sichtbare",
	"Meldung, wenn lange Befehle fertig sind":                 "+ Desktop",
	"Tastenzeile: Tasten erklären":                            "aus",
	"Tipps zu Tasten":                                         "an",
	"Fenster beim Start":                                      "Vollbild",
	"Farbschema wählen …":                                     "Dunkel (Nacht)",
	"Symbole":                                                 "Text-Tags",
	"Fensterstil":                                             "eckig + Schatten",
	"Bewegung":                                                "voll",
	"Barrierearm: keine Animationen, Formen statt nur Farben": "aus",
	"Geteilte Ansicht: andere Bereiche abdunkeln":             "aus",
	"Einfügeschutz: Vorschau bei mehreren Zeilen":             "an",
	"Server darf in die Zwischenablage kopieren (OSC 52)":     "an",
	"Automatisch sperren bei Leerlauf":                        "aus",
	"Quelle":                                                  "fragt beim ersten Mal",
	"Laufende Container mitprüfen":                            "an",
	"KI-Datenschutz":                                          "alle Anbieter",
	"MCP-Zugriff für KI-Clients (Claude, …)":                  "aus",
	"KI-Begleiter":                                            "Aus",
	"Höchstens KI-Anfragen pro Stunde":                        "30",
	"Tagesbudget (geschätzte Tokens)":                         "150k",
	"Begleiter nur mit eigener KI":                            "aus",
	"Filter aus bei eigener KI (Geheimnisse nicht entfernen)": "aus",
	"Coach: Rückblick beim Schließen des Tabs":                "an",
	"Sitzungsprotokoll schreiben":                             "an",
	"Logwache":                                                "aus",
	"Syslog-Empfang (Switches, Firewalls)":                    "aus",
	"Syslog auch von unbekannten Absendern annehmen":          "aus",
	"Audit-Protokoll (wer, wann, was; fälschungssicher)":      "aus",
	"Updates anbieten":                                        "sofort",
	"Debug-Protokoll für Fehlerberichte":                      "aus",
	"Leistungsanzeige (Bildrate, Zeichenzeit)":                "aus",
}

// palValue: angezeigter Wert einer Einstellung.
func palValue(it palItem) string {
	switch it.state {
	case palOn:
		return i18n.T("an")
	case palOff:
		return i18n.T("aus")
	}
	return it.hint
}

// palDefault: Standardwert (übersetzt) und ob der Eintrag davon abweicht.
func palDefault(it palItem) (string, bool) {
	if it.menu == "" || it.open != "" {
		return "", false
	}
	d, ok := palDefaults[i18n.German(it.label)]
	if !ok {
		return "", false
	}
	d = i18n.T(d)
	return d, palValue(it) != d
}

// palChangedItems: alle Einstellungen, die vom Standard abweichen („!“).
func (a *App) palChangedItems() []palItem {
	var out []palItem
	for _, it := range a.palMenuItems() {
		if _, ch := palDefault(it); ch {
			it.sub = palWhereSub(it, "")
			it.home, it.group = it.group, pgChanged
			it.changed = true
			out = append(out, it)
		}
	}
	return out
}

// palReset stellt eine Einstellung auf den Standard zurück: Schalter einmal
// umlegen, Durchschalt-Werte weiterdrehen, bis der Standard erscheint.
func (a *App) palReset(it palItem) tea.Cmd {
	def, ch := palDefault(it)
	if !ch {
		a.flash(i18n.T("Steht schon auf dem Standard"))
		return nil
	}
	if i18n.German(it.label) == "Farbschema wählen …" {
		return a.setTheme(Nacht.Name)
	}
	if it.state != palNoState {
		cmd := it.run()
		a.flash(i18n.T("Zurückgesetzt: ") + it.label + " → " + def)
		return cmd
	}
	if it.cycle == nil {
		a.flash(i18n.T("Diese Einstellung lässt sich hier nicht zurücksetzen"))
		return nil
	}
	var cmds []tea.Cmd
	for range 40 {
		cmds = append(cmds, it.cycle(1))
		cur, ok := a.palFindSetting(it.label, it.menu)
		if !ok || palValue(cur) == def {
			break
		}
	}
	a.flash(i18n.T("Zurückgesetzt: ") + it.label + " → " + def)
	return tea.Batch(cmds...)
}

// palFindSetting: aktueller Stand einer Einstellung (nach dem Umschalten).
func (a *App) palFindSetting(label, menu string) (palItem, bool) {
	for _, x := range a.palMenuItems() {
		if x.label == label && x.menu == menu {
			return x, true
		}
	}
	return palItem{}, false
}

// ---------------------------------------------------------------- Markieren

// palMark setzt NEU- und Geändert-Marken.
func palMark(items []palItem) []palItem {
	for i := range items {
		if isFresh(items[i].label) {
			items[i].fresh = true
		}
		if _, ch := palDefault(items[i]); ch {
			items[i].changed = true
		}
	}
	return items
}
