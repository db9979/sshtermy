package ui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/ui/theme"
)

// Die Farbschemata selbst (Farbwerte, eigene Farben, Kontrastprüfung) liegen
// im Paket theme. Hier stehen nur die Namen, unter denen die Oberfläche sie
// benutzt, und die daraus abgeleiteten Stile.

type Theme = theme.Theme

const CustomName = theme.CustomName

var (
	Themes      = theme.All
	Nacht       = theme.Nacht
	Hell        = theme.Hell
	Kontrast    = theme.Kontrast
	Matrix      = theme.Matrix
	Farbenblind = theme.Farbenblind

	ThemeByName      = theme.ByName
	CustomTheme      = theme.Custom
	themeOf          = theme.Of
	themeGroup       = theme.Group
	parseHex         = theme.ParseHex
	hexOf            = theme.Hex
	mixColor         = theme.Mix
	luminance        = theme.Luminance
	contrast         = theme.Contrast
	contrastProblems = theme.Problems
	rgbOf            = theme.RGB
)

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
