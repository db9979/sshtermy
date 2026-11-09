package i18n

// KI schneller und gezielter: schnelles Modell, Hausregeln, Antwortzeiten
// je Aufgabe (ui/llmhealth.go, ui/mgrtree.go). Eigene Datei, damit
// parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enAITune {
		en[k] = v
	}
}

var enAITune = map[string]string{
	" · Ghost-Text, Folgebefehle, Begleiter": " · ghost text, follow-ups, companion",
	" · seit dem Start, Mittel und zuletzt":  " · since start, average and last",
	"Agent":                                  "Agent",
	"Antwortzeit je Aufgabe":                 "Response time per task",
	"Befehl anpassen":                        "Adapt command",
	"Der ganze Pfad lässt sich ändern, so wandert der Ordner samt Inhalt woanders hin.": "You can change the whole path; the folder and its contents move there.",
	"Fehler erklären":   "Explain error",
	"Folgebefehle":      "Follow-ups",
	"Hausregeln":        "House rules",
	"KI-Hausregeln":     "AI house rules",
	"Logdiagnose":       "Log diagnosis",
	"Name":              "Name",
	"Ordner bearbeiten": "Edit folder",
	"Runbook prüfen":    "Check runbook",
	"Schnell":           "Fast",
	"Schnelles Modell":  "Fast model",
	"Zusammenfassung":   "Summary",
	"gelten für alle Verbindungen hier und in Unterordnern, z. B. „Aruba 2930F, nur lesen ohne Rückfrage“": "apply to all connections here and in subfolders, e.g. “Aruba 2930F, read-only without asking”",
	"gelten für jede KI-Aufgabe, z. B. „Debian 12, nie reboot vorschlagen“; Ordner: e im Manager":          "apply to every AI task, e.g. “Debian 12, never suggest reboot”; folders: e in the manager",
	"optional, selber Endpunkt: übernimmt Ghost-Text, Folgebefehle und Begleiter":                          "optional, same endpoint: handles ghost text, follow-ups and companion",
}
