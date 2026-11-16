package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Lesbarkeit in allen Farbschemata
//
// Dominik (2026-11-16): In einem hellen Theme war die Ausgabe von Claude
// Code weiß auf weiß, Markierungen teils unlesbar. Die Tests hier zeichnen
// echte Bilder in jedem Theme, lesen sie mit einem Terminal-Emulator Zelle
// für Zelle zurück und prüfen den Kontrast jeder Schrift zu ihrem Grund.

// colorZoo: Ausgabe, wie Programme sie schreiben, die das Theme nicht
// kennen: feste Truecolor-Schrift (Monokai aus Claude Code, reines Weiß und
// Schwarz), ANSI 0–15 als Schrift und als Grund, 256er-Farben, Umkehren,
// Abgedunkelt, Fett.
func colorZoo() string {
	var b strings.Builder
	b.WriteString("\x1b[1mWrite\x1b[0m(/opt/lib/agentkonto.py)\n")
	for _, hex := range []string{"f8f8f2", "ffffff", "e6db74", "f92672", "75715e", "a6e22e", "000000", "1a1b26", "f5f5f7", "808080"} {
		var r, g, bl int
		_, _ = fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &bl)
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%s \x1b[0m", r, g, bl, hex)
	}
	b.WriteString("\n")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&b, "\x1b[%dmF%02d\x1b[0m ", map[bool]int{true: 30 + i, false: 90 + i - 8}[i < 8], i)
	}
	b.WriteString("\n")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&b, "\x1b[%dmB%02d\x1b[0m ", map[bool]int{true: 40 + i, false: 100 + i - 8}[i < 8], i)
	}
	b.WriteString("\n")
	for i := 0; i < 256; i += 9 {
		fmt.Fprintf(&b, "\x1b[38;5;%dm%03d\x1b[0m", i, i)
	}
	b.WriteString("\n")
	for i := 0; i < 256; i += 11 {
		fmt.Fprintf(&b, "\x1b[48;5;%dm%03d\x1b[0m", i, i)
	}
	b.WriteString("\n\x1b[7mumgekehrt\x1b[0m \x1b[2mabgedunkelt … +164 lines\x1b[0m \x1b[1;37mfett weiß\x1b[0m ")
	b.WriteString("\x1b[7;38;2;248;248;242mumgekehrt hell\x1b[0m \x1b[48;2;255;255;255mauf Weiß\x1b[0m \x1b[48;2;0;0;0mauf Schwarz\x1b[0m\n")
	b.WriteString("\x1b[38;2;248;248;242;48;2;61;1;1m- alte Zeile\x1b[0m \x1b[48;2;2;40;0m+ neue Zeile\x1b[0m\n")
	return b.String()
}

// cell: Schrift und Grund einer Zelle, wie das äußere Terminal sie zeigt.
type cellInk struct {
	x, y   int
	text   string
	fg, bg color.Color
}

// inkCells liest ein gezeichnetes Bild Zelle für Zelle zurück. Leere Zellen
// und Zeichen, deren Farbe ein Bild ergibt (Rahmen, Blöcke), fehlen.
func inkCells(view string, w, h int, t Theme) []cellInk {
	e := vt.NewEmulator(w, h)
	_, _ = e.Write([]byte(strings.ReplaceAll(view, "\n", "\r\n")))
	var out []cellInk
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := e.CellAt(x, y)
			if c == nil || strings.TrimSpace(c.Content) == "" || termemu.DecorGlyph(c.Content) {
				continue
			}
			fg, bg := c.Style.Fg, c.Style.Bg
			if fg == nil {
				fg = t.Text
			}
			if bg == nil {
				bg = t.Bg
			}
			if c.Style.Attrs&uv.AttrReverse != 0 {
				fg, bg = bg, fg
			}
			if c.Style.Attrs&uv.AttrFaint != 0 {
				fg = mixColor(fg, bg, 0.4) // so dunkeln die meisten Terminals ab
			}
			out = append(out, cellInk{x, y, c.Content, fg, bg})
		}
	}
	return out
}

// lowContrast: Zellen unter need, je Farbpaar einmal mit Textprobe.
func lowContrast(cells []cellInk, need float64, in func(x, y int) bool) []string {
	seen := map[string]bool{}
	var bad []string
	for i, c := range cells {
		if in != nil && !in(c.x, c.y) {
			continue
		}
		r := contrast(c.fg, c.bg)
		// Auf mittleren Gründen (z. B. 256er-Farbe als Hintergrund) ist 7:1
		// nicht erreichbar; dann zählt, ob das Bestmögliche gewählt ist.
		best := max(contrast(color.Black, c.bg), contrast(color.White, c.bg))
		if r >= min(need, best-0.05) {
			continue
		}
		key := hexOf(c.fg) + "/" + hexOf(c.bg)
		if seen[key] {
			continue
		}
		seen[key] = true
		sample := ""
		for _, d := range cells[i:] {
			if d.y != c.y || len([]rune(sample)) >= 24 {
				break
			}
			sample += d.text
		}
		bad = append(bad, fmt.Sprintf("Zeile %d: %q %s auf %s = %.2f:1", c.y, sample, hexOf(c.fg), hexOf(c.bg), r))
	}
	return bad
}

// Serverausgabe: jede Schrift erreicht in jedem Theme den Mindestkontrast,
// auch markiert und unter Suchtreffern.
func TestTerminalOutputReadableInAllThemes(t *testing.T) {
	zoo := strings.ReplaceAll(colorZoo(), "\n", "\r\n")
	for _, th := range Themes {
		term := termemu.New(120, 14, discard{})
		_, _ = term.Write([]byte(zoo))
		need := minTermContrast(th)
		hl := &termemu.HL{Look: lookOf(th)}
		sel := &termemu.Sel{A: termemu.Pos{Line: 1, Col: 0}, B: termemu.Pos{Line: 4, Col: 30}}
		marks := []termemu.Mark{{Line: 2, Col: 0, Len: 20}, {Line: 6, Col: 0, Len: 12, Current: true}}
		ms := termemu.MarkStyle{Hit: uv.Style{Fg: th.Bg, Bg: th.Warn}, Current: uv.Style{Fg: th.Bg, Bg: th.Danger}}
		for name, view := range map[string]string{
			"normal":   term.RenderHL(0, nil, hl),
			"markiert": term.RenderHL(0, sel, hl),
			"Treffer":  term.RenderMarked(0, nil, marks, ms, hl),
			"Leerlauf": term.ViewHL("", func(s string) string { return s }, hl),
		} {
			cells := inkCells(view, 120, 14, th)
			if len(cells) < 400 {
				t.Fatalf("%s/%s: nur %d Zellen gelesen", th.Name, name, len(cells))
			}
			// Abgedunkeltes darf leiser sein (3,5:1), alles andere need.
			var bad []string
			for _, b := range lowContrast(cells, need, nil) {
				if !strings.Contains(b, "abgedunkelt") && !strings.Contains(b, "+164") {
					bad = append(bad, b)
				}
			}
			bad = append(bad, lowContrast(cells, minMutedFor(th), nil)...)
			if len(bad) > 0 {
				t.Errorf("%s/%s: schlecht lesbar:\n  %s", th.Name, name, strings.Join(bad, "\n  "))
			}
		}
		term.Close()
	}
}

// Markierung im Terminal: Theme-Farben statt Umkehren. Umkehren machte aus
// heller Programmschrift auf hellem Theme eine helle Fläche mit heller Schrift.
func TestTerminalSelectionUsesThemeColors(t *testing.T) {
	th := Hell
	term := termemu.New(40, 3, discard{})
	defer term.Close()
	_, _ = term.Write([]byte("\x1b[38;2;248;248;242mweiss\x1b[0m normal"))
	view := term.RenderHL(0, &termemu.Sel{A: termemu.Pos{Line: 0, Col: 0}, B: termemu.Pos{Line: 0, Col: 11}}, &termemu.HL{Look: lookOf(th)})
	for _, c := range inkCells(view, 40, 3, th) {
		if c.x > 11 {
			continue
		}
		if hexOf(c.bg) != hexOf(th.Accent) {
			t.Fatalf("Zelle %d %q: Grund %s, erwartet Akzent %s", c.x, c.text, hexOf(c.bg), hexOf(th.Accent))
		}
		if r := contrast(c.fg, c.bg); r < minText {
			t.Fatalf("Zelle %d %q: Kontrast %.2f", c.x, c.text, r)
		}
	}
}

// readableCases: zusätzlich zu den Golden-Bildern Terminal mit fremden
// Farben, Markierung und Suche.
var readableCases = []goldenCase{
	{name: "terminal-farben", w: 120, h: 30, setup: func(g *gold) {
		s := g.session("web-01", colorZoo())
		s.sel = &termemu.Sel{A: termemu.Pos{Line: 1, Col: 0}, B: termemu.Pos{Line: 3, Col: 40}}
	}},
	{name: "terminal-farben-suche", w: 120, h: 30, setup: func(g *gold) {
		g.session("web-01", colorZoo())
		g.keys("ctrl+f").typ("lines")
	}},
}

// Jeder Bildschirm in jedem Theme: Schrift mindestens 3,5:1 (die Regel für
// Gedämpftes), Hochkontrast-Themes 4,5:1. Unter einem offenen Fenster wird
// nur das Fenster geprüft; der Rest ist absichtlich abgedunkelt.
func TestScreensReadableInAllThemes(t *testing.T) {
	cases := append(append([]goldenCase{}, goldenCases...), readableCases...)
	for _, c := range cases {
		if c.empty {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			g := newGold(t, i18n.DE, c.w, c.h, true)
			c.setup(g)
			for i, th := range Themes {
				if g.a.scr == scrThemes {
					// Farbschema-Auswahl: dort zeigt die Vorschau das Theme
					g.a.themePick.sel = i
					g.a.previewTheme()
				} else {
					_ = g.a.st.UpdateSettings(func(s *domain.Settings) { s.Theme = th.Name })
					g.a.applyLook()
				}
				th = g.a.s.T // Sperrbildschirm: immer das Standard-Theme
				g.a.fx.ghost = ""
				view := g.a.View().Content
				in := func(x, y int) bool { return true }
				if gh := g.a.fx.ghost; gh != "" {
					mx, my, mw, mh := g.a.fx.x, g.a.fx.y, lipgloss.Width(gh), lipgloss.Height(gh)
					in = func(x, y int) bool { return x >= mx && x < mx+mw && y >= my && y < my+mh }
				}
				need := minMutedFor(th)
				if bad := lowContrast(inkCells(view, c.w, c.h, th), need, in); len(bad) > 0 {
					t.Errorf("%s: schlecht lesbar:\n  %s", th.Name, strings.Join(bad, "\n  "))
				}
			}
		})
	}
}
