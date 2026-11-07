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
	"Symbole: ":                      "Icons: ",
	"Text-Tags":                      "text tags",
	"Unicode-Formen":                 "Unicode shapes",
	"Minimal":                        "minimal",
	"Nerd Font":                      "Nerd Font",
	"braucht Nerd Font":              "needs a Nerd Font",
	"Nerd-Font-Symbole an":           "Nerd Font icons on",
	"Nerd-Font-Symbole einschalten?": "Turn on Nerd Font icons?",
	"Probe:":                         "Test:",
	"Siehst du Zahnrad, Schlüssel, Lupe, Pinguin, Apfel, Windows, Server?\nDann j. Kästchen oder „?“: deinem Terminal fehlt eine Nerd Font,\ndann n (die Text-Tags [+] [k] [/] laufen mit jeder Schrift).": "Do you see a gear, key, magnifier, penguin, apple, Windows, server?\nThen y. Boxes or \"?\": your terminal has no Nerd Font,\nthen n (the text tags [+] [k] [/] work with any font).",
	"Mac: brew install --cask font-caskaydia-mono-nerd-font\ndann Terminal › Einstellungen › Profile › Text › Schrift:\n„CaskaydiaMono Nerd Font“ (iTerm2: Settings › Profiles › Text › Font).":            "Mac: brew install --cask font-caskaydia-mono-nerd-font\nthen Terminal › Settings › Profiles › Text › Font:\n\"CaskaydiaMono Nerd Font\" (iTerm2: Settings › Profiles › Text › Font).",
	"Windows: „Cascadia Mono NF“ installieren, zu finden unter\ngithub.com/microsoft/cascadia-code/releases, dann Windows Terminal ›\nEinstellungen › Profil › Darstellung › Schriftart.":                  "Windows: install \"Cascadia Mono NF\" from\ngithub.com/microsoft/cascadia-code/releases, then Windows Terminal ›\nSettings › Profile › Appearance › Font face.",
	"Linux: eine Nerd Font von nerdfonts.com installieren (z. B.\nCaskaydiaMono) und im Terminalprofil als Schrift wählen.":                                                                                "Linux: install a Nerd Font from nerdfonts.com (e.g. CaskaydiaMono)\nand pick it as the font in your terminal profile.",

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
