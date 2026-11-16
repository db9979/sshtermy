package i18n

// Texte für „+“/Alt+N: neuer Tab über den Verbindungs-Manager (internal/ui/tabs.go).
func init() {
	for de, e := range map[string]string{
		"in neuem Tab öffnen":                     "open in new tab",
		"in neuem Tab":                            "in new tab",
		"neuer Tab: Verbindung im Manager wählen": "new tab: pick a connection in the manager",
	} {
		en[de] = e
	}
}
