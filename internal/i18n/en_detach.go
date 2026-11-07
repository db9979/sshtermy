package i18n

// Tab in eigenes Fenster (ui/detach.go, internal/detach).
// Eigene Datei, damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enDetach {
		en[k] = v
	}
}

var enDetach = map[string]string{
	" ist zurück im Tab":                                              " is back in its tab",
	" läuft in eigenem Fenster":                                       " is running in its own window",
	" läuft jetzt in eigenem Fenster":                                 " now runs in its own window",
	"Das neue Fenster hat sich nicht gemeldet, der Tab bleibt hier":   "The new window did not report back, the tab stays here",
	"Der Tab ist nicht mehr offen.":                                   "The tab is no longer open.",
	"Die Verbindung zu sshTermy ist weg (Hauptfenster geschlossen?).": "Lost the connection to sshTermy (main window closed?).",
	"Eigenes Fenster geht nicht: ":                                    "Cannot open own window: ",
	"Fenster":                                                         "window",
	"Fenster ließ sich nicht öffnen: ":                                "Could not open the window: ",
	"Kein Terminalprogramm gefunden. Eigenes festlegen mit SSHTERMY_TERMINAL, z. B. \"kitty {cmd}\"": "No terminal program found. Set your own with SSHTERMY_TERMINAL, e.g. \"kitty {cmd}\"",
	"Keine Verbindung zu sshTermy: %v":                                        "Cannot reach sshTermy: %v",
	"Nur eine verbundene Sitzung lässt sich in ein eigenes Fenster abtrennen": "Only a connected session can be moved to its own window",
	"Sitzung in sshTermy beendet.":                                            "Session ended in sshTermy.",
	"Tab aus eigenem Fenster zurückholen":                                     "Bring tab back from its own window",
	"Tab in eigenes Fenster abtrennen":                                        "Move tab to its own window",
	"Tab in sshTermy zurückgeholt.":                                           "Tab moved back into sshTermy.",
	"Taste drücken zum Schließen …":                                           "Press a key to close …",
	"Verbindung abgerissen, verbinde neu …":                                   "Connection lost, reconnecting …",
	"Wieder verbunden.":                                                       "Reconnected.",
	"hierher zurückholen":                                                     "bring back here",
	"seit %s · %d×%d · Fenster schließen holt den Tab zurück":                 "since %s · %d×%d · closing the window brings the tab back",
	"zurückholen":                                   "bring back",
	"⠿ Öffne eigenes Fenster für ":                  "⠿ Opening own window for ",
	"Tab in eigenes Fenster / zurückholen":          "Tab to own window / bring back",
	"Tab in eigenes Fenster (Sitzung bleibt offen)": "Tab to own window (session stays open)",
}
