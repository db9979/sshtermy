package i18n

// Texte für KI-Antwortkarten und Toasts (eigene Datei, damit parallele
// Änderungen an en.go nicht kollidieren).
func init() {
	for de, e := range map[string]string{
		". Nochmal a zum Ausführen":       ". Press a again to run",
		"Ausführen":                       "Run",
		"Befehlskarte wählen":             "choose command card",
		"Befehl ausführen (rote zweimal)": "run command (red ones twice)",
		"Befehl kopieren":                 "copy command",
		"Erklärung ein-/ausklappen":       "collapse/expand explanation",
		"Befehl wählen":                   "choose command",
		"Erklärung":                       "Explanation",
		"Skript · %d Zeilen":              "Script · %d lines",
		"Verbunden: ":                     "Connected: ",
		"ausklappen":                      "expand",
		"einklappen":                      "collapse",
		"sicher? nochmal a":               "sure? press a again",
	} {
		if _, ok := en[de]; !ok {
			en[de] = e
		}
	}
}
