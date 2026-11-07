package i18n

// Alle Einstellungen im Sync (ui/look.go). Eigene Datei, damit parallele
// Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enSyncSettings {
		en[k] = v
	}
}

var enSyncSettings = map[string]string{
	"Nerd-Font-Symbole sind auf einem anderen Rechner an; hier erst nach der Probe (Einstellungen › Symbole)": "Nerd Font icons are on on another computer; here only after the test (Settings › Icons)",
}
