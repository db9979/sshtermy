package ui

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// Startanimation „Entschlüsseln“: Zufallszeichen rasten einzeln in das
// große Logo ein, danach erscheinen Slogan und Credit. Unter 2 Sekunden,
// jede Taste und jeder Klick überspringt sie.
//
// Abschalten: Palette „Startanimation aus“, Startparameter --no-intro oder
// SSHTERMY_NO_INTRO=1. Die Einstellung muss vor dem Entsperren lesbar sein,
// darum liegt sie als leere Datei „no-intro“ im Konfigurationsordner und
// nicht im verschlüsselten Store.
//
// Nur Zeichen, die das Windows Terminal (Cascadia) sicher darstellt:
// Halbblöcke, ░▒▓, ASCII, Braille-Spinner und ✓.

const (
	introDur     = 1.9 // Sekunden bis zum Entsperren-Bildschirm
	introLockEnd = 1.15
	introFPS     = 30
	noIntroFile  = "no-intro"
)

type introState struct {
	start time.Time
	// unlocked: Store per DPAPI schon offen, dann darf „entschlüsselt“ dastehen.
	unlocked bool
}

type introFrameMsg time.Time

func introFrame() tea.Cmd {
	return tea.Tick(time.Second/introFPS, func(t time.Time) tea.Msg { return introFrameMsg(t) })
}

// IntroEnabled: Startanimation an? (Datei no-intro und SSHTERMY_NO_INTRO schalten sie ab)
func IntroEnabled(dir string) bool {
	if v := os.Getenv("SSHTERMY_NO_INTRO"); v != "" && v != "0" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, noIntroFile))
	return err != nil
}

// SkipIntro: für --no-intro.
func (a *App) SkipIntro() { a.intro = nil }

func (a *App) introOn() bool { return IntroEnabled(a.st.Dir) }

func (a *App) toggleIntro() tea.Cmd {
	p := filepath.Join(a.st.Dir, noIntroFile)
	var err error
	if a.introOn() {
		err = os.WriteFile(p, nil, 0o600)
	} else {
		err = os.Remove(p)
	}
	if err != nil {
		a.flash(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	if a.introOn() {
		a.flash(i18n.T("Startanimation an"))
	} else {
		a.flash(i18n.T("Startanimation aus"))
	}
	return nil
}

func introLabel(on bool) string {
	if on {
		return i18n.T("Startanimation aus")
	}
	return i18n.T("Startanimation an")
}

// introMsg: läuft vor allen anderen Nachrichten. ok=false: normal weiter.
func (a *App) introMsg(msg tea.Msg) (tea.Cmd, bool) {
	if a.intro == nil {
		return nil, false
	}
	switch m := msg.(type) {
	case introFrameMsg:
		if time.Time(m).Sub(a.intro.start).Seconds() >= introDur {
			a.intro = nil
			return nil, true
		}
		return introFrame(), true
	case tea.KeyPressMsg, tea.MouseClickMsg:
		a.intro = nil // überspringen; die Taste selbst wirkt nicht weiter
		return nil, true
	case unlockedMsg:
		if m.err == nil {
			a.intro.unlocked = true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------- Logo

// Pixel-Bitmaps: zwei Pixelzeilen ergeben eine Zeichenzeile (▀ ▄ █).
var introGlyphs = map[rune][]string{
	's': {"", "", "", "", ".####.", "##....", ".####.", "....##", "....##", "#####."},
	'h': {"##....", "##....", "##....", "##....", "#####.", "##..##", "##..##", "##..##", "##..##", "##..##"},
	'T': {"########", "########", "...##...", "...##...", "...##...", "...##...", "...##...", "...##...", "...##...", "...##..."},
	'e': {"", "", "", "", ".####.", "##..##", "######", "##....", "##....", ".#####"},
	'r': {"", "", "", "", "##.###", "###...", "##....", "##....", "##....", "##...."},
	'm': {"", "", "", "", "#######.", "##.##.##", "##.##.##", "##.##.##", "##.##.##", "##.##.##"},
	'y': {"", "", "", "", "##..##", "##..##", "##..##", "##..##", ".#####", "....##", "....##", "#####."},
}

type introCell struct {
	x, y int
	ch   rune
}

// introLogo: Zellen des Logos „sshTermy“, Breite und Höhe in Zeichen.
func introLogo() ([]introCell, int, int) {
	const word, ph = "sshTermy", 12
	var px [ph][]bool
	for i, r := range word {
		gl := introGlyphs[r]
		w := 0
		for _, row := range gl {
			w = max(w, len(row))
		}
		for y := 0; y < ph; y++ {
			row := ""
			if y < len(gl) {
				row = gl[y]
			}
			for x := 0; x < w; x++ {
				px[y] = append(px[y], x < len(row) && row[x] == '#')
			}
			if i < len(word)-1 {
				px[y] = append(px[y], false, false)
			}
		}
	}
	W := len(px[0])
	var out []introCell
	for cy := 0; cy < ph/2; cy++ {
		for x := 0; x < W; x++ {
			t, b := px[cy*2][x], px[cy*2+1][x]
			switch {
			case t && b:
				out = append(out, introCell{x, cy, '█'})
			case t:
				out = append(out, introCell{x, cy, '▀'})
			case b:
				out = append(out, introCell{x, cy, '▄'})
			}
		}
	}
	return out, W, ph / 2
}

// ---------------------------------------------------------------- Zeichnen

type rgb struct{ r, g, b float64 }

func toRGB(c color.Color) rgb {
	r, g, b, _ := c.RGBA()
	return rgb{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
}

func clamp01(t float64) float64 { return math.Max(0, math.Min(1, t)) }

func (c rgb) mix(o rgb, t float64) rgb {
	t = clamp01(t)
	return rgb{c.r + (o.r-c.r)*t, c.g + (o.g-c.g)*t, c.b + (o.b-c.b)*t}
}

type introGrid struct {
	w, h int
	ch   [][]rune
	fg   [][]rgb
	bold [][]bool
}

func newIntroGrid(w, h int, fg rgb) *introGrid {
	g := &introGrid{w: w, h: h}
	for y := 0; y < h; y++ {
		g.ch = append(g.ch, []rune(strings.Repeat(" ", w)))
		row := make([]rgb, w)
		for x := range row {
			row[x] = fg
		}
		g.fg = append(g.fg, row)
		g.bold = append(g.bold, make([]bool, w))
	}
	return g
}

func (g *introGrid) set(x, y int, r rune, fg rgb, bold bool) {
	if x >= 0 && y >= 0 && x < g.w && y < g.h {
		g.ch[y][x], g.fg[y][x], g.bold[y][x] = r, fg, bold
	}
}

func (g *introGrid) center(y int, s string, fg rgb) {
	r := []rune(s)
	x := (g.w - len(r)) / 2
	for i, c := range r {
		g.set(x+i, y, c, fg, false)
	}
}

// String: ANSI mit 24-Bit-Farben; bubbletea rechnet sie für ältere
// Konsolen (256/16 Farben) selbst herunter.
func (g *introGrid) String() string {
	var b strings.Builder
	for y := 0; y < g.h; y++ {
		var last rgb
		lastBold, first := false, true
		for x := 0; x < g.w; x++ {
			c, bold := g.fg[y][x], g.bold[y][x]
			if g.ch[y][x] != ' ' && (first || c != last || bold != lastBold) {
				w := 22
				if bold {
					w = 1
				}
				fmt.Fprintf(&b, "\x1b[%d;38;2;%d;%d;%dm", w, int(c.r), int(c.g), int(c.b))
				last, lastBold, first = c, bold, false
			}
			b.WriteRune(g.ch[y][x])
		}
		b.WriteString("\x1b[0m")
		if y < g.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func typed(s string, t, dur float64) string {
	r := []rune(s)
	return string(r[:int(clamp01(t/dur)*float64(len(r)))])
}

// renderIntro: Bild zum Zeitpunkt t (Sekunden seit Start).
func renderIntro(th Theme, t float64, w, h int, unlocked bool) string {
	bg, text, muted, border := toRGB(th.Bg), toRGB(th.Text), toRGB(th.Muted), toRGB(th.Border)
	accent, ai, ok := toRGB(th.Accent), toRGB(th.AI), toRGB(th.OK)
	flashC := rgb{255, 255, 255}
	if !th.Dark {
		flashC = text
	}
	g := newIntroGrid(w, h, text)
	cells, W, H := introLogo()
	ox, oy := (w-W)/2, h/2-H-1
	grad := func(x int) rgb { return accent.mix(ai, float64(x)/float64(W-1)) }

	noise := []rune(`01░▒▓<>/\{}#$%&*+=`)
	rnd := rand.New(rand.NewSource(int64(t*introFPS) + 7))
	for i, c := range cells {
		lock := 0.25 + float64((i*7919)%997)/997*(introLockEnd-0.3) // eigene Einrastzeit je Zelle
		switch {
		case t < 0.08:
		case t < lock:
			g.set(ox+c.x, oy+c.y, noise[rnd.Intn(len(noise))], border.mix(ok, 0.5+0.5*rnd.Float64()), false)
		case t < lock+0.12:
			g.set(ox+c.x, oy+c.y, c.ch, flashC, true)
		default:
			g.set(ox+c.x, oy+c.y, c.ch, grad(c.x), true)
		}
	}

	ly := oy + H + 2
	if t < introLockEnd {
		spin := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")[int(t*introFPS/2)%10]
		g.center(ly, string(spin)+i18n.T(" entschlüssele …"), ai)
	} else {
		done := i18n.T("✓ bereit")
		if unlocked {
			done = i18n.T("✓ entschlüsselt")
		}
		g.center(ly, done, bg.mix(ok, (t-introLockEnd)/0.2))
	}
	g.center(ly+1, typed(i18n.T(version.Slogan)+" · v"+version.Version, t-introLockEnd, 0.35), text)
	g.center(ly+2, i18n.T(version.Idea), bg.mix(muted, (t-introLockEnd-0.25)/0.3))
	g.center(h-1, i18n.T("beliebige Taste überspringt"), bg.mix(border, t/0.3))
	return g.String()
}

func (a *App) viewIntro() string {
	t := time.Since(a.intro.start).Seconds()
	return renderIntro(a.s.T, t, a.w, a.h, a.intro.unlocked)
}

// introFits: Logo braucht etwa 70×14 Zeichen; kleinere Fenster sehen
// gleich den Entsperren-Bildschirm.
func introFits(w, h int) bool {
	_, W, H := introLogo()
	return w >= W+4 && h >= H+8
}
