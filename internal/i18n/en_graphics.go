package i18n

// Grafik, Runde 2: Live-Graphen in F6 (ui/hostlive.go), Ping-Kurve in der
// Statuszeile, Fensterstil (ui/frames.go).
func init() {
	for k, v := range enGraphics {
		en[k] = v
	}
}

var enGraphics = map[string]string{
	"  Anteil an der ganzen Maschine":                  "  share of the whole machine",
	"1 h: der erste Punkt kommt nach 24 s · t = 5 min": "1 h: the first point arrives after 24 s · t = 5 min",
	"5 min":                  "5 min",
	"1 h":                    "1 h",
	"groß":                   "large",
	"schmal":                 "narrow",
	"Live-Graphen starten …": "starting live graphs …",
	"Netz":                   "Network",
	"Top-Prozesse":           "Top processes",
	"jetzt":                  "now",
	"▲ zäh · ":               "▲ sluggish · ",
	"Fensterstil":            "Window style",
	"eckig + Schatten":       "square + shadow",
	"rund + Schatten":        "round + shadow",
	"eckig, flach":           "square, flat",
}

func init() {
	en["Graphen: 5 min / 1 h"] = "graphs: 5 min / 1 h"
	en["Graphen groß"] = "graphs large"
}

func init() {
	for k, v := range map[string]string{
		"Jan": "Jan", "Feb": "Feb", "Mär": "Mar", "Apr": "Apr", "Mai": "May", "Jun": "Jun",
		"Jul": "Jul", "Aug": "Aug", "Sep": "Sep", "Okt": "Oct", "Nov": "Nov", "Dez": "Dec",
	} {
		en[k] = v
	}
}

func init() {
	for k, v := range map[string]string{
		"Aktivität":                              "Activity",
		"Tageszeit":                              "Time of day",
		"    weniger ":                           "    less ",
		"mehr":                                   "more",
		"  %d Wochen · an %d Tagen gearbeitet":   "  %d weeks · active on %d days",
		"%d ok":                                  "%d ok",
		"%d Fehler":                              "%d failed",
		"%d abgebrochen":                         "%d canceled",
		"Datei %d von %d · ":                     "file %d of %d · ",
		"läuft … ":                               "running … ",
		"noch ":                                  "left ",
		"→ Exit %d":                              "→ exit %d",
		"+%d weitere":                            "+%d more",
		"Weitere Verbindungen (Ring/Redundanz):": "More links (ring/redundancy):",
	} {
		en[k] = v
	}
}
