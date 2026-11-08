package i18n

// Ruhezustand/Netzwechsel (internal/netwatch, ui/netwatch.go).
func init() {
	for k, v := range map[string]string{
		"Netzwerk gewechselt: Verbindungen werden geprüft": "Network changed: checking connections",
		"Ruhezustand beendet: Verbindungen werden geprüft": "Woke from sleep: checking connections",
		"kein Netz, wartet auf eine Verbindung …":          "no network, waiting for a connection …",
	} {
		en[k] = v
	}
}

// MCP-Server (internal/mcp, ui/mcp.go).
func init() {
	for k, v := range map[string]string{
		"\n\nj = ausführen · n = ablehnen":                 "\n\ny = run · n = decline",
		"Ein KI-Client möchte auf %s ausführen:\n\n  %s\n": "An AI client wants to run on %s:\n\n  %s\n",
		"MCP-Zugriff für KI-Clients (Claude, …)":           "MCP access for AI clients (Claude, …)",
		"MCP-Zugriff: ":         "MCP access: ",
		"MCP: %s auf %s":        "MCP: %s on %s",
		"MCP: Befehl erlauben?": "MCP: allow command?",
		"mit Rückfrage":         "with confirmation",
		"nur lesen":             "read only",
	} {
		en[k] = v
	}
}
