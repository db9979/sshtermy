package i18n

// Code-Review 2026-11-11: zweites sshTermy, Speicherfehler, Update-Selbsttest.
func init() {
	for k, v := range map[string]string{
		"Ein anderes sshTermy hat gespeichert: Verbindungen und Einstellungen wurden neu geladen.":                                                               "Another sshTermy saved: connections and settings were reloaded.",
		"Speichern fehlgeschlagen: %s. Die Änderung gilt nur, bis sshTermy beendet wird.":                                                                        "Saving failed: %s. The change only lasts until sshTermy is closed.",
		"Das Update auf v%s ist zweimal nicht sauber gestartet und wurde zurückgenommen. Es wird nicht erneut angeboten; die nächste Version kommt wie gewohnt.": "The update to v%s failed to start cleanly twice and was rolled back. It will not be offered again; the next version arrives as usual.",
		"sshTermy v%s ist zweimal nicht sauber gestartet: zurück zu v%s.":                                                                                        "sshTermy v%s failed to start cleanly twice: back to v%s.",
		"update: die neue Programmdatei meldet Version %s statt %s, Update verworfen":                                                                            "update: the new program file reports version %s instead of %s, update discarded",
		"update: die neue Programmdatei startet nicht (%v), Update verworfen":                                                                                    "update: the new program file does not start (%v), update discarded",
	} {
		en[k] = v
	}
}
