package ui

// Ziel-, Agent- und Plan-Modus: Fortschritt als Zeitleiste. Erledigte
// Schritte mit ✓ und Dauer, darunter die letzte Zeile der Ausgabe; der
// laufende Schritt mit Spinner und Lichtlauf, geplante Schritte hohl. Die
// Linie links verbindet alles und ist bis zum laufenden Schritt farbig.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// stepTimes: Fertig-Zeitpunkt je Schritt (neue Schritte bekommen „jetzt“).
func (g *goalState) stepTimes(now time.Time) []time.Time {
	if len(g.stepAt) > len(g.steps) {
		g.stepAt = g.stepAt[:len(g.steps)]
	}
	for len(g.stepAt) < len(g.steps) {
		g.stepAt = append(g.stepAt, now)
	}
	return g.stepAt
}

// lastLine: letzte nicht leere Zeile einer Ausgabe.
func lastLine(out string) string {
	ls := strings.Split(strings.TrimRight(out, "\n \t"), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(ls[i]); t != "" {
			return t
		}
	}
	return ""
}

func (a *App) viewTimeline(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	now := time.Now()
	at := g.stepTimes(now)
	line := func(c lipgloss.Style) string { return c.Render("│") + " " }
	done := lipgloss.NewStyle().Foreground(mixColor(st.T.OK, st.T.Bg, 0.35))
	dim := lipgloss.NewStyle().Foreground(st.T.Border)
	prev := g.started
	running := g.waiting || g.manualPending || g.busy || g.writing
	for i, d := range g.steps {
		mark, ms := "✓", st.OK
		switch {
		case d.Skipped:
			mark, ms = "»", st.Muted
		case d.File != "" && d.ExitHint == "":
			mark, ms = "▪", st.AI
		case d.ExitHint != "" || (d.Exit != nil && *d.Exit != 0):
			mark, ms = "×", st.Danger
		}
		took := ""
		if !prev.IsZero() && i < len(at) && !d.Skipped {
			if dt := at[i].Sub(prev); dt > 0 && dt < time.Hour {
				took = fmtLatency(dt)
			}
		}
		if i < len(at) {
			prev = at[i]
		}
		cmd := trunc(d.Command, max(4, w-4-len([]rune(took))-1))
		head := ms.Bold(true).Render(mark) + " " + st.Text.Render(cmd)
		if took != "" {
			head += strings.Repeat(" ", max(1, w-lipgloss.Width(head)-len([]rune(took)))) + st.Muted.Render(took)
		}
		b.WriteString(head + "\n")
		last := i == len(g.steps)-1 && !running && len(g.plan) == 0
		conn := done
		if last {
			conn = dim
		}
		var sub string
		switch {
		case d.ExitHint != "":
			sub = st.Danger.Render("→ " + trunc(d.ExitHint, max(4, w-4)))
		case d.Exit != nil && *d.Exit != 0:
			sub = st.Danger.Render(fmt.Sprintf(i18n.T("→ Exit %d"), *d.Exit))
			if l := lastLine(d.Output); l != "" {
				sub += st.Muted.Render(" · " + trunc(l, max(4, w-14)))
			}
		case d.Output != "":
			if l := lastLine(d.Output); l != "" {
				sub = st.Muted.Render("→ " + trunc(l, max(4, w-4)))
			}
		}
		if sub != "" && !last {
			b.WriteString(line(conn) + sub + "\n")
		} else if sub != "" {
			b.WriteString("  " + sub + "\n")
		}
	}
	// laufender Schritt
	switch {
	case g.waiting:
		b.WriteString(st.Warn.Bold(true).Render(spinOr(now, "◌")) + " " + st.Warn.Render(i18n.T("läuft … ")) +
			st.Muted.Render(fmt.Sprintf("%ds", int(now.Sub(g.waitSince).Seconds()))) + "\n")
		b.WriteString(line(dim) + a.sweepBar(0, max(8, min(w-2, 40)), now, true) + "\n")
	case g.manualPending:
		b.WriteString(st.Warn.Render(i18n.T("◌ eingefügt, drück im Terminal ⏎")) + "\n")
	}
	// geplante Schritte
	if len(g.plan) > 0 && !g.done {
		if len(g.steps) > 0 || running {
			b.WriteString(line(dim) + "\n")
		}
		for i, p := range g.plan {
			b.WriteString(dim.Render("○") + " " + st.Muted.Render(trunc(p, w-3)) + "\n")
			if i < len(g.plan)-1 {
				b.WriteString(line(dim) + "\n")
			}
		}
	}
}

// spinOr: Spinner, wenn Bewegung an ist, sonst das feste Zeichen.
func spinOr(now time.Time, still string) string {
	if motionLevel() > 0 {
		return spinGlyph(now)
	}
	return still
}
