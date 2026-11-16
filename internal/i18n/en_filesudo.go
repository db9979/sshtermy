package i18n

// Texte für Dateiaktionen mit sudo (internal/ui/filesudo.go, sshx/sudo.go).
func init() {
	for de, e := range map[string]string{
		"\n… und %d weitere": "\n… and %d more",
		"%s\n\n%s meldet: keine Berechtigung (%s).\n\nMit sudo (als root) wiederholen?": "%s\n\n%s reports: no permission (%s).\n\nRepeat with sudo (as root)?",
		"Falsches Passwort. ":                          "Wrong password. ",
		"Gespeichert (sudo) auf ":                      "Saved (sudo) on ",
		"Hochladen nach ":                              "Upload to ",
		"Keine Berechtigung – mit sudo?":               "No permission – use sudo?",
		"Kopieren: ":                                   "Copy: ",
		"Löschen:\n":                                   "Delete:\n",
		"Neuer Ordner: ":                               "New folder: ",
		"Rechte ändern: %s → %o":                       "Change permissions: %s → %o",
		"Schutz an: j zweimal drücken.":                "Protection on: press y twice.",
		"Speichern aus dem Editor: ":                   "Save from your editor: ",
		"Umbenennen: ":                                 "Rename: ",
		"Zwischenordner auf dem Server ging nicht: %s": "Could not create a temporary folder on the server: %s",
		"sudo geht hier nicht (nicht installiert oder für diesen Benutzer nicht erlaubt)": "sudo is not available here (not installed or not allowed for this user)",
		"sudo hat das Passwort nicht angenommen":                                          "sudo did not accept the password",
		"sudo-Passwort für %s@%s":                                                         "sudo password for %s@%s",
		"sudo: ":                                                                          "sudo: ",
	} {
		en[de] = e
	}
}
