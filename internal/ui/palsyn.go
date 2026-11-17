package ui

// palSyn: Alltagswörter je Eintrag (deutsche Beschriftung → Suchwörter auf
// Deutsch und Englisch). Ergänzt palItem.keys; die englische Beschriftung
// sucht die Palette ohnehin mit, ebenso den Hilfesatz. TestPalSearchWords
// verlangt für jede Aktion und Einstellung Suchwörter hier oder in
// palItem.keys, TestPalSearchProbes prüft typische Suchanfragen.
var palSyn = map[string]string{
	// KI
	"Ziel-Modus öffnen":                        "ziel goal aufgabe task ki ai automatisch erledigen assistent",
	"Agent-Modus öffnen (läuft selbstständig)": "agent autopilot automatisch selbstständig ki ai autonom",
	"KI fragen": "frage ask chat ki ai hilfe erklären explain chatgpt claude",

	// Diese Sitzung
	"Dateien (SFTP)": "dateien files sftp scp upload download hochladen herunterladen datei kopieren transfer explorer",
	"Host-Übersicht": "host übersicht overview cpu ram speicher memory disk platte dienste services auslastung load uptime monitoring",
	"Aufnahme starten (abspielbar mit asciinema)": "aufnahme aufzeichnen record recording video asciinema mitschnitt",
	"Aufnahme beenden":                            "aufnahme stop beenden record recording",
	"Terminal-Verlauf als Text exportieren":       "export speichern save verlauf scrollback text txt datei screenshot mitschnitt log",
	"Markieren per Tastatur (v, y kopiert)":       "text markieren selection auswählen",
	"Break senden (serielle Konsole)":             "break serial seriell konsole console",
	"Jetzt sperren":                               "sperren lock bildschirmsperre abschließen pause weg",

	// Schnellbefehle & Tunnel
	"Tunnel-Panel":              "tunnel port weiterleiten forwarding portforward socks proxy weiterleitung -L -R",
	"Schnellbefehle bearbeiten": "schnellbefehle quick commands snippets makros befehle favoriten",

	// Verbindungen & Daten
	"Neue Verbindung": "neu new verbindung connection server host anlegen hinzufügen add ssh telnet seriell",
	"SSH-Schlüssel erzeugen / auf den Server übertragen": "ssh key schlüssel keygen ssh-copy-id public private passwortlos login",
	"Verbindungs-Manager":   "manager verbindungen liste server hosts übersicht connections",
	"Jetzt synchronisieren": "sync synchronisieren abgleichen cloud rechner",

	// Mehrere Hosts & Netzwerk
	"Broadcast: Befehl auf mehreren Hosts":      "broadcast mehrere alle hosts gleichzeitig parallel multi cluster ansible",
	"Zeitpläne: Broadcast/Runbook nach Uhrzeit": "zeitplan schedule cron timer uhrzeit automatisch regelmäßig planen",
	"Netzwerk-Scan: SSH-Geräte finden":          "scan netzwerk network geräte devices finden discover nmap ip bereich subnetz",

	// Team & Teilen
	"Mit dem Team teilen (verschlüsselt, ohne Passwörter)": "team teilen share kollegen exportieren paket verbindungen weitergeben",
	"Team-Paket übernehmen":                                "team paket import übernehmen kollegen sshteam",
	"Team-Ordner (veröffentlicht / abonniert)":             "team ordner abo subscribe veröffentlichen publish freigabe",

	// Suchen & Logs
	"Im Verlauf suchen":              "suchen search find grep verlauf scrollback finden text",
	"Fehlerzeilen im Verlauf zeigen": "fehler errors error zeilen verlauf rot failed",
	"Sitzungsprotokolle durchsuchen": "protokolle logs sitzung history verlauf früher gestern alt suchen session",

	// Tabs & Ansicht
	"Aufteilung":                          "aufteilung split layout geteilt nebeneinander fenster",
	"Tab in eigenes Fenster abtrennen":    "fenster window abtrennen detach eigenes zweiter monitor",
	"Tab aus eigenem Fenster zurückholen": "fenster window zurückholen attach",
	"Alle Tabs zeigen":                    "tabs liste list alle übersicht wechseln",
	"Neuer Tab":                           "tab neu new öffnen verbindung zweite",
	"Tab umbenennen":                      "tab name umbenennen rename titel",
	"Tab duplizieren":                     "tab duplizieren duplicate kopie klonen clone zweite shell",
	"Tab schließen":                       "tab schließen close beenden trennen disconnect exit",
	"Bereich zoomen (ganzes Fenster)":     "zoom groß maximieren bereich",
	"Zoom beenden (alle Bereiche zeigen)": "zoom zurück alle bereiche",
	"Alle Bereiche gleich groß":           "gleich groß equal size aufteilung",

	// Einstellungen
	"Farbschema wählen …":      "dunkel hell dark light mode darkmode lightmode nachtmodus design look farben",
	"Symbole":                  "schrift font schriftart nerd icons zeichen",
	"Bewegung":                 "animation animationen effekte",
	"Fenster beim Start":       "vollbild fullscreen maximiert",
	"Sprache / Language":       "sprache language englisch english deutsch",
	"Updates anbieten":         "updates aktualisieren upgrade version",
	"Master-Passwort ändern …": "passwort kennwort password ändern master",

	// Hilfe
	"Was ist neu (Changelog)": "neu neues changelog änderungen release notes version news",
}
