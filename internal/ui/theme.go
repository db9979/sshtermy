package ui

import (
	"image/color"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// Theme "Nacht": ruhiges Dunkelblau, eine Akzentfarbe für Navigation (Blau),
// eine für die KI (Violett), Ampelfarben nur für Zustände. So bleibt sofort
// erkennbar, was vom Modell kommt (violett) und was gefährlich ist (rot).
type Theme struct {
	Name, Label                                                              string
	Dark                                                                     bool
	Bg, Surface, Surface2, Border, Text, Muted, Accent, AI, OK, Warn, Danger color.Color
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
	Danger:   lipgloss.Color("#ff3030"),
}

// Themes in der Reihenfolge der Auswahl.
var Themes = []Theme{Nacht, Hell, Kontrast}

// ThemeByName: unbekannte oder leere Namen ergeben „Nacht“.
func ThemeByName(name string) Theme {
	for _, t := range Themes {
		if t.Name == name {
			return t
		}
	}
	return Nacht
}

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
	s.Panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).Padding(0, 1)
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
	s.Modal = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent).Padding(1, 2)
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
	s.Blurred.Text = lipgloss.NewStyle().Foreground(t.Muted)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(t.Muted)
	s.Blurred.Placeholder = s.Focused.Placeholder
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(t.Accent)
	s.Blurred.Prompt = s.Focused.Prompt
	s.Cursor.Color = t.Accent
	return s
}
