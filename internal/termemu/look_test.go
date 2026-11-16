package termemu

import (
	"image/color"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

var lightLook = &Look{
	Fg: color.RGBA{0x1f, 0x23, 0x35, 0xff}, Bg: color.RGBA{0xf5, 0xf5, 0xf7, 0xff},
	ANSI: StandardANSI(false), SelFg: color.RGBA{0xf5, 0xf5, 0xf7, 0xff}, SelBg: color.RGBA{0x2e, 0x59, 0xc9, 0xff},
	MinContrast: 4.5, MinFaint: 3.5,
}

func applied(c uv.Cell, sel bool) uv.Cell {
	lightLook.apply(&c, sel, newLookMemo())
	return c
}

// Claude Code im dunklen Modus auf hellem Theme: #f8f8f2 wird lesbar.
func TestLookLiftsLightTextOnLightTheme(t *testing.T) {
	c := applied(uv.Cell{Content: "x", Width: 1, Style: uv.Style{Fg: color.RGBA{0xf8, 0xf8, 0xf2, 0xff}}}, false)
	if r := Contrast(c.Style.Fg, c.Style.Bg); r < 4.5 {
		t.Fatalf("Kontrast %.2f", r)
	}
	if toRGB(c.Style.Bg) != toRGB(lightLook.Bg) {
		t.Fatalf("Grund %v", c.Style.Bg)
	}
}

// Gut lesbare Farben bleiben genau so.
func TestLookKeepsGoodColors(t *testing.T) {
	red := color.RGBA{0xc4, 0x12, 0x2f, 0xff}
	c := applied(uv.Cell{Content: "x", Width: 1, Style: uv.Style{Fg: red}}, false)
	if toRGB(c.Style.Fg) != toRGB(red) {
		t.Fatalf("geändert: %v", c.Style.Fg)
	}
}

// Umkehren, ANSI-Palette, Verborgen, Blockzeichen.
func TestLookAttributes(t *testing.T) {
	c := applied(uv.Cell{Content: "x", Width: 1, Style: uv.Style{Attrs: uv.AttrReverse}}, false)
	if c.Style.Attrs&uv.AttrReverse != 0 || toRGB(c.Style.Bg) != toRGB(lightLook.Fg) {
		t.Fatalf("Umkehren nicht ausgerechnet: %+v", c.Style)
	}
	c = applied(uv.Cell{Content: "x", Width: 1, Style: uv.Style{Fg: ansi.BasicColor(7)}}, false)
	if r := Contrast(c.Style.Fg, c.Style.Bg); r < 4.5 {
		t.Fatalf("ANSI weiß: %.2f", r)
	}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	c = applied(uv.Cell{Content: "█", Width: 1, Style: uv.Style{Fg: white}}, false)
	if toRGB(c.Style.Fg) != toRGB(white) {
		t.Fatal("Blockzeichen (Bilder) dürfen nicht umgefärbt werden")
	}
	c = applied(uv.Cell{Content: "x", Width: 1, Style: uv.Style{Fg: white, Attrs: uv.AttrConceal}}, false)
	if toRGB(c.Style.Fg) != toRGB(white) {
		t.Fatal("Verborgenes bleibt verborgen")
	}
	c = applied(uv.Cell{Content: "x", Width: 1, Style: uv.Style{Attrs: uv.AttrFaint}}, false)
	if c.Style.Attrs&uv.AttrFaint != 0 || Contrast(c.Style.Fg, c.Style.Bg) < 3.5 {
		t.Fatalf("Abgedunkelt: %+v", c.Style)
	}
}

// Markierung: Theme-Farben, auch über umgekehrtem Text.
func TestLookSelection(t *testing.T) {
	for _, st := range []uv.Style{{}, {Attrs: uv.AttrReverse}, {Fg: color.RGBA{0xf8, 0xf8, 0xf2, 0xff}}} {
		c := applied(uv.Cell{Content: "x", Width: 1, Style: st}, true)
		if toRGB(c.Style.Bg) != toRGB(lightLook.SelBg) || Contrast(c.Style.Fg, c.Style.Bg) < 4.5 {
			t.Fatalf("%+v → %+v", st, c.Style)
		}
	}
}
