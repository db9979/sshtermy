package i18n

// Texte für Beenden-Rückfrage, Klartext-Fehler, Host-Key übernehmen,
// Rückgängig, Verbindung testen, einfacher Modus und Fensterstart (eigene
// Datei, damit parallele Änderungen an en.go nicht kollidieren).
func init() {
	for de, e := range map[string]string{
		// Beenden
		"sshTermy beenden?":                            "Quit sshTermy?",
		"Diese Verbindung wird getrennt:":              "This connection will be closed:",
		"Diese Verbindungen werden getrennt:":          "These connections will be closed:",
		"… und %d weitere":                             "… and %d more",
		"Trotzdem beenden? (%s noch einmal = beenden)": "Quit anyway? (%s again = quit)",
		"beenden (fragt bei offenen Verbindungen)":     "quit (asks if connections are open)",
		"Vollbild an/aus (eigenes Fenster)":            "full screen on/off (own window)",

		// Klartext-Fehler
		"Abgebrochen":                      "Canceled",
		"Die Rückfrage wurde abgebrochen.": "The prompt was canceled.",
		"Der Server zeigt einen anderen Host-Key als beim letzten Mal":                                                                     "The server shows a different host key than last time",
		"Nach Neuinstallation, Werksreset oder Gerätetausch ist das normal. Sonst sitzt vielleicht jemand dazwischen (Man-in-the-Middle).": "Normal after a reinstall, factory reset or device swap. Otherwise someone may be in between (man-in-the-middle).",
		"Schlüssel vergleichen und übernehmen":                                                                                             "compare and accept key",
		"Schlüsseldatei fehlt auf diesem Rechner":                                                                                          "Key file missing on this computer",
		"Der Pfad stammt vielleicht von einem anderen Rechner. Datei wählen oder Schlüssel einbetten.":                                     "The path may come from another computer. Pick the file or embed the key.",
		"Anmeldung abgelehnt": "Login rejected",
		"Benutzer „%s“ und Passwort bzw. Schlüssel passen nicht zusammen. ⏎ fragt neu nach, falls nichts gespeichert ist.": "User “%s” and password or key don't match. ⏎ asks again if nothing is saved.",
		"Passphrase des Schlüssels falsch":                                                                       "Wrong key passphrase",
		"Der Schlüssel ist verschlüsselt und ließ sich nicht öffnen.":                                            "The key is encrypted and could not be opened.",
		"Host-Key nicht bestätigt":                                                                               "Host key not confirmed",
		"Ohne Bestätigung des Fingerprints verbindet sshTermy nicht. ⏎ fragt noch einmal.":                       "sshTermy won't connect without confirming the fingerprint. ⏎ asks again.",
		"Kein Zugriff auf den seriellen Port":                                                                    "No access to the serial port",
		"Linux: Benutzer in die Gruppe „dialout“ aufnehmen (sudo usermod -aG dialout $USER), dann neu anmelden.": "Linux: add your user to the “dialout” group (sudo usermod -aG dialout $USER), then log in again.",
		"Serieller Port ist belegt":                                                                              "Serial port is busy",
		"Ein anderes Programm (PuTTY, screen, minicom) hat den Port noch offen.":                                 "Another program (PuTTY, screen, minicom) still has the port open.",
		"Konsolenkabel nicht gefunden":                                                                           "Console cable not found",
		"Kabel eingesteckt? Unter Windows ggf. Treiber (CH340, FTDI, PL2303) installieren.":                      "Cable plugged in? On Windows you may need a driver (CH340, FTDI, PL2303).",
		"Name „%s“ nicht gefunden":                                                                               "Name “%s” not found",
		"Tippfehler? Interne Namen gehen oft nur im VPN oder Firmennetz. Sonst die IP-Adresse eintragen.":        "Typo? Internal names often only work on the VPN or company network. Otherwise enter the IP address.",
		"%s nimmt keine Verbindung an":                                                                           "%s refuses the connection",
		"Das Gerät antwortet, aber auf diesem Port läuft kein Dienst. Dienst aus, anderer Port oder falsche Verbindungsart (SSH/Telnet)?": "The device answers, but nothing listens on this port. Service down, another port, or wrong protocol (SSH/Telnet)?",
		"Netz nicht erreichbar": "Network unreachable",
		"Kein Weg zu diesem Netz: VPN aus, falsches WLAN oder Kabel ab?":           "No route to this network: VPN off, wrong Wi-Fi or cable unplugged?",
		"Gerät aus, Firewall blockt, oder nur über VPN bzw. Jump-Host erreichbar.": "Device off, blocked by a firewall, or only reachable via VPN or a jump host.",
		"Gerät aus, Firewall blockt, oder der Jump-Host erreicht es nicht.":        "Device off, blocked by a firewall, or the jump host can't reach it.",
		"Keine Antwort von %s":                       "No answer from %s",
		"Kein gemeinsames Verschlüsselungsverfahren": "No common encryption method",
		"Meist ein sehr altes Gerät. Firmware aktualisieren oder über Telnet bzw. Konsolenkabel verbinden.":                                      "Usually a very old device. Update the firmware or connect via Telnet or console cable.",
		"Zertifikat des Servers wird nicht anerkannt":                                                                                            "Server certificate not accepted",
		"Bei selbst signiertem Zertifikat im eigenen Netz: im Formular „Zertifikat nicht prüfen“ einschalten.":                                   "For a self-signed certificate on your own network: turn on “Don't verify certificate” in the form.",
		"Der Server hat die Verbindung sofort beendet":                                                                                           "The server closed the connection right away",
		"Oft: zu viele Fehlversuche (fail2ban), SSH nur von bestimmten Adressen erlaubt, oder Gerät ausgelastet. Kurz warten und neu versuchen.": "Often: too many failed attempts (fail2ban), SSH only allowed from certain addresses, or the device is busy. Wait a moment and retry.",
		"Verbindung bearbeiten":                         "edit connection",
		"getrennt: e / k":                               "closed: e / k",
		"Verbindung bearbeiten / neuen Host-Key prüfen": "edit connection / check new host key",

		// Host-Key
		"Server: ":            "Server: ",
		"Bisher gespeichert:": "Saved so far:",
		"Jetzt gemeldet:":     "Now reported:",
		"Nur übernehmen, wenn du weißt, warum sich der Schlüssel geändert hat. Im Zweifel den Fingerprint am Gerät prüfen (ssh-keygen -lf /etc/ssh/ssh_host_*_key.pub bzw. „show crypto host-public-key“).": "Only accept if you know why the key changed. If in doubt, check the fingerprint on the device (ssh-keygen -lf /etc/ssh/ssh_host_*_key.pub or “show crypto host-public-key”).",
		"Neuen Schlüssel übernehmen und verbinden?": "Accept the new key and connect?",
		"Host-Key hat sich geändert":                "Host key changed",
		"Host-Key nicht gespeichert: ":              "Host key not saved: ",
		"Neuer Host-Key gespeichert":                "New host key saved",

		// Rückgängig
		" oder Klick: rückgängig":     " or click: undo",
		"Nichts rückgängig zu machen": "Nothing to undo",
		"Rückgängig fehlgeschlagen: ": "Undo failed: ",
		"Wiederhergestellt":           "Restored",
		"Runbook „":                   "Runbook “",
		"“ aufgelöst":                 "” dissolved",
		"Löschen rückgängig machen":   "Undo delete",
		"Löschen rückgängig (10 s)":   "undo delete (10 s)",

		// Testen
		"Verbindung testen": "Test connection",
		"Prüfe ":            "Checking ",
		"Für diese Verbindungsart gibt es keinen Test": "There is no test for this connection type",
		"%s erreichbar": "%s reachable",
		"Die Anmeldung prüft sshTermy beim Verbinden.": "sshTermy checks the login when connecting.",
		"Erreichbar":                      "Reachable",
		"Host-Key":                        "Host key",
		"Test erfolgreich":                "Test passed",
		"Test fehlgeschlagen":             "Test failed",
		"Alles bereit: Strg+S speichert.": "All set: Ctrl+S saves.",
		"im Formular: Verbindung testen":  "in the form: test connection",

		// Einfacher Modus, Fenster
		"Einfacher Modus: Profi-Funktionen ausblenden":                                          "Simple mode: hide pro features",
		"Einfacher Modus an: Profi-Funktionen findest du weiter über die Suche in der Palette.": "Simple mode on: pro features are still found by searching in the palette.",
		"Einfacher Modus aus: alle Funktionen sichtbar":                                         "Simple mode off: all features visible",
		"Fenster beim Start":          "Window at startup",
		"Fenster beim Start: ":        "Window at startup: ",
		" (F11 schaltet Vollbild um)": " (F11 toggles full screen)",
		"Vollbild":                    "full screen",
		"maximiert":                   "maximized",
		"wie zuletzt":                 "as last time",
	} {
		en[de] = e
	}
}
