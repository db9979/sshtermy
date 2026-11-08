package i18n

// Bewegung (ui/motion.go), Halten zum Bestätigen (ui/dangerguard.go),
// lebendige Verbindungsliste (ui/reach.go, ui/manager.go).
func init() {
	for k, v := range enMotion {
		en[k] = v
	}
}

var enMotion = map[string]string{
	"Bewegung": "Motion",
	"voll":     "full",
	"dezent":   "subtle",
	"Barrierearm ist an: Bewegung bleibt aus, bis du es abschaltest.": "Accessible mode is on: motion stays off until you turn it off.",
	"Klick wechseln · Mitte schließen · Rechtsklick Menü":             "click switches · middle closes · right-click menu",
	"verbunden seit %s":               "connected for %s",
	"Latenz":                          "Latency",
	"Platte":                          "Disk",
	"  je Minute":                     "  one per minute",
	"Antwortet nicht mehr: ":          "Stopped responding: ",
	" und %d weitere":                 " and %d more",
	"„Bewegung“: voll · dezent · aus": "“Motion”: full · subtle · off",
	"Maus auf Tab":                    "Mouse on tab",
	"Karte: Ziel, Dauer, Latenz":      "card: target, duration, latency",
	"Verbinde mit ":                   "Connecting to ",
	"Verbindung aufbauen":             "Open connection",
	"Host-Schlüssel prüfen":           "Check host key",
	"Shell öffnen":                    "Open shell",
	"⏎ halten":                        "hold ⏎",
	"weiter halten …":                 "keep holding …",
	"losgelassen · nichts passiert":   "released · nothing happened",
	"⏎ gedrückt halten oder %s drücken zum Ausführen":                    "hold ⏎ or press %s to run",
	"Ein kurzes ⏎ reicht hier nicht: ⏎ gedrückt halten oder %s drücken.": "A short ⏎ is not enough here: hold ⏎ or press %s.",
}
