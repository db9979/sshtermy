package i18n

// Master-Passwort, DPAPI, Team-Adressen, Agent-Inline-Code (Gesamtprüfung Teil 2)
func init() {
	for k, v := range map[string]string{
		"  … und %d weitere":                           "  … and %d more",
		"+ Windows-Anmeldung":                          "+ Windows sign-in",
		"Bisheriges Passwort":                          "Current password",
		"Bisheriges Passwort fehlt":                    "Current password is missing",
		"Bisheriges Passwort stimmt nicht":             "Current password is wrong",
		"Die neuen Passwörter sind nicht gleich":       "The new passwords do not match",
		"Master-Passwort und Entsperren":               "Master password and unlocking",
		"Master-Passwort ändern …":                     "Change master password …",
		"Mit der Windows-Anmeldung entsperren (DPAPI)": "Unlock with the Windows sign-in (DPAPI)",
		"Neue Adresse vom Team":                        "New address from the team",
		"Neue Adresse übernommen (%d)":                 "New address applied (%d)",
		"Neues Passwort":                               "New password",
		"Neues Passwort wiederholen":                   "Repeat new password",
		"Nichts geändert: neues Passwort eintragen oder die Windows-Anmeldung umschalten":                                            "Nothing changed: enter a new password or switch the Windows sign-in",
		"an: sshTermy öffnet sich nach der Windows-Anmeldung ohne Eingabe · das Master-Passwort bleibt der Weg auf anderen Rechnern": "on: sshTermy opens after the Windows sign-in without asking · the master password stays the way in on other computers",
		"mindestens 8 Zeichen · leer lassen = Passwort bleibt":                                                                       "at least 8 characters · leave empty = keep the password",
		"zur Bestätigung, auch wenn du nur die Windows-Anmeldung umschaltest":                                                        "to confirm, even if you only switch the Windows sign-in",
		"Das Team-Paket ändert die Adresse von Verbindungen, für die hier Passwort, Key oder Agent-Weiterleitung gespeichert sind:":  "The team package changes the address of connections that have a password, key or agent forwarding stored here:",
		"Übernehmen? Ohne Bestätigung bleibt die bisherige Adresse.":                                                                 "Apply? Without confirmation the old address stays.",
		"startet Code aus der Zeile (python -c, bash -c, eval …)":                                                                    "runs code from the command line (python -c, bash -c, eval …)",
		"alles außer Gefahr, sudo und Code in der Zeile":                                                                             "everything except danger, sudo and inline code",
	} {
		en[k] = v
	}
}
