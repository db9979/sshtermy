package i18n

// Stand der Technik (Prüfung 2026-11-12): Update-Version, alte SSH-Verfahren, KI
func init() {
	for k, v := range map[string]string{
		"update: signierte Prüfsummen gehören zu %s, nicht zu v%s – Download verworfen":                                                              "update: signed checksums belong to %s, not to v%s – download discarded",
		"update: signierte Prüfsummen nennen keine Version – Download verworfen":                                                                     "update: signed checksums name no version – download discarded",
		"Vorsicht: %s spricht nur alte SSH-Verfahren (%s). Verschlüsselt, aber nicht mehr Stand der Technik – Firmware aktualisieren, wenn möglich.": "Caution: %s only speaks old SSH algorithms (%s). Encrypted, but no longer state of the art – update the firmware if you can.",
		"LLM: Claude hat die Anfrage abgelehnt (Sicherheitsfilter). Anders formulieren oder ein anderes Modell wählen.":                              "LLM: Claude declined the request (safety filter). Rephrase it or choose another model.",
		"alt: ":            "old: ",
		" · quantensicher": " · quantum-safe",
	} {
		en[k] = v
	}
}
