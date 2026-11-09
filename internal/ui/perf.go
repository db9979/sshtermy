package ui

// Adaptive Bildrate und Leistungsanzeige. Jedes Bild wird gemessen. Kommt
// sehr viel Ausgabe (z. B. cat auf eine große Datei), zeichnet sshTermy
// nicht für jedes Stück neu, sondern höchstens alle paar Millisekunden,
// abhängig davon, wie lange ein Bild dauert; Tastendruck und Maus zeichnen
// immer sofort. Langsame Bilder landen im Debug-Protokoll. Die Anzeige
// (Palette › „Leistungsanzeige“) zeigt Bildrate, Zeichenzeit und Ausgabe.

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"

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

	rect rect // wo die Anzeige im letzten Bild stand (Toasts weichen aus)
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

// termMemo: das zuletzt gezeichnete Terminalbild einer Sitzung. Viele
// Bilder entstehen ohne neue Ausgabe (Maus, Uhr, Toasts, Leistungsanzeige);
// dann ist das Terminal unverändert und muss nicht neu aufgebaut werden.
type termMemo struct {
	gen   uint64
	ghost string
	hl    any
	style int
	w, h  int
	out   string
	ok    bool
}

// termView: Terminalfläche, aus dem Gedächtnis, solange sich nichts geändert hat.
func (a *App) termView(s *session, ghost string) string {
	gen := s.term.Gen() // vor dem Zeichnen lesen: kommt währenddessen Ausgabe, zählt das nächste Bild neu
	w, h := s.term.Size()
	var hl any
	if set := a.hlSet(); set != nil {
		hl = set
	}
	m := &s.viewMemo
	if m.ok && m.gen == gen && m.ghost == ghost && m.hl == hl && m.style == a.styleGen && m.w == w && m.h == h {
		return m.out
	}
	st := a.s
	out := s.term.ViewHL(ghost, func(g string) string { return st.Ghost.Render(g) }, a.termHL())
	*m = termMemo{gen: gen, ghost: ghost, hl: hl, style: a.styleGen, w: w, h: h, out: out, ok: true}
	return out
}

// beside: links ein Block mit fester Breite lw, rechts daneben right (oben
// bündig). Wie lipgloss.JoinHorizontal mit Width, aber ohne alles neu zu
// umbrechen; bei 200 Spalten spart das je Bild spürbar Zeit.
func beside(left string, lw int, right string) string {
	ls, rs := strings.Split(left, "\n"), strings.Split(right, "\n")
	rw := 0
	rws := make([]int, len(rs))
	for i, r := range rs {
		rws[i] = ansi.StringWidth(r)
		rw = max(rw, rws[i])
	}
	n := max(len(ls), len(rs))
	var b strings.Builder
	b.Grow(len(left) + len(right) + n*8)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		l := ""
		if i < len(ls) {
			l = ls[i]
		}
		w := ansi.StringWidth(l)
		if w > lw {
			l = ansi.Truncate(l, lw, "")
			w = ansi.StringWidth(l)
		}
		b.WriteString(l)
		if strings.IndexByte(l, 0x1b) >= 0 {
			b.WriteString(sgrReset)
		}
		b.WriteString(strings.Repeat(" ", lw-w))
		if i < len(rs) {
			b.WriteString(rs[i])
			b.WriteString(strings.Repeat(" ", rw-rws[i]))
		} else {
			b.WriteString(strings.Repeat(" ", rw))
		}
	}
	return b.String()
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

// perfGrade: Zeichenzeit in Worten. Ein Bildschirm zeigt 60 Bilder/s, für
// ein Bild bleiben also 16,7 ms; darunter wirkt alles flüssig.
func (a *App) perfGrade(d time.Duration) (string, lipgloss.Style) {
	st := a.s
	switch {
	case d < 8*time.Millisecond:
		return i18n.T("flüssig"), st.OK
	case d < 17*time.Millisecond:
		return i18n.T("gut"), st.OK
	case d < perfSlow:
		return i18n.T("etwas träge"), st.Warn
	default:
		return i18n.T("ruckelt"), st.Danger
	}
}

// perfBars: zwei Zeilen Balken (16 Stufen), je Bild einer, gefärbt nach
// Bewertung. Volle Höhe = 33 ms (zwei Bildschirmbilder); flach = gut.
func (a *App) perfBars(durs []time.Duration, w int) []string {
	r := []rune(" ▁▂▃▄▅▆▇█")
	if len(durs) > w {
		durs = durs[len(durs)-w:]
	}
	full := float64(2 * perfMinGap)
	var top, bot strings.Builder
	pad := strings.Repeat(" ", w-len(durs))
	top.WriteString(pad)
	bot.WriteString(pad)
	for _, d := range durs {
		lv := max(1, min(16, int(math.Ceil(float64(d)/full*16))))
		_, c := a.perfGrade(d)
		top.WriteString(c.Render(string(r[max(0, lv-8)])))
		bot.WriteString(c.Render(string(r[min(8, lv)])))
	}
	return []string{top.String(), bot.String()}
}

const perfBoxW = 42 // Inhalt samt Innenabstand, Rahmen kommt dazu

// perfBox: die Anzeige. Jede Zahl bekommt eine Bewertung in Worten, damit
// man ohne Vorwissen sieht, ob alles in Ordnung ist.
func (a *App) perfBox(now time.Time) string {
	p := &a.perf
	st := a.s
	const inner = perfBoxW - 4
	row := func(label, val string, c lipgloss.Style, note string, nc lipgloss.Style) string {
		l := st.Muted.Render(fmt.Sprintf("%-10s", label)) + c.Bold(true).Render(fmt.Sprintf("%-9s", val))
		return l + nc.Render(trunc(note, inner-19))
	}
	cur := time.Duration(0)
	if len(p.durs) > 0 {
		cur = p.durs[len(p.durs)-1]
	}
	peak := time.Duration(0)
	for _, d := range p.durs {
		peak = max(peak, d)
	}
	grade, gc := a.perfGrade(p.ema)
	lines := []string{
		row(i18n.T("Zeichnen"), fmtMs(p.ema), gc, "● "+grade, gc),
		st.Muted.Render(trunc(fmt.Sprintf(i18n.T("zuletzt %s · Spitze %s"), fmtMs(cur), fmtMs(peak)), inner)),
	}
	if len(p.durs) > 0 {
		lines = append(lines, a.perfBars(p.durs, inner)...)
		lines = append(lines, st.Muted.Render(trunc(i18n.T("ein Balken je Bild · voll = 33 ms"), inner)))
	}
	lines = append(lines, "")
	fps := p.fps(now)
	fpsNote := ""
	if fps <= 5 {
		fpsNote = i18n.T("nur bei Änderung")
	}
	lines = append(lines, row(i18n.T("Bilder/s"), fmt.Sprintf("%d", fps), st.Text, fpsNote, st.Muted))
	if p.ema > 0 {
		// so viele Bilder je Sekunde gingen bei dieser Zeichenzeit
		lines = append(lines, row(i18n.T("möglich"), fmt.Sprintf("%d", min(999, int(time.Second/p.ema))), st.Text, i18n.T("Bilder/s"), st.Muted))
	}
	outNote, oc := i18n.T("sofort gezeigt"), st.Muted
	if p.heavy() {
		outNote, oc = fmt.Sprintf(i18n.T("viel: alle %d ms"), p.gap().Milliseconds()), st.Warn
	}
	lines = append(lines, row(i18n.T("Ausgabe"), humanBytes(int64(p.outRate))+"/s", st.Text, outNote, oc))
	if p.skips > 0 {
		lines = append(lines, st.Muted.Render(trunc(fmt.Sprintf(i18n.T("%d× übersprungen bei Flut (gewollt)"), p.skips), inner)))
	}
	box := lipgloss.NewStyle().Border(frameBorder).BorderForeground(st.T.Border).Padding(0, 1).Width(perfBoxW).
		Render(strings.Join(lines, "\n"))
	// Titel in die obere Linie
	bl := strings.SplitN(box, "\n", 2)
	if len(bl) == 2 {
		tc, _ := frameCorners()
		title := st.AI.Bold(true).Render(" " + i18n.T("Leistung") + " ")
		bs := lipgloss.NewStyle().Foreground(st.T.Border)
		bw := lipgloss.Width(bl[0])
		box = bs.Render(string(tc[0])+"─") + title + bs.Render(strings.Repeat("─", max(0, bw-3-lipgloss.Width(title)))+string(tc[1])) + "\n" + bl[1]
	}
	return box
}

// withPerf: Anzeige über das Bild legen. Im Terminal rechts oben in der
// Terminalfläche, also links neben einer offenen Seitenleiste (F6), damit
// sie die Host-Übersicht nicht verdeckt.
func (a *App) withPerf(base string) string {
	a.perf.rect = rect{}
	if !a.perf.on || a.w < perfBoxW+6 || a.h < 16 {
		return base
	}
	box := a.perfBox(time.Now())
	bw := lipgloss.Width(box)
	x, y := a.w-bw-1, 1
	if s := a.current(); a.scr == scrTerminal && s != nil {
		tr := a.termRect(s)
		cols, _ := a.paneSize(s)
		y = tr.y
		if cols >= bw+2 {
			x = tr.x + cols - bw - 1
		}
	}
	x = max(0, x)
	a.perf.rect = rect{x, y, bw, lipgloss.Height(box)}
	return stamp(base, box, x, y, a.w)
}
