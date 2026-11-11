package i18n

// Texte für Antwortkarten nach Fehlern (cardfix.go).
func init() {
	for de, e := range map[string]string{
		"Fix übernehmen": "use fix",
		"Fix übernommen": "Fix applied",
		"Fix übernommen · %d weitere Karten angepasst": "Fix applied · %d more cards adjusted",
		"Fix:":                 "Fix:",
		"KI sucht einen Fix …": "AI is looking for a fix …",
		"Kein Fix per Befehl":  "No fix by command",
		"erklärt":              "explains",
		"fehlgeschlagen: ":     "failed: ",
		"korrigiert":           "corrected",
		"so hast du es auf diesem Gerät schon einmal korrigiert": "you corrected it like this on this device before",
		"✓ angepasst wie Karte %d":                               "✓ adjusted like card %d",
		"nach Fehler: Fix in die Karte übernehmen":               "after an error: put the fix into the card",
	} {
		en[de] = e
	}
}
