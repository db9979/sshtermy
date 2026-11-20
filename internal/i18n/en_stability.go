package i18n

// Stabilität (2026-11-20): Absturzschutz (ui/safety.go, internal/safe),
// Selbstprüfung (sshtermy doctor).
func init() {
	for k, v := range enStability {
		en[k] = v
	}
}

var enStability = map[string]string{
	"Fehler abgefangen, sshTermy lief weiter":            "Error caught, sshTermy kept running",
	"Interner Fehler abgefangen, sshTermy läuft weiter.": "Internal error caught, sshTermy keeps running.",
	"update: Download kam %s lang nicht voran":           "update: download made no progress for %s",
}
