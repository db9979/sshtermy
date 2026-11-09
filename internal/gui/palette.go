package gui

import (
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Standardfarben wie „Campbell“ in Windows Terminal: Dominik kennt sie, und
// die Themes von sshTermy sind darauf abgestimmt.
var (
	defaultFg = color.RGBA{0xCC, 0xCC, 0xCC, 0xFF}
	defaultBg = color.RGBA{0x0C, 0x0C, 0x0C, 0xFF}
	cursorClr = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
)

var basic16 = [16]color.RGBA{
	{0x0C, 0x0C, 0x0C, 0xFF}, {0xC5, 0x0F, 0x1F, 0xFF}, {0x13, 0xA1, 0x0E, 0xFF}, {0xC1, 0x9C, 0x00, 0xFF},
	{0x00, 0x37, 0xDA, 0xFF}, {0x88, 0x17, 0x98, 0xFF}, {0x3A, 0x96, 0xDD, 0xFF}, {0xCC, 0xCC, 0xCC, 0xFF},
	{0x76, 0x76, 0x76, 0xFF}, {0xE7, 0x48, 0x56, 0xFF}, {0x16, 0xC6, 0x0C, 0xFF}, {0xF9, 0xF1, 0xA5, 0xFF},
	{0x3B, 0x78, 0xFF, 0xFF}, {0xB4, 0x00, 0x9E, 0xFF}, {0x61, 0xD6, 0xD6, 0xFF}, {0xF2, 0xF2, 0xF2, 0xFF},
}

// rgba wandelt eine Zellfarbe um; nil heißt Standardfarbe.
func rgba(c color.Color, def color.RGBA) color.RGBA {
	switch v := c.(type) {
	case nil:
		return def
	case ansi.BasicColor:
		return basic16[int(v)&15]
	case ansi.IndexedColor:
		if v < 16 {
			return basic16[v]
		}
	case color.RGBA:
		return v
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return def
	}
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xFF}
}

// cellColors: Vorder- und Hintergrund einer Zelle nach Fett/Blass/Invers.
// bgSet sagt, ob der Hintergrund eigens gezeichnet werden muss.
func cellColors(s *uv.Style) (fg, bg color.RGBA, bgSet bool) {
	fg = rgba(s.Fg, defaultFg)
	// Fett mit Grundfarbe 0–7: hellere Variante, wie die meisten Terminals
	if s.Attrs&uv.AttrBold != 0 {
		if b, ok := s.Fg.(ansi.BasicColor); ok && b < 8 {
			fg = basic16[b+8]
		}
	}
	bg = rgba(s.Bg, defaultBg)
	bgSet = s.Bg != nil
	if s.Attrs&uv.AttrReverse != 0 {
		fg, bg = bg, fg
		bgSet = true
	}
	if s.Attrs&uv.AttrFaint != 0 {
		fg = mix(fg, bg, 0.55)
	}
	if s.Attrs&uv.AttrConceal != 0 {
		fg = bg
	}
	return fg, bg, bgSet
}

// mix: a mit Anteil f, Rest b.
func mix(a, b color.RGBA, f float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x)*f + float64(y)*(1-f) + 0.5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xFF}
}
