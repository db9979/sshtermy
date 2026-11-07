package i18n

// Icon-Stil, Themes und „Farben anpassen“ (ui/icons.go, ui/colors.go).
// Eigene Datei, damit parallele Änderungen an en.go nicht kollidieren.
func init() {
	for k, v := range enLook {
		en[k] = v
	}
}

var enLook = map[string]string{
	"← / Bksp":          "← / Bksp",
	"Matrix (grün)":     "Matrix (green)",
	"Bernstein (Retro)": "Amber (retro)",
	"Neon (Cyber)":      "Neon (cyber)",
	"Eigene Farben":     "Custom colours",

	"Farben anpassen (eigene Farben)": "Customise colours",
	"Editor":                          "editor",
	"Nerd-Font-Symbole (braucht z. B. Cascadia Mono NF)":                                                 "Nerd Font icons (needs e.g. Cascadia Mono NF)",
	"Nerd-Font-Symbole an · fehlen Kästchen, im Terminal eine Nerd Font wählen (z. B. Cascadia Mono NF)": "Nerd Font icons on · if you see boxes, pick a Nerd Font in your terminal (e.g. Cascadia Mono NF)",
	"Text-Symbole an: [+] [k] [/] … laufen mit jeder Schrift":                                            "Text icons on: [+] [k] [/] … work with any font",

	"Farben anpassen": "Customise colours",
	"   Vorschau sofort, Strg+S speichert · geht mit dem Sync mit": "   live preview, Ctrl+S saves · synced to your other computers",
	"Vorlage":                                "Template",
	"Hintergrund":                            "Background",
	"Gedämpft":                               "Muted",
	"Rahmen":                                 "Border",
	"Akzent":                                 "Accent",
	"Warnung":                                "Warning",
	"Gefahr":                                 "Danger",
	" geändert":                              " changed",
	"Vorschau":                               "Preview",
	"KI: nginx läuft, keine Fehler im Log":   "AI: nginx is running, no errors in the log",
	"Platte fast voll":                       "disk almost full",
	"blockiert":                              "blocked",
	"Schlecht lesbar: ":                      "Hard to read: ",
	"Kontrast-Check: alle Farben gut lesbar": "Contrast check: all colours readable",
	"Bitte sechs Hex-Ziffern, z. B. #39ff14": "Please enter six hex digits, e.g. #39ff14",
	"Eigene Farben gespeichert":              "Custom colours saved",
	"Farbton":                                "hue",
	"heller/dunkler":                         "lighter/darker",
	"Hex eingeben":                           "enter hex",
	"wie Vorlage":                            "as template",
}
