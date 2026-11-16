package termemu

import (
	"image/color"
	"math"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Lesbarkeit der Serverausgabe
//
// Programme auf dem Server wissen nicht, welches Farbschema sshTermy zeigt.
// Claude Code, bat, delta & Co. schreiben im dunklen Modus z. B. fest
// #f8f8f2 als Schrift – auf einem hellen Theme ist das weiß auf weiß. Das
// äußere Terminal kennt außerdem nur seine eigene ANSI-Palette, die zu
// sshTermys Hintergrund nicht passen muss.
//
// Look löst darum jede Zelle in echte Farben auf (Standardfarbe → Theme,
// ANSI 0–15 → Palette passend zu hell/dunkel, 256er-Farben → xterm-Werte,
// Umkehren und Abgedunkelt ausgerechnet) und hebt die Schrift so weit an,
// dass sie mindestens MinContrast zum Hintergrund hat (wie VS Code
// „minimumContrastRatio“). Rahmen-, Block- und Braille-Zeichen bleiben
// unverändert, sonst würden Bilder aus Halbblöcken und Diagramme verfälscht.

// Look: Farben des Themes für die Terminalfläche.
type Look struct {
	Fg, Bg       color.Color     // Standardschrift und -grund
	ANSI         [16]color.Color // Palette für ANSI 0–15 (nil = xterm-Wert)
	SelFg, SelBg color.Color     // Markierung (nil = Fg/Bg vertauscht)
	MinContrast  float64         // 0 = Schrift nie anheben
	MinFaint     float64         // für Abgedunkeltes (0 = MinContrast)
}

// rgb: Farbe als 8-Bit-Anteile.
type rgb struct{ r, g, b uint8 }

func toRGB(c color.Color) rgb {
	r, g, b, _ := c.RGBA()
	return rgb{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

func (c rgb) color() color.Color { return color.RGBA{c.r, c.g, c.b, 0xff} }

func chan01(v uint8) float64 {
	x := float64(v) / 255
	if x <= 0.03928 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

// lum: relative Helligkeit nach WCAG 2.
func (c rgb) lum() float64 {
	return 0.2126*chan01(c.r) + 0.7152*chan01(c.g) + 0.0722*chan01(c.b)
}

// Contrast: Kontrastverhältnis nach WCAG 2 (1 bis 21).
func Contrast(a, b color.Color) float64 { return contrastRGB(toRGB(a), toRGB(b)) }

func contrastRGB(a, b rgb) float64 {
	la, lb := a.lum()+0.05, b.lum()+0.05
	return math.Max(la, lb) / math.Min(la, lb)
}

func mixRGB(a, b rgb, t float64) rgb {
	m := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return rgb{m(a.r, b.r), m(a.g, b.g), m(a.b, b.b)}
}

// Readable: fg so wenig wie nötig Richtung Weiß oder Schwarz schieben, bis
// es need:1 zu bg erreicht. Der Farbton bleibt erkennbar.
func Readable(fg, bg color.Color, need float64) color.Color {
	return readable(toRGB(fg), toRGB(bg), need).color()
}

func readable(fg, bg rgb, need float64) rgb {
	if need <= 1 || contrastRGB(fg, bg) >= need {
		return fg
	}
	white, black := rgb{255, 255, 255}, rgb{0, 0, 0}
	first, second := black, white
	if bg.lum() < 0.18 { // dunkler Grund: heller machen
		first, second = white, black
	}
	best := fg
	for _, end := range []rgb{first, second} {
		if contrastRGB(end, bg) < need {
			if contrastRGB(end, bg) > contrastRGB(best, bg) {
				best = end // Ziel nicht erreichbar: wenigstens das Beste
			}
			continue
		}
		lo, hi := 0.0, 1.0
		for i := 0; i < 12; i++ {
			mid := (lo + hi) / 2
			if contrastRGB(mixRGB(fg, end, mid), bg) >= need {
				hi = mid
			} else {
				lo = mid
			}
		}
		return mixRGB(fg, end, hi)
	}
	return best
}

// Paletten für ANSI 0–15 (Werte wie im Terminal von VS Code, Dark+/Light+).
var (
	ansiDark = [16]rgb{
		{0x00, 0x00, 0x00}, {0xcd, 0x31, 0x31}, {0x0d, 0xbc, 0x79}, {0xe5, 0xe5, 0x10},
		{0x24, 0x72, 0xc8}, {0xbc, 0x3f, 0xbc}, {0x11, 0xa8, 0xcd}, {0xe5, 0xe5, 0xe5},
		{0x66, 0x66, 0x66}, {0xf1, 0x4c, 0x4c}, {0x23, 0xd1, 0x8b}, {0xf5, 0xf5, 0x43},
		{0x3b, 0x8e, 0xea}, {0xd6, 0x70, 0xd6}, {0x29, 0xb8, 0xdb}, {0xff, 0xff, 0xff},
	}
	ansiLight = [16]rgb{
		{0x00, 0x00, 0x00}, {0xcd, 0x31, 0x31}, {0x00, 0xbc, 0x00}, {0x94, 0x98, 0x00},
		{0x04, 0x51, 0xa5}, {0xbc, 0x05, 0xbc}, {0x05, 0x98, 0xbc}, {0x55, 0x55, 0x55},
		{0x66, 0x66, 0x66}, {0xcd, 0x31, 0x31}, {0x14, 0xce, 0x14}, {0xb5, 0xba, 0x00},
		{0x04, 0x51, 0xa5}, {0xbc, 0x05, 0xbc}, {0x05, 0x98, 0xbc}, {0xa5, 0xa5, 0xa5},
	}
)

// StandardANSI: Palette für ANSI 0–15 passend zu hellem oder dunklem Grund.
func StandardANSI(dark bool) [16]color.Color {
	src := ansiLight
	if dark {
		src = ansiDark
	}
	var out [16]color.Color
	for i, c := range src {
		out[i] = c.color()
	}
	return out
}

// resolve: Zellfarbe als RGB (nil = Standardfarbe def).
func (l *Look) resolve(c color.Color, def rgb) rgb {
	switch v := c.(type) {
	case nil:
		return def
	case ansi.BasicColor:
		if v < 16 && l.ANSI[v] != nil {
			return toRGB(l.ANSI[v])
		}
	case ansi.IndexedColor:
		if v < 16 && l.ANSI[v] != nil {
			return toRGB(l.ANSI[v])
		}
	}
	return toRGB(c)
}

// DecorGlyph: Zeichen, deren Farbe ein Bild ergibt (Rahmen, Blöcke,
// Braille-Diagramme, Powerline-Pfeile) – hier nichts anheben.
func DecorGlyph(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x2500 && r <= 0x259f, // Rahmen + Blöcke
			r >= 0x2800 && r <= 0x28ff,   // Braille
			r >= 0xe0b0 && r <= 0xe0d7,   // Powerline
			r >= 0x1fb00 && r <= 0x1fbff: // Legacy Computing (Sextanten …)
			return true
		}
	}
	return false
}

// lookKey: Zellstil vor dem Umrechnen. Gleiche Stile kommen auf einem
// Bildschirm ständig vor; der Merker spart Rechnen und Speicher je Zelle.
type lookKey struct {
	fg, bg      color.Color
	attrs       uint8
	sel, glyphs bool // markiert; Schrift zählt (kein Leerzeichen/Blockzeichen)
}

type lookVal struct {
	fg, bg color.Color
	attrs  uint8
}

// lookMemo: Merker für einen Bildaufbau. Meist hat die Nachbarzelle den
// gleichen Stil; der letzte Treffer liegt darum vorn.
type lookMemo struct {
	m     map[lookKey]lookVal
	lastK lookKey
	lastV lookVal
	has   bool
}

func newLookMemo() *lookMemo { return &lookMemo{m: make(map[lookKey]lookVal, 32)} }

// apply schreibt die echten Farben in die Zelle. sel: Zelle ist markiert.
func (l *Look) apply(c *uv.Cell, sel bool, memo *lookMemo) {
	glyphs := !(c.Content == "" || c.Content == " " || DecorGlyph(c.Content))
	k := lookKey{c.Style.Fg, c.Style.Bg, c.Style.Attrs, sel, glyphs}
	var v lookVal
	if memo.has && memo.lastK == k {
		v = memo.lastV
	} else {
		var ok bool
		if v, ok = memo.m[k]; !ok {
			v = l.compute(k)
			memo.m[k] = v
		}
		memo.lastK, memo.lastV, memo.has = k, v, true
	}
	c.Style.Fg, c.Style.Bg, c.Style.Attrs = v.fg, v.bg, v.attrs
}

func (l *Look) compute(k lookKey) lookVal {
	defFg, defBg := toRGB(l.Fg), toRGB(l.Bg)
	fg := l.resolve(k.fg, defFg)
	bg := l.resolve(k.bg, defBg)
	a := k.attrs
	if a&uv.AttrReverse != 0 {
		fg, bg = bg, fg
		a &^= uv.AttrReverse
	}
	if k.sel {
		if l.SelBg != nil && l.SelFg != nil {
			fg, bg = toRGB(l.SelFg), toRGB(l.SelBg)
		} else {
			fg, bg = bg, fg
		}
	}
	need := l.MinContrast
	if a&uv.AttrFaint != 0 {
		// Abgedunkelt selbst ausrechnen: Terminals machen das sehr
		// unterschiedlich, manche gar nicht oder bis zur Unlesbarkeit.
		fg = mixRGB(fg, bg, 0.4)
		a &^= uv.AttrFaint
		if l.MinFaint > 0 {
			need = math.Min(need, l.MinFaint)
		}
	}
	// verborgen (Passwort-Echo) bleibt verborgen
	if k.glyphs && a&uv.AttrConceal == 0 {
		fg = readable(fg, bg, need)
	}
	return lookVal{fg.color(), bg.color(), a}
}
