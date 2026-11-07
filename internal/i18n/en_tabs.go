package i18n

// Tab-Verwaltung (ui/tabs.go): „+“, „×“, Rechtsklick-Menü, Tab-Liste.
// Eigene Datei, damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enTabs {
		en[k] = v
	}
}

var enTabs = map[string]string{
	"Neuer Tab":                           "New tab",
	"Alle Tabs (Liste)":                   "All tabs (list)",
	"alle Tabs (Liste)":                   "all tabs (list)",
	"neuer Tab":                           "new tab",
	"Tab-Liste":                           "tab list",
	"Rechtsklick Tab":                     "Right-click tab",
	"Mittelklick Tab":                     "Middle-click tab",
	"Menü: umbenennen, schließen …":       "Menu: rename, close …",
	"Tab schließen?":                      "Close tab?",
	"Die Verbindung zu %s wird getrennt.": "The connection to %s will be closed.",
	"Läuft dort noch etwas, bleibt es nur in tmux oder screen erhalten.": "Anything still running there survives only inside tmux or screen.",
	"Andere Tabs schließen?": "Close other tabs?",
	"%d Tabs außer %s werden geschlossen und ihre Verbindungen getrennt.": "%d tabs other than %s will be closed and disconnected.",
	"Tab umbenennen": "Rename tab",
	"Leer lassen für den Namen der Verbindung.": "Leave empty to use the connection name.",
	"Umbenennen":                      "Rename",
	"Duplizieren":                     "Duplicate",
	"Neu verbinden":                   "Reconnect",
	"In eigenes Fenster":              "Move to own window",
	"Aus eigenem Fenster zurückholen": "Bring back from own window",
	"Nach links":                      "Move left",
	"Nach rechts":                     "Move right",
	"Andere Tabs schließen":           "Close other tabs",
	"offen":                           "open",
	"öffnen":                          "open",
	"Neue Verbindung anlegen …":       "Create new connection …",
	"verbindet …":                     "connecting …",
	"verbindet neu …":                 "reconnecting …",
	"Offene Tabs":                     "Open tabs",
	"Name, Ordner oder Host":          "name, folder or host",
	"wechseln":                        "switch",
	"springen":                        "jump",
	"duplizieren":                     "duplicate",
	"Alle Tabs zeigen":                "Show all tabs",
	"Tab duplizieren":                 "Duplicate tab",
}
