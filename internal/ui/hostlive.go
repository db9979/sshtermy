package ui

// F6 als Dashboard: Solange die Host-Übersicht eines Linux-Servers offen ist,
// läuft ein kleines, nur lesendes Skript (hostinfo.LiveScript) in einer
// eigenen Exec-Sitzung und liefert alle zwei Sekunden CPU, RAM, Netz und
// Platte. Daraus werden Braille-Graphen (graph.go) über 5 Minuten oder eine
// Stunde. Panel zu = Skript beendet; der Verlauf bleibt bis zum Trennen.
// Netzwerkgeräte nie (Exec-Kanäle trennen dort die Verbindung).

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
)

const (
	liveFineMax   = 300 // 10 min, alle 2 s
	liveCoarseN   = 12  // 12 × 2 s = 24 s je Punkt in der Stundenansicht
	liveCoarseMax = 150 // 1 h
)

type liveState struct {
	cancel context.CancelFunc
	seq    int
	fine   []hostinfo.LivePoint
	coarse []hostinfo.LivePoint
	acc    []hostinfo.LivePoint
	hour   bool   // Zeitraum 1 h statt 5 min
	wide   bool   // ganze Breite statt Seitenpanel
	off    bool   // Gegenstelle ohne /proc
	err    string // letzter Abbruch
	failAt time.Time
}

func (l *liveState) stop() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
}

type liveEvent struct {
	p      hostinfo.LivePoint
	noLive bool
	err    error
	done   bool
}

// liveWant: soll für diese Sitzung gerade gemessen werden?
func (a *App) liveWant(s *session) bool {
	return s.panel == panelHost && a.scr == scrTerminal && s.state == stConnected && a.hostServerOK(s) &&
		!s.host.live.off && a.lock == nil && !a.blurred
}

// liveTick (jede Sekunde): Messung starten oder beenden.
func (a *App) liveTick(s *session) tea.Cmd {
	l := &s.host.live
	if !a.liveWant(s) {
		l.stop()
		return nil
	}
	if l.cancel != nil || time.Since(l.failAt) < 30*time.Second {
		return nil
	}
	return a.liveStart(s)
}

func (a *App) liveStart(s *session) tea.Cmd {
	l := &s.host.live
	l.stop()
	l.seq++
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	ch := make(chan liveEvent, 8)
	cl := s.client
	go func() {
		defer close(ch)
		var p hostinfo.LiveParser
		var prev *hostinfo.LiveSample
		err := cl.StreamScript(ctx, hostinfo.LiveScript, func(line string) {
			smp, ok := p.Line(line)
			if p.NoLive {
				select {
				case ch <- liveEvent{noLive: true}:
				default:
				}
				return
			}
			if !ok {
				return
			}
			if prev != nil {
				if pt, ok := hostinfo.Rates(*prev, smp); ok {
					select {
					case ch <- liveEvent{p: pt}:
					default: // Oberfläche hängt hinterher: Punkt auslassen
					}
				}
			}
			prev = &smp
		})
		if ctx.Err() == nil {
			ch <- liveEvent{err: err, done: true}
		}
	}()
	return liveWait(s.id, l.seq, ch)
}

func liveWait(sid, seq int, ch <-chan liveEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return liveEventMsg{sid: sid, seq: seq, ev: ev, ch: ch}
	}
}

type liveEventMsg struct {
	sid, seq int
	ev       liveEvent
	ch       <-chan liveEvent
}

func (a *App) liveMsg(m liveEventMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil || m.seq != s.host.live.seq {
		return nil
	}
	l := &s.host.live
	switch {
	case m.ev.noLive:
		l.off = true
		l.stop()
		return nil
	case m.ev.done:
		l.cancel = nil
		l.failAt = time.Now()
		if m.ev.err != nil {
			l.err = m.ev.err.Error()
		}
		return nil
	}
	l.err = ""
	l.add(m.ev.p)
	return liveWait(m.sid, m.seq, m.ch)
}

func (l *liveState) add(p hostinfo.LivePoint) {
	l.fine = append(l.fine, p)
	if len(l.fine) > liveFineMax {
		l.fine = l.fine[len(l.fine)-liveFineMax:]
	}
	l.acc = append(l.acc, p)
	if len(l.acc) >= liveCoarseN {
		var avg hostinfo.LivePoint
		for _, x := range l.acc {
			avg.CPU += x.CPU
			avg.Mem += x.Mem
			avg.Net += x.Net
			avg.NetRx += x.NetRx
			avg.NetTx += x.NetTx
			avg.Disk += x.Disk
		}
		n := float64(len(l.acc))
		avg.CPU, avg.Mem, avg.Net, avg.NetRx, avg.NetTx, avg.Disk = avg.CPU/n, avg.Mem/n, avg.Net/n, avg.NetRx/n, avg.NetTx/n, avg.Disk/n
		avg.At, avg.HasDisk = p.At, p.HasDisk
		l.coarse = append(l.coarse, avg)
		if len(l.coarse) > liveCoarseMax {
			l.coarse = l.coarse[len(l.coarse)-liveCoarseMax:]
		}
		l.acc = l.acc[:0]
	}
}

// points: Punkte des gewählten Zeitraums.
func (l *liveState) points() ([]hostinfo.LivePoint, time.Duration) {
	if l.hour {
		return l.coarse, liveCoarseN * hostinfo.LiveEvery
	}
	if len(l.fine) > 150 {
		return l.fine[len(l.fine)-150:], hostinfo.LiveEvery
	}
	return l.fine, hostinfo.LiveEvery
}

// hasLive: gibt es schon Graphen?
func (l *liveState) hasLive() bool { return len(l.fine) >= 2 }

// viewLive: Graphen für CPU, RAM, Netz und Platte. Schmal untereinander,
// breit (z) im Raster 2 × 2 mit den Prozessen darunter.
func (a *App) viewLive(b *strings.Builder, s *session, w int) {
	st := a.s
	l := &s.host.live
	if l.off {
		return
	}
	pts, step := l.points()
	if len(pts) < 2 {
		if l.cancel != nil {
			b.WriteString(st.Muted.Render(spinGlyph(time.Now())+" "+i18n.T("Live-Graphen starten …")) + "\n")
		} else if l.hour && len(l.fine) >= 2 {
			b.WriteString(st.Muted.Render(i18n.T("1 h: der erste Punkt kommt nach 24 s · t = 5 min")) + "\n")
		}
		return
	}
	lim := a.monLimits()
	cur := pts[len(pts)-1]
	get := func(f func(hostinfo.LivePoint) float64) []float64 {
		out := make([]float64, len(pts))
		for i, p := range pts {
			out[i] = f(p)
		}
		return out
	}
	cpu := get(func(p hostinfo.LivePoint) float64 { return p.CPU })
	mem := get(func(p hostinfo.LivePoint) float64 { return p.Mem })
	net := get(func(p hostinfo.LivePoint) float64 { return p.Net })
	dsk := get(func(p hostinfo.LivePoint) float64 { return p.Disk })
	type graph struct {
		name, val string
		vals      []float64
		lo, hi    lipgloss.Style
		warn      bool
	}
	gs := []graph{
		{"CPU", fmt.Sprintf("%d %%", int(cur.CPU*100+0.5)), cpu, st.Accent, st.AI, lim.CPU > 0 && cur.CPU*100 >= float64(lim.CPU)},
		{"RAM", fmt.Sprintf("%d %%", int(cur.Mem*100+0.5)), mem, st.OK, st.Accent, lim.RAM > 0 && cur.Mem*100 >= float64(lim.RAM)},
		{i18n.T("Netz"), "↓ " + hostinfo.Rate(cur.NetRx) + "  ↑ " + hostinfo.Rate(cur.NetTx), scaleTo(net, 64<<10), st.Accent, st.OK, false},
	}
	if cur.HasDisk {
		gs = append(gs, graph{i18n.T("Platte"), hostinfo.Rate(cur.Disk), scaleTo(dsk, 1<<20), st.Warn, st.Danger, false})
	}
	span := time.Duration(min(len(pts), 2*w)) * step
	axis := func(gw int) string {
		left := "−" + fmtSpan(span)
		right := i18n.T("jetzt")
		return st.Muted.Render(left + strings.Repeat(" ", max(1, gw-len([]rune(left))-len([]rune(right)))) + right)
	}
	one := func(g graph, gw, rows int) string {
		head := st.Text.Bold(true).Render(g.name)
		val := st.Text.Render(g.val)
		if g.warn {
			val = st.Warn.Bold(true).Render("▲ " + g.val)
		}
		head += strings.Repeat(" ", max(1, gw-lipgloss.Width(head)-lipgloss.Width(val))) + val
		lo, hi := g.lo.GetForeground(), g.hi.GetForeground()
		if g.warn {
			lo, hi = st.T.Warn, st.T.Danger
		}
		return head + "\n" + graphBlock(g.vals, gw, rows, lo, hi)
	}
	if !l.wide || w < 70 {
		for _, g := range gs {
			b.WriteString(one(g, w, 2) + "\n")
		}
		b.WriteString(axis(w) + "\n")
		return
	}
	// breit: 2 × 2
	gw := (w - 3) / 2
	rows := 4
	var cells []string
	for _, g := range gs {
		cells = append(cells, lipgloss.NewStyle().Width(gw).Render(one(g, gw, rows)+"\n"+axis(gw)))
	}
	for i := 0; i < len(cells); i += 2 {
		row := cells[i]
		if i+1 < len(cells) {
			row = lipgloss.JoinHorizontal(lipgloss.Top, cells[i], "   ", cells[i+1])
		}
		b.WriteString(row + "\n\n")
	}
	if len(cur.TopProcs) > 0 {
		b.WriteString(st.Text.Bold(true).Render(i18n.T("Top-Prozesse")) + st.Muted.Render(i18n.T("  Anteil an der ganzen Maschine")) + "\n")
		for _, p := range cur.TopProcs {
			bw := 20
			b.WriteString(st.Text.Render(fmt.Sprintf("%-18s ", trunc(p.Name, 18))) +
				smoothBar(p.Pct, bw, mixColor(st.T.Accent, st.T.AI, p.Pct), st.T.Surface2) +
				st.Muted.Render(fmt.Sprintf(" %5.1f %%", p.Pct*100)) + "\n")
		}
		b.WriteString("\n")
	}
}

// fmtSpan: 2 min, 40 s, 1 h.
func fmtSpan(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()+0.5))
	case d >= 2*time.Minute:
		return fmt.Sprintf("%d min", int(d.Minutes()+0.5))
	}
	return fmt.Sprintf("%d s", int(d.Seconds()))
}
