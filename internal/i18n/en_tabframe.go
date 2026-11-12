package i18n

// Farbige Rahmen je Tab (eigene Datei gegen Konflikte in en.go).
func init() {
	for de, e := range map[string]string{
		"Farbiger Rahmen um jeden Tab":                     "Colored frame around each tab",
		"Jeder Tab hat jetzt einen Rahmen in seiner Farbe": "Each tab now has a frame in its own color",
		"Rahmen um die Tabs aus":                           "Tab frames off",
		"Rahmen und Name im Tab, Statuszeile, Liste · automatisch: prod rot, test grün, labor blau, sonst je Tab eine eigene Farbe (nie Rot)": "frame and name in the tab, status line, list · automatic: prod red, test green, lab blue, otherwise each tab its own color (never red)",
	} {
		en[de] = e
	}
}
