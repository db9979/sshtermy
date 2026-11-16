package i18n

// Zurückblättern im Verlauf mit der Tastatur (ui/scrollkeys.go).
func init() {
	for k, v := range enScrollKeys {
		en[k] = v
	}
}

var enScrollKeys = map[string]string{
	"Im Verlauf eine Seite zurückblättern":   "Scroll history up one page",
	"Im Verlauf eine Seite vorblättern":      "Scroll history down one page",
	"Zum Anfang des Verlaufs":                "Jump to start of history",
	"Zurück nach unten (Ende des Verlaufs)":  "Back to the bottom (end of history)",
	"Mausrad / ⇧Bild↑ Bild↓":                 "Mouse wheel / ⇧PgUp PgDn",
	"im Verlauf zurückblättern (Esc: unten)": "scroll back in history (Esc: bottom)",
	"⇧Pos1 / ⇧Ende":                          "⇧Home / ⇧End",
	"Anfang / Ende des Verlaufs":             "start / end of history",
	"tmux: Verlauf im Copy-Mode · markieren kopiert · Esc/q oder ganz nach unten beendet": "tmux: history in copy mode · select to copy · Esc/q or scrolling to the bottom ends it",
	"Hole den Verlauf aus tmux …": "Fetching the history from tmux …",
}
