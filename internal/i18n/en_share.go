package i18n

// Texte für „Sitzung live teilen“.
var enShare = map[string]string{
	"%s: %d Zuschauer":                       "%s: %d viewers",
	"%s: Zuschauer dazugekommen (%d)":        "%s: a viewer joined (%d)",
	": Teilen beendet":                       ": sharing stopped",
	": Teilen beendet (Vermittler getrennt)": ": sharing stopped (relay disconnected)",
	"Beenden: Palette → „Teilen beenden“ oder Tab schließen.": "Stop: palette → “Stop sharing” or close the tab.",
	"Eine geteilte Sitzung lässt sich nicht weiterteilen":     "A shared session cannot be shared again",
	"Freigabe wird angelegt …":                                "Creating the share …",
	"Geteilte Sitzung ansehen":                                "Watch shared session",
	"Geteilte Sitzung ansehen …":                              "Watch shared session …",
	"Link":                                                    "link",
	"Link zum Zusehen (in der Zwischenablage):":               "Watch link (in the clipboard):",
	"Link zum Zusehen anzeigen":                               "Show watch link",
	"Link zum Zusehen einfügen (https://…#teilen=…)":          "Paste the watch link (https://…#teilen=…)",
	"Link zum Zusehen ohne „#teilen=…“":                       "Watch link without “#teilen=…”",
	"Link zum Zusehen: Kennung ungültig":                      "Watch link: invalid id",
	"Link zum Zusehen: Schlüssel fehlt oder ist ungültig":     "Watch link: key missing or invalid",
	"Nur Zusehen: Tasten gehen nirgendwo hin":                 "Watch only: keys go nowhere",
	"Rahmen lässt sich nicht entschlüsseln (falscher Link?)":  "Frame cannot be decrypted (wrong link?)",
	"Rahmen zu kurz":                                          "Frame too short",
	"Sitzung live teilen (Link zum Zusehen) …":                "Share session live (watch link) …",
	"Sitzung live teilen: ":                                   "Share session live: ",
	"Teilen beenden (%d Zuschauer)":                           "Stop sharing (%d viewers)",
	"Teilen geht über einen eigenen Vermittler: erst „sshtermy vermittler“ einrichten und koppeln (Palette → „Vermittler koppeln“)": "Sharing works through your own relay: set up “sshtermy vermittler” and pair it first (palette → “Pair / manage relays”)",
	"Teilen: ":                      "Sharing: ",
	"Verbindung zum Vermittler: %w": "Connection to the relay: %w",
	"Vermittler hat keine gültige Kennung geschickt":                                                                     "The relay sent no valid id",
	"Wer den Link hat, sieht diesen Tab live in sshTermy (Link in die Palette einfügen).":                                "Anyone with the link sees this tab live in sshTermy (paste the link into the palette).",
	"Zuschauer können nichts tippen. Der Vermittler sieht nur verschlüsselte Daten, der Schlüssel steht allein im Link.": "Viewers cannot type. The relay only sees encrypted data, the key is only in the link.",
	"„teilen“ / Link":                 "“share” / link",
	"live teilen / zusehen":           "share live / watch",
	"Zusehen":                         "Watching",
	"geteilt · %d":                    "shared · %d",
	"live":                            "live",
	"nur Zusehen":                     "watch only",
	"Über welchen Vermittler teilen?": "Share through which relay?",
	"Über: %s · Zuschauer gerade: %d": "Via: %s · viewers now: %d",
}

func init() {
	for k, v := range enShare {
		en[k] = v
	}
}
