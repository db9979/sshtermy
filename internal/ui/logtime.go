package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
)

// Zeitleiste des Log-Blicks (t): Fehler und Warnungen pro Zeitabschnitt,
// darunter Marken für Neustarts, Paket-Updates, Konfigurationsänderungen und
// die eigenen Befehle aus dem Sitzungsprotokoll. ←→ wählt einen Abschnitt,
// darunter stehen seine Zeilen; ⏎ zeigt alle, k lässt die KI sie auswerten.

type timeCursor struct {
	sel  int // gewählter Abschnitt
	n    int // Abschnitte beim letzten Zeichnen
	init bool
}

// addOwnCommands: eigene Befehle dieser Sitzung als Marken.
func (a *App) addOwnCommands(s *session, r *logscan.Report) {
	if s.log == nil {
		return
	}
	since := r.Since()
	for _, e := range s.log.Entries {
		if e.At.Before(since) || strings.TrimSpace(e.Command) == "" {
			continue
		}
		r.Marks = append(r.Marks, logscan.Mark{Time: e.At, Kind: logscan.MarkCmd, Text: e.Command})
	}
}

func (a *App) timeOpen() {
	a.lscan.tl.init = false
}

// timeSlot: Beginn und Ende von Abschnitt i bei n Abschnitten.
func timeSlot(r *logscan.Report, i, n int) (time.Time, time.Time) {
	since := r.Since()
	step := r.At.Sub(since) / time.Duration(max(1, n))
	from := since.Add(time.Duration(i) * step)
	return from, from.Add(step)
}

// timeCounts: Fehler/Warnungen je Abschnitt.
func timeCounts(r *logscan.Report, n int) (errs, warns []int) {
	errs, warns = make([]int, n), make([]int, n)
	since := r.Since()
	span := r.At.Sub(since)
	if span <= 0 {
		return
	}
	for _, e := range r.Entries {
		if e.Time.IsZero() || e.Time.Before(since) || e.Key() == "" {
			continue
		}
		i := clamp(int(float64(e.Time.Sub(since))/float64(span)*float64(n)), 0, n-1)
		switch {
		case e.Sev >= logscan.SevErr:
			errs[i]++
		case e.Sev == logscan.SevWarn:
			warns[i]++
		}
	}
	return
}

func (a *App) timeKey(s *session, key string) (tea.Cmd, bool) {
	l := &a.lscan
	tc := &l.tl
	if l.rep == nil || tc.n == 0 {
		return nil, false
	}
	switch key {
	case "left", "h":
		tc.sel = max(0, tc.sel-1)
	case "right", "l":
		tc.sel = min(tc.n-1, tc.sel+1)
	case "shift+left", "pgup":
		tc.sel = max(0, tc.sel-8)
	case "shift+right", "pgdown":
		tc.sel = min(tc.n-1, tc.sel+8)
	case "home":
		tc.sel = 0
	case "end":
		tc.sel = tc.n - 1
	case "enter":
		from, to := timeSlot(l.rep, tc.sel, tc.n)
		es := l.rep.EntriesBetween(from, to)
		var lines []string
		for _, e := range es {
			lines = append(lines, e.Sev.Label()+"  "+e.Line())
		}
		for _, m := range l.rep.Marks {
			if !m.Time.Before(from) && m.Time.Before(to) {
				lines = append(lines, markGlyph(m.Kind)+" "+m.Time.Local().Format("15:04:05")+"  "+m.Text)
			}
		}
		if len(lines) == 0 {
			lines = []string{i18n.T("Keine Meldungen in diesem Abschnitt")}
		}
		a.modal = textModal(fmt.Sprintf(i18n.T("Meldungen %s–%s"), from.Local().Format("02.01. 15:04"), to.Local().Format("15:04")), lines)
	case "k":
		if s == nil {
			return nil, true
		}
		from, to := timeSlot(l.rep, tc.sel, tc.n)
		es := l.rep.EntriesBetween(from, to)
		if len(es) == 0 {
			a.flash(i18n.T("Keine Meldungen in diesem Abschnitt"))
			return nil, true
		}
		if len(es) > 150 {
			es = es[len(es)-150:]
		}
		var b strings.Builder
		for _, m := range l.rep.Marks {
			if !m.Time.Before(from.Add(-30*time.Minute)) && m.Time.Before(to) {
				fmt.Fprintf(&b, "Ereignis %s: %s\n", m.Time.Local().Format("15:04:05"), m.Text)
			}
		}
		for _, e := range es {
			b.WriteString(e.Line() + "\n")
		}
		if !a.st.LLM().SendScreen {
			a.flash(i18n.T("Terminal-Inhalt an die KI ist in den KI-Einstellungen ausgeschaltet"))
			return nil, true
		}
		a.scr = scrTerminal
		return a.diagnoseWith(s, b.String(), fmt.Sprintf(i18n.T("Was passierte %s–%s?"), from.Local().Format("15:04"), to.Local().Format("15:04"))), true
	default:
		return nil, false
	}
	return nil, true
}

func markGlyph(k logscan.MarkKind) string {
	switch k {
	case logscan.MarkBoot:
		return "◉"
	case logscan.MarkPkg, logscan.MarkConfig:
		return "■"
	}
	return "◆"
}

// viewTime zeichnet die Zeitleiste (w Spalten).
func (a *App) viewTime(b *strings.Builder, w, h int) {
	st := a.s
	l := &a.lscan
	r := l.rep
	labelW := 9
	n := max(12, w-labelW-2)
	tc := &l.tl
	errs, warns := timeCounts(r, n)
	if tc.n != n || !tc.init {
		// Start: der letzte Abschnitt mit Fehlern, sonst ganz rechts
		tc.sel = n - 1
		for i := n - 1; i >= 0; i-- {
			if errs[i] > 0 {
				tc.sel = i
				break
			}
		}
		tc.n, tc.init = n, true
	}
	step := r.At.Sub(r.Since()) / time.Duration(n)
	b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("pro %s · "), durShort(step))) + st.Danger.Render(i18n.T("■ Fehler/kritisch")) + st.Muted.Render(" · ") +
		st.Warn.Render(i18n.T("■ Warnung")) + st.Muted.Render(i18n.T(" · Marken: ")) + st.Accent.Render(i18n.T("◉ Neustart")) + st.Muted.Render(" · ") +
		st.OK.Render(i18n.T("■ Update/Config")) + st.Muted.Render(" · ") + st.AI.Render(i18n.T("◆ deine Befehle")) + "\n\n")
	row := func(label string, vals []int, style func(string) string) {
		bars := []rune(sparkInts(vals))
		var o strings.Builder
		for i, c := range bars {
			cell := string(c)
			if vals[i] == 0 {
				cell = "▁"
			}
			if i == tc.sel {
				o.WriteString(st.Selected.Render(cell))
				continue
			}
			if vals[i] == 0 {
				o.WriteString(st.Muted.Render(cell))
			} else {
				o.WriteString(style(cell))
			}
		}
		b.WriteString(st.Muted.Render(padRight(label, labelW)) + o.String() + "\n")
	}
	row(i18n.T("Fehler"), errs, func(s string) string { return st.Danger.Render(s) })
	row(i18n.T("Warnung"), warns, func(s string) string { return st.Warn.Render(s) })
	marks := make([]string, n)
	for _, m := range r.Marks {
		if m.Time.Before(r.Since()) {
			continue
		}
		i := clamp(int(float64(m.Time.Sub(r.Since()))/float64(r.At.Sub(r.Since()))*float64(n)), 0, n-1)
		g := markGlyph(m.Kind)
		switch m.Kind {
		case logscan.MarkBoot:
			marks[i] = st.Accent.Render(g)
		case logscan.MarkCmd:
			if marks[i] == "" {
				marks[i] = st.AI.Render(g)
			}
		default:
			marks[i] = st.OK.Render(g)
		}
	}
	var mo strings.Builder
	for i := range marks {
		if marks[i] == "" {
			mo.WriteString(" ")
		} else {
			mo.WriteString(marks[i])
		}
	}
	b.WriteString(st.Muted.Render(padRight(i18n.T("Marken"), labelW)) + mo.String() + "\n")
	// Zeitachse: alle ~12 Spalten eine Uhrzeit
	axis := []rune(strings.Repeat(" ", n+6))
	for i := 0; i+5 <= n; i += max(12, n/8) {
		from, _ := timeSlot(r, i, n)
		for j, c := range from.Local().Format("15:04") {
			axis[i+j] = c
		}
	}
	b.WriteString(strings.Repeat(" ", labelW) + st.Muted.Render(strings.TrimRight(string(axis), " ")) + "\n")
	from, to := timeSlot(r, tc.sel, n)
	sel := fmt.Sprintf(" %s–%s ", from.Local().Format("15:04"), to.Local().Format("15:04"))
	pad := clamp(labelW+tc.sel-len([]rune(sel))/2, 0, max(0, w-len([]rune(sel))))
	b.WriteString(strings.Repeat(" ", pad) + st.Selected.Render(sel) + "\n\n")

	// Marken im Zeitraum als Liste (ohne eigene Befehle außerhalb des Abschnitts)
	used := 6
	for _, m := range r.Marks {
		if m.Kind == logscan.MarkCmd || used > 9 {
			continue
		}
		b.WriteString(markLine(st, m, w) + "\n")
		used++
	}
	es := r.EntriesBetween(from, to)
	head := fmt.Sprintf(i18n.T("Abschnitt %s–%s · %s"), from.Local().Format("02.01. 15:04"), to.Local().Format("15:04"), plural(len(es), i18n.T("Zeile"), i18n.T("Zeilen")))
	b.WriteString("\n" + st.Accent.Render(head) + st.Muted.Render(i18n.T("  ←→ Abschnitt · ⏎ alle Zeilen · k KI: was passierte hier?")) + "\n")
	room := max(3, h-used-4)
	var lines []string
	for _, m := range r.Marks {
		if m.Kind == logscan.MarkCmd && !m.Time.Before(from) && m.Time.Before(to) {
			lines = append(lines, st.Muted.Render(m.Time.Local().Format("15:04:05")+" ")+st.AI.Render("◆ "+trunc(i18n.T("du: ")+m.Text, w-12)))
		}
	}
	for _, e := range es {
		style := st.Text
		switch {
		case e.Sev >= logscan.SevErr:
			style = st.Danger
		case e.Sev == logscan.SevWarn:
			style = st.Warn
		}
		src := e.Source
		if src != "" {
			src += ": "
		}
		lines = append(lines, st.Muted.Render(e.Time.Local().Format("15:04:05")+" ")+style.Render(trunc(src+e.Msg, w-12)))
	}
	if len(lines) == 0 {
		lines = append(lines, st.Muted.Render(i18n.T("keine Meldungen in diesem Abschnitt")))
	}
	if len(lines) > room {
		more := len(lines) - room + 1
		lines = append(lines[:room-1], st.Muted.Render(fmt.Sprintf(i18n.T("… %d weitere (⏎)"), more)))
	}
	b.WriteString(strings.Join(lines, "\n"))
}

func markLine(st Styles, m logscan.Mark, w int) string {
	g := markGlyph(m.Kind)
	gs := st.OK
	if m.Kind == logscan.MarkBoot {
		gs = st.Accent
	}
	return "  " + gs.Render(g+" "+m.Time.Local().Format("02.01. 15:04")) + "  " + st.Text.Render(trunc(m.Text, w-20))
}

// sparkInts: wie sparkline, für Zähler.
func sparkInts(v []int) string {
	f := make([]float64, len(v))
	for i, x := range v {
		f[i] = float64(x)
	}
	return sparkline(f)
}

func durShort(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%.0f h", d.Hours())
	case d >= time.Minute:
		return fmt.Sprintf("%.0f min", d.Minutes())
	}
	return fmt.Sprintf("%.0f s", d.Seconds())
}
