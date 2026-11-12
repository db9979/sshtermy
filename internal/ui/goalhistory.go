package ui

// Zielverlauf je Verbindung (2026-11-12): jedes Ziel, das erreicht oder mit
// x beendet wird, landet mit seinen Befehlen im Verlauf (nur auf diesem
// Rechner, store/goalruns.go). Im Eingabebild des Ziel-Modus steht er unter
// den Runbooks; ↑↓ wählt, ⏎ startet das Ziel erneut (erreichte Ziele mit
// ihren Befehlen als Vorlage), Strg+D löscht den Eintrag.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vault"
)

const goalHistShown = 5

// recordGoal schreibt das aktuelle Ziel in den Verlauf (einmal je Ziel).
func (a *App) recordGoal(s *session, state string) {
	g := &s.goal
	if g.recorded || strings.TrimSpace(g.goal) == "" || g.undoing {
		return
	}
	if len(g.steps) == 0 && state != "done" {
		return
	}
	g.recorded = true
	var steps []string
	for _, d := range g.steps {
		if d.Skipped || d.File != "" {
			continue
		}
		steps = append(steps, d.Command)
	}
	if len(steps) > 60 {
		steps = steps[len(steps)-60:]
	}
	mode := ""
	switch {
	case g.approved || g.planFirst:
		mode = "plan"
	case g.auto:
		mode = "agent"
	}
	took := 0
	if !g.goalAt.IsZero() {
		took = int(time.Since(g.goalAt).Seconds())
	}
	r := domain.GoalRun{ConnID: s.conn.ID, Goal: g.goal, Mode: mode, Steps: steps, State: state, At: time.Now(), Took: took}
	if state == "done" {
		r.Result = g.doneReason
		if rs := []rune(r.Result); len(rs) > 1000 {
			r.Result = string(rs[:1000]) + " …"
		}
	}
	if err := a.st.AddGoalRun(r); err != nil && !errors.Is(err, vault.ErrLocked) {
		a.flashErr(i18n.T("Zielverlauf: ") + err.Error())
	}
}

// goalHist: Verlaufseinträge, wie das Eingabebild sie zeigt.
func (a *App) goalHist(s *session) []domain.GoalRun {
	h := a.st.GoalRuns(s.conn.ID)
	if len(h) > goalHistShown {
		h = h[:goalHistShown]
	}
	return h
}

// goalPickCount: Runbooks + Verlauf (gemeinsame Auswahl mit ↑↓).
func (a *App) goalPickCount(s *session) (rbs []domain.Runbook, hist []domain.GoalRun) {
	return a.st.Runbooks(s.conn.ID), a.goalHist(s)
}

// goalPickSet: Auswahl i (Runbooks zuerst, dann Verlauf) in die Eingabe.
func (a *App) goalPickSet(s *session, i int) {
	g := &s.goal
	rbs, hist := a.goalPickCount(s)
	g.rbSel = i
	switch {
	case i >= 0 && i < len(rbs):
		g.input.SetValue(rbs[i].Goal)
	case i >= len(rbs) && i-len(rbs) < len(hist):
		g.input.SetValue(hist[i-len(rbs)].Goal)
	default:
		g.rbSel = -1
		g.input.SetValue("")
	}
	g.input.CursorEnd()
}

// startGoalRun: ein Ziel aus dem Verlauf noch einmal.
func (a *App) startGoalRun(s *session, r domain.GoalRun) tea.Cmd {
	if r.State == "done" && len(r.Steps) > 0 {
		rb := domain.Runbook{Goal: r.Goal, Steps: r.Steps}
		cmd := a.startGoalWith(s, r.Goal, &rb)
		s.goal.runbookID = ""
		return cmd
	}
	return a.startGoal(s, r.Goal)
}

func (a *App) deleteGoalRun(s *session, r domain.GoalRun) {
	if err := a.st.DeleteGoalRun(r.ID); err != nil {
		a.flashErr(err.Error())
		return
	}
	s.goal.rbSel = -1
	s.goal.input.SetValue("")
	a.flash(i18n.T("Aus dem Zielverlauf entfernt"))
}

func goalRunMeta(r domain.GoalRun) string {
	mark := "✓"
	switch r.State {
	case "stopped":
		mark = "×"
	case "lost":
		mark = "▲"
	}
	when := r.At.Format("02.01. 15:04")
	if i18n.Lang() == "en" {
		when = r.At.Format("Jan 2 15:04")
	}
	if time.Since(r.At) < 20*time.Hour && r.At.Day() == time.Now().Day() {
		when = r.At.Format("15:04")
	}
	return fmt.Sprintf("%s %s · %d", mark, when, len(r.Steps))
}

// viewGoalHist: „ZULETZT“ unter den Runbooks im Eingabebild.
func (a *App) viewGoalHist(b *strings.Builder, s *session, w, base int) {
	st := a.s
	g := &s.goal
	hist := a.goalHist(s)
	if len(hist) == 0 {
		return
	}
	b.WriteString("\n\n" + st.Muted.Render(i18n.T("ZULETZT")+"  ") + a.hintsIn(s, st.Muted, [2]string{"↑↓", i18n.T("wählen")}, [2]string{"⏎", i18n.T("erneut")}, [2]string{"Strg+D", i18n.T("entfernen")}) + "\n")
	for i, r := range hist {
		meta := goalRunMeta(r)
		line := "◷ " + trunc(r.Goal, max(8, w-lipgloss.Width(meta)-4))
		gap := max(1, w-lipgloss.Width(line)-lipgloss.Width(meta))
		ms := st.Muted
		switch r.State {
		case "done":
			ms = st.OK
		case "stopped", "lost":
			ms = st.Warn
		}
		row := st.Muted.Render("◷ ") + st.Text.Render(strings.TrimPrefix(line, "◷ ")) + strings.Repeat(" ", gap) + ms.Render(meta)
		idx := base + i
		if idx == g.rbSel {
			row = st.Selected.Width(w).Render(line + strings.Repeat(" ", gap) + meta)
		}
		r := r
		b.WriteString(a.z.click(row, func(_ tea.Mouse, double bool) tea.Cmd {
			s.panelFocus = true
			a.goalPickSet(s, idx)
			if double {
				return a.startGoalRun(s, r)
			}
			return g.input.Focus()
		}) + "\n")
	}
}
