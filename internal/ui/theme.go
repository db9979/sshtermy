package ui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
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

// Themes in der Reihenfolge der Auswahl: dunkle, helle, barrierefreie
// (siehe themeGroup).
var Themes = []Theme{
	Nacht, Polar, Tiefsee, Graphit, Vampir, Kirschbluete, Abendrot, Neon, Tusche,
	Wald, Kreide, Lagune, Solar, Lagerfeuer, Bernstein, Matrix,
	Hell, Papier, Frost, Sand, Morgenrot, Minze,
	Kontrast, KontrastHell, Farbenblind, FarbenblindHell,
}

// themeGroup: Abschnitt in der Theme-Auswahl.
func themeGroup(t Theme) string {
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

// ThemeByName: unbekannte oder leere Namen ergeben „Nacht“.
func ThemeByName(name string) Theme {
	for _, t := range Themes {
		if t.Name == name {
			return t
		}
	}
	return Nacht
}

// themeOf: das Theme der Einstellungen, eigene Farben eingerechnet.
func themeOf(set domain.Settings) Theme {
	if set.Theme == CustomName && set.Colors != nil {
		return CustomTheme(*set.Colors)
	}
	return ThemeByName(set.Theme)
}

// CustomTheme: Vorlage plus eigene Farben. Ungültige Werte bleiben bei der
// Vorlage; die Flächen werden aus Hintergrund und Text gemischt.
func CustomTheme(c domain.CustomColors) Theme {
	t := ThemeByName(c.Base)
	t.Name, t.Label = CustomName, "Eigene Farben"
	set := func(dst *color.Color, hex string) {
		if v, ok := parseHex(hex); ok {
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
	if _, ok := parseHex(c.Bg); ok {
		t.Surface, t.Surface2 = mixColor(t.Bg, t.Text, 0.05), mixColor(t.Bg, t.Text, 0.13)
		if dark := luminance(rgbOf(t.Bg)) < 0.4; dark != t.Dark {
			t.Dark, t.Orange, t.Cyan = dark, nil, nil // Zusatzfarben der Vorlage passen nicht mehr
		}
	}
	return t
}

// parseHex liest "#rrggbb" (auch ohne #).
func parseHex(s string) (color.Color, bool) {
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

// hexOf: Farbe als "#rrggbb".
func hexOf(c color.Color) string {
	v := rgbOf(c)
	return fmt.Sprintf("#%02x%02x%02x", v[0], v[1], v[2])
}

func mixColor(a, b color.Color, f float64) color.Color {
	x, y := rgbOf(a), rgbOf(b)
	m := func(i int) uint8 { return uint8(math.Round(float64(x[i]) + (float64(y[i])-float64(x[i]))*f)) }
	return color.RGBA{m(0), m(1), m(2), 0xff}
}

// luminance nach WCAG 2.
func luminance(c [3]int) float64 {
	f := func(v int) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c[0]) + 0.7152*f(c[1]) + 0.0722*f(c[2])
}

// contrast: Kontrastverhältnis zweier Farben (1 … 21).
func contrast(a, b color.Color) float64 {
	la, lb := luminance(rgbOf(a))+0.05, luminance(rgbOf(b))+0.05
	return math.Max(la, lb) / math.Min(la, lb)
}

// contrastProblems: Farben, die auf dem Hintergrund schlecht lesbar sind
// (Text 4,5:1, Gedämpftes 3,5:1, Hochkontrast 7:1).
func contrastProblems(t Theme) []string {
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
			min = 3.5 // Nebensachen dürfen in den normalen Themes leiser sein
		}
		if r := contrast(c.c, t.Bg); r < min {
			out = append(out, fmt.Sprintf("%s %.1f:1", i18n.T(c.name), r))
		}
	}
	return out
}

// frameBorder: eckige Rahmen für den Terminal-Look (┌─┐ statt ╭─╮).
var frameBorder = lipgloss.NormalBorder()

// activeTheme: für Eingabefelder, die ohne App-Zugriff gebaut werden.
var activeTheme = Nacht

// Styles, abgeleitet aus dem Theme.
type Styles struct {
	T                                   Theme
	Header, Logo, Version, Credit       lipgloss.Style
	Footer, Key, KeyDesc                lipgloss.Style
	Panel, PanelFocus, PanelTitle       lipgloss.Style
	Muted, Text, Accent, AI, OK, Warn   lipgloss.Style
	Danger, DangerBox, Ghost, Selected  lipgloss.Style
	Chip, ChipDanger, Badge, BadgeAI    lipgloss.Style
	Modal, Input, Label, Tab, TabActive lipgloss.Style
	Code                                lipgloss.Style

	// zoneKey macht Tastenknöpfe in Dialogen klickbar (setzt die App).
	zoneKey func(label, rendered string) string
}

func NewStyles(t Theme) Styles {
	s := Styles{T: t}
	s.Header = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Text)
	s.Logo = lipgloss.NewStyle().Background(t.Accent).Foreground(t.Bg).Bold(true).Padding(0, 1)
	s.Version = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Muted).Padding(0, 1)
	s.Credit = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Muted).Italic(true)
	s.Footer = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Muted)
	s.Key = lipgloss.NewStyle().Background(t.Surface2).Foreground(t.Accent).Bold(true).Padding(0, 1)
	s.KeyDesc = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Muted).Padding(0, 1, 0, 0)
	s.Panel = lipgloss.NewStyle().Border(frameBorder).BorderForeground(t.Border).Padding(0, 1)
	s.PanelFocus = s.Panel.BorderForeground(t.AI)
	s.PanelTitle = lipgloss.NewStyle().Foreground(t.AI).Bold(true)
	s.Muted = lipgloss.NewStyle().Foreground(t.Muted)
	s.Text = lipgloss.NewStyle().Foreground(t.Text)
	s.Accent = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	s.AI = lipgloss.NewStyle().Foreground(t.AI)
	s.OK = lipgloss.NewStyle().Foreground(t.OK)
	s.Warn = lipgloss.NewStyle().Foreground(t.Warn)
	s.Danger = lipgloss.NewStyle().Foreground(t.Danger).Bold(true)
	s.DangerBox = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(t.Danger).Padding(0, 1)
	s.Ghost = lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
	s.Selected = lipgloss.NewStyle().Background(t.Surface2).Foreground(t.Text).Bold(true)
	s.Chip = lipgloss.NewStyle().Background(t.Surface2).Foreground(t.Text).Padding(0, 1)
	s.ChipDanger = lipgloss.NewStyle().Background(t.Danger).Foreground(t.Bg).Padding(0, 1)
	s.Badge = lipgloss.NewStyle().Background(t.Surface2).Foreground(t.OK).Padding(0, 1)
	s.BadgeAI = lipgloss.NewStyle().Background(t.AI).Foreground(t.Bg).Bold(true).Padding(0, 1)
	s.Modal = lipgloss.NewStyle().Border(frameBorder).BorderForeground(t.Accent).Padding(1, 2)
	s.Input = lipgloss.NewStyle().Foreground(t.Text)
	s.Label = lipgloss.NewStyle().Foreground(t.Muted).Width(20)
	s.Tab = lipgloss.NewStyle().Background(t.Surface).Foreground(t.Muted).Padding(0, 1)
	s.TabActive = lipgloss.NewStyle().Background(t.Bg).Foreground(t.Text).Bold(true).Padding(0, 1)
	s.Code = lipgloss.NewStyle().Foreground(t.OK)
	return s
}

// inputStyles: Textfelder passend zum Theme (sonst wäre Text auf Hell
// unsichtbar, weil bubbles Standardfarben für dunkle Terminals setzt).
func inputStyles(t Theme) textinput.Styles {
	s := textinput.DefaultStyles(t.Dark)
	s.Focused.Text = lipgloss.NewStyle().Foreground(t.Text)
	s.Blurred.Text = lipgloss.NewStyle().Foreground(t.Text)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(t.Muted).Faint(true).Italic(true)
	s.Blurred.Placeholder = s.Focused.Placeholder
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(t.Accent)
	s.Blurred.Prompt = s.Focused.Prompt
	s.Cursor.Color = t.Accent
	return s
}
