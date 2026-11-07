package i18n

// Texte für die lokale Shell (eigene Datei, damit parallele Änderungen an
// en.go nicht kollidieren).
var enLocal = map[string]string{
	") · z. B. pwsh, cmd, wsl, zsh, bash -l": ") · e.g. pwsh, cmd, wsl, zsh, bash -l",
	"Lokal":                        "Local",
	"lokal":                        "local",
	"Lokal (dieser Rechner)":       "Local (this computer)",
	"Lokale Shell":                 "Local shell",
	"Lokale Shell in neuem Tab":    "Local shell in a new tab",
	"Lokale Shell startet nicht: ": "Local shell does not start: ",
	"Lokale Shell öffnen":          "Open local shell",
	"Lokale Shell öffnen: ":        "Open local shell: ",
	"Standard-Shell":               "default shell",
	"Startordner":                  "Start folder",
	"Telnet = unverschlüsselt (ältere Switches) · Seriell = USB-Konsolenkabel · Lokal = Shell auf diesem Rechner": "Telnet = unencrypted (older switches) · Serial = USB console cable · Local = shell on this computer",
	"dieser Rechner": "this computer",
	"leer = Benutzerordner · z. B. ~/projekte": "empty = home folder · e.g. ~/projects",
	"leer = Standard dieses Rechners (jetzt: ": "empty = this computer's default (now: ",
	"● dieser Rechner":                         "● this computer",
}

func init() {
	for k, v := range enLocal {
		en[k] = v
	}
}
