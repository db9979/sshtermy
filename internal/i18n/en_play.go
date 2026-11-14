package i18n

// Aufnahme abspielen (ui/player.go).
func init() {
	for k, v := range enPlay {
		en[k] = v
	}
}

var enPlay = map[string]string{
	"Aufnahme abspielen":             "Play recording",
	"Pause / weiter":                 "pause / resume",
	"← → · ⇧← ⇧→":                    "← → · ⇧← ⇧→",
	"5 s · 30 s spulen":              "skip 5 s · 30 s",
	"↑ ↓":                            "↑ ↓",
	"Tempo 0,25× … 16×":              "speed 0.25× … 16×",
	"/ · n":                          "/ · n",
	"Text suchen · nächster Treffer": "search text · next match",
	"m · .":                          "m · .",
	"nächste Marke · Einzelschritt":  "next marker · single step",
	"lange Pausen kürzen":            "shorten long pauses",
	"Andere Datei …":                 "Other file …",
	"Pfad eingeben":                  "enter path",
	"Datei suchen …":                 "Search file …",
	"Pfad zur .cast-Datei:":          "Path to the .cast file:",
	"Aufnahmen aus %s (Palette › Aufnahme starten nimmt einen Tab auf).": "Recordings from %s (palette › Start recording records a tab).",
	"Datei nicht gefunden: ":                         "File not found: ",
	"Aufnahme zu groß":                               "Recording too large",
	"Keine abspielbare Aufnahme: ":                   "Not a playable recording: ",
	"Keine weitere Marke":                            "No further marker",
	"In der Aufnahme suchen":                         "Search the recording",
	"Text, der irgendwann auf dem Bildschirm stand:": "Text that appeared on screen at some point:",
	"Text":                        "Text",
	"Suche von vorn":              "Search from the start",
	"„%s“ kommt nicht vor":        "“%s” does not occur",
	"Treffer bei %s · n nächster": "Match at %s · n next",
	"Pause":                       "Pause",
	"Abspielen":                   "Play",
	"Pausen kürzen":               "shorten pauses",
	"Originalzeit":                "original timing",
	"Tempo":                       "speed",
	"Marke":                       "marker",
	"  · Pausen gekürzt":          "  · pauses shortened",
	"  · Fenster kleiner als Aufnahme (%d×%d)": "  · window smaller than recording (%d×%d)",
	"Gleich hier abspielen":                    "Play it here",
	"Aufnahme abspielen (.cast) …":             "Play recording (.cast) …",
}
