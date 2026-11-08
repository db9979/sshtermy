package ui

// Adaptive Bildrate und Leistungsanzeige. Jedes Bild wird gemessen. Kommt
// sehr viel Ausgabe (z. B. cat auf eine große Datei), zeichnet sshTermy
// nicht für jedes Stück neu, sondern höchstens alle paar Millisekunden,
// abhängig davon, wie lange ein Bild dauert; Tastendruck und Maus zeichnen
// immer sofort. Langsame Bilder landen im Debug-Protokoll. Die Anzeige
// (Palette › „Leistungsanzeige“) zeigt Bildrate, Zeichenzeit und Ausgabe.

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/debuglog"
	"github.com/db9979/sshtermy/internal/i18n"
)

const (
	perfHeavy   = 256 << 10 // Bytes/s: ab hier gilt die Ausgabe als Flut
	perfMinGap  = 16 * time.Millisecond
	perfMaxGap  = 125 * time.Millisecond
	perfSlow    = 50 * time.Millisecond // so ein Bild kommt ins Debug-Protokoll
	perfHistory = 120
)

// outBytes: alle Sitzungen zusammen, vom Lese-Goroutine gezählt.
var outBytes atomic.Int64

type (
	perfRedrawMsg struct{}
	perfTickMsg   struct{}
)

type perfState struct {
	on bool // Anzeige sichtbar

	durs  []time.Duration // Zeichenzeit der letzten Bilder
	ats   []time.Time     // wann sie gezeichnet wurden
	ema   time.Duration   // gleitender Mittelwert
	peak  time.Duration
	last  time.Time
	skips int // ausgelassene Bilder seit dem Start

	cached tea.View
	have   bool
	cw, ch int
	skip   bool // dieses Bild auslassen (viel Ausgabe, letztes ist frisch)
	redraw bool // Nachzeichnen ist bestellt

	outAt   time.Time
	outPrev int64
	outRate float64 // Bytes/s
	slowAt  time.Time
}

// record: Dauer eines Bildes merken.
func (p *perfState) record(at time.Time, d time.Duration) {
	p.durs = append(p.durs, d)
	p.ats = append(p.ats, at)
	if len(p.durs) > perfHistory {
		p.durs, p.ats = p.durs[1:], p.ats[1:]
	}
	if p.ema == 0 {
		p.ema = d
	} else {
		p.ema = (p.ema*4 + d) / 5
	}
	p.peak = max(p.peak, d)
	p.last = at
}

// fps: gezeichnete Bilder in der letzten Sekunde.
func (p *perfState) fps(now time.Time) int {
	n := 0
	for i := len(p.ats) - 1; i >= 0 && now.Sub(p.ats[i]) < time.Second; i-- {
		n++
	}
	return n
}

// gap: Mindestabstand zweier Bilder bei Ausgabeflut: das Dreifache einer
// Zeichenzeit, damit zwischen den Bildern Zeit für die Eingabe bleibt.
func (p *perfState) gap() time.Duration {
	return min(perfMaxGap, max(perfMinGap, 3*p.ema))
}

// heavy: kommt gerade eine Ausgabeflut?
func (p *perfState) heavy() bool { return p.outRate >= perfHeavy }

// noteOutput: Ausgaberate nachführen (höchstens alle 200 ms neu).
func (p *perfState) noteOutput(now time.Time) {
	n := outBytes.Load()
	if p.outAt.IsZero() {
		p.outAt, p.outPrev = now, n
		return
	}
	if dt := now.Sub(p.outAt); dt >= 200*time.Millisecond {
		p.outRate = float64(n-p.outPrev) / dt.Seconds()
		p.outAt, p.outPrev = now, n
	}
}

// onOutput: neue Ausgabe da. Bei Flut und frischem letzten Bild wird das
// nächste Bild ausgelassen und ein Nachzeichnen bestellt.
func (a *App) onOutputFrame(now time.Time) tea.Cmd {
	p := &a.perf
	p.noteOutput(now)
	if !p.heavy() || !p.have {
		return nil
	}
	since := now.Sub(p.last)
	if since >= p.gap() {
		return nil
	}
	p.skip = true
	if p.redraw {
		return nil
	}
	p.redraw = true
	return tea.Tick(p.gap()-since, func(time.Time) tea.Msg { return perfRedrawMsg{} })
}

// view: misst ein Bild oder gibt bei Flut das letzte zurück.
func (a *App) view() tea.View {
	p := &a.perf
	if p.skip && p.have && p.cw == a.w && p.ch == a.h {
		p.skips++
		return p.cached
	}
	t0 := time.Now()
	v := a.viewFrame()
	d := time.Since(t0)
	p.record(t0, d)
	p.cached, p.have, p.cw, p.ch = v, true, a.w, a.h
	if d >= perfSlow && debuglog.On() && t0.Sub(p.slowAt) > time.Second {
		p.slowAt = t0
		debuglog.Printf("frame", "langsames Bild %s · Ø %s · Ausgabe %s/s · Fenster %dx%d · Bereich %d",
			fmtMs(d), fmtMs(p.ema), humanBytes(int64(p.outRate)), a.w, a.h, a.scr)
	}
	return v
}

func fmtMs(d time.Duration) string {
	return strings.Replace(fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000), ".", i18n.Pick(",", "."), 1)
}

// togglePerf: Leistungsanzeige an/aus (Palette).
func (a *App) togglePerf() tea.Cmd {
	a.perf.on = !a.perf.on
	if a.perf.on {
		return a.perfTick()
	}
	return nil
}

// perfTick: Anzeige einmal je Sekunde auffrischen, solange sie offen ist.
func (a *App) perfTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return perfTickMsg{} })
}

// perfBox: die Anzeige oben rechts.
func (a *App) perfBox(now time.Time) string {
	p := &a.perf
	st := a.s
	const w = 36
	row := func(label, val string, c lipgloss.Style) string {
		return st.Muted.Render(fmt.Sprintf("%-10s", label)) + c.Bold(true).Render(val)
	}
	mode := i18n.T("sofort")
	if p.heavy() {
		mode = fmt.Sprintf(i18n.T("alle %d ms"), p.gap().Milliseconds())
	}
	cur := time.Duration(0)
	if len(p.durs) > 0 {
		cur = p.durs[len(p.durs)-1]
	}
	drawC := st.Text
	if p.ema >= perfMinGap {
		drawC = st.Warn
	}
	lines := []string{
		row(i18n.T("Bilder/s"), fmt.Sprintf("%-4d", p.fps(now)), st.Text) + st.OK.Render(mode),
		row(i18n.T("Zeichnen"), fmtMs(cur), drawC) + st.Muted.Render(" · Ø "+fmtMs(p.ema)),
		row(i18n.T("Ausgabe"), humanBytes(int64(p.outRate))+"/s", st.Text),
	}
	vals := make([]float64, len(p.durs))
	for i, d := range p.durs {
		vals[i] = float64(d.Microseconds())
	}
	if len(vals) > 0 {
		lines = append(lines, "")
		lines = append(lines, strings.Split(graphBlock(scaleTo(vals, float64(perfMinGap.Microseconds())), w-4, 2, st.T.Warn, st.T.OK), "\n")...)
	}
	lines = append(lines, st.Muted.Render(fmt.Sprintf(i18n.T("Spitze %s · %d ausgelassen"), fmtMs(p.peak), p.skips)))
	box := lipgloss.NewStyle().Border(frameBorder).BorderForeground(st.T.Border).Padding(0, 1).Width(w).
		Render(strings.Join(lines, "\n"))
	// Titel in die obere Linie
	bl := strings.SplitN(box, "\n", 2)
	if len(bl) == 2 {
		tc, _ := frameCorners()
		title := st.AI.Bold(true).Render(" " + i18n.T("Leistung") + " ")
		bs := lipgloss.NewStyle().Foreground(st.T.Border)
		box = bs.Render(string(tc[0])+"─") + title + bs.Render(strings.Repeat("─", max(0, w-2-lipgloss.Width(title)-2))+string(tc[1])) + "\n" + bl[1]
	}
	return box
}

// withPerf: Anzeige über das Bild legen.
func (a *App) withPerf(base string) string {
	if !a.perf.on || a.w < 44 || a.h < 14 {
		return base
	}
	box := a.perfBox(time.Now())
	x := max(0, a.w-lipgloss.Width(box)-1)
	y := 1
	if a.scr == scrTerminal {
		y = 2
	}
	return lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(box).X(x).Y(y).Z(50)).Render()
}
