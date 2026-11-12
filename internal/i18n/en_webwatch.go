package i18n

// Texte für Zusehen im Browser (ohne sshTermy).
var enWebWatch = map[string]string{
	"# Zuschauer im Browser abgelehnt (nicht freigegeben): %s (%s)": "# Browser viewer refused (not enabled): %s (%s)",
	"Auch im Browser zusehen": "Also watch in a browser",
	"Der Browser warnt einmal vor dem Zertifikat (selbst ausgestellt, SHA-256 %s…): „Erweitert › Weiter“.":                                                                                                      "The browser warns once about the certificate (self-signed, SHA-256 %s…): “Advanced › Proceed”.",
	"Der Browser warnt einmal vor dem Zertifikat, weil es selbst ausgestellt ist: „Erweitert“ und „Weiter“ wählen.":                                                                                             "The browser warns once about the certificate because it is self-signed: choose “Advanced” and “Proceed”.",
	"Keine Verbindung zur Freigabe (beendet, Code schon benutzt oder abgelaufen?). Safari: bitte Chrome, Edge oder Firefox nehmen – Safari erlaubt hier keine Verbindung zu selbst ausgestellten Zertifikaten.": "No connection to the share (ended, code already used or expired?). Safari: please use Chrome, Edge or Firefox – Safari does not allow connections to self-signed certificates here.",
	"Der Gastgeber antwortet nicht auf den Code (sshTermy beim Gastgeber noch offen und aktuell?)":                                                                                                              "The host does not answer the code (is sshTermy still open and up to date on the host?)",
	"%s: Jemand (%s) wollte zusehen, aber die Einladung ist verbraucht oder gesperrt. Neuer Code: Palette › Teilen verwalten":                                                                                   "%s: someone (%s) wanted to watch, but the invitation is used up or locked. New code: palette › Manage sharing",
	"# Zusehen abgewiesen (Einladung verbraucht oder gesperrt): %s":                                                                                                                                             "# Viewer turned away (invitation used up or locked): %s",
	"Einladung schon benutzt, abgelaufen oder gesperrt – bitte beim Gastgeber einen neuen Code holen":                                                                                                           "Invitation already used, expired or locked – please get a new code from the host",
	"Link für sshTermy": "Link for sshTermy",
	"Link und QR-Code für Zuschauer ohne sshTermy (gleicher Code, gleiches Anklopfen)": "Link and QR code for viewers without sshTermy (same code, same knocking)",
	"Mit sshTermy:":  "With sshTermy:",
	"Nur über https": "Only over https",
	"Ohne sshTermy, direkt im Browser (auch am Handy):":                                       "Without sshTermy, right in the browser (phones too):",
	"Ohne sshTermy: im Browser":                                                               "Without sshTermy: in a browser",
	"QR-Code: Browser-Link – mit der Handy-Kamera scannen, ohne App":                          "QR code: browser link – scan with the phone camera, no app needed",
	"im Browser (ohne sshTermy)":                                                              "in a browser (without sshTermy)",
	"▲ Die Seite kommt vom öffentlichen Vermittler: Browser-Link nur, wenn du ihm vertraust.": "▲ The page comes from the public relay: use the browser link only if you trust it.",
}

func init() {
	for k, v := range enWebWatch {
		en[k] = v
	}
}
