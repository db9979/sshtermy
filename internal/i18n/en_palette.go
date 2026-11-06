package i18n

// Verbindungen in der Befehlspalette (ui/palconn.go).
// Eigene Datei, damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enPalette {
		en[k] = v
	}
}

var enPalette = map[string]string{
	"Alle %d Verbindungen im Manager":      "All %d connections in the manager",
	"Name, Ordner oder #Tag tippen":        "type a name, folder or #tag",
	"%d weitere Treffer im Manager zeigen": "Show %d more matches in the manager",
	"genauer tippen":                       "type more",
	"VERBINDUNGEN · beste Treffer":         "CONNECTIONS · best matches",
	"VERBINDUNGEN · zuletzt benutzt":       "CONNECTIONS · recently used",
	"VERBINDUNGEN":                         "CONNECTIONS",
}
