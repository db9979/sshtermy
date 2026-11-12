package ui

// Aktives Fenster deutlich zeigen: Der aktive Tab oben und der aktive
// Bereich der geteilten Ansicht haben eine gefüllte Fläche in der Tab-Farbe
// (ohne eigene Farbe: Akzent), Schrift in Kontrastfarbe, fett und ein „▶“
// vorn. Inaktive liegen gedämpft auf eigener Fläche. Damit bleibt es auch
// ohne Farbe (Monochrom, Farbenblind) erkennbar: ▶, Fettdruck, dicker Rahmen.

import (
	"image/color"
)

// activeMark: Zeichen vor dem aktiven Tab / Bereich (Form statt nur Farbe).
const activeMark = "▶"

// activeFill: Füllfarbe des aktiven Tabs bzw. Bereichstitels.
func (a *App) activeFill(s *session) color.Color {
	if col := a.tabColor(s); col != nil {
		return col
	}
	return a.s.T.Accent
}

// inkOn: gut lesbare Schrift auf der Fläche bg. Erst Theme-Hintergrund oder
// -Text (passt zum Schema), reicht beides nicht, Schwarz oder Weiß.
func inkOn(t Theme, bg color.Color) color.Color {
	best, bc := t.Bg, contrast(t.Bg, bg)
	if c := contrast(t.Text, bg); c > bc {
		best, bc = t.Text, c
	}
	if bc >= 4.5 {
		return best
	}
	black, white := color.Color(color.RGBA{0, 0, 0, 0xff}), color.Color(color.RGBA{0xff, 0xff, 0xff, 0xff})
	if contrast(black, bg) >= contrast(white, bg) {
		return black
	}
	return white
}
