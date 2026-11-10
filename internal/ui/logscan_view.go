package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
)

// ---------------------------------------------------------------- Ansicht

func (a *App) logScanKeys() [][2]string {
	l := &a.lscan
	if l.busy {
		return [][2]string{{"x", i18n.T("abbrechen")}, {"Esc", i18n.T("zurück")}}
	}
	switch l.view {
	case lsNews:
		k := [][2]string{}
		if len(a.lsNoise()) > 0 {
			k = append(k, [2]string{"↑↓", i18n.T("wählen")}, [2]string{"u", i18n.T("zurückholen")})
		}
		return append(k, [2]string{"n", i18n.T("Liste")}, [2]string{"Esc", i18n.T("zurück")})
	case lsTime:
		return [][2]string{{"←→", i18n.T("Abschnitt")}, {"⏎", i18n.T("Zeilen")}, {"k", i18n.T("KI: was passierte hier?")}, {"t", i18n.T("Liste")}, {"Esc", i18n.T("zurück")}}
	}
	k := [][2]string{}
	if len(a.lsVisible()) > 0 {
		k = append(k, [2]string{"↑↓", i18n.T("wählen")}, [2]string{"⏎", i18n.T("Zeilen")}, [2]string{"1…4", i18n.T("Prüfbefehl einfügen")},
			[2]string{"k", i18n.T("KI erklärt")}, [2]string{"b", i18n.T("KI-Lagebericht")}, [2]string{"x", i18n.T("ignorieren")})
	}
	return append(k, [2]string{"n", i18n.T("Was ist neu?")}, [2]string{"t", i18n.T("Zeitleiste")}, [2]string{"z", i18n.T("Zeitraum")},
		[2]string{"w", i18n.T("Logwache")}, [2]string{"r", i18n.T("neu lesen")}, [2]string{"c", i18n.T("kopieren")}, [2]string{"Esc", i18n.T("zurück")})
}

func (a *App) viewLogScan() string {
	st := a.s
	l := &a.lscan
	w := max(40, a.w-2)
	h := max(8, a.h-4)
	var b strings.Builder
	r := l.rep
	// Kopf
	title := i18n.T("Log-Blick · ") + l.conn.Alias
	if r != nil && r.System != "" {
		title += " · " + r.System
	}
	title += fmt.Sprintf(i18n.T(" · letzte %s"), hoursText(l.hours))
	head := st.Accent.Render(title)
	if r != nil {
		crit, errs, warn := r.Counts()
		var parts []string
		if crit > 0 {
			parts = append(parts, st.Danger.Render(fmt.Sprintf(i18n.T("%d kritisch"), crit)))
		}
		if errs > 0 {
			parts = append(parts, st.Danger.Render(plural(errs, i18n.T("Fehler"), i18n.T("Fehler"))))
		}
		if warn > 0 {
			parts = append(parts, st.Warn.Render(plural(warn, i18n.T("Warnung"), i18n.T("Warnungen"))))
		}
		if len(parts) == 0 {
			parts = append(parts, st.OK.Render(i18n.T("✓ ruhig")))
		}
		parts = append(parts, st.Muted.Render(fmt.Sprintf(i18n.T("%s gelesen in %s"), plural(r.Lines, i18n.T("Zeile"), i18n.T("Zeilen")), durText(r.Took))))
		head += st.Muted.Render(" · ") + strings.Join(parts, st.Muted.Render(" · "))
	}
	b.WriteString(ansi.Truncate(head, w, "…") + "\n")
	switch {
	case l.busy:
		b.WriteString(st.AI.Render(spinGlyph(time.Now())+" "+l.step) + "\n")
	case r != nil && len(r.Read) > 0:
		b.WriteString(st.Muted.Render(trunc(i18n.T("Gelesen (nur lesend): ")+strings.Join(r.Read, " · "), w)) + "\n")
	default:
		b.WriteString("\n")
	}
	if l.err != "" {
		b.WriteString(lipgloss.NewStyle().Width(w).Render(st.Danger.Render("× "+l.err)) + "\n")
	}
	if r == nil {
		return b.String()
	}
	for _, n := range r.Notes {
		b.WriteString(st.Warn.Render(trunc("▲ "+n, w)) + "\n")
	}
	used := strings.Count(b.String(), "\n")
	switch l.view {
	case lsNews:
		a.viewNews(&b, w)
		return b.String()
	case lsTime:
		b.WriteString("\n")
		a.viewTime(&b, w, h-used-1)
		return b.String()
	}
	if len(r.Ports) > 0 {
		b.WriteString(a.portBar(r, w) + "\n")
		used++
	}
	b.WriteString(a.newsLine(w))
	if n := a.lwatch.unread; n > 0 {
		b.WriteString(st.Danger.Render(fmt.Sprintf(i18n.T("  ▲ Logwache: %d neue Meldungen (w)"), n)))
	}
	b.WriteString("\n\n")
	used += 2
	vis := a.lsVisible()
	if len(vis) == 0 {
		b.WriteString(st.OK.Render(i18n.T("✓ Keine Auffälligkeiten im Zeitraum.")) + "\n")
		if n := len(a.lsNoise()); n > 0 {
			b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("%s ausgeblendet (n zeigt sie)"), plural(n, i18n.T("Muster"), i18n.T("Muster")))) + "\n")
		}
		return b.String()
	}
	cols := lsCols(w)
	b.WriteString(st.Muted.Render(cols.header()) + "\n")
	detail := a.findingDetail(vis[clamp(l.sel, 0, len(vis)-1)], w)
	room := max(3, h-used-2-len(detail))
	l.sel = clamp(l.sel, 0, len(vis)-1)
	if l.sel < l.top {
		l.top = l.sel
	}
	if l.sel >= l.top+room {
		l.top = l.sel - room + 1
	}
	l.top = clamp(l.top, 0, max(0, len(vis)-room))
	for i := l.top; i < min(len(vis), l.top+room); i++ {
		i := i
		row := a.findingRow(vis[i], cols, i == l.sel)
		b.WriteString(a.z.click(row, func(_ tea.Mouse, double bool) tea.Cmd {
			l.sel = i
			if double {
				a.showFindingLines(vis[i])
			}
			return nil
		}) + "\n")
	}
	if rest := len(vis) - (l.top + room); rest > 0 {
		b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("  … %d weitere (↓)"), rest)) + "\n")
	}
	b.WriteString(strings.Join(detail, "\n"))
	return b.String()
}

func hoursText(h int) string {
	if h >= 48 && h%24 == 0 {
		return plural(h/24, i18n.T("Tag"), i18n.T("Tage"))
	}
	return fmt.Sprintf(i18n.T("%d h"), h)
}

func durText(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1f s", d.Seconds())
}

type lsColumns struct{ msg, src int }

func lsCols(w int) lsColumns {
	fixed := 7 + 1 + 6 + 2 + 5 + 2 + logscan.Buckets + 2
	src, tag := 10, 10
	msg := w - fixed - src - tag - 2
	if msg < 20 {
		src = 0
		msg = max(10, w-fixed-tag-1)
	}
	return lsColumns{msg: msg, src: src}
}

func (c lsColumns) header() string {
	h := padRight(i18n.T("Schwere"), 7) + " " + fmt.Sprintf("%6s", i18n.T("Anzahl")) + "  " + padRight(i18n.T("Zeit"), 5) + "  " +
		padRight(i18n.T("Verlauf"), logscan.Buckets) + "  " + padRight(i18n.T("Meldung (gleiche zusammengefasst)"), c.msg)
	if c.src > 0 {
		h += "  " + i18n.T("Quelle")
	}
	return h
}

func (a *App) sevStyle(s logscan.Sev) lipgloss.Style {
	st := a.s
	switch s {
	case logscan.SevCrit, logscan.SevErr:
		return st.Danger
	case logscan.SevWarn:
		return st.Warn
	}
	return st.Muted
}

func sevSym(s logscan.Sev) string {
	switch s {
	case logscan.SevCrit, logscan.SevErr:
		return "●"
	case logscan.SevWarn:
		return "▲"
	}
	return "·"
}

func (a *App) findingRow(f logscan.Finding, c lsColumns, sel bool) string {
	st := a.s
	ss := a.sevStyle(f.Sev)
	last := "–"
	if !f.Last.IsZero() {
		last = f.Last.Local().Format("15:04")
		if time.Since(f.Last) > 20*time.Hour {
			last = f.Last.Local().Format("02.01")
		}
	}
	spark := sparkInts(f.Buckets)
	if f.Last.IsZero() {
		spark = strings.Repeat(" ", logscan.Buckets)
	}
	tag := ""
	switch {
	case f.Status == "new":
		tag = i18n.T("neu")
	case f.Status == "more":
		tag = fmt.Sprintf(i18n.T("×%d häufiger"), f.Factor)
	case f.Trend == "up":
		tag = i18n.T("↑ steigt")
	case f.Trend == "always":
		tag = i18n.T("immer")
	}
	count := fmt.Sprintf("%5d×", f.Count)
	msg := padRight(trunc(f.Title, c.msg), c.msg)
	src := ""
	if c.src > 0 {
		src = "  " + padRight(trunc(f.Source, c.src), c.src)
	}
	if sel {
		plain := padRight(sevSym(f.Sev)+" "+f.Sev.Label(), 7) + " " + count + "  " + padRight(last, 5) + "  " + spark + "  " + msg + src + " " + padRight(tag, 10)
		return st.Selected.Render(plain)
	}
	tagS := st.Muted.Render(tag)
	switch {
	case f.Status == "new":
		tagS = lipgloss.NewStyle().Background(st.T.OK).Foreground(st.T.Bg).Bold(true).Render(" " + tag + " ")
	case f.Status == "more" || f.Trend == "up":
		tagS = st.Warn.Render(tag)
	}
	return ss.Render(padRight(sevSym(f.Sev)+" "+f.Sev.Label(), 7)) + " " + st.Text.Render(count) + "  " + st.Muted.Render(padRight(last, 5)) + "  " +
		ss.UnsetBold().Render(spark) + "  " + st.Text.Render(msg) + st.Muted.Render(src) + " " + tagS
}

// findingDetail: Kasten unter der Liste zum gewählten Muster.
func (a *App) findingDetail(f logscan.Finding, w int) []string {
	st := a.s
	out := []string{st.Muted.Render(strings.Repeat("─", w))}
	when := ""
	if !f.First.IsZero() {
		when = fmt.Sprintf(i18n.T("zuerst %s · zuletzt %s"), f.First.Local().Format("02.01. 15:04"), f.Last.Local().Format("02.01. 15:04"))
	}
	if f.Source != "" {
		when = strings.TrimPrefix(when+" · "+f.Source, " · ")
	}
	out = append(out, ansi.Truncate(st.Accent.Render("▶ "+f.Title)+"  "+st.Muted.Render(when), w, "…"))
	if f.Sample != "" {
		out = append(out, st.Muted.Render(padRight(i18n.T("Beispiel"), 10))+st.Text.Render(trunc(f.Sample, w-10)))
	}
	if f.Hint != "" {
		out = append(out, st.Muted.Render(padRight(i18n.T("Hinweis"), 10))+st.Warn.Render(trunc(f.Hint, w-10)))
	}
	if len(f.Checks) > 0 {
		var chips []string
		used := 10
		for i, c := range f.Checks {
			if i >= 4 {
				break
			}
			lbl := fmt.Sprintf("%d", i+1)
			chip := st.Accent.Render(lbl) + " " + st.Chip.Render(trunc(c, max(12, w/2)))
			cw := lipgloss.Width(chip) + 2
			if used+cw > w && len(chips) > 0 {
				break
			}
			used += cw
			s := a.byID(a.lscan.sid)
			cmd := c
			chips = append(chips, a.z.click(chip, func(tea.Mouse, bool) tea.Cmd {
				if s == nil {
					return nil
				}
				a.scr = scrTerminal
				return a.insertCommand(s, cmd)
			}))
		}
		out = append(out, st.Muted.Render(padRight(i18n.T("Prüfen"), 10))+strings.Join(chips, "  ")+st.Muted.Render(i18n.T("  (nur einfügen)")))
	}
	return out
}

// portBar: Switch-Ports als Leiste (Fehler rot, Warnung gelb, oben grün).
func (a *App) portBar(r *logscan.Report, w int) string {
	st := a.s
	worst := map[string]logscan.Sev{}
	for _, f := range r.Findings {
		if f.Noise {
			continue
		}
		for _, p := range f.Ports {
			if cur, ok := worst[p]; !ok || f.Sev > cur {
				worst[p] = f.Sev
			}
		}
	}
	var b strings.Builder
	shown := 0
	room := w - 30
	for _, p := range r.Ports {
		if shown >= room {
			break
		}
		shown++
		sev, hit := worst[p.Name]
		if !hit {
			sev, hit = worst[shortPort(p.Name)]
		}
		switch {
		case hit && sev >= logscan.SevErr:
			b.WriteString(st.Danger.Render("█"))
		case hit && sev == logscan.SevWarn:
			b.WriteString(st.Warn.Render("█"))
		case p.Up:
			b.WriteString(st.OK.Render("▪"))
		default:
			b.WriteString(st.Muted.Render("▪"))
		}
	}
	label := fmt.Sprintf(i18n.T("Ports (%d) "), len(r.Ports))
	return st.Muted.Render(label) + b.String() + "  " + st.Danger.Render("█") + st.Muted.Render(i18n.T(" Fehler ")) + st.Warn.Render("█") +
		st.Muted.Render(i18n.T(" Warnung ")) + st.OK.Render("▪") + st.Muted.Render(i18n.T(" oben"))
}

// shortPort: „GigabitEthernet1/0/5“ → „1/0/5“ (wie die Meldungen es nennen).
func shortPort(p string) string {
	for i, c := range p {
		if c >= '0' && c <= '9' {
			return p[i:]
		}
	}
	return p
}

// newsLine: eine Zeile zu „Was ist neu?“.
func (a *App) newsLine(w int) string {
	st := a.s
	l := &a.lscan
	if l.prev == nil {
		return st.Muted.Render(trunc(i18n.T("Erste Prüfung dieses Hosts: ab dem nächsten Mal zeigt n, was neu ist."), w))
	}
	var nw, more int
	for _, f := range a.lsVisible() {
		switch f.Status {
		case "new":
			nw++
		case "more":
			more++
		}
	}
	parts := []string{}
	if nw > 0 {
		parts = append(parts, st.OK.Render(plural(nw, i18n.T("neues Muster"), i18n.T("neue Muster"))))
	}
	if more > 0 {
		parts = append(parts, st.Warn.Render(fmt.Sprintf(i18n.T("%d deutlich häufiger"), more)))
	}
	if len(l.gone) > 0 {
		parts = append(parts, st.OK.Render(fmt.Sprintf(i18n.T("%d verschwunden"), len(l.gone))))
	}
	if n := len(a.lsNoise()); n > 0 {
		parts = append(parts, st.Muted.Render(fmt.Sprintf(i18n.T("%d ausgeblendet"), n)))
	}
	if len(parts) == 0 {
		parts = append(parts, st.Muted.Render(i18n.T("nichts Neues")))
	}
	return st.Muted.Render(fmt.Sprintf(i18n.T("Seit dem letzten Blick (%s): "), localTime(l.prev.At))) + strings.Join(parts, st.Muted.Render(" · ")) + st.Muted.Render(i18n.T(" · n zeigt mehr"))
}

// viewNews: Was ist neu? und ausgeblendetes Rauschen.
func (a *App) viewNews(b *strings.Builder, w int) {
	st := a.s
	l := &a.lscan
	b.WriteString("\n")
	if l.prev == nil {
		b.WriteString(st.Muted.Render(i18n.T("Erste Prüfung dieses Hosts: Die Grundlinie ist jetzt gespeichert (nur auf diesem Rechner). Ab dem nächsten Mal steht hier, was neu, häufiger oder verschwunden ist.")) + "\n\n")
	} else {
		b.WriteString(st.Accent.Render(fmt.Sprintf(i18n.T("Seit deinem letzten Blick (%s, letzte %s)"), localTime(l.prev.At), hoursText(l.prev.Hours))) + "\n\n")
		any := false
		for _, f := range a.lsVisible() {
			if f.Status == "" {
				continue
			}
			any = true
			tag := lipgloss.NewStyle().Background(st.T.OK).Foreground(st.T.Bg).Bold(true).Render(i18n.T(" neu "))
			info := fmt.Sprintf(i18n.T("%d× · gab es beim letzten Mal nicht"), f.Count)
			if f.Status == "more" {
				tag = st.Warn.Render(fmt.Sprintf(" ×%d ", f.Factor))
				info = fmt.Sprintf(i18n.T("%d× · %d-mal so häufig wie beim letzten Mal"), f.Count, f.Factor)
			}
			b.WriteString("  " + padRight(tag, 6) + "  " + a.sevStyle(f.Sev).Render(trunc(f.Title, w-50)) + "  " + st.Muted.Render(info) + "\n")
		}
		for _, g := range l.gone {
			any = true
			b.WriteString("  " + st.OK.Render(i18n.T(" ✓ weg")) + "  " + st.Muted.Render(trunc(g.Title, w-40)+fmt.Sprintf(i18n.T("  (vorher %d×)"), g.N)) + "\n")
		}
		if !any {
			b.WriteString(st.Muted.Render(i18n.T("  Nichts Neues: gleiche Muster wie beim letzten Mal.")) + "\n")
		}
	}
	b.WriteString("\n" + st.Accent.Render(i18n.T("Ausgeblendet als Rauschen")) + st.Muted.Render(i18n.T("  (zählt weiter, steht nur nicht in der Liste · x in der Liste blendet aus, X für den ganzen Ordner)")) + "\n\n")
	noise := a.lsNoise()
	if len(noise) == 0 {
		b.WriteString(st.Muted.Render(i18n.T("  nichts ausgeblendet")) + "\n")
		return
	}
	l.newsSel = clamp(l.newsSel, 0, len(noise)-1)
	for i, f := range noise {
		line := fmt.Sprintf("  %s  %s", padRight(trunc(f.Title, w-30), w-30), fmt.Sprintf("%d×", f.Count))
		if i == l.newsSel {
			b.WriteString(st.Selected.Render(padRight(line, w)) + "\n")
		} else {
			b.WriteString(st.Muted.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + st.Muted.Render(i18n.T("Ausgeblendetes geht mit dem Sync auf deine anderen Rechner.")) + "\n")
}
