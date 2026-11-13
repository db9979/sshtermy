package i18n

// Texte für „docker exec ohne -it“ (internal/ui/ttyhint.go).
func init() {
	for de, e := range map[string]string{
		"%s braucht ein Terminal: %s ergänzt. ⏎ führt aus (ohne: Zeile neu tippen)": "%s needs a terminal: added %s. ⏎ runs it (without: retype the line)",
	} {
		en[de] = e
	}
}
