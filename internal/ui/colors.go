package ui

import (
	"fmt"
	"image/color"
	"math"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Farben anpassen: eine Vorlage (eines der Themes) wählen und einzelne
// Farben ändern. Die ganze Oberfläche zeigt die Änderung sofort; gespeichert
// wird erst mit Strg+S (Theme „eigene“, geht mit dem Sync auf andere Rechner).

type colorEditor struct {
	c       domain.CustomColors // Arbeitsstand
	sel     int                 // 0 = Vorlage, 1… = Farben
	editing bool                // Hex-Wert wird getippt
	buf     string
	err     string
}

// colorField: eine einstellbare Farbe.
type colorField struct {
	label string
	get   func(*domain.CustomColors) *string
	of    func(Theme) color.Color
}

var colorFields = []colorField{
	{"Hintergrund", func(c *domain.CustomColors) *string { return &c.Bg }, func(t Theme) color.Color { return t.Bg }},
	{"Text", func(c *domain.CustomColors) *string { return &c.Text }, func(t Theme) color.Color { return t.Text }},
	{"Gedämpft", func(c *domain.CustomColors) *string { return &c.Muted }, func(t Theme) color.Color { return t.Muted }},
	{"Rahmen", func(c *domain.CustomColors) *string { return &c.Border }, func(t Theme) color.Color { return t.Border }},
	{"Akzent", func(c *domain.CustomColors) *string { return &c.Accent }, func(t Theme) color.Color { return t.Accent }},
	{"KI", func(c *domain.CustomColors) *string { return &c.AI }, func(t Theme) color.Color { return t.AI }},
	{"OK", func(c *domain.CustomColors) *string { return &c.OK }, func(t Theme) color.Color { return t.OK }},
	{"Warnung", func(c *domain.CustomColors) *string { return &c.Warn }, func(t Theme) color.Color { return t.Warn }},
	{"Gefahr", func(c *domain.CustomColors) *string { return &c.Danger }, func(t Theme) color.Color { return t.Danger }},
}

func (a *App) openColors() tea.Cmd {
	set := a.st.Settings()
	e := colorEditor{sel: 1}
	switch {
	case set.Theme == CustomName && set.Colors != nil:
		e.c = *set.Colors
	default:
		e.c.Base = ThemeByName(set.Theme).Name
	}
	a.colEd = e
	a.pal = nil
	a.scr = scrColors
	a.previewColors()
	return nil
}

// previewColors färbt die ganze Oberfläche mit dem Arbeitsstand.
func (a *App) previewColors() {
	t := CustomTheme(a.colEd.c)
	activeTheme = t
	a.setStyles(t)
}

// closeColors: zurück, ohne Speichern gilt wieder das alte Aussehen.
func (a *App) closeColors() {
	a.colEd.editing = false
	a.scr = scrManager
	a.lookSig = ""
	a.applyLook()
}

func (a *App) saveColors() {
	c := a.colEd.c
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Theme, s.Colors = CustomName, &c }); err != nil {
		a.colEd.err = err.Error()
		return
	}
	a.closeColors()
	a.flash(i18n.T("Eigene Farben gespeichert") + " · " + fontHint())
}

func (a *App) colorsKeys() [][2]string {
	if a.colEd.editing {
		return [][2]string{{"⏎", i18n.T("übernehmen")}, {"Esc", i18n.T("abbrechen")}}
	}
	if a.colEd.sel == 0 {
		return [][2]string{{"↑↓", i18n.T("Farbe")}, {"←→", i18n.T("Vorlage")}, {"Strg+S", i18n.T("speichern")}, {"Esc", i18n.T("abbrechen")}}
	}
	return [][2]string{{"↑↓", i18n.T("Farbe")}, {"←→", i18n.T("Farbton")}, {"+/-", i18n.T("heller/dunkler")}, {"⏎", i18n.T("Hex eingeben")},
		{"Entf", i18n.T("wie Vorlage")}, {"Strg+S", i18n.T("speichern")}, {"Esc", i18n.T("abbrechen")}}
}

func (a *App) updateColors(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	e := &a.colEd
	key := k.String()
	if e.editing {
		switch key {
		case "esc":
			e.editing, e.err = false, ""
		case "enter", "tab":
			if _, ok := parseHex(e.buf); !ok {
				e.err = i18n.T("Bitte sechs Hex-Ziffern, z. B. #39ff14")
				return nil
			}
			*colorFields[e.sel-1].get(&e.c) = "#" + strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e.buf), "#"))
			e.editing, e.err = false, ""
			a.previewColors()
		case "backspace":
			if r := []rune(e.buf); len(r) > 0 {
				e.buf = string(r[:len(r)-1])
			}
		default:
			if t := k.Text; t != "" && len(e.buf) < 7 && strings.Trim(strings.ToLower(t), "#0123456789abcdef") == "" {
				e.buf += t
			}
		}
		return nil
	}
	switch key {
	case "esc", "q":
		a.closeColors()
	case "ctrl+s":
		a.saveColors()
	case "up", "k":
		e.sel = max(0, e.sel-1)
	case "down", "j", "tab":
		e.sel = min(len(colorFields), e.sel+1)
	case "left", "right", "h", "l":
		step := 1
		if key == "left" || key == "h" {
			step = -1
		}
		if e.sel == 0 {
			e.c = domain.CustomColors{Base: Themes[(themeIndex(e.c.Base)+step+len(Themes))%len(Themes)].Name}
		} else {
			e.shift(15*float64(step), 0)
		}
		a.previewColors()
	case "+", "=", "-":
		if e.sel > 0 {
			d := 0.05
			if key == "-" {
				d = -d
			}
			e.shift(0, d)
			a.previewColors()
		}
	case "enter":
		if e.sel > 0 {
			e.editing, e.err = true, ""
			e.buf = hexOf(colorFields[e.sel-1].of(CustomTheme(e.c)))
		}
	case "delete", "backspace":
		if e.sel > 0 {
			*colorFields[e.sel-1].get(&e.c) = ""
			a.previewColors()
		}
	case "ctrl+r":
		e.c = domain.CustomColors{Base: e.c.Base}
		a.previewColors()
	}
	return nil
}

func themeIndex(name string) int {
	for i, t := range Themes {
		if t.Name == name {
			return i
		}
	}
	return 0
}

// shift dreht den Farbton der gewählten Farbe (Grad) und ändert die Helligkeit.
func (e *colorEditor) shift(hue, light float64) {
	f := colorFields[e.sel-1]
	h, s, l := toHSL(f.of(CustomTheme(e.c)))
	if hue != 0 && s < 0.05 {
		s = 0.6 // Grau hat keinen Farbton: erst etwas Farbe geben
	}
	h = math.Mod(h+hue+360, 360)
	l = math.Max(0, math.Min(1, l+light))
	*f.get(&e.c) = hexOf(fromHSL(h, s, l))
}

func (a *App) viewColors() string {
	s := a.s
	e := &a.colEd
	t := CustomTheme(e.c)
	var b strings.Builder
	b.WriteString(s.Accent.Render(" "+ic("colors")+" "+i18n.T("Farben anpassen")) + s.Muted.Render(i18n.T("   Vorschau sofort, Strg+S speichert · geht mit dem Sync mit")) + "\n\n")

	// Vorlagen
	pre := " "
	if e.sel == 0 {
		pre = s.Accent.Render("▸")
	}
	line := pre + s.Muted.Render(fmt.Sprintf("%-13s", i18n.T("Vorlage")))
	for _, th := range Themes {
		th := th
		short, _, _ := strings.Cut(i18n.T(th.Label), " (") // „Matrix (grün)“ → „Matrix“
		chip := s.Chip.Render(short)
		if th.Name == ThemeByName(e.c.Base).Name {
			chip = s.Badge.Foreground(t.Accent).Bold(true).Render("✓ " + short)
		}
		line += a.z.click(chip, func(tea.Mouse, bool) tea.Cmd {
			e.sel, e.c = 0, domain.CustomColors{Base: th.Name}
			a.previewColors()
			return nil
		}) + " "
	}
	b.WriteString(ansiTrunc(line, a.w-2) + "\n\n")

	for i, f := range colorFields {
		i, f := i, f
		c := f.of(t)
		sw := lipgloss.NewStyle().Background(c).Render("    ")
		if f.label == "Hintergrund" {
			sw = lipgloss.NewStyle().Background(c).Foreground(t.Border).Render("····")
		}
		val := s.Text.Render(hexOf(c))
		if e.sel == i+1 && e.editing {
			val = s.BadgeAI.Render(e.buf + "▌")
		}
		mark := ""
		if *f.get(&e.c) != "" {
			mark = s.AI.Render(" ●") + s.Muted.Render(i18n.T(" geändert"))
		}
		name := fmt.Sprintf(" %-13s", i18n.T(f.label))
		if e.sel == i+1 {
			name = s.Selected.Render("▸" + name[1:])
		} else {
			name = s.Text.Render(name)
		}
		row := name + sw + "  " + val + mark
		b.WriteString(a.z.click(row, func(_ tea.Mouse, double bool) tea.Cmd {
			e.sel = i + 1
			if double {
				e.editing, e.buf = true, hexOf(f.of(CustomTheme(e.c)))
			}
			return nil
		}) + "\n")
	}

	// Vorschau mit den echten Rollen
	ps := NewStyles(t)
	b.WriteString("\n" + s.Muted.Render(" "+groupHead(i18n.T("Vorschau"))) + "\n")
	b.WriteString(" " + ps.OK.Render("deploy@web-01") + ps.Muted.Render(":") + ps.Accent.Render("~") + ps.Text.Render("$ systemctl status nginx") + "\n")
	b.WriteString(" " + ps.AI.Render(ic("ai")+" "+i18n.T("KI: nginx läuft, keine Fehler im Log")) + "\n")
	b.WriteString(" " + ps.Warn.Render("▲ "+i18n.T("Platte fast voll")) + "   " + ps.Danger.Render("▲ rm -rf / "+i18n.T("blockiert")) + "\n\n")

	if p := contrastProblems(t); len(p) > 0 {
		b.WriteString(" " + s.Warn.Render("▲ "+i18n.T("Schlecht lesbar: ")+strings.Join(p, ", ")) + "\n")
	} else {
		b.WriteString(" " + s.OK.Render("✓ "+i18n.T("Kontrast-Check: alle Farben gut lesbar")) + "\n")
	}
	if e.err != "" {
		b.WriteString(" " + s.Danger.Render(e.err) + "\n")
	}
	return b.String()
}

func ansiTrunc(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// toggleNerdIcons schaltet zwischen Text-Tags und Nerd-Font-Symbolen. Welche
// Schrift das Terminal nutzt, kann ein Programm darin nicht erfahren; darum
// zeigt das Einschalten erst eine Probezeile, und nur wer dort Symbole statt
// Kästchen sieht, schaltet um.
func (a *App) toggleNerdIcons() tea.Cmd {
	if iconSet == iconNerd {
		a.setIcons("")
		a.flash(i18n.T("Text-Symbole an: [+] [k] [/] … laufen mit jeder Schrift"))
		return nil
	}
	a.modal = confirmModal(i18n.T("Nerd-Font-Symbole einschalten?"), nerdProbe(), func() tea.Cmd {
		a.setIcons(iconNerd)
		a.flash(i18n.T("Nerd-Font-Symbole an"))
		return nil
	})
	return nil
}

func (a *App) setIcons(set string) {
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Icons = set }); err != nil {
		a.flashErr(err.Error())
		return
	}
	a.applyLook()
}

// nerdProbe: Probezeile mit Nerd-Font-Symbolen und wie man die Schrift bekommt.
func nerdProbe() string {
	probe := strings.Join([]string{iconDefs["settings"].nerd, iconDefs["sshkey"].nerd, iconDefs["search"].nerd,
		devNerd[devLinux], devNerd[devMac], devNerd[devWindows], iconDefs["manager"].nerd}, "  ")
	return i18n.T("Probe:") + "  " + probe + "\n\n" +
		i18n.T("Siehst du Zahnrad, Schlüssel, Lupe, Pinguin, Apfel, Windows, Server?\nDann j. Kästchen oder „?“: deinem Terminal fehlt eine Nerd Font,\ndann n (die Text-Tags [+] [k] [/] laufen mit jeder Schrift).") +
		"\n\n" + nerdFontHint()
}

// nerdFontHint: wo es eine Nerd Font gibt und wo man sie einstellt.
func nerdFontHint() string {
	switch runtime.GOOS {
	case "darwin":
		return i18n.T("Mac: brew install --cask font-caskaydia-mono-nerd-font\ndann Terminal › Einstellungen › Profile › Text › Schrift:\n„CaskaydiaMono Nerd Font“ (iTerm2: Settings › Profiles › Text › Font).")
	case "windows":
		return i18n.T("Windows: „Cascadia Mono NF“ installieren, zu finden unter\ngithub.com/microsoft/cascadia-code/releases, dann Windows Terminal ›\nEinstellungen › Profil › Darstellung › Schriftart.")
	}
	return i18n.T("Linux: eine Nerd Font von nerdfonts.com installieren (z. B.\nCaskaydiaMono) und im Terminalprofil als Schrift wählen.")
}

// ---------------------------------------------------------------- HSL

func toHSL(c color.Color) (h, s, l float64) {
	v := rgbOf(c)
	r, g, b := float64(v[0])/255, float64(v[1])/255, float64(v[2])/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

func fromHSL(h, s, l float64) color.Color {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	u := func(v float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(1, v+m)) * 255)) }
	return color.RGBA{u(r), u(g), u(b), 0xff}
}
