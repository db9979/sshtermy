package i18n

// Ideen aus termilly (eigene Datei gegen Konflikte in en.go).
func init() {
	for de, e := range map[string]string{
		" · Tokens hin ↑ und zurück ↓, ~ = geschätzt": " · tokens sent ↑ and received ↓, ~ = estimated",
		"%.1f Mio.":   "%.1fM",
		"%d Anfragen": "%d requests",
		"30 Tage":     "30 days",
		"Eingabe / Ausgabe, z. B. 3 / 15; leer = keine Kosten zeigen (eigener Server)": "input / output, e.g. 3 / 15; empty = show no cost (own server)",
		"Ghost-Text nur noch von der KI":                                               "Ghost text only from the AI",
		"Ghost-Text schlägt zuerst Befehle aus deinem Verlauf vor":                     "Ghost text suggests commands from your history first",
		"Ghost-Text zuerst aus dem Verlauf":                                            "Ghost text from history first",
		"Im Web suchen":                                                                "Search the web",
		"KI-Verbrauch (Tokens, Kosten)":                                                "AI usage (tokens, cost)",
		"Kosten: Preis je 1 Mio. Tokens in den KI-Einstellungen eintragen (s)":         "Cost: enter the price per 1M tokens in the AI settings (s)",
		"Markierung am Prompt":                                                         "selection at the prompt",
		"Nichts zu suchen: erst Text markieren":                                        "Nothing to search: select some text first",
		"Noch nichts kopiert, seit sshTermy läuft":                                     "Nothing copied yet since sshTermy started",
		"Preis/1 Mio. Tokens":                                                          "Price per 1M tokens",
		"Suchmaschine für „Im Web suchen“":                                             "Search engine for “Search the web”",
		"Verbrauch":                                                                    "Usage",
		"Wieder in der Zwischenablage: ":                                               "Back in the clipboard: ",
		"Zuletzt Kopiertes im Web suchen":                                              "Search the web for what you copied last",
		"Zwischenablage-Verlauf":                                                       "Clipboard history",
		"Zwischenablage-Verlauf (zuletzt Kopiertes einfügen)":                          "Clipboard history (paste something copied earlier)",
		"Zwischenablage-Verlauf · ⏎ fügt ein":                                          "Clipboard history · ⏎ pastes",
		"heute meist: ":                                                                "today mostly: ",
		"heute":                                                                        "today",
		"7 Tage":                                                                       "7 days",
		"Zwischenablage-Verlauf: wieder einfügen":                                      "clipboard history: paste again",
		"Rechtsklick auf Markierung":                                                   "right-click a selection",
		"kopieren, im Web suchen":                                                      "copy, search the web",
		"Ghost-Text übernehmen (zuerst aus dem Verlauf)":                               "accept ghost text (history first)",
		"wie bisher":                                                                   "as before",
	} {
		en[de] = e
	}
}
