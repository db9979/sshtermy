package i18n

// Bereiche der Befehlspalette (ui/palette.go, palAreas).
func init() {
	for k, v := range enPaletteAreas {
		en[k] = v
	}
}

var enPaletteAreas = map[string]string{
	"HÄUFIG":                                    "FREQUENT",
	"ALLE AKTIONEN":                             "ALL ACTIONS",
	"Schnellbefehle & Tunnel":                   "Quick commands & tunnels",
	"Befehle, Runbooks, Tunnel":                 "commands, runbooks, tunnels",
	"Ziel, Agent, Frage, Vorgang, Begleiter":    "goal, agent, ask, task, companion",
	"Diese Sitzung":                             "This session",
	"Host, Dateien, Container, Aufnahme":        "host, files, containers, recording",
	"Suchen & Logs":                             "Search & logs",
	"Verlauf, alle Tabs, Log-Blick, Protokolle": "scrollback, all tabs, log glance, session logs",
	"Tabs & Ansicht":                            "Tabs & view",
	"neu, teilen, Zoom, Arbeitsplätze":          "new, split, zoom, workspaces",
	"Mehrere Hosts & Netzwerk":                  "Several hosts & network",
	"Broadcast, Scan, Zeitpläne, Switches":      "broadcast, scan, schedules, switches",
	"Team & Teilen":                             "Team & sharing",
	"live teilen, Team-Ordner":                  "live sharing, team folders",
	"Verbindungen & Daten":                      "Connections & data",
	"neu, SSH-Key, Import/Export, Sicherung":    "new, SSH key, import/export, backup",
	"Dateien (SFTP)":                            "Files (SFTP)",

	// Einstellungen und Hilfe & Info
	"ALLGEMEIN":         "GENERAL",
	"AUSSEHEN":          "APPEARANCE",
	"Protokolle & Logs": "Logs & audit",
	"Sitzungsprotokoll, Logwache, Syslog, Audit": "session log, log watch, syslog, audit",
	"Netzwerkgeräte": "Network devices",
	"Befehlssätze, Regeln für Switch-Configs": "command sets, switch config rules",
	"Fehlersuche":                       "Troubleshooting",
	"Debug-Protokoll, Leistungsanzeige": "debug log, performance overlay",
	"Endpunkt & Modell …":               "Endpoint & model …",

	// Rechtsklick im Manager (ui/mgrmenu.go)
	"Bearbeiten …":                          "Edit …",
	"In Ordner verschieben …":               "Move to folder …",
	"Tunnel …":                              "Tunnel …",
	"SSH-Schlüssel erzeugen / übertragen …": "Create / copy SSH key …",
	"Schnellbefehle …":                      "Quick commands …",
	"Aufwecken (Wake-on-LAN)":               "Wake up (Wake-on-LAN)",
	"Markieren (für Sammeländerung)":        "Mark (for bulk edit)",
	"Markierung aufheben":                   "Unmark",
	"Zuklappen":                             "Collapse",
	"Aufklappen":                            "Expand",
	"Neue Verbindung hier …":                "New connection here …",
	"Alle als Raster öffnen":                "Open all as grid",
	"Löschen":                               "Delete",
	"Verbinden":                             "Connect",
	"Löschen …":                             "Delete …",
	"Menü zu Verbindung oder Ordner":        "menu for a connection or folder",
}
