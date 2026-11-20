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
	" (kleiner als 80×20: manches wird gekürzt)":         " (smaller than 80×20: some things get cut off)",
	" + Windows-Konto":                       " + Windows account",
	" antwortet":                             " answers",
	" antwortet nicht: ":                     " does not answer: ",
	"%d im Ordner %s":                        "%d in folder %s",
	"Absturzberichte":                        "Crash reports",
	"Alles in Ordnung.":                      "Everything is fine.",
	"Ausgabe ist kein Terminal (umgeleitet)": "Output is not a terminal (redirected)",
	"Es gibt Probleme (Zeilen mit ×). Text kopieren und dem Fehlerbericht beilegen.": "There are problems (lines with ×). Copy this text and attach it to your bug report.",
	"GitHub erreichbar":                                        "GitHub reachable",
	"GitHub erreichbar, Uhr stimmt":                            "GitHub reachable, clock is correct",
	"GitHub nicht erreichbar, Updates gehen nicht: ":           "GitHub not reachable, updates won't work: ",
	"Internet und Uhr":                                         "Internet and clock",
	"Läuft, aber nicht alles ist eingerichtet (Zeilen mit !).": "Works, but not everything is set up (lines with !).",
	"Selbstprüfung":                                            "Self-check",
	"Selbstprüfung (Ordner, Tresor, KI, Internet, Uhr)":        "Self-check (folder, vault, AI, internet, clock)",
	"Selbstprüfung läuft …":                                    "Self-check running …",
	"Tresor":                                                   "Vault",
	"Uhr geht %s falsch: Anmeldung mit Einmal-Codes oder Zertifikaten kann scheitern": "Clock is off by %s: sign-in with one-time codes or certificates may fail",
	"Update-Signatur": "Update signature",
	"Updates werden vor dem Einspielen geprüft": "Updates are verified before installing",
	"beschreibbar": "writable",
	"eigener Build ohne Schlüssel: Updates werden nicht geprüft": "own build without key: updates are not verified",
	"für andere lesbar (%o), besser: chmod 700":                  "readable by others (%o), better: chmod 700",
	"ist eine Datei, kein Ordner":                                "is a file, not a folder",
	"lesbar, Master-Passwort":                                    "readable, master password",
	"nicht beschreibbar: ":                                       "not writable: ",
	"nicht eingerichtet":                                         "not set up",
	"nicht erreichbar (nur nötig für Schlüssel im Agenten): ":    "not reachable (only needed for keys in the agent): ",
	"nicht verfügbar: Passwörter liegen nur im Tresor":           "not available: passwords are kept in the vault only",
	"noch nicht angelegt (erster Start)":                         "not created yet (first start)",
	"sshTermy-Selbstprüfung …":                                   "sshTermy self-check …",
	"unbekannt (kein Benutzerordner?)":                           "unknown (no user folder?)",
	"verfügbar (Passwörter können dort liegen)":                  "available (passwords can be stored there)",
}
