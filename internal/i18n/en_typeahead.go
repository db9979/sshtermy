package i18n

// Texte für Vorausgetipptes beim Verbinden (internal/ui/typeahead.go).
func init() {
	for de, e := range map[string]string{
		"Vorausgetippter Text verworfen: kein Prompt erkannt": "Text typed ahead discarded: no prompt detected",
		"Getippt: ":                          "Typed: ",
		"kommt am Prompt an · Esc bricht ab": "goes to the prompt · Esc cancels",
	} {
		en[de] = e
	}
}
