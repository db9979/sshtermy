package i18n

// Texte fürs Blättern in der Seitenleiste und die vollständige
// Zusammenfassung im Ziel-/Agent-Modus (eigene Datei gegen Konflikte in en.go).
func init() {
	for de, e := range map[string]string{
		"Kopiert: Zusammenfassung":                 "Copied: summary",
		"Ziel: ":                                   "Goal: ",
		"Bild↑ Bild↓":                              "PgUp PgDn",
		"ganz lesen":                               "read all",
		"Ergebnis kopieren / ganz lesen":           "copy result / read all",
		"im Panel blättern (auch Mausrad)":         "scroll the panel (mouse wheel too)",
		"↑ %d weitere Zeilen · Bild↑ oder Mausrad": "↑ %d more lines · PgUp or mouse wheel",
		"↓ %d weitere Zeilen · Bild↓ oder Mausrad": "↓ %d more lines · PgDn or mouse wheel",
	} {
		en[de] = e
	}
}
