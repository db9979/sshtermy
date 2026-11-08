package i18n

// Bereiche der Befehlspalette (ui/palette.go, palAreas).
func init() {
	for k, v := range enPaletteAreas {
		en[k] = v
	}
}

var enPaletteAreas = map[string]string{
	"HÄUFIG":                             "FREQUENT",
	"ALLE AKTIONEN":                      "ALL ACTIONS",
	"Schnellbefehle & Tunnel":            "Quick commands & tunnels",
	"Befehle, Runbooks, Tunnel":          "commands, runbooks, tunnels",
	"Diese Sitzung":                      "This session",
	"Agent, Container, Aufnahme, Export": "agent, containers, recording, export",
	"Tabs & Fenster":                     "Tabs & windows",
	"neu, lokal, umbenennen, abtrennen":  "new, local, rename, detach",
	"Ansicht & Aufteilung":               "View & split",
	"teilen, Zoom, Arbeitsplätze":        "split, zoom, workspaces",
	"Mehrere Hosts":                      "Several hosts",
	"Broadcast, Zeitpläne, Scan":         "broadcast, schedules, scan",
	"Netzwerk & Switches":                "Network & switches",
	"Gerät finden, Topologie":            "find device, topology",
	"Team & Teilen":                      "Team & sharing",
	"live teilen, Team-Ordner":           "live sharing, team folders",
	"Verbindungen & Schlüssel":           "Connections & keys",
	"Manager, neu, SSH-Key, Protokolle":  "manager, new, SSH key, logs",
}
