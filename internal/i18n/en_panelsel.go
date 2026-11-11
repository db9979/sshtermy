package i18n

// Markieren und Kopieren im Seitenpanel (eigene Datei gegen Konflikte in en.go).
func init() {
	for de, e := range map[string]string{
		"markieren + kopieren, auch im Panel": "select + copy, also in the panel",
	} {
		en[de] = e
	}
}
