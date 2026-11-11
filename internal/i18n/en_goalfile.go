package i18n

// Ziel-Modus 2026-11-11: mehrzeilige Inhalte als Datei, Konfiguration
// ergänzen statt ersetzen.
func init() {
	for k, v := range map[string]string{
		"ersetzt eine Datei im Container": "replaces a file in the container",
		"Datei anlegen":                   "create file",
		"ansehen":                         "view",
		"Arbeitsordner löschen":           "delete work folder",
	} {
		en[k] = v
	}
}
