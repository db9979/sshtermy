package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// ---------------------------------------------------------------- Plan-Modus
//
// „Erst planen“ im Ziel-Panel: die KI erkundet nur mit lesenden Befehlen und
// schreibt dann einen vollständigen Plan (Schritte, Dateien, Risiken,
// Prüfung). Du streichst oder änderst Schritte, lässt neu planen und gibst
// ihn frei; ausgeführt wird er dann im Agent-Modus (Stufe nach Wahl) oder
// Schritt für Schritt.

// planRunbookID markiert in startGoalWith einen freigegebenen Plan.
const planRunbookID = "\x00plan"

// planReview: Plan liegt vor und wartet auf Freigabe.
func (g *goalState) planReview() bool { return g.fp != nil && g.active }

// planStepMsg: Antwort des Modells während der Erkundung.
func (a *App) planStepMsg(s *session, st agent.Step) tea.Cmd {
	g := &s.goal
	if st.Full != nil {
		g.fp, g.fpPrev, g.planning = st.Full, nil, false
		g.planOff = make([]bool, len(st.Full.Steps))
		g.planSel, g.cur = 0, nil
		a.flash(i18n.T("Plan fertig: bitte prüfen und freigeben"))
		return nil
	}
	if st.File != nil || st.Command == "" {
		return nil
	}
	g.explored++
	if !safety.ReadOnly(st.Command, a.safetyKind(s)) || len(st.Danger) > 0 {
		g.steps = append(g.steps, agent.DoneStep{Command: st.Command, Skipped: true,
			ExitHint: i18n.T("nicht ausgeführt: beim Planen nur lesende Befehle")})
		return a.requestStep(s)
	}
	if why := safety.Sensitive(st.Command); why != "" {
		g.steps = append(g.steps, agent.DoneStep{Command: st.Command, Skipped: true,
			ExitHint: i18n.T("nicht ausgeführt: ") + why + i18n.T(" (Ausgabe ginge an die KI)")})
		return a.requestStep(s)
	}
	if s.state != stConnected || s.term == nil {
		g.err = i18n.T("keine Verbindung: erst neu verbinden")
		return nil
	}
	g.cur = &st
	a.runProposal(s, st.Command, false)
	return nil
}

// planOn: aktive Schritte des Plans.
func (g *goalState) planOn() []agent.PlanStep {
	var out []agent.PlanStep
	for i, st := range g.fp.Steps {
		if i < len(g.planOff) && g.planOff[i] {
			continue
		}
		out = append(out, st)
	}
	return out
}

// planRunbook: der Plan als Runbook (Befehle + Dateien, Arbeitsordner als Platzhalter).
func (g *goalState) planRunbook() domain.Runbook {
	rb := domain.Runbook{Name: g.goal, Goal: g.goal}
	for _, st := range g.planOn() {
		if st.File != nil {
			rb.Files = append(rb.Files, domain.RunbookFile{Name: st.File.Name, Content: st.File.Content})
			rb.Steps = append(rb.Steps, "Datei "+domain.AgentDirToken+"/"+st.File.Name+" anlegen") // Text fürs Modell
			continue
		}
		cmd := st.Command
		if g.runID != "" {
			cmd = strings.ReplaceAll(cmd, g.agentDir(), domain.AgentDirToken)
		}
		rb.Steps = append(rb.Steps, cmd)
	}
	return rb
}

// runPlan: Plan freigeben und ausführen (Agent oder Schritt für Schritt).
func (a *App) runPlan(s *session) tea.Cmd {
	g := &s.goal
	if len(g.planOn()) == 0 {
		a.flash(i18n.T("Der Plan hat keine Schritte"))
		return nil
	}
	rb := g.planRunbook()
	rb.ID = planRunbookID
	pf := g.planFirst
	g.planFirst = false
	cmd := a.startGoalWith(s, g.goal, &rb)
	s.goal.planFirst, s.goal.runbookID = pf, ""
	return cmd
}

// replan: Anweisung an das Modell, den Plan zu überarbeiten.
func (a *App) replan(s *session) tea.Cmd {
	g := &s.goal
	a.modal = inputModal(i18n.T("Plan ändern"), i18n.T("Was soll am Plan anders werden? Die KI plant danach neu."), "", func(x string) tea.Cmd {
		x = strings.TrimSpace(x)
		if x == "" {
			return nil
		}
		var off []string
		for i, o := range g.planOff {
			if o {
				off = append(off, fmt.Sprint(i+1))
			}
		}
		if len(off) > 0 {
			x += i18n.T(" (gestrichene Schritte: ") + strings.Join(off, ", ") + ")"
		}
		g.answers = append(g.answers, i18n.T("Änderungswunsch am Plan: ")+x)
		g.fpPrev, g.fp, g.planning = g.fp, nil, true
		return a.requestStep(s)
	})
	return nil
}

// editPlanStep: Befehl eines Schritts ändern.
func (a *App) editPlanStep(s *session) {
	g := &s.goal
	if g.planSel < 0 || g.planSel >= len(g.fp.Steps) || g.fp.Steps[g.planSel].File != nil {
		return
	}
	i := g.planSel
	a.modal = inputModal(i18n.T("Schritt ändern"), fmt.Sprintf(i18n.T("Befehl für Schritt %d"), i+1), g.fp.Steps[i].Command, func(x string) tea.Cmd {
		x = strings.TrimSpace(x)
		if x == "" || g.fp == nil || i >= len(g.fp.Steps) {
			return nil
		}
		fresh := agent.ParseStepCommand(x, a.safetyKind(s))
		fresh.Reason = i18n.T("von dir geändert")
		g.fp.Steps[i].Suggestion = fresh
		return nil
	})
}

// planView: eine Datei des Plans ansehen.
func (a *App) planView(s *session) {
	g := &s.goal
	if g.planSel < 0 || g.planSel >= len(g.fp.Steps) || g.fp.Steps[g.planSel].File == nil {
		return
	}
	f := g.fp.Steps[g.planSel].File
	var lines []string
	for i, l := range strings.Split(strings.TrimRight(f.Content, "\n"), "\n") {
		lines = append(lines, fmt.Sprintf("%3d  %s", i+1, l))
	}
	a.modal = textModal(f.Name, lines)
}

// planKey: Tasten, solange ein Plan zur Freigabe vorliegt.
func (a *App) planKey(s *session, key string) tea.Cmd {
	g := &s.goal
	n := len(g.fp.Steps)
	switch key {
	case "up", "k":
		g.planSel = max(0, g.planSel-1)
	case "down", "j":
		g.planSel = min(n-1, g.planSel+1)
	case "space", "d", "delete":
		if g.planSel >= 0 && g.planSel < n {
			g.planOff[g.planSel] = !g.planOff[g.planSel]
		}
	case "e":
		a.editPlanStep(s)
	case "v":
		a.planView(s)
	case "?":
		return a.replan(s)
	case "tab":
		g.cycleExec()
	case "enter":
		return a.runPlan(s)
	case "x":
		g.cancel()
		g.active, g.fp, g.busy, g.cur, g.rbSel = false, nil, false, nil, -1
		g.input.SetValue(g.goal)
		return g.input.Focus()
	}
	return nil
}

// cycleExec: Tab im Plan: wie soll er laufen?
func (g *goalState) cycleExec() {
	switch {
	case !g.auto:
		g.auto, g.level = true, lvlCareful
	case g.level < lvlFull:
		g.level++
	default:
		g.auto = false
	}
}

func execName(g *goalState) string {
	if !g.auto {
		return i18n.T("Schritt für Schritt")
	}
	return i18n.T("Agent · ") + levelName(g.level)
}

// viewPlan: der Plan zur Freigabe.
func (a *App) viewPlan(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	p := g.fp
	on := len(g.planOn())
	b.WriteString(st.AI.Bold(true).Render(i18n.T("◇ Plan")) + "  " + st.Muted.Render(fmt.Sprintf(i18n.T("%d von %d Schritten aktiv"), on, len(p.Steps))) + "\n\n")
	b.WriteString(wrap(st.Text.Bold(true).Render(g.goal), w) + "\n")
	if p.Summary != "" {
		b.WriteString(wrap(st.Text.Render(p.Summary), w) + "\n")
	}
	b.WriteString("\n" + st.Muted.Render(i18n.T("SCHRITTE")) + "\n")
	for i, ps := range p.Steps {
		num := st.Muted.Render(fmt.Sprintf("%2d ", i+1))
		label := ps.Command
		mark, style := "  ", st.Text
		switch {
		case ps.File != nil:
			label = fmt.Sprintf(i18n.T("Datei %s (%d Zeilen)"), ps.File.Name, lineCount(ps.File.Content))
			mark, style = st.AI.Render("▪ "), st.AI
		}
		if len(ps.Danger) > 0 {
			mark = st.Danger.Render("▲ ")
		} else if ps.Elevated {
			mark = st.Warn.Render("↑ ")
		}
		text := trunc(label, max(4, w-6))
		line := num + mark + style.Render(text)
		if g.planOff[i] {
			line = num + st.Muted.Render("− ") + st.Muted.Strikethrough(true).Render(text)
		}
		if i == g.planSel {
			line = st.Selected.Width(w).Render(fmt.Sprintf("%2d ", i+1) + map[bool]string{true: "− ", false: "  "}[g.planOff[i]] + text)
		}
		i := i
		b.WriteString(a.z.click(line, func(_ tea.Mouse, double bool) tea.Cmd {
			s.panelFocus, g.planSel = true, i
			if double {
				g.planOff[i] = !g.planOff[i]
			}
			return nil
		}) + "\n")
		if i == g.planSel && ps.File == nil && text != label { // ausgewählt: ganzer Befehl
			b.WriteString(lipglossIndent(wrap(st.Code.Render(label), max(4, w-5)), 5) + "\n")
		}
		if i == g.planSel {
			if len(ps.Danger) > 0 {
				b.WriteString("     " + st.Danger.Render(trunc(i18n.T("Gefahr: ")+ps.Danger[0].Reason, max(4, w-5))) + "\n")
			}
			if ps.Reason != "" {
				b.WriteString(lipglossIndent(wrap(st.Muted.Render(ps.Reason), w-5), 5) + "\n")
			}
		}
	}
	b.WriteString("\n" + st.Muted.Render(i18n.T("AUSFÜHREN ALS")+"  ") + st.AI.Bold(true).Render(execName(g)) + st.Muted.Render("  (Tab)") + "\n")
	b.WriteString(a.hintsIn(s, st.Muted, [2]string{"⏎", i18n.T("freigeben und ausführen")}) + "\n")
	if len(p.Risks) > 0 {
		b.WriteString("\n" + st.Muted.Render(i18n.T("RISIKEN")) + "\n")
		for _, r := range p.Risks {
			b.WriteString(bullet(st.Warn.Render("▲ "), st.Text.Render(r), w) + "\n")
		}
	}
	if len(p.Checks) > 0 {
		b.WriteString("\n" + st.Muted.Render(i18n.T("PRÜFUNG")) + "\n")
		for _, c := range p.Checks {
			b.WriteString(bullet(st.OK.Render("✓ "), st.Text.Render(c), w) + "\n")
		}
	}
}

// bullet: Aufzählungspunkt mit eingerücktem Umbruch.
func bullet(mark, text string, w int) string {
	return mark + strings.TrimPrefix(lipglossIndent(wrap(text, max(4, w-2)), 2), "  ")
}

// viewPlanning: Kopf während der Erkundung.
func (a *App) viewPlanning(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	b.WriteString(st.AI.Bold(true).Render(i18n.T("◇ Plan-Modus ")) + st.Warn.Render(i18n.T("erkundet")) + "\n")
	b.WriteString(st.Muted.Render(trunc(fmt.Sprintf(i18n.T("nur lesende Befehle · %d von höchstens %d"), g.explored, agent.MaxExplore), w)) + "\n\n")
}

func lipglossIndent(s string, n int) string {
	if n <= 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func (a *App) planPanelKeys(s *session) [][2]string {
	g := &s.goal
	if g.planReview() {
		k := [][2]string{{"⏎", i18n.T("freigeben")}, {"↑↓", i18n.T("Schritt")}, {i18n.T("Leertaste"), i18n.T("streichen")}}
		if g.planSel >= 0 && g.planSel < len(g.fp.Steps) && g.fp.Steps[g.planSel].File != nil {
			k = append(k, [2]string{"v", i18n.T("Datei ansehen")})
		} else {
			k = append(k, [2]string{"e", i18n.T("ändern")})
		}
		return append(k, [2]string{"?", i18n.T("neu planen")}, [2]string{"Tab", i18n.T("Ausführung")}, [2]string{"Strg+S", "Runbook"}, [2]string{"x", i18n.T("verwerfen")})
	}
	if g.planning && g.active {
		return [][2]string{{"x", i18n.T("abbrechen")}, {"Esc", "Terminal"}}
	}
	return nil
}

// planHasCmd: steht der (ändernde) Befehl so im freigegebenen Plan?
func (g *goalState) planHasCmd(cmd string) bool {
	norm := func(x string) string { return strings.Join(strings.Fields(x), " ") }
	c := norm(cmd)
	for _, r := range g.recipe {
		if norm(r) == c {
			return true
		}
	}
	return false
}

// planHasFile: sieht der freigegebene Plan diese Datei vor?
func (g *goalState) planHasFile(name string) bool {
	for _, f := range g.recipeFiles {
		if f.Name == name {
			return true
		}
	}
	return false
}
