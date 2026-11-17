package i18n

// Menüeinträge leichter finden: Suche, Häufig, Anheften, Hier, Tasten,
// NEU, geänderte Einstellungen, F1 ausführbar (ui/palfind.go, palstate.go,
// palhere.go, palask.go).
func init() {
	for k, v := range enMenuFind {
		en[k] = v
	}
}

var enMenuFind = map[string]string{
	// Gruppen
	"TREFFER":                     "MATCHES",
	"HIER":                        "HERE",
	"ANGEHEFTET":                  "PINNED",
	"TASTE":                       "KEY",
	"MEINTEST DU":                 "DID YOU MEAN",
	"NEU SEIT DEM LETZTEN UPDATE": "NEW SINCE THE LAST UPDATE",
	"GEÄNDERT":                    "CHANGED",
	"DATEIEN":                     "FILES",
	"NEU":                         "NEW",

	// Palette
	"%s ist frei":                            "%s is free",
	"Frei für eigene Belegung: ":             "Free for your own binding: ",
	"anpassen":                               "customize",
	"dort zeigen":                            "show where",
	"anheften":                               "pin",
	"lösen":                                  "unpin",
	"auf Standard":                           "reset",
	"Angeheftet: ":                           "Pinned: ",
	"Gelöst: ":                               "Unpinned: ",
	"Diesen Eintrag kann man nicht anheften": "This entry can't be pinned",
	"Alle Einstellungen stehen auf dem Standard.": "All settings are at their defaults.",
	"Standard: ":                   "Default: ",
	"Zu finden unter: ":            "Found under: ",
	"Zurückgesetzt: ":              "Reset: ",
	"Steht schon auf dem Standard": "Already at the default",
	"Diese Einstellung lässt sich hier nicht zurücksetzen": "This setting can't be reset here",
	"Tipps zu Tasten": "Shortcut tips",
	"Tipp: „%s“ geht direkt mit %s. Abschalten: in der Palette „Tipps“ suchen.": "Tip: “%s” is also on %s. To turn tips off, search “tips” in the palette.",
	"Neu seit dem letzten Update":     "New since the last update",
	"neue Einträge, 14 Tage markiert": "new entries, marked for 14 days",

	// KI: Wo finde ich …?
	"KI fragen: Wo finde ich „%s“?":    "Ask AI: where do I find “%s”?",
	"KI sucht den passenden Eintrag …": "AI is looking for the right entry …",
	"KI: ":                             "AI: ",
	"Dazu gibt es keinen Eintrag.":     "There is no entry for that.",
	"Menü-Suche":                       "Menu search",

	// Hilfe (F1)
	"Diese Zeile ist keine eigene Aktion. Passende Einträge:": "This line is not an action of its own. Matching entries:",
	"Zeile der Hilfe ausführen":                               "run the selected help line",
	"dort zeigen, wo er im Menü steht":                        "show where it lives in the menu",
	"anheften (oben in der Übersicht)":                        "pin (top of the overview)",
	"→ (Treffer)":                                             "→ (match)",
	"* / Rechtsklick":                                         "* / right-click",
	"„alt+e“, „f6“":                                           "“alt+e”, “f6”",
	"„!“":                                                     "“!”",
	"Taste nachschlagen":                                      "look up a key",
	"geänderte Einstellungen zeigen":                          "show changed settings",
	"Einstellung auf Standard":                                "reset setting to default",
}
