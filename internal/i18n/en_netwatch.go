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
