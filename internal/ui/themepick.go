package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Farbschema wählen: Liste aller Themes nach Abschnitten (dunkel, hell,
// barrierefrei), daneben eine Vorschau mit allen Rollen. Die ganze
// Oberfläche zeigt das markierte Theme sofort; ⏎ übernimmt, Esc stellt das
// alte Aussehen wieder her.

type themePicker struct {
	sel  int    // Index in Themes
	top  int    // erste sichtbare Listenzeile
	back screen // wohin Esc/⏎ zurückführt
}

// themeRow: eine Zeile der Liste (Abschnittskopf oder Theme).
type themeRow struct {
	head string
	idx  int
}

func themeRows() []themeRow {
	var rows []themeRow
	last := ""
	for i, t := range Themes {
		if g := themeGroup(t); g != last {
			if last != "" {
				rows = append(rows, themeRow{idx: -1})
			}
			rows = append(rows, themeRow{head: g, idx: -1})
			last = g
		}
		rows = append(rows, themeRow{idx: i})
	}
	return rows
}

func (a *App) openThemes() tea.Cmd {
	back := a.scr
	if back == scrThemes || back == scrColors {
		back = scrManager
	}
	a.themePick = themePicker{sel: themeIndex(ThemeByName(a.st.Settings().Theme).Name), back: back}
	a.pal = nil
	a.scr = scrThemes
	a.previewTheme()
	return nil
}

// previewTheme färbt die ganze Oberfläche mit dem markierten Theme.
func (a *App) previewTheme() {
	t := Themes[a.themePick.sel]
	activeTheme = t
	a.setStyles(t)
}

// closeThemes: zurück; ohne Übernehmen gilt wieder das alte Aussehen.
func (a *App) closeThemes() {
	a.scr = a.themePick.back
	a.lookSig = ""
	a.applyLook()
}

func (a *App) applyThemePick() tea.Cmd {
	a.scr = a.themePick.back
	return a.setTheme(Themes[a.themePick.sel].Name)
}

func (a *App) themesKeys() [][2]string {
	return [][2]string{{"↑↓", i18n.T("Farbschema")}, {"⏎", i18n.T("übernehmen")}, {"e", i18n.T("Farben anpassen")}, {"Esc", i18n.T("zurück")}}
}

func (a *App) updateThemes(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	p := &a.themePick
	n := len(Themes)
	switch k.String() {
	case "esc", "q":
		a.closeThemes()
		return nil
	case "enter", "space":
		return a.applyThemePick()
	case "e", "f":
		base := Themes[p.sel].Name
		a.scr = p.back
		a.openColors()
		a.colEd.c = domain.CustomColors{Base: base}
		a.previewColors()
		return nil
	case "up", "k", "left", "h", "shift+tab":
		p.sel = (p.sel - 1 + n) % n
	case "down", "j", "right", "l", "tab":
		p.sel = (p.sel + 1) % n
	case "pgup":
		p.sel = max(0, p.sel-8)
	case "pgdown":
		p.sel = min(n-1, p.sel+8)
	case "home", "g":
		p.sel = 0
	case "end", "G":
		p.sel = n - 1
	default:
		return nil
	}
	a.previewTheme()
	return nil
}

// swatch: die Signalfarben eines Themes auf seinem eigenen Hintergrund.
func swatch(t Theme) string {
	st := lipgloss.NewStyle().Background(t.Bg)
	out := st.Render(" ")
	for _, c := range []color.Color{t.Accent, t.AI, t.OK, t.Warn, t.Danger} {
		out += st.Foreground(c).Render("■")
	}
	return out + st.Foreground(t.Text).Render(" Aa ")
}

func (a *App) viewThemes() string {
	s := a.s
	p := &a.themePick
	cur := ThemeByName(a.st.Settings().Theme).Name
	if a.st.Settings().Theme == CustomName {
		cur = CustomName
	}
	var b strings.Builder
	b.WriteString(s.Accent.Render(" "+ic("theme")+" "+i18n.T("Farbschema wählen")) +
		s.Muted.Render(fmt.Sprintf(i18n.T("   %d Themes · Vorschau sofort, ⏎ übernimmt · geht mit dem Sync mit"), len(Themes))) + "\n\n")

	// Liste links
	const listW = 35
	var list []string
	selRow := 0
	for _, r := range themeRows() {
		switch {
		case r.head != "":
			list = append(list, s.Muted.Render(" "+groupHead(i18n.T(r.head))))
		case r.idx < 0:
			list = append(list, "")
		default:
			i, t := r.idx, Themes[r.idx]
			name := i18n.T(t.Label)
			mark := "  "
			if t.Name == cur {
				mark = s.OK.Render("✓ ")
			}
			row := fmt.Sprintf(" %-20s", ansiTrunc(name, 20))
			if i == p.sel {
				selRow = len(list)
				row = s.Selected.Render("▸" + row[1:])
			} else {
				row = s.Text.Render(row)
			}
			line := row + swatch(t) + " " + mark
			list = append(list, a.z.click(line, func(_ tea.Mouse, double bool) tea.Cmd {
				p.sel = i
				a.previewTheme()
				if double {
					return a.applyThemePick()
				}
				return nil
			}))
		}
	}
	// nur der sichtbare Ausschnitt, die Auswahl bleibt im Bild
	h := max(5, a.h-6)
	if selRow < p.top+1 {
		p.top = max(0, selRow-1)
	}
	if selRow >= p.top+h-1 {
		p.top = selRow - h + 2
	}
	p.top = max(0, min(p.top, len(list)-h))
	end := min(len(list), p.top+h)
	vis := list[p.top:end]
	if p.top > 0 {
		vis[0] = s.Muted.Render("   ↑ " + i18n.T("mehr"))
	}
	if end < len(list) {
		vis[len(vis)-1] = s.Muted.Render("   ↓ " + i18n.T("mehr"))
	}
	left := lipgloss.NewStyle().Width(listW + 2).Render(strings.Join(vis, "\n"))

	if a.w < listW+2+44 {
		b.WriteString(left)
		return b.String()
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", themePreview(Themes[p.sel], min(66, a.w-listW-6))))
	return b.String()
}

// themePreview: Musterausschnitt mit allen Rollen des Themes.
func themePreview(t Theme, w int) string {
	ps := NewStyles(t)
	in := w - 4
	pad := func(s string) string { return ansiTrunc(s, in) }
	var l []string
	ver := ps.Version.Render("v1.0")
	head := ps.Logo.Render(">_ sshTermy") + ps.Header.Render(" ")
	gap := max(1, in-lipgloss.Width(head)-lipgloss.Width(ver))
	l = append(l, head+ps.Header.Render(strings.Repeat(" ", gap))+ver)
	l = append(l, "")
	l = append(l, ps.Muted.Render(groupHead(i18n.T("Produktion"))))
	dot := func(c color.Color) string { return lipgloss.NewStyle().Foreground(c).Render("●") }
	bar := func(id string) string { return lipgloss.NewStyle().Foreground(accentColor(t, id)).Render("▌") }
	host := func(acc string, d devClass, name, addr string, up color.Color, sel bool) string {
		nm := ps.Text.Render(fmt.Sprintf(" %-11s", name))
		if sel {
			nm = ps.Selected.Render(fmt.Sprintf(" %-11s", name))
		}
		return bar(acc) + " " + dot(up) + " " + ps.Muted.Render(padIcon(devGlyph(d))) + nm + " " + ps.Muted.Render(addr)
	}
	l = append(l, host("red", devLinux, "web-01", "deploy@10.0.1.11", t.OK, true))
	l = append(l, host("orange", devLinux, "db-01", "postgres@10.0.2.5", t.OK, false))
	l = append(l, host("cyan", devSwitch, "core-sw-01", "manager@10.0.0.2", t.Warn, false))
	l = append(l, host("green", devWindows, "build-win", "ci@10.0.3.20", t.Danger, false))
	l = append(l, "")
	l = append(l, ps.OK.Render("deploy@web-01")+ps.Muted.Render(":")+ps.Accent.Render("~")+ps.Text.Render("$ systemctl status nginx"))
	l = append(l, ps.AI.Render("◆ "+i18n.T("KI: nginx läuft, keine Fehler im Log")))
	l = append(l, ps.Warn.Render("▲ "+i18n.T("Platte fast voll"))+"  "+ps.Danger.Render("▲ rm -rf / "+i18n.T("blockiert")))
	l = append(l, ps.OK.Render("✓ "+i18n.T("Sicherung fertig")))
	l = append(l, "")
	l = append(l, ps.BadgeAI.Render(i18n.T("KI"))+" "+ps.ChipDanger.Render("prod")+" "+ps.Chip.Render("lab")+" "+ps.Badge.Render("● online"))
	l = append(l, "")
	keys := ps.Key.Render("⏎") + ps.KeyDesc.Render(i18n.T("verbinden")) + ps.Key.Render("F1") + ps.KeyDesc.Render(i18n.T("Hilfe"))
	l = append(l, keys)
	l = append(l, "")
	acc := ps.Muted.Render(i18n.T("Verbindungsfarben") + " ")
	for _, id := range []string{"red", "orange", "yellow", "green", "cyan", "blue", "purple"} {
		acc += lipgloss.NewStyle().Foreground(accentColor(t, id)).Render("■")
	}
	l = append(l, acc)
	if p := contrastProblems(t); len(p) > 0 {
		l = append(l, ps.Warn.Render("▲ "+i18n.T("Schlecht lesbar: ")+strings.Join(p, ", ")))
	} else {
		l = append(l, ps.OK.Render("✓ "+i18n.T("Kontrast-Check: alle Farben gut lesbar")))
	}
	for i := range l {
		l[i] = pad(l[i])
	}
	box := lipgloss.NewStyle().Border(frameBorder).BorderForeground(t.Accent).Padding(0, 1).Width(w).Render(strings.Join(l, "\n"))
	return ps.PanelTitle.Render(" "+i18n.T("Vorschau")+": "+i18n.T(t.Label)) + "\n" + box
}
