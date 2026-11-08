package i18n

// Debug-Protokoll (internal/debuglog, ui/debugreport.go) und Barrierearm (ui/a11y.go).
func init() {
	for k, v := range enDebugLog {
		en[k] = v
	}
}

var enDebugLog = map[string]string{
	"Debug-Protokoll für diesen Start schreiben (debug.log im Datenordner)": "write a debug log for this run (debug.log in the data folder)",
	"Debug-Protokoll: ":                                       "Debug log: ",
	"Debug-Protokoll an: ":                                    "Debug log on: ",
	"Debug-Protokoll aus":                                     "Debug log off",
	"Debug-Protokoll geleert":                                 "Debug log cleared",
	"Debug-Protokoll für Fehlerberichte":                      "Debug log for bug reports",
	"Debug-Protokoll bereinigt speichern (für Fehlerbericht)": "Save cleaned debug log (for bug reports)",
	"Debug-Protokoll leeren":                                  "Clear debug log",
	"Noch kein Debug-Protokoll. Erst einschalten, dann den Fehler nachstellen.":                           "No debug log yet. Turn it on first, then reproduce the problem.",
	"Bereinigtes Debug-Protokoll gespeichert: ":                                                           "Cleaned debug log saved: ",
	"Barrierearm: keine Animationen, Formen statt nur Farben":                                             "Accessible: no animations, shapes instead of color only",
	"Barrierearm an: keine Animationen, × für nicht erreichbar. Farbschema „Farbenblind“ unter Aussehen.": "Accessible mode on: no animations, × for unreachable. Color scheme “Color-blind” under Appearance.",
	"Barrierearm aus":            "Accessible mode off",
	"Farbenblind (dunkel)":       "Color-blind (dark)",
	"Farbenblind (hell)":         "Color-blind (light)",
	"Speichern fehlgeschlagen: ": "Saving failed: ",
}
