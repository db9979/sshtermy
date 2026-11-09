package i18n

// Texte für Abbrechen und Folgefragen im Frage-Panel (eigene Datei, damit
// parallele Änderungen an en.go nicht kollidieren).
func init() {
	for de, e := range map[string]string{
		"Antwort abbrechen":             "stop answer",
		"Folgefrage":                    "follow-up",
		"Neue Frage":                    "New question",
		"└ Folgefrage (%d) zu: ":        "└ follow-up (%d) to: ",
		"■ abgebrochen":                 "■ stopped",
		"Esc / Strg+C":                  "Esc / Ctrl+C",
		"Folgefrage (Verlauf geht mit)": "follow-up (keeps the conversation)",
		"neue Frage (ohne Verlauf)":     "new question (fresh)",
	} {
		en[de] = e
	}
}
