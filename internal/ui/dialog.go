package ui

import (
	"image/color"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Einheitliche Dialoge
//
// Jedes Popup (Bestätigung, Hilfe, Palette, Auswahl, Menü …) läuft durch
// dialog.render: gleicher Rahmen, Titel oben im Rahmen, gleiche Abstände
// und unten eine Zeile mit anklickbaren Tastenhinweisen im Stil der
// Fußzeile. Vorher hatte fast jedes Popup eigene Ränder, Titel im Text und
// Hinweise doppelt (als Text im Dialog und als Knöpfe darunter).

type dialog struct {
	icon, title string
	sub         string      // grau hinter dem Titel (Host, Version, Zähler)
	tone        color.Color // Rahmen und Titel; nil = Akzentfarbe
	body        string
	keys        [][2]string // Taste, Beschreibung; Taste "" = nur Hinweistext
	info        string      // grauer Text vor den Tasten (z. B. „Zeile 1–20 von 80“)
	width       int         // feste Innenbreite; 0 = nach Inhalt
	maxW        int         // größte Außenbreite; 0 = 90
	compact     bool        // Menüs und Listen: kein Leerraum oben und unten
	noClose     bool        // kein × oben rechts
}

const dialogMaxW = 90

// dialogPad: Innenabstand links/rechts und oben/unten.
func (d dialog) pad() (int, int) {
	if d.compact {
		return 1, 0
	}
	return 2, 1
}

// innerMax: größte Innenbreite bei Fensterbreite w.
func (d dialog) innerMax(w int) int {
	px, _ := d.pad()
	mw := d.maxW
	if mw <= 0 {
		mw = dialogMaxW
	}
	return max(8, min(mw, w-2)-2-2*px)
}

// dialogInner: Innenbreite, die ein Dialog bei Fensterbreite w höchstens hat
// (für Inhalte, die vorab umbrechen müssen).
func dialogInner(w int) int { return dialog{}.innerMax(w) }

// btn: Taste + Beschreibung als Knopf (klickbar, wenn die App das Zonen-
// Werkzeug gesetzt hat). Taste "" = grauer Hinweis.
func (s Styles) btn(key, desc string) string {
	if key == "" {
		return s.Muted.Render(desc)
	}
	key = i18n.Keys(key)
	out := s.Key.Render(key)
	if desc != "" {
		out += s.KeyDesc.Render(desc)
	}
	if s.zoneKey != nil {
		return s.zoneKey(key, out)
	}
	return out
}

// keyRows bricht die Tastenhinweise auf Zeilen der Breite w um.
func (s Styles) keyRows(info string, keys [][2]string, w int) []string {
	var parts []string
	if info != "" {
		parts = append(parts, s.Muted.Render(info))
	}
	for _, k := range keys {
		parts = append(parts, s.btn(k[0], k[1]))
	}
	var rows []string
	cur, cw := "", 0
	for _, p := range parts {
		pw := lipgloss.Width(p)
		if cw > 0 && cw+2+pw > w {
			rows = append(rows, cur)
			cur, cw = "", 0
		}
		if cw > 0 {
			cur += "  "
			cw += 2
		}
		if pw > w {
			p, pw = ansi.Truncate(p, w, "…"), w
		}
		cur += p
		cw += pw
	}
	if cw > 0 {
		rows = append(rows, cur)
	}
	return rows
}

// render zeichnet den Dialog für ein Fenster der Breite w.
func (s Styles) dialog(d dialog, w int) string {
	tone := d.tone
	if tone == nil {
		tone = s.T.Accent
	}
	bc := lipgloss.NewStyle().Foreground(tone)
	px, py := d.pad()
	maxIn := d.innerMax(w)

	body := strings.TrimRight(d.body, "\n")
	var lines []string
	if body != "" || !d.compact {
		lines = strings.Split(body, "\n")
	}
	title := d.title
	if d.icon != "" {
		title = d.icon + " " + title
	}
	titleR := bc.Bold(true).Render(title)
	if d.sub != "" {
		titleR += s.Muted.Render("  " + d.sub)
	}
	closeW := 0
	if !d.noClose {
		closeW = 4 // " × ─"
	}

	inner := d.width
	if inner <= 0 {
		for _, l := range lines {
			inner = max(inner, lipgloss.Width(l))
		}
		inner = max(inner, lipgloss.Width(titleR)+3+closeW-2*px)
		kw := 0
		for _, r := range s.keyRows(d.info, d.keys, 1<<20) {
			kw = max(kw, lipgloss.Width(r))
		}
		inner = max(inner, kw)
	}
	inner = max(8, min(inner, maxIn))
	full := inner + 2*px // Breite zwischen den Rahmenstrichen

	var out strings.Builder
	// Kopf: ┌─ ◆ Titel  sub ─────────── × ─┐
	room := full - 3 - closeW
	if lipgloss.Width(titleR) > room {
		titleR = ansi.Truncate(titleR, max(1, room), "…")
	}
	fill := max(0, full-3-lipgloss.Width(titleR)-closeW)
	out.WriteString(bc.Render("┌─ ") + titleR + bc.Render(" "+strings.Repeat("─", fill)))
	if !d.noClose {
		x := s.Muted.Render("×")
		if s.zoneKey != nil {
			x = s.zoneKey("Esc", x)
		}
		out.WriteString(" " + x + bc.Render(" ─"))
	}
	out.WriteString(bc.Render("┐") + "\n")

	side := bc.Render("│")
	padL, padR := strings.Repeat(" ", px), strings.Repeat(" ", px)
	row := func(l string) {
		lw := lipgloss.Width(l)
		if lw > inner {
			l = ansi.Truncate(l, inner, "…")
			lw = lipgloss.Width(l)
		}
		out.WriteString(side + padL + l + strings.Repeat(" ", max(0, inner-lw)) + padR + side + "\n")
	}
	for i := 0; i < py; i++ {
		row("")
	}
	for _, l := range lines {
		row(l)
	}
	if kr := s.keyRows(d.info, d.keys, inner); len(kr) > 0 {
		if len(lines) > 0 {
			row("")
		}
		for _, l := range kr {
			row(l)
		}
	}
	for i := 0; i < py; i++ {
		row("")
	}
	out.WriteString(bc.Render("└" + strings.Repeat("─", full) + "┘"))
	return out.String()
}

// dialogChrome: Zeilen, die Rahmen, Abstand und Tastenzeile eines normalen
// Dialogs brauchen (für Dialoge, die ihre Höhe selbst begrenzen).
const dialogChrome = 2 + 2 + 2 // Rahmen, Abstand oben/unten, Leerzeile + Tasten

// ---------------------------------------------------------------- Ein- und Ausblenden

// Terminals kennen keine Transparenz. Ein Popup blendet trotzdem sanft ein,
// indem seine Farben in wenigen Bildern vom Hintergrund zur vollen Farbe
// laufen; beim Schließen läuft es kurz rückwärts. Die Tasten gehen dabei
// sofort an das, was als Nächstes dran ist: die Animation hält nichts auf.

const (
	fadeInDur  = 140 * time.Millisecond
	fadeOutDur = 110 * time.Millisecond
	fadeFrame  = 30 * time.Millisecond
)

type fadeMsg struct{}

type fadeFx struct {
	id    overlayKey
	start time.Time
	out   bool
	ghost string // letztes Bild des Popups (ohne Zonen) fürs Ausblenden
	x, y  int
}

// overlayKey: welches Popup gerade offen ist. Dialoge zählen nach Art, damit
// ein neu aufgebauter Dialog gleicher Art (z. B. nach Sprachwechsel) nicht
// erneut einblendet.
type overlayKey struct {
	layer int // 0 = nichts, 1 = Modal, 2 = Auswahl, 3 = Palette
	kind  modalKind
	p     any
}

func animEnabled() bool {
	v := os.Getenv("SSHTERMY_NO_ANIM")
	return (v == "" || v == "0") && !accessible
}

func fadeTick() tea.Cmd {
	return tea.Tick(fadeFrame, func(time.Time) tea.Msg { return fadeMsg{} })
}

func (a *App) overlayID() overlayKey {
	switch {
	case a.modal != nil:
		return overlayKey{layer: 1, kind: a.modal.kind}
	case a.pick != nil:
		return overlayKey{layer: 2, p: a.pick}
	case a.pal != nil:
		return overlayKey{layer: 3, p: a.pal}
	}
	return overlayKey{}
}

// trackFade startet die Animation, wenn ein Popup auf- oder zugeht.
func (a *App) trackFade() tea.Cmd {
	id := a.overlayID()
	if id == a.fx.id {
		return nil
	}
	prev := a.fx.id
	a.fx.id = id
	if !animEnabled() || a.intro != nil {
		a.fx.start = time.Time{}
		return nil
	}
	now := time.Now()
	switch {
	case id.layer != 0 && prev.layer != 0:
		// Wechsel von Popup zu Popup: nur kurz, sonst flackert es
		a.fx.start, a.fx.out = now.Add(-fadeInDur/2), false
	case id.layer != 0:
		a.fx.start, a.fx.out = now, false
	case a.fx.ghost != "":
		a.fx.start, a.fx.out = now, true
	default:
		return nil
	}
	return fadeTick()
}

// fadeLevel: Sichtbarkeit 0…1 des Popups und ob gerade animiert wird.
func (a *App) fadeLevel() (float64, bool) {
	if a.fx.start.IsZero() {
		return 1, false
	}
	d := fadeInDur
	if a.fx.out {
		d = fadeOutDur
	}
	t := float64(time.Since(a.fx.start)) / float64(d)
	if t >= 1 {
		if a.fx.out {
			return 0, false
		}
		return 1, false
	}
	t = max(0, t)
	t = 1 - (1-t)*(1-t) // weich auslaufen
	if a.fx.out {
		return 1 - t, true
	}
	return t, true
}

func (a *App) fadeRunning() bool {
	_, run := a.fadeLevel()
	if !run {
		if a.fx.out {
			a.fx.ghost = ""
		}
		a.fx.start = time.Time{}
	}
	return run
}

var (
	sgrRe  = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)
	zoneRe = regexp.MustCompile(`\x1b\[\d+z`)
)

func rgbOf(c color.Color) [3]int {
	r, g, b, _ := c.RGBA()
	return [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
}

func mix(to, from [3]int, k float64) [3]int {
	var o [3]int
	for i := range o {
		o[i] = from[i] + int(float64(to[i]-from[i])*k+0.5)
	}
	return o
}

func sgrRGB(kind int, c [3]int) string {
	return strconv.Itoa(kind) + ";2;" + strconv.Itoa(c[0]) + ";" + strconv.Itoa(c[1]) + ";" + strconv.Itoa(c[2])
}

// fadeANSI mischt alle Farben in s mit dem Hintergrund des Themes:
// k = 1 zeigt s unverändert, k = 0 nur Hintergrund. Text ohne eigene Farbe
// bekommt die Textfarbe des Themes, damit er mit ausblendet.
func fadeANSI(s string, t Theme, k float64) string {
	if k >= 1 {
		return s
	}
	bg, fg := rgbOf(t.Bg), rgbOf(t.Text)
	defFG := "\x1b[" + sgrRGB(38, mix(fg, bg, k)) + "m"
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = sgrRe.ReplaceAllStringFunc(l, func(seq string) string {
			ps := strings.FieldsFunc(seq[2:len(seq)-1], func(r rune) bool { return r == ';' || r == ':' })
			if len(ps) == 0 {
				ps = []string{"0"}
			}
			var out []string
			resetFG := false
			for j := 0; j < len(ps); j++ {
				n, _ := strconv.Atoi(ps[j])
				switch {
				case (n == 38 || n == 48) && j+4 < len(ps) && ps[j+1] == "2":
					var c [3]int
					for x := 0; x < 3; x++ {
						c[x], _ = strconv.Atoi(ps[j+2+x])
					}
					out = append(out, sgrRGB(n, mix(c, bg, k)))
					if n == 38 {
						resetFG = false
					}
					j += 4
				case (n == 38 || n == 48) && j+2 < len(ps) && ps[j+1] == "5":
					out = append(out, ps[j], ps[j+1], ps[j+2])
					if n == 38 {
						resetFG = false
					}
					j += 2
				case n == 0 || n == 39:
					out = append(out, ps[j])
					resetFG = true
				default:
					out = append(out, ps[j])
					if (n >= 30 && n <= 37) || (n >= 90 && n <= 97) {
						resetFG = false
					}
				}
			}
			r := "\x1b[" + strings.Join(out, ";") + "m"
			if resetFG {
				r += defFG
			}
			return r
		})
		lines[i] = defFG + l
	}
	return strings.Join(lines, "\n")
}
