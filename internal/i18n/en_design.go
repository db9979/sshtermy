package i18n

// Übersichtlicheres Design: eine Kopfzeile, F-Tasten-Leiste, Manager, F1-Reiter.
func init() {
	for de, e := range map[string]string{
		"▲ PRODUKTIV":      "▲ PRODUCTION",
		"▲ PROD":           "▲ PROD",
		"Sync aus":         "sync off",
		"In diesem Ordner": "In this folder",
		"Reiter":           "tab",
		"Suche kürzen":     "shorten search",
		"  (alle Reiter)":  "  (all tabs)",
		"  tippen filtert": "  type to filter",
		"Keine Taste passt. Backspace löscht, Esc leert die Suche.":                                         "No key matches. Backspace deletes, Esc clears the search.",
		"Doppelklick verbindet · Rechtsklick öffnet das Menü · Zeilen links lassen sich auf Ordner ziehen.": "Double-click connects · right-click opens the menu · drag rows on the left onto folders.",
	} {
		en[de] = e
	}
}
