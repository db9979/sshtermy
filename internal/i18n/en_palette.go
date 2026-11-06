package i18n

// Verbindungen in der Befehlspalette (ui/palconn.go).
// Eigene Datei, damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enPalette {
		en[k] = v
	}
}

var enPalette = map[string]string{
	"Alle %d Verbindungen im Manager":      "All %d connections in the manager",
	"Name, Ordner oder #Tag tippen":        "type a name, folder or #tag",
	"%d weitere Treffer im Manager zeigen": "Show %d more matches in the manager",
	"genauer tippen":                       "type more",
	"VERBINDUNGEN · beste Treffer":         "CONNECTIONS · best matches",
	"VERBINDUNGEN · zuletzt benutzt":       "CONNECTIONS · recently used",
	"VERBINDUNGEN":                         "CONNECTIONS",

	// Untermenüs Einstellungen / Hilfe & Info (ui/palette.go)
	"Hilfe & Info":                                         "Help & info",
	"Maus, Aussehen, Sprache, KI …":                        "mouse, look, language, AI …",
	"Tasten, Tour, Neues, Update":                          "keys, tour, news, update",
	"VERHALTEN":                                            "BEHAVIOUR",
	"AUSSEHEN & SPRACHE":                                   "LOOK & LANGUAGE",
	"EINRICHTEN":                                           "SET UP",
	"HILFE & INFO":                                         "HELP & INFO",
	"ausführen / umschalten":                               "run / toggle",
	"Maus: klicken, markieren, scrollen":                   "Mouse: click, select, scroll",
	"Auto-Reconnect: nach Abbruch neu verbinden":           "Auto-reconnect after a dropped session",
	"Online-Punkt: Erreichbarkeit der Hosts prüfen":        "Online dot: check whether hosts are reachable",
	"Sitzungsprotokoll schreiben":                          "Write session log",
	"Einfügeschutz: Vorschau bei mehreren Zeilen":          "Paste guard: preview multi-line pastes",
	"Gefahren-Check auf allen Hosts (sonst nur Produktiv)": "Danger check on all hosts (otherwise production only)",
	"◈ Einstellungen ›":                                    "◈ Settings ›",
	"Schalter, Aussehen, Sprache":                          "toggles, look, language",
	"Esc / ← (Untermenü)":                                  "Esc / ← (submenu)",
	"Startanimation":                                       "Startup animation",
}
