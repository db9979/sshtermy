package ui

// Bewegung (Settings.Motion)
//
// Terminals kennen keine Animationen, sshTermy zeichnet sie Bild für Bild:
// ein Spinner im Tab beim Verbinden, ein kurzes Aufleuchten des Tabs bei
// Erfolg oder Fehler, ein pulsierender Punkt, wenn ein Tab im Hintergrund
// neue Ausgabe hat, „atmende“ Online-Punkte und Balken, die sich füllen.
// Fährt die Maus über einen Tab, erscheint nach kurzem Verweilen eine
// kleine Karte mit Ziel, Dauer und Latenz.
//
// Drei Stufen: voll (Standard), dezent (nur kurze Übergänge, kein
// Dauer-Pulsieren, keine Hover-Karten) und aus. Barrierearm und
// SSHTERMY_NO_ANIM=1 schalten alles ab. Der Takt läuft nur, solange
// wirklich etwas in Bewegung ist; ohne Bewegung zeichnet nichts extra.

import (
	"image/color"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
)

const (
	motionVoll   = ""
	motionDezent = "dezent"
	motionAus    = "aus"
)

var motionValues = []string{motionVoll, motionDezent, motionAus}

// motionSetting: aktueller Stand (applyLook), für Stellen ohne App.
var motionSetting string

// motionLevel: 0 = aus, 1 = dezent, 2 = voll.
func motionLevel() int {
	if !animEnabled() {
		return 0
	}
	if motionSetting == motionDezent {
		return 1
	}
	return 2
}

func motionLabel(v string) string {
	switch v {
	case motionDezent:
		return i18n.T("dezent")
	case motionAus:
		return i18n.T("aus")
	}
	return i18n.T("voll")
}

func (a *App) cycleMotion(dir int) tea.Cmd {
	cur := indexOf(motionValues, a.st.Settings().Motion)
	next := motionValues[cycleIndex(cur, len(motionValues), dir)]
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Motion = next }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	motionSetting = next
	a.hov = hoverState{}
	if accessible && next != motionAus {
		a.flash(i18n.T("Barrierearm ist an: Bewegung bleibt aus, bis du es abschaltest."))
	}
	return nil
}

// ---------------------------------------------------------------- Takt

const (
	animFast    = 100 * time.Millisecond // Spinner, Aufleuchten, Balken
	animSlow    = 250 * time.Millisecond // Pulsieren, Atmen
	tabFlashDur = 1200 * time.Millisecond
	barFillDur  = 450 * time.Millisecond
	selFadeDur  = 160 * time.Millisecond
	hoverDelay  = 450 * time.Millisecond
	downBlink   = 4 * time.Second // Manager: Punkt blinkt, nachdem ein Host ausfiel
)

type animMsg struct{}

// animTick: nächstes Bild anfordern, falls etwas in Bewegung ist.
func (a *App) animTick() tea.Cmd {
	if a.animTicking {
		return nil
	}
	d := a.animNeed(time.Now())
	if d == 0 {
		return nil
	}
	a.animTicking = true
	return tea.Tick(d, func(time.Time) tea.Msg { return animMsg{} })
}

// animNeed: wie schnell neu gezeichnet werden muss (0 = gar nicht).
func (a *App) animNeed(now time.Time) time.Duration {
	if m := a.modal; m != nil && m.kind == mDanger && m.danger != nil && m.danger.hold.active(now) {
		return animFast // Halten zum Bestätigen läuft auch ohne Bewegung
	}
	lvl := motionLevel()
	if lvl == 0 || a.lock != nil || a.intro != nil || a.blurred {
		return 0
	}
	slow := false
	if m := a.modal; m != nil && m.kind == mDanger && lvl == 2 {
		slow = true // Rahmen pulsiert
	}
	if a.pal != nil && a.pal.animating(now) {
		return animFast
	}
	if a.scr == scrBroadcast && a.bc != nil && a.bc.running && a.bc.phase == bcRun {
		return animFast // Kacheln: Spinner und pulsierender Rahmen
	}
	if a.scr == scrTerminal && a.gliding(now) {
		return fadeFrame // Rahmen gleitet: ein paar flüssige Bilder
	}
	if a.scr == scrTerminal {
		for _, s := range a.sessions {
			if s.state == stConnecting || s.state == stReconnect || now.Sub(s.flashAt) < tabFlashDur ||
				now.Sub(s.revealAt) < revealDur || now.Sub(s.panelAt) < fadeInDur*2 {
				return animFast
			}
			if s.panel == panelAsk && (s.ask.busy || s.ask.typing()) {
				return animFast
			}
			if lvl == 2 && s.unseen {
				slow = true
			}
			if s.panel == panelGoal && s.goal.waiting {
				slow = true // Zeitleiste: laufender Schritt (timeline.go)
			}
		}
	}
	if a.hov.tip != nil && now.Sub(a.hov.since) < hoverDelay+animFast {
		return animFast
	}
	if a.scr == scrManager && a.modal == nil && a.pal == nil && a.pick == nil {
		if now.Sub(a.mgr.selAt) < barFillDur {
			return animFast
		}
		for _, t := range a.reach.downAt {
			if now.Sub(t) < downBlink {
				return animFast
			}
		}
		if lvl == 2 && len(a.reach.res) > 0 {
			slow = true
		}
	}
	if slow {
		return animSlow
	}
	return 0
}

// ---------------------------------------------------------------- Bausteine

// pulse: weiche Welle 0 … 1 … 0 mit der Periode p.
func pulse(now time.Time, p time.Duration) float64 {
	ph := float64(now.UnixNano()%int64(p)) / float64(p)
	return (1 - math.Cos(2*math.Pi*ph)) / 2
}

// easeOut: schnell los, sanft am Ende.
func easeOut(t float64) float64 {
	t = math.Max(0, math.Min(1, t))
	return 1 - math.Pow(1-t, 3)
}

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinGlyph(now time.Time) string {
	return spinFrames[(now.UnixMilli()/int64(animFast/time.Millisecond))%int64(len(spinFrames))]
}

// sparkline: Werte als ▁▂▃▄▅▆▇█ (höchster Wert = volle Höhe).
func sparkline(vals []float64) string {
	const bars = "▁▂▃▄▅▆▇█"
	r := []rune(bars)
	hi := 0.0
	for _, v := range vals {
		hi = math.Max(hi, v)
	}
	var b strings.Builder
	for _, v := range vals {
		i := 0
		if hi > 0 {
			i = int(math.Round(v / hi * float64(len(r)-1)))
		}
		b.WriteRune(r[max(0, min(len(r)-1, i))])
	}
	return b.String()
}

// smoothBar: Balken mit Achtel-Blöcken (weicher als ganze Zellen), f 0 … 1.
func smoothBar(f float64, w int, fg, bg color.Color) string {
	eighths := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	f = math.Max(0, math.Min(1, f))
	n := f * float64(w)
	full := int(n)
	rest := int((n - float64(full)) * 8)
	s := strings.Repeat("█", full)
	used := full
	if full < w && rest > 0 {
		s += eighths[rest]
		used++
	}
	return lipgloss.NewStyle().Foreground(fg).Background(bg).Render(s + strings.Repeat(" ", max(0, w-used)))
}

// ---------------------------------------------------------------- Tabs

// flashTab: Tab leuchtet kurz in c auf (verbunden, Fehler, Meldung).
func (s *session) flashTab(c color.Color) {
	if motionLevel() == 0 {
		return
	}
	s.flashAt, s.flashCol = time.Now(), c
}

// tabFlashBg: Hintergrund eines Tabs während des Aufleuchtens.
func tabFlashBg(s *session, bg color.Color, now time.Time) color.Color {
	d := now.Sub(s.flashAt)
	if s.flashCol == nil || d >= tabFlashDur || d < 0 || bg == nil {
		return bg
	}
	return mixColor(mixColor(bg, s.flashCol, 0.6), bg, easeOut(float64(d)/float64(tabFlashDur)))
}

// tabDot: Punkt im Tab (Spinner beim Verbinden, Puls bei neuer Ausgabe).
func (a *App) tabDot(x *session, now time.Time) string {
	st := a.s
	lvl := motionLevel()
	switch x.state {
	case stConnecting, stReconnect:
		if lvl > 0 {
			return st.Warn.Render(spinGlyph(now))
		}
		return st.Warn.Render("◌")
	case stClosed:
		return st.Danger.Render("○")
	}
	if x.unseen && !x.detached() {
		if accessible {
			return st.Accent.Render("•") // Form statt nur Farbe
		}
		col := st.T.Accent
		if lvl == 2 {
			col = mixColor(st.T.Muted, st.T.Accent, 0.25+0.75*pulse(now, 1400*time.Millisecond))
		}
		return glyph("●", col, nil)
	}
	return st.OK.Render("●")
}

// latSpark: Latenzverlauf für die Statuszeile, jede Säule in der Farbe
// ihrer Stufe. Bezug mindestens 50 ms, damit Rauschen flach bleibt.
func (a *App) latSpark(h []time.Duration) string {
	const bars = "▁▂▃▄▅▆▇█"
	r := []rune(bars)
	hi := 50 * time.Millisecond
	for _, l := range h {
		hi = max(hi, l)
	}
	t := a.s.T
	var b strings.Builder
	for _, l := range h {
		i := int(math.Round(float64(l) / float64(hi) * float64(len(r)-1)))
		c := mixColor(t.OK, t.Accent, 0.3)
		switch {
		case l >= 250*time.Millisecond:
			c = t.Danger
		case l >= 80*time.Millisecond:
			c = t.Warn
		}
		b.WriteString(lipgloss.NewStyle().Foreground(c).Background(t.Surface2).Render(string(r[max(0, min(len(r)-1, i))])))
	}
	return b.String()
}

// recordLatency: Latenzverlauf für Hover-Karte und Statuszeile (alle 5 s ein Wert).
func (s *session) recordLatency(now time.Time) {
	if s.client == nil || s.state != stConnected || now.Sub(s.latAt) < 5*time.Second {
		return
	}
	s.latAt = now
	if l := s.client.Latency(); l > 0 {
		s.latHist = append(s.latHist, l)
		if len(s.latHist) > 16 {
			s.latHist = s.latHist[len(s.latHist)-16:]
		}
	}
}

// tabTip: Karte beim Verweilen der Maus über einem Tab.
func (a *App) tabTip(x *session) string {
	st := a.s
	bg := st.T.Surface
	on := func(c color.Color, bold bool) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c).Background(bg).Bold(bold)
	}
	var lines []string
	lines = append(lines, on(st.T.Text, true).Render(x.tabName()))
	lines = append(lines, on(st.T.Muted, false).Render(x.conn.Target()))
	switch x.state {
	case stConnected:
		lines = append(lines, on(st.T.OK, false).Render("● "+i18n.F("verbunden seit %s", fmtDur(time.Since(x.connectedAt)))))
	case stConnecting:
		lines = append(lines, on(st.T.Warn, false).Render("◌ "+i18n.T("verbindet …")))
	case stReconnect:
		lines = append(lines, on(st.T.Warn, false).Render("◌ "+i18n.T("verbindet neu …")))
	default:
		lines = append(lines, on(st.T.Danger, false).Render("○ "+i18n.T("getrennt")))
	}
	if n := len(x.latHist); n > 0 {
		vals := make([]float64, n)
		for i, l := range x.latHist {
			vals[i] = float64(l.Microseconds())
		}
		last := x.latHist[n-1]
		lines = append(lines, on(st.T.Text, false).Render(i18n.T("Latenz")+" "+fmtLatency(last)+"  ")+on(st.T.Accent, false).Render(sparkline(vals)))
	}
	if n := len(x.history); n > 0 {
		lines = append(lines, on(st.T.Muted, false).Render("$ "+trunc(x.history[n-1], 34)))
	}
	lines = append(lines, on(st.T.Muted, false).Render(i18n.T("Klick wechseln · Mitte schließen · Rechtsklick Menü")))
	w := 0
	for _, l := range lines {
		w = max(w, lipgloss.Width(l))
	}
	for i, l := range lines { // jede Zeile bis zum Rand auf der Kartenfarbe
		lines[i] = l + on(st.T.Text, false).Render(strings.Repeat(" ", w-lipgloss.Width(l)))
	}
	return lipgloss.NewStyle().Border(frameBorder).BorderForeground(st.T.Accent).BorderBackground(st.T.Bg).
		Background(bg).Padding(0, 1).Render(strings.Join(lines, "\n"))
}

// ---------------------------------------------------------------- Hover

type hoverState struct {
	tip    func() string
	x, y   int // Ecke oben links der Karte
	key    [2]int
	since  time.Time
	mx, my int
}

// remoteWantsMotion: ein Programm im Terminal will jede Mausbewegung.
func (a *App) remoteWantsMotion() bool {
	s := a.current()
	return a.scr == scrTerminal && s != nil && s.term != nil && a.modal == nil && a.pal == nil &&
		s.term.MouseTracking() == termemu.MouseAny
}

// trackHover: Maus bewegt sich (ohne Taste). Merkt sich die Zone mit Karte.
func (a *App) trackHover(m tea.Mouse) {
	if motionLevel() < 2 || a.modal != nil || a.pal != nil || a.pick != nil {
		a.hov = hoverState{}
		return
	}
	act, r := a.z.tipAt(m.X, m.Y)
	if act == nil {
		a.hov = hoverState{}
		return
	}
	key := [2]int{r.x0, r.y0}
	if a.hov.tip != nil && a.hov.key == key {
		return
	}
	a.hov = hoverState{tip: act.tip, x: r.x0, y: r.y1 + 1, key: key, since: time.Now(), mx: m.X, my: m.Y}
}

// withHover legt die Karte über das Bild, sobald die Maus kurz verweilt.
func (a *App) withHover(body string) string {
	h := a.hov
	if h.tip == nil || time.Since(h.since) < hoverDelay || motionLevel() < 2 || a.modal != nil || a.pal != nil || a.pick != nil {
		return body
	}
	card := h.tip()
	if card == "" {
		return body
	}
	w := lipgloss.Width(card)
	x := max(0, min(h.x, a.w-w))
	y := min(h.y, max(0, a.h-lipgloss.Height(card)))
	c := lipgloss.NewCompositor(lipgloss.NewLayer(body), lipgloss.NewLayer(card).X(x).Y(y).Z(2))
	return c.Render()
}
