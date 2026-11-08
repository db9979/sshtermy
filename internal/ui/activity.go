package ui

// Sitzungsprotokolle: Aktivitätskalender. Jedes Kästchen ist ein Tag,
// je heller, desto mehr Befehle; darunter, zu welcher Tageszeit gearbeitet
// wurde. Klick auf einen Tag springt zu diesem Tag in der Liste.

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

// activityLevels: Farbe für Stufe 0 (nichts) bis 4 (viel).
func (a *App) activityLevels() [5]color.Color {
	t := a.s.T
	return [5]color.Color{t.Surface2, mixColor(t.Surface2, t.OK, 0.3), mixColor(t.Surface2, t.OK, 0.55), mixColor(t.Surface2, t.OK, 0.8), t.OK}
}

// activityCounts: Befehle (mindestens 1 je Sitzung) pro Tag und Sitzungen
// pro Stunde.
func activityCounts(logs []*sessionlog.Log) (perDay map[time.Time]int, perHour [24]int) {
	perDay = map[time.Time]int{}
	for _, l := range logs {
		d := dayStart(l.Started)
		perDay[d] += max(1, len(l.Entries))
		perHour[l.Started.Local().Hour()]++
	}
	return perDay, perHour
}

// activityBlock: Kalender der letzten Wochen (so viele, wie in w passen,
// höchstens 26) mit dem Tag mark hervorgehoben.
func (a *App) activityBlock(rb *strings.Builder, logs []*sessionlog.Log, w int, mark, now time.Time) {
	s := a.s
	if len(logs) == 0 || w < 30 {
		return
	}
	perDay, perHour := activityCounts(logs)
	weeks := min(26, (w-5)/2)
	// Montag der aktuellen Woche, dann zurück
	today := dayStart(now)
	wd := (int(today.Weekday()) + 6) % 7 // Mo = 0
	first := today.AddDate(0, 0, -wd-7*(weeks-1))
	hi := 0
	nDays := 0
	for d, n := range perDay {
		if !d.Before(first) && !d.After(today) {
			hi = max(hi, n)
			nDays++
		}
	}
	lv := a.activityLevels()
	level := func(n int) int {
		if n <= 0 || hi == 0 {
			return 0
		}
		return min(4, 1+(n*4-1)/hi)
	}
	rb.WriteString(" " + s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T("Aktivität")) +
		s.Muted.Render(fmt.Sprintf(i18n.T("  %d Wochen · an %d Tagen gearbeitet"), weeks, nDays)) + "\n")
	// Monatsnamen über der ersten Woche eines Monats
	months := []rune(strings.Repeat(" ", 2*weeks+1))
	lastM := time.Month(0)
	for wk := 0; wk < weeks; wk++ {
		d := first.AddDate(0, 0, 7*wk)
		if m := d.Month(); m != lastM {
			lastM = m
			name := []rune(i18n.T(monthShort[m-1]))
			if pos := 2 * wk; pos+len(name) <= len(months) && (pos == 0 || months[pos-1] == ' ') {
				copy(months[pos:], name)
			}
		}
	}
	rb.WriteString("     " + s.Muted.Render(strings.TrimRight(string(months), " ")) + "\n")
	days := []string{i18n.T("Mo"), "", i18n.T("Mi"), "", i18n.T("Fr"), "", i18n.T("So")}
	for row := 0; row < 7; row++ {
		var line strings.Builder
		line.WriteString(" " + s.Muted.Render(fmt.Sprintf("%-3s ", days[row])))
		for wk := 0; wk < weeks; wk++ {
			d := first.AddDate(0, 0, 7*wk+row)
			if d.After(today) {
				break
			}
			c := lv[level(perDay[d])]
			cell := lipgloss.NewStyle().Foreground(c).Render("■")
			if !mark.IsZero() && sameDay(d, mark) {
				cell = lipgloss.NewStyle().Foreground(s.T.Text).Bold(true).Render("■")
			}
			if n := perDay[d]; n > 0 {
				day := d
				cell = a.z.click(cell, func(tea.Mouse, bool) tea.Cmd { a.jumpToDay(day); return nil })
			}
			line.WriteString(cell + " ")
		}
		rb.WriteString(line.String() + "\n")
	}
	legend := " " + s.Muted.Render(i18n.T("    weniger "))
	for _, c := range lv {
		legend += lipgloss.NewStyle().Foreground(c).Render("■") + " "
	}
	rb.WriteString(legend + s.Muted.Render(i18n.T("mehr")) + "\n")
	// Tageszeit
	hmax := 0
	for _, n := range perHour {
		hmax = max(hmax, n)
	}
	if hmax > 0 && w >= 50 {
		const bars = "▁▂▃▄▅▆▇█"
		r := []rune(bars)
		var hl, axis strings.Builder
		for h, n := range perHour {
			g := " "
			if n > 0 {
				g = string(r[min(7, (n*7+hmax-1)/hmax)])
			}
			hl.WriteString(lipgloss.NewStyle().Foreground(mixColor(s.T.Accent, s.T.AI, float64(n)/float64(hmax))).Render(g + g))
			if h%6 == 0 {
				axis.WriteString(fmt.Sprintf("%02d", h))
			} else {
				axis.WriteString("  ")
			}
		}
		rb.WriteString("\n " + s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T("Tageszeit")) + "\n")
		rb.WriteString(" " + hl.String() + "\n " + s.Muted.Render(axis.String()) + "\n")
	}
	rb.WriteString("\n")
}

var monthShort = []string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}

// jumpToDay: Liste auf den Tag stellen (Ansicht nach Zeit, Filter alle).
func (a *App) jumpToDay(d time.Time) {
	m := &a.logs
	if m.byHost {
		m.byHost = false
	}
	m.filter = lfAll
	for i, r := range a.logRows() {
		if r.kind == lrDay && sameDay(r.day, d) {
			a.selectLog(i)
			return
		}
	}
}
