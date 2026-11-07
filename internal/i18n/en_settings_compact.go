package i18n

// Kompaktes Einstellungsmenü der Befehlspalette (ui/palette.go).
// Eigene Datei, damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enSettingsCompact {
		en[k] = v
	}
}

var enSettingsCompact = map[string]string{
	"SICHERHEIT":     "SECURITY",
	"Farbschema":     "Color scheme",
	"Eigene Farben":  "Custom colors",
	"Symbole":        "Icons",
	"Gefahren-Check": "Danger check",
	"alle Hosts":     "all hosts",
	"nur Produktiv":  "production only",
	"Meldung, wenn lange Befehle fertig sind": "Notify when long commands finish",
	"in sshTermy":                       "in sshTermy",
	"+ Desktop":                         "+ desktop",
	"durchschalten":                     "cycle",
	"Endpunkt, Ersatz-Anbieter, Status": "endpoint, fallback providers, status",
	"Server hinter NAT/Firewall":        "servers behind NAT/firewall",
	"Import / Export":                   "Import / export",
	"ssh-config, PuTTY, Ansible, CSV":   "ssh-config, PuTTY, Ansible, CSV",
}
