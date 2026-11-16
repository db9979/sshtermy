package ui

// Farbverläufe: Logo in der Kopfzeile von Akzent nach KI, Kopf- und
// Sitzungszeile mit einem sanften Schimmer der Umgebungsfarbe, der nach
// rechts in die normale Leiste übergeht. Aus bei Fensterstil „flach“ und im
// ruhigen Modus (gleiche Schalter wie die Schatten, frames.go).

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// gradientsOn: Verläufe zeichnen?
func gradientsOn() bool { return shadowsOn() }

// tintRun: Text mit dem Stil st, Hintergrund je Spalte aus bgAt (x zählt
// ab x0, damit mehrere Stücke einer Leiste einen Verlauf teilen). Gleiche
// Farben nebeneinander werden zusammengefasst.
func tintRun(s string, x0 int, st lipgloss.Style, bgAt func(x int) color.Color) string {
	st = st.UnsetPadding()
	// Hintergrund je Lauf als eigene Sequenz in einen einzigen Render: je Lauf
	// ein eigenes Style.Render kostete in Kopf- und Sitzungszeile ~10 % eines Bildes.
	var b strings.Builder
	var runBg color.Color
	runs, start := 0, true
	for i, ch := range []rune(s) {
		bg := bgAt(x0 + i)
		if start || !sameColor(bg, runBg) {
			runBg, start = bg, false
			runs++
			if bg == nil {
				b.WriteString("\x1b[49m")
			} else {
				b.WriteString(ansi.Style{}.BackgroundColor(bg).String())
			}
		}
		b.WriteRune(ch)
	}
	if runs <= 1 {
		return st.Background(runBg).Render(s)
	}
	return st.Render(b.String())
}

// barTint: Hintergrund einer Leiste, links ein Hauch von tint, nach fade
// Spalten ganz base.
func barTint(tint, base color.Color, strength float64, fade int) func(x int) color.Color {
	return func(x int) color.Color {
		if tint == nil || !gradientsOn() {
			return base
		}
		k := 1 - easeInOut(min(1, float64(x)/float64(max(1, fade))))
		return mixColor(base, tint, strength*k)
	}
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	return rgbOf(a) == rgbOf(b)
}

func easeInOut(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - (-2*t+2)*(-2*t+2)/2
}

// logoChip: „>_ sshTermy“ als Schild, Hintergrund von Akzent nach KI.
func (a *App) logoChip(text string) string {
	t := a.s.T
	if !gradientsOn() {
		return a.s.Logo.Render(text)
	}
	text = " " + text + " "
	n := len([]rune(text))
	to := mixColor(t.Accent, t.AI, 0.85)
	return tintRun(text, 0, a.s.Logo, func(x int) color.Color {
		return mixColor(t.Accent, to, float64(x)/float64(max(1, n-1)))
	})
}
