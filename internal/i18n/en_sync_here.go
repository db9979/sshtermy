package i18n

// Sync: Werte nur für diesen Rechner, Zeitpläne je Rechner, Übersicht im
// Assistenten. Eigene Datei, damit parallele Änderungen nicht kollidieren.
func init() {
	for k, v := range enSyncHere {
		en[k] = v
	}
}

var enSyncHere = map[string]string{
	"   ⏎/Doppelklick belegt neu · gilt auf allen Rechnern · l = nur hier": "   ⏎/double-click rebinds · applies on all computers · l = here only",
	"   ◆ neue Tasten gelten nur auf diesem Rechner":                       "   ◆ new keys apply to this computer only",
	"  ← dieser Rechner":         "  ← this computer",
	" (dieser Rechner)":          " (this computer)",
	" noch kein Abgleich":        " not synced yet",
	" · l = wieder alle Rechner": " · l = all computers again",
	" ◆ nur hier":                " ◆ here only",
	"Befehlsverlauf":             "Command history",
	"Bleibt hier: ":              "Stays here: ",
	"Eigene Tasten dieses Rechners entfernen? Dann gilt die Belegung aus dem Sync.": "Remove this computer's own keys? The synced bindings apply again.",
	"Geht mit: ":         "Synced: ",
	"Läuft auf":          "Runs on",
	"Nur dieser Rechner": "This computer only",
	"Rechner im Sync":    "Computers in sync",
	"Shell, Startordner bzw. Schnittstelle gelten nur hier; die anderen Rechner im Sync behalten ihre (z. B. pwsh am PC, zsh am Mac, COM3 ↔ /dev/ttyUSB0)":                                               "Shell, start folder or port apply here only; the other computers in sync keep theirs (e.g. pwsh on the PC, zsh on the Mac, COM3 ↔ /dev/ttyUSB0)",
	"Sync-Ort, eigene Key-Pfade, Shell/Schnittstelle „nur dieser Rechner“, Tasten „nur hier“, Passwörter im Schlüsselbund, Team-Ordner, Vermittler, Audit, Debug, MCP, Editor, Schriftgröße im Fenster.": "Sync location, own key paths, shell/port “this computer only”, keys “here only”, passwords in the keychain, team folders, relays, audit, debug, MCP, editor, window font size.",
	"Verbindungen, Ordner, Tunnel, Runbooks, Arbeitsplätze, Zeitpläne (laufen nur auf ihrem Rechner), KI samt Hausregeln, alle Einstellungen und Tasten.":                                                "Connections, folders, tunnels, runbooks, workspaces, schedules (run only on their computer), AI including house rules, all settings and keys.",
	"Zeitpläne gehen mit dem Sync, laufen aber nur auf diesem einen Rechner (sonst liefe derselbe Job überall)":                                                                                          "Schedules are synced but run only on this one computer (otherwise the same job would run everywhere)",
	"Zählung der Befehle je Verbindung mitnehmen: Favoriten-Vorschläge (F9) kommen dann von allen Rechnern":                                                                                              "Sync the command counts per connection: favorite suggestions (F9) then come from all computers",
	"alle Rechner":       "all computers",
	"läuft auf ":         "runs on ",
	"nur dieser Rechner": "this computer only",
}
