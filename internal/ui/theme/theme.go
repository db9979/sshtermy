// Package theme: die Farbschemata von sshTermy (Farbwerte, Auswahl nach
// Namen, eigene Farben, Kontrastprüfung). Reine Daten und Farbrechnung ohne
// Bezug zur Oberfläche; internal/ui leitet daraus seine Stile ab.
package theme

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Theme "Nacht": ruhiges Dunkelblau, eine Akzentfarbe für Navigation (Blau),
// eine für die KI (Violett), Ampelfarben nur für Zustände. So bleibt sofort
// erkennbar, was vom Modell kommt (violett) und was gefährlich ist (rot).
type Theme struct {
	Name, Label                                                              string
	Dark                                                                     bool
	Bg, Surface, Surface2, Border, Text, Muted, Accent, AI, OK, Warn, Danger color.Color
	// Orange und Cyan: Zusatzfarben für Verbindungen (accent.go); nil heißt
	// Standardton passend zu hell/dunkel.
	Orange, Cyan color.Color
}

var Nacht = Theme{
	Name: "nacht", Label: "Dunkel (Nacht)", Dark: true,
	Bg:       lipgloss.Color("#1a1b26"),
	Surface:  lipgloss.Color("#1f2335"),
	Surface2: lipgloss.Color("#292e42"),
	Border:   lipgloss.Color("#3b4261"),
	Text:     lipgloss.Color("#c0caf5"),
	Muted:    lipgloss.Color("#737aa2"),
	Accent:   lipgloss.Color("#7aa2f7"),
	AI:       lipgloss.Color("#bb9af7"),
	OK:       lipgloss.Color("#9ece6a"),
	Warn:     lipgloss.Color("#e0af68"),
	Danger:   lipgloss.Color("#f7768e"),
}

// Hell: für helle Umgebungen und Beamer. Gleiche Bedeutung der Farben,
// aber dunkle, kräftige Töne auf hellem Grund.
var Hell = Theme{
	Name: "hell", Label: "Hell", Dark: false,
	Bg:       lipgloss.Color("#f5f5f7"),
	Surface:  lipgloss.Color("#e4e6ee"),
	Surface2: lipgloss.Color("#d2d6e4"),
	Border:   lipgloss.Color("#a4aac4"),
	Text:     lipgloss.Color("#1f2335"),
	Muted:    lipgloss.Color("#5a607e"),
	Accent:   lipgloss.Color("#2e59c9"),
	AI:       lipgloss.Color("#7a3cc4"),
	OK:       lipgloss.Color("#3a7a12"),
	Warn:     lipgloss.Color("#8f5a00"),
	Danger:   lipgloss.Color("#c4122f"),
}

// Kontrast: Schwarz/Weiß mit reinen Signalfarben (WCAG AAA für Text),
// für schlechte Displays, Sonnenlicht und Sehschwäche.
var Kontrast = Theme{
	Name: "kontrast", Label: "Hochkontrast", Dark: true,
	Bg:       lipgloss.Color("#000000"),
	Surface:  lipgloss.Color("#000000"),
	Surface2: lipgloss.Color("#303030"),
	Border:   lipgloss.Color("#ffffff"),
	Text:     lipgloss.Color("#ffffff"),
	Muted:    lipgloss.Color("#d0d0d0"),
	Accent:   lipgloss.Color("#00e5ff"),
	AI:       lipgloss.Color("#ff80ff"),
	OK:       lipgloss.Color("#00ff5f"),
	Warn:     lipgloss.Color("#ffff00"),
	Danger:   lipgloss.Color("#ff6060"),
}

// KontrastHell: Hochkontrast auf Weiß, für Tageslicht und Beamer, wenn
// „Hell“ zu blass ist. Reines Schwarz, kräftige dunkle Signalfarben.
var KontrastHell = Theme{
	Name: "kontrasthell", Label: "Hochkontrast hell", Dark: false,
	Bg:       lipgloss.Color("#ffffff"),
	Surface:  lipgloss.Color("#ffffff"),
	Surface2: lipgloss.Color("#d8d8d8"),
	Border:   lipgloss.Color("#000000"),
	Text:     lipgloss.Color("#000000"),
	Muted:    lipgloss.Color("#333333"),
	Accent:   lipgloss.Color("#0033cc"),
	AI:       lipgloss.Color("#7a00b3"),
	OK:       lipgloss.Color("#006400"),
	Warn:     lipgloss.Color("#7a4100"),
	Danger:   lipgloss.Color("#b00000"),
}

// Matrix: Phosphorgrün auf Schwarz wie ein alter Monochrom-Monitor. Die KI
// bleibt cyan, damit man ihre Vorschläge vom eigenen Text unterscheidet.
var Matrix = Theme{
	Name: "matrix", Label: "Matrix (grün)", Dark: true,
	Bg:       lipgloss.Color("#050a05"),
	Surface:  lipgloss.Color("#0b140b"),
	Surface2: lipgloss.Color("#123018"),
	Border:   lipgloss.Color("#1f6b2e"),
	Text:     lipgloss.Color("#b6f7c1"),
	Muted:    lipgloss.Color("#5a9c66"),
	Accent:   lipgloss.Color("#39ff14"),
	AI:       lipgloss.Color("#00e5ff"),
	OK:       lipgloss.Color("#7dff6b"),
	Warn:     lipgloss.Color("#ffd400"),
	Danger:   lipgloss.Color("#ff4040"),
}

// Bernstein: warmes Amber wie ein Retro-Terminal, schont nachts die Augen.
var Bernstein = Theme{
	Name: "bernstein", Label: "Bernstein (Retro)", Dark: true,
	Bg:       lipgloss.Color("#110b02"),
	Surface:  lipgloss.Color("#1a1104"),
	Surface2: lipgloss.Color("#33230a"),
	Border:   lipgloss.Color("#6b4a12"),
	Text:     lipgloss.Color("#ffd27a"),
	Muted:    lipgloss.Color("#b0843c"),
	Accent:   lipgloss.Color("#ffb000"),
	AI:       lipgloss.Color("#ff8fd8"),
	OK:       lipgloss.Color("#c6e65a"),
	Warn:     lipgloss.Color("#ffe14d"),
	Danger:   lipgloss.Color("#ff5a36"),
}

// Neon: Cyan und Magenta auf Nachtblau.
var Neon = Theme{
	Name: "neon", Label: "Neon (Cyber)", Dark: true,
	Bg:       lipgloss.Color("#0a0b18"),
	Surface:  lipgloss.Color("#10132a"),
	Surface2: lipgloss.Color("#1e2250"),
	Border:   lipgloss.Color("#4a3a8c"),
	Text:     lipgloss.Color("#e2e7ff"),
	Muted:    lipgloss.Color("#8a90c8"),
	Accent:   lipgloss.Color("#00f0ff"),
	AI:       lipgloss.Color("#ff2bd6"),
	OK:       lipgloss.Color("#39ff88"),
	Warn:     lipgloss.Color("#ffe600"),
	Danger:   lipgloss.Color("#ff3860"),
}

// Farbenblind: Okabe-Ito-Farben, für Rot-Grün-Schwäche (Deuteranopie,
// Protanopie) unterscheidbar: OK blau, Warnung gelb, Gefahr orange.
var Farbenblind = Theme{
	Name: "farbenblind", Label: "Farbenblind (dunkel)", Dark: true,
	Bg:       lipgloss.Color("#16181d"),
	Surface:  lipgloss.Color("#1e2128"),
	Surface2: lipgloss.Color("#2a2e38"),
	Border:   lipgloss.Color("#4a5060"),
	Text:     lipgloss.Color("#e8e8e8"),
	Muted:    lipgloss.Color("#9a9fab"),
	Accent:   lipgloss.Color("#56b4e9"),
	AI:       lipgloss.Color("#cc79a7"),
	OK:       lipgloss.Color("#56b4e9"),
	Warn:     lipgloss.Color("#f0e442"),
	Danger:   lipgloss.Color("#e69f00"),
}

// FarbenblindHell: dieselbe Idee auf hellem Grund.
var FarbenblindHell = Theme{
	Name: "farbenblindhell", Label: "Farbenblind (hell)", Dark: false,
	Bg:       lipgloss.Color("#fafafa"),
	Surface:  lipgloss.Color("#eceef2"),
	Surface2: lipgloss.Color("#dcdfe6"),
	Border:   lipgloss.Color("#9aa0ad"),
	Text:     lipgloss.Color("#1a1a1a"),
	Muted:    lipgloss.Color("#555a66"),
	Accent:   lipgloss.Color("#005a8c"),
	AI:       lipgloss.Color("#8c3d6a"),
	OK:       lipgloss.Color("#005a8c"),
	Warn:     lipgloss.Color("#6e5a00"),
	Danger:   lipgloss.Color("#a33d00"),
}

// All in der Reihenfolge der Auswahl: dunkle, helle, barrierefreie
// (siehe Group).
var All = []Theme{
	Nacht, Polar, Tiefsee, Graphit, Vampir, Kirschbluete, Abendrot, Neon, Tusche,
	Wald, Kreide, Lagune, Solar, Lagerfeuer, Bernstein, Matrix,
	Hell, Papier, Frost, Sand, Morgenrot, Minze,
	Kontrast, KontrastHell, Farbenblind, FarbenblindHell,
}

// Group: Abschnitt in der Theme-Auswahl.
func Group(t Theme) string {
	switch {
	case strings.HasPrefix(t.Name, "kontrast"), strings.HasPrefix(t.Name, "farbenblind"):
		return "Barrierefrei"
	case t.Dark:
		return "Dunkel"
	}
	return "Hell"
}

// CustomName: Theme aus eigenen Farben („Farben anpassen“).
const CustomName = "eigene"

// ByName: unbekannte oder leere Namen ergeben „Nacht“.
func ByName(name string) Theme {
	for _, t := range All {
		if t.Name == name {
			return t
		}
	}
	return Nacht
}

// Of: das Theme der Einstellungen, eigene Farben eingerechnet.
func Of(set domain.Settings) Theme {
	if set.Theme == CustomName && set.Colors != nil {
		return Custom(*set.Colors)
	}
	return ByName(set.Theme)
}

// Custom: Vorlage plus eigene Farben. Ungültige Werte bleiben bei der
// Vorlage; die Flächen werden aus Hintergrund und Text gemischt.
func Custom(c domain.CustomColors) Theme {
	t := ByName(c.Base)
	t.Name, t.Label = CustomName, "Eigene Farben"
	set := func(dst *color.Color, hex string) {
		if v, ok := ParseHex(hex); ok {
			*dst = v
		}
	}
	set(&t.Bg, c.Bg)
	set(&t.Text, c.Text)
	set(&t.Muted, c.Muted)
	set(&t.Border, c.Border)
	set(&t.Accent, c.Accent)
	set(&t.AI, c.AI)
	set(&t.OK, c.OK)
	set(&t.Warn, c.Warn)
	set(&t.Danger, c.Danger)
	if _, ok := ParseHex(c.Bg); ok {
		t.Surface, t.Surface2 = Mix(t.Bg, t.Text, 0.05), Mix(t.Bg, t.Text, 0.13)
		if dark := Luminance(RGB(t.Bg)) < 0.4; dark != t.Dark {
			t.Dark, t.Orange, t.Cyan = dark, nil, nil // Zusatzfarben der Vorlage passen nicht mehr
		}
	}
	return t
}

// ParseHex liest "#rrggbb" (auch ohne #).
func ParseHex(s string) (color.Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return nil, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil, false
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}, true
}

// Hex: Farbe als "#rrggbb".
func Hex(c color.Color) string {
	v := RGB(c)
	return fmt.Sprintf("#%02x%02x%02x", v[0], v[1], v[2])
}

func Mix(a, b color.Color, f float64) color.Color {
	x, y := RGB(a), RGB(b)
	m := func(i int) uint8 { return uint8(math.Round(float64(x[i]) + (float64(y[i])-float64(x[i]))*f)) }
	return color.RGBA{m(0), m(1), m(2), 0xff}
}

// Luminance nach WCAG 2.
func Luminance(c [3]int) float64 {
	f := func(v int) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c[0]) + 0.7152*f(c[1]) + 0.0722*f(c[2])
}

// Contrast: Kontrastverhältnis zweier Farben (1 … 21).
func Contrast(a, b color.Color) float64 {
	la, lb := Luminance(RGB(a))+0.05, Luminance(RGB(b))+0.05
	return math.Max(la, lb) / math.Min(la, lb)
}

// Problems: Farben, die auf dem Hintergrund schlecht lesbar sind
// (Text 4,5:1, Gedämpftes 3,5:1, Hochkontrast 7:1).
func Problems(t Theme) []string {
	need := 4.5
	if strings.HasPrefix(t.Name, "kontrast") {
		need = 7
	}
	var out []string
	for _, c := range []struct {
		name string
		c    color.Color
	}{{"Text", t.Text}, {"Gedämpft", t.Muted}, {"Akzent", t.Accent}, {"KI", t.AI}, {"OK", t.OK}, {"Warnung", t.Warn}, {"Gefahr", t.Danger}} {
		min := need
		if c.name == "Gedämpft" && need < 7 {
			min = 3.5 // Nebensachen dürfen in den normalen All leiser sein
		}
		if r := Contrast(c.c, t.Bg); r < min {
			out = append(out, fmt.Sprintf("%s %.1f:1", i18n.T(c.name), r))
		}
	}
	return out
}

// RGB: Farbe als 8-Bit-Anteile.
func RGB(c color.Color) [3]int {
	r, g, b, _ := c.RGBA()
	return [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
}
