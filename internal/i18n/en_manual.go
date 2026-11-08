package i18n

// Handbuch auf der Webseite (Palette › Hilfe).
func init() {
	for k, v := range map[string]string{
		"Handbuch & FAQ (Webseite)": "Manual & FAQ (website)",
	} {
		en[k] = v
	}
}
