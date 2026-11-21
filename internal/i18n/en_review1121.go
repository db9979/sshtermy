package i18n

// Code-Review 2026-11-21: Prüfsummen, Pfadschutz, Maus.
func init() {
	for de, e := range map[string]string{
		"%s: keine Prüfsumme, Herunterladen abgebrochen":                                      "%s: no checksum, download stopped",
		"für %s gibt es keine Prüfsumme; installiere llama.cpp selbst (llama-server im PATH)": "there is no checksum for %s; install llama.cpp yourself (llama-server in PATH)",
		"Port %d ist von einem anderen Programm belegt":                                       "port %d is used by another program",
		"ungültiger Name: %q": "invalid name: %q",
		"Auf ntfy.sh kann jeder ein kurzes Thema erraten und mitlesen: mindestens %d Zeichen, am besten zufällig": "On ntfy.sh anyone can guess a short topic and read along: at least %d characters, ideally random",
		"Aufgaben …": "Tasks …",
		"Diese Befehle laufen bei jedem Lauf auf allen Servern im Ziel, ohne Aufsicht:":        "These commands run on every server in the target on each run, unattended:",
		"Ein bestätigter Prüfbefehl gilt nicht mehr als sicher: Zeitplan öffnen und speichern": "A confirmed check command is no longer considered safe: open and save the schedule",
		"KI wählt die Prüfbefehle …": "AI is choosing the check commands …",
		"Nach Tag filtern …":         "Filter by tag …",
		"Netz scannen …":             "Scan network …",
		"Neue Verbindung …":          "New connection …",
		"Neuer Ordner hier …":        "New folder here …",
		"Neuer Ordner …":             "New folder …",
		"Programm starten?":          "Run program?",
		"Prüfbefehle bestätigen":     "Confirm check commands",
		"Prüfbefehle bestätigt":      "Check commands confirmed",
		"Prüfbefehle noch nicht bestätigt: Zeitplan öffnen und speichern": "Check commands not confirmed yet: open and save the schedule",
		"Verbindungen importieren …":                                      "Import connections …",
		"auf diesem Rechner":                                              "on this computer",
		"vom Server":                                                      "from the server",
		"Übernehmen? Ohne Bestätigung läuft die KI-Prüfung nicht.":        "Accept? Without confirmation the AI check does not run.",
		"„%s“ %s ist ein Programm oder Skript. Mit dem Standardprogramm öffnen heißt hier: es läuft auf diesem Rechner, mit deinen Rechten.\n\nNur starten, wenn du der Datei vertraust.": "“%s” %s is a program or script. Opening it with the default app means: it runs on this computer, with your permissions.\n\nOnly run it if you trust the file.",
	} {
		en[de] = e
	}
}
