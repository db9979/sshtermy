package i18n

// Gesamtsicherung (internal/fullbackup, ui/fullbackup.go, cmd/sshtermy/backupcmd.go)
// und KI-Datenschutz (llm/privacy.go, ui/aiprivacy.go).
func init() {
	for k, v := range enBackup {
		en[k] = v
	}
}

var enBackup = map[string]string{
	"Alle Daten in %s ersetzen? [j/N] ":                 "Replace all data in %s? [y/N] ",
	"Aufruf: sshtermy restore <datei>":                  "Usage: sshtermy restore <file>",
	"Eingespielt. Bisheriger Stand: %s":                 "Restored. Previous state: %s",
	"Gesamtsicherung eingespielt. Bisheriger Stand: ":   "Full backup restored. Previous state: ",
	"Gesamtsicherung einspielen":                        "Restore full backup",
	"Gesamtsicherung einspielen …":                      "Restore full backup …",
	"Gesamtsicherung gespeichert: %s (%d Dateien)":      "Full backup saved: %s (%d files)",
	"Gesamtsicherung nicht eingespielt: ":               "Full backup not restored: ",
	"Gesamtsicherung speichern":                         "Save full backup",
	"Gesamtsicherung speichern (für Umzug/Sicherung) …": "Save full backup (to move or back up) …",
	"KI aus": "AI off",
	"KI ist ausgeschaltet (Einstellungen › KI › KI-Datenschutz)": "AI is turned off (Settings › AI › AI privacy)",
	"KI ist für diese Verbindung aus (Tag ki-aus)":               "AI is off for this connection (tag no-ai)",
	"KI-Datenschutz":                  "AI privacy",
	"KI-Datenschutz: ":                "AI privacy: ",
	"Kein Store in ":                  "No store in ",
	"Keine sshTermy-Sicherung":        "Not an sshTermy backup",
	"Letzte KI-Anfrage":               "Last AI request",
	"Letzte KI-Anfrage ansehen":       "Show last AI request",
	"Master-Passwort der Sicherung":   "Master password of the backup",
	"Master-Passwort der Sicherung: ": "Master password of the backup: ",
	"Master-Passwort fehlt":           "Master password missing",
	"Master-Passwort stimmt nicht":    "Master password is wrong",
	"Master-Passwort: ":               "Master password: ",
	"Nur eigene KI erlaubt, aber kein Endpunkt im eigenen Netz oder als eigener KI-Server eingetragen (z. B. Ollama auf localhost)": "Only your own AI allowed, but no endpoint on your network or registered as your own AI server (e.g. Ollama on localhost)",
	"Passwort stimmt nicht oder die Datei ist beschädigt":                                                                           "Wrong password or damaged file",
	"Passwörter, Keys und Tokens werden vor dem Senden entfernt.":                                                                   "Passwords, keys and tokens are removed before sending.",
	"Pfad zur .sshtbak-Datei":                     "Path to the .sshtbak file",
	"Seit dem Start ging noch nichts an eine KI.": "Nothing has been sent to an AI since start.",
	"Sicherung vom %s (v%s, %s, %d Dateien).":     "Backup from %s (v%s, %s, %d files).",
	"Sicherung vom %s (v%s, %s, %d Dateien).\n\nAlle Daten hier werden durch die Sicherung ersetzt. Der bisherige Stand bleibt im Ordner\n%s.vorher-…\n\nsshTermy startet dafür neu, offene Sitzungen werden getrennt.": "Backup from %s (v%s, %s, %d files).\n\nAll data here will be replaced by the backup. The previous state stays in the folder\n%s.vorher-…\n\nsshTermy restarts for this; open sessions will be disconnected.",
	"alle Anbieter":                                                      "all providers",
	"alles, verschlüsselt":                                               "everything, encrypted",
	"außerhalb des eigenen Netzes":                                       "outside your own network",
	"das Passwort, mit dem die Sicherung gespeichert wurde":              "the password the backup was saved with",
	"die Sicherung wird damit verschlüsselt":                             "the backup is encrypted with it",
	"enthält alle Verbindungen, Schlüssel, Protokolle und Einstellungen": "contains all connections, keys, logs and settings",
	"im eigenen Netz":                                                    "on your own network",
	"nur lokale KI":                                                      "local AI only",
	"was ging hinaus":                                                    "what was sent",
}

// Audit-Protokoll (internal/audit, ui/audit.go) und Herstellerprofile.
func init() {
	for k, v := range map[string]string{
		"Audit-Protokoll (wer, wann, was; fälschungssicher)": "Audit log (who, when, what; tamper-evident)",
		"Audit-Protokoll an: ":                               "Audit log on: ",
		"Audit-Protokoll aus":                                "Audit log off",
		"Audit-Protokoll geprüft (%d Einträge, unverändert) und gespeichert: %s": "Audit log verified (%d entries, unchanged) and saved: %s",
		"Audit-Protokoll ist aus (Einstellungen › Sicherheit)":                   "Audit log is off (Settings › Security)",
		"Audit-Protokoll prüfen und speichern":                                   "Verify and save audit log",
		"Audit-Protokoll: ":                                                      "Audit log: ",
		"Audit-Protokoll: %d Auffälligkeiten! Bericht: %s":                       "Audit log: %d problems found! Report: %s",
		"Gerät kennt „%s“ nicht":                                                 "Device does not know “%s”",
		"Prüfung: %d Auffälligkeiten":                                            "Check: %d problems",
		"Prüfung: %d Einträge, Kette lückenlos und unverändert":                  "Check: %d entries, chain complete and unchanged",
	} {
		en[k] = v
	}
}
