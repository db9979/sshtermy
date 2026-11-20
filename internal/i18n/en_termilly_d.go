package i18n

// Ideen aus termilly, Paket D: Panel andocken, F5/Log-Blick daneben, Statuszeile je Bereich.
func init() {
	for de, e := range map[string]string{
		"F5 und Log-Blick füllen wieder die ganze Fläche":                "F5 and log view fill the whole area again",
		"F5 und Log-Blick neben dem Terminal (breite Fenster)":           "F5 and log view beside the terminal (wide windows)",
		"F5 und Log-Blick stehen in breiten Fenstern neben dem Terminal": "F5 and log view now sit beside the terminal in wide windows",
		"Panelrand ziehen": "drag the panel edge",
		"Panel breiter/höher (Palette: links, rechts, unten)": "panel wider/taller (palette: left, right, bottom)",
		"Panel steht jetzt ":                                    "Panel is now on the ",
		"Panelgröße wieder automatisch":                         "Panel size automatic again",
		"Seitenpanel andocken":                                  "Dock the side panel",
		"Statuszeile je Bereich (Ordner, letzter Befehl, Last)": "Status line per pane (folder, last command, load)",
		"links":  "left",
		"rechts": "right",
		"unten":  "bottom",
	} {
		en[de] = e
	}
}
