package ui

// Farbverläufe: Logo in der Kopfzeile von Akzent nach KI, Kopf- und
// Sitzungszeile mit einem sanften Schimmer der Umgebungsfarbe, der nach
// rechts in die normale Leiste übergeht. Aus bei Fensterstil „flach“ und im
// ruhigen Modus (gleiche Schalter wie die Schatten, frames.go).

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// gradientsOn: Verläufe zeichnen?
func gradientsOn() bool { return shadowsOn() }

// tintRun: Text mit dem Stil st, Hintergrund je Spalte aus bgAt (x zählt
// ab x0, damit mehrere Stücke einer Leiste einen Verlauf teilen). Gleiche
// Farben nebeneinander werden zusammengefasst.
func tintRun(s string, x0 int, st lipgloss.Style, bgAt func(x int) color.Color) string {
	st = st.UnsetPadding()
	var b strings.Builder
	var run []rune
	var runBg color.Color
	flush := func() {
		if len(run) > 0 {
			b.WriteString(st.Background(runBg).Render(string(run)))
			run = run[:0]
		}
	}
	for i, ch := range []rune(s) {
		bg := bgAt(x0 + i)
		if !sameColor(bg, runBg) {
			flush()
			runBg = bg
		}
		run = append(run, ch)
	}
	flush()
	return b.String()
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

// gradFg: Schrift im Verlauf fgA → fgB auf festem Hintergrund.
func gradFg(s string, fgA, fgB, bg color.Color, bold bool) string {
	r := []rune(s)
	var b strings.Builder
	for i, ch := range r {
		k := 0.0
		if len(r) > 1 {
			k = float64(i) / float64(len(r)-1)
		}
		st := lipgloss.NewStyle().Foreground(mixColor(fgA, fgB, k)).Bold(bold)
		if bg != nil {
			st = st.Background(bg)
		}
		b.WriteString(st.Render(string(ch)))
	}
	return b.String()
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
