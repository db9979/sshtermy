package ui

// Fensterstil der Dialoge: Schatten unter Dialogen, Palette und Auswahl,
// dahinter wird es etwas dunkler; die Ecken wahlweise rund. „flach“ ist der
// Stil bis v1.0.265 (eckig, ohne Schatten). Barrierearm: immer flach.

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

const (
	framesEckig = ""
	framesRund  = "rund"
	framesFlach = "flach"
)

var framesValues = []string{framesEckig, framesRund, framesFlach}

// framesSetting: aktueller Stand (applyLook), für Stellen ohne App.
var framesSetting string

// setFrames übernimmt den Stil; die Rahmen der Styles folgen mit
// setStyles (applyLook baut sie bei einer Änderung neu).
func setFrames(v string) {
	framesSetting = v
	if v == framesRund {
		frameBorder = lipgloss.RoundedBorder()
	} else {
		frameBorder = lipgloss.NormalBorder()
	}
}

func framesLabel(v string) string {
	switch v {
	case framesRund:
		return i18n.T("rund + Schatten")
	case framesFlach:
		return i18n.T("eckig, flach")
	}
	return i18n.T("eckig + Schatten")
}

func (a *App) cycleFrames(dir int) tea.Cmd {
	cur := indexOf(framesValues, a.st.Settings().Frames)
	next := framesValues[cycleIndex(cur, len(framesValues), dir)]
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Frames = next }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	a.applyLook()
	return nil
}

// frameCorners: Ecken oben (links, rechts) und unten im aktuellen Stil.
func frameCorners() (top, bottom [2]rune) {
	if framesSetting == framesRund {
		return [2]rune{'╭', '╮'}, [2]rune{'╰', '╯'}
	}
	return [2]rune{'┌', '┐'}, [2]rune{'└', '┘'}
}

// shadowsOn: Schatten und abgedunkelter Hintergrund?
func shadowsOn() bool {
	return framesSetting != framesFlach && !accessible
}

// shadowColor: etwas dunkler als der Hintergrund (auf hellen Themes nur
// ein Hauch, sonst wirkt es schmutzig).
func shadowColor(t Theme) color.Color {
	bg := rgbOf(t.Bg)
	lum := (0.2126*float64(bg[0]) + 0.7152*float64(bg[1]) + 0.0722*float64(bg[2])) / 255
	if lum > 0.5 {
		return mixColor(t.Bg, color.Black, 0.16)
	}
	return mixColor(t.Bg, color.Black, 0.55)
}

// shadowLayers: Hintergrund abdunkeln und einen Schatten rechts unten unter
// den Kasten (bw × bh an x, y) legen. Die Zeichen darunter bleiben lesbar,
// nur dunkler (wie ein echter Schatten).
func (a *App) shadowLayers(base string, x, y, bw, bh int) []*lipgloss.Layer {
	dimmed := fadeANSI(base, a.s.T, 0.62)
	bg := shadowColor(a.s.T)
	sc := lipgloss.NewStyle().Background(bg).Foreground(mixColor(bg, a.s.T.Text, 0.3))
	lines := strings.Split(base, "\n")
	cut := func(row, x0, w int) string {
		if row < 0 || row >= len(lines) || w <= 0 {
			return ""
		}
		seg := ansi.Strip(ansi.Cut(lines[row], x0, x0+w))
		if d := w - ansi.StringWidth(seg); d > 0 {
			seg += strings.Repeat(" ", d)
		}
		return sc.Render(seg)
	}
	ls := []*lipgloss.Layer{lipgloss.NewLayer(dimmed)}
	if rx := x + bw; rx < a.w && bh > 1 {
		w := min(2, a.w-rx)
		var r []string
		for row := y + 1; row < y+bh; row++ {
			r = append(r, cut(row, rx, w))
		}
		ls = append(ls, lipgloss.NewLayer(strings.Join(r, "\n")).X(rx).Y(y+1).Z(1))
	}
	if by := y + bh; by < a.h {
		bx := x + 2
		w := min(bw, a.w-bx)
		if w > 0 {
			ls = append(ls, lipgloss.NewLayer(cut(by, bx, w)).X(bx).Y(by).Z(1))
		}
	}
	return ls
}
