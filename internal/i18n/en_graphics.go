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
