package i18n

// Menüfenster: Detailspalte, Hilfesätze, Reiter (ui/palstyle.go).
func init() {
	for k, v := range enMenuDesign {
		en[k] = v
	}
}

var enMenuDesign = map[string]string{
	"Darin:":                "Inside:",
	"Jetzt: ":               "Now: ",
	"Probe":                 "Sample",
	"So sehen Dialoge aus.": "This is how dialogs look.",
	"So sehen Symbole aus:": "This is how icons look:",
	"Taste / Wert: ":        "Key / value: ",
	"und":                   "and",
	"weitere":               "more",
	"zweimal bestätigen":    "confirm twice",
	"Wirkt sofort: Diese Palette blendet dann ein bzw. bleibt ruhig.": "Takes effect at once: this palette then fades in or stays still.",

	"Blendet Profi-Einträge aus Palette und Fußzeilen aus. Die Suche findet weiter alles.":       "Hides pro entries from the palette and footers. Search still finds everything.",
	"Sprache der Oberfläche. Wirkt sofort, auch auf Hilfe und Kurzreferenz.":                     "Language of the interface. Takes effect at once, also for help and quick reference.",
	"Aus: Das Terminal bekommt die Maus, markieren geht dann wie im normalen Terminal.":          "Off: the terminal gets the mouse, selecting works like in a plain terminal.",
	"Bricht eine Verbindung ab, verbindet sshTermy von selbst neu und zeigt den Versuch im Tab.": "When a connection drops, sshTermy reconnects on its own and shows the attempt in the tab.",
	"Grüner Punkt im Manager, wenn ein Host antwortet. Nur sichtbare oder alle Hosts.":           "Green dot in the manager when a host answers. Visible hosts only or all of them.",
	"Meldet Befehle, die länger als 10 s liefen, in sshTermy oder zusätzlich auf dem Desktop.":   "Reports commands that ran longer than 10 s, in sshTermy or also on the desktop.",
	"Zeigt in der Fußzeile zu jeder Taste ein Wort mehr.":                                        "Shows one more word for every key in the footer.",
	"Vollbild, maximiert oder so wie beim letzten Beenden.":                                      "Full screen, maximized or as it was when you last quit.",
	"26 Farbschemas, hell und dunkel, mit Vorschau. ⏎ öffnet die Auswahl.":                       "26 color schemes, light and dark, with preview. ⏎ opens the picker.",
	"Eigenes Farbschema: jede Farbe einzeln, mit Kontrastprüfung.":                               "Your own color scheme: every color on its own, with a contrast check.",
	"Färbt Fehler, Warnungen, IPs und MACs in der Terminalausgabe ein.":                          "Colors errors, warnings, IPs and MACs in the terminal output.",
	"Text-Tags laufen überall, Nerd Font braucht eine passende Schrift.":                         "Text tags work everywhere, Nerd Font needs a matching font.",
	"Ecken der Dialoge (rund oder eckig) und ob sie einen Schatten werfen.":                      "Corners of dialogs (round or square) and whether they cast a shadow.",
	"Übergänge, Einblenden und Startanimation. Aus = alles ruhig.":                               "Transitions, fade-ins and the start animation. Off = everything still.",
	"Keine Bewegung, Zustände zusätzlich als Form (▲ ● ○) statt nur als Farbe.":                  "No motion, states also shown as shapes (▲ ● ○) instead of color only.",
	"In der geteilten Ansicht treten die nicht aktiven Bereiche zurück.":                         "In split view, the inactive panes step back.",
	"Mehrzeiliges Einfügen zeigt erst eine Vorschau, bevor etwas beim Server ankommt.":           "Pasting several lines shows a preview first, before anything reaches the server.",
	"Erlaubt vim, tmux & Co. auf dem Server, in deine Zwischenablage zu schreiben.":              "Lets vim, tmux and friends on the server write to your clipboard.",
	"Sperrt sshTermy nach der gewählten Zeit ohne Eingabe; entsperren mit dem Master-Passwort.":  "Locks sshTermy after the chosen idle time; unlock with the master password.",
	"Schützt gespeicherte Passwörter und Schlüssel. Unter Windows auch per Windows-Anmeldung.":   "Protects stored passwords and keys. On Windows also via your Windows sign-in.",
	"Endpunkt und Modell, Ersatz-Anbieter, KI-Begleiter, Datenschutz und Status.":                "Endpoint and model, fallback providers, AI companion, privacy and status.",
	"Sitzungsprotokoll, Logwache, Syslog-Empfang und Audit-Protokoll.":                           "Session log, log watch, syslog receiver and audit log.",
	"Befehlssätze je Hersteller und Regeln für Switch-Configs.":                                  "Command sets per vendor and rules for switch configs.",
	"Jede Taste frei belegen; Konflikte zeigt sshTermy gleich an.":                               "Rebind any key; sshTermy shows conflicts right away.",
	"Welcher Editor Dateien vom Server öffnet: eingebaut oder dein eigener.":                     "Which editor opens files from the server: built in or your own.",
	"Verbindungen, Ordner und Einstellungen verschlüsselt zwischen deinen Rechnern abgleichen.":  "Sync connections, folders and settings between your computers, encrypted.",
	"Sitzungen teilen, auch wenn der Server hinter NAT oder einer Firewall steht.":               "Share sessions even when the server sits behind NAT or a firewall.",
	"Beschreibe, was erreicht werden soll; die KI schlägt Befehle vor, du bestätigst jeden.":     "Describe what should be achieved; the AI suggests commands, you confirm each one.",
	"Die KI arbeitet ein Ziel selbst ab und hält vor allem Gefährlichen an.":                     "The AI works through a goal on its own and stops before anything dangerous.",
	"Frage zur Sitzung oder zur letzten Ausgabe; die Antwort erscheint im Seitenpanel.":          "Ask about the session or the last output; the answer appears in the side panel.",
	"CPU, Speicher, Platten, Dienste und Netz des Hosts auf einen Blick.":                        "CPU, memory, disks, services and network of the host at a glance.",
	"Ein Ziel über mehrere Tabs: die KI plant je Fenster und schützt das Zielfenster.":           "One goal across several tabs: the AI plans per window and protects the target window.",
	"Sucht im Zurückblättern dieses Tabs; ↑↓ springt zwischen Treffern.":                         "Searches this tab's scrollback; ↑↓ jumps between hits.",
	"Sucht gleichzeitig im Verlauf aller offenen Tabs.":                                          "Searches the scrollback of all open tabs at once.",
	"Liest journal, syslog oder Eventlog und zeigt Fehler, Warnungen und neue Muster.":           "Reads journal, syslog or the event log and shows errors, warnings and new patterns.",
	"Mehrere Tabs nebeneinander: sieben Aufteilungen mit Skizze.":                                "Several tabs side by side: seven layouts with a sketch.",
	"Öffnet den Verbindungs-Manager; ⏎ verbindet in einem zusätzlichen Tab.":                     "Opens the connection manager; ⏎ connects in an additional tab.",
	"Formular für eine neue Verbindung: SSH, Telnet, seriell, lokal oder PowerShell.":            "Form for a new connection: SSH, Telnet, serial, local or PowerShell.",
	"Dateien von Server und Rechner nebeneinander, mit Maus und Ziehen.":                         "Files of server and computer side by side, with mouse and drag.",
	"Sperrt sshTermy sofort; entsperren mit dem Master-Passwort.":                                "Locks sshTermy right away; unlock with the master password.",
}
