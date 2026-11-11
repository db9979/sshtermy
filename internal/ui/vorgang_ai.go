package ui

// KI im Vorgang: Fragen über alle Fenster (3), Plan in Bahnen (5),
// Begleiter-Hinweise über Fenster hinweg (6) und Rückblick (8).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

type vgAnswerMsg struct {
	seq  int
	plan bool
	ans  agent.MultiAnswer
	err  error
}

type vgCrossMsg struct {
	from int
	hint agent.CrossHint
	stil bool
	err  error
}

type vgRecapMsg struct {
	r   agent.MultiRecap
	err error
}

// vgAsk: Frage (oder Planauftrag) an die KI mit allen Fenstern.
func (a *App) vgAsk(q string, plan, follow bool) tea.Cmd {
	g := a.vg
	q = strings.TrimSpace(q)
	if g == nil || q == "" {
		return nil
	}
	wins, conns := a.vgWindows()
	if len(wins) == 0 {
		g.err = i18n.T("Kein Fenster im Vorgang, das die KI sehen darf (Tag ki-aus?)")
		return nil
	}
	ag, err := a.agentClientFor(conns...)
	if err != nil {
		g.err = err.Error()
		return nil
	}
	var prev []agent.Turn
	if follow && g.ans != nil {
		prev = append(append(prev, g.history...), agent.Turn{Q: g.question, A: g.ans.Summary + " " + stepsText(g.ans.Steps)})
	}
	g.cancel()
	g.history = prev
	g.question, g.busy, g.err, g.followNext = q, true, "", false
	g.ans, g.stepSel, g.armedIns = nil, 0, -1
	g.seq++
	g.in.Blur()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	g.cancelFn = cancel
	seq, goal := g.seq, g.name
	return func() tea.Msg {
		defer cancel()
		ans, err := ag.MultiAsk(ctx, goal, q, wins, prev, plan)
		return vgAnswerMsg{seq: seq, plan: plan, ans: ans, err: err}
	}
}

func stepsText(st []agent.MultiStep) string {
	var b strings.Builder
	for i, s := range st {
		fmt.Fprintf(&b, "%d) Fenster %d: %s; ", i+1, s.Win, s.Command)
	}
	return b.String()
}

// vgMsg verteilt die asynchronen Ergebnisse des Vorgangs.
func (a *App) vgMsg(msg tea.Msg) (tea.Cmd, bool) {
	g := a.vg
	switch m := msg.(type) {
	case vgAnswerMsg:
		if g == nil || m.seq != g.seq {
			return nil, true
		}
		g.busy = false
		if m.err != nil && len(m.ans.Steps) == 0 && m.ans.Summary == "" {
			g.err = m.err.Error()
			return nil, true
		}
		ans := m.ans
		g.ans = &ans
		g.in.Blur() // Tasten gehören jetzt den Schritten
		g.steps = nil
		for _, st := range ans.Steps {
			g.steps = append(g.steps, vgStep{MultiStep: st})
		}
		g.cur, g.why = 0, ""
		if m.plan {
			g.mode = vgLanes
			a.resizeAll()
		}
		if !g.open {
			a.flash(i18n.T("Vorgang: Antwort der KI ist da (Alt+V)"))
		}
		return nil, true
	case vgCrossMsg:
		if g == nil {
			return nil, true
		}
		g.crossBusy = false
		if m.err != nil || m.stil {
			return nil, true
		}
		h := m.hint
		g.hint = &vgHint{from: m.from, win: h.Win, text: h.Text, cmd: h.Command, danger: len(h.Danger) > 0, at: time.Now()}
		a.companionSpent(h.Tokens)
		if !g.open {
			g.open = true
			a.resizeAll()
		}
		a.flash(i18n.F("Begleiter: Ursache vermutlich in Fenster %d – siehe Vorgang (Alt+V)", h.Win))
		return nil, true
	case vgRecapMsg:
		if g == nil {
			return nil, true
		}
		g.recapBusy = false
		if m.err != nil {
			g.recapErr = m.err.Error()
			return nil, true
		}
		r := m.r
		g.recap, g.recapErr = &r, ""
		return nil, true
	}
	return nil, false
}

// ---------------------------------------------------------------- 6. Begleiter über alle Fenster

type vgCross struct {
	sid int
	f   agent.Failure
}

// vgCrossWanted: soll der Begleiter einen Fehlschlag gegen die anderen
// Fenster prüfen? Nur mit Begleiter an, mindestens zwei Fenstern, ohne
// Sperren des Fensters, im Rahmen des Begleiter-Budgets.
func (a *App) vgCrossWanted(s *session) bool {
	g := a.vg
	if g == nil || !g.active || len(g.members) < 2 || g.member(s.id) == nil {
		return false
	}
	if a.compLevel() == domain.CompanionOff || hasTag(s.conn, "begleiter-aus", "companion-off") {
		return false
	}
	if g.crossBusy || time.Since(g.crossAt) < 20*time.Second || a.companionBudget() != "" {
		return false
	}
	return true
}

func (a *App) vgCrossTick() tea.Cmd {
	g := a.vg
	if g == nil || g.crossDue == nil {
		return nil
	}
	due := g.crossDue
	g.crossDue = nil
	s := a.byID(due.sid)
	if s == nil || !a.vgCrossWanted(s) {
		return nil
	}
	wins, conns := a.vgWindows()
	if len(wins) < 2 {
		return nil
	}
	ag, err := a.agentClientFor(conns...)
	if err != nil {
		return nil
	}
	g.crossBusy, g.crossAt = true, time.Now()
	a.compUse.hits = append(a.compUse.hits, time.Now()) // zählt wie eine Begleiter-Anfrage
	a.compUse.requests++
	from, goal, f := a.winNo(s), g.name, due.f
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		h, still, err := ag.Cross(ctx, from, f, goal, wins)
		return vgCrossMsg{from: from, hint: h, stil: still, err: err}
	}
}

// vgHintInsert: Befehl aus dem Begleiter-Hinweis im genannten Fenster vortippen.
func (a *App) vgHintInsert() tea.Cmd {
	g := a.vg
	if g == nil || g.hint == nil || g.hint.cmd == "" {
		return nil
	}
	s := a.byWin(g.hint.win)
	if s == nil {
		a.flash(i18n.T("Das Fenster ist nicht mehr offen"))
		return nil
	}
	if g.hint.danger && g.armedIns != -2 {
		g.armedIns = -2
		a.flash(i18n.T("Gefahr: nochmal ⏎ zum Vortippen, ausgeführt wird erst mit ⏎ im Fenster"))
		return nil
	}
	g.armedIns = -1
	if a.vgTypeInto(s, g.hint.cmd) {
		g.hint = nil
	}
	return nil
}

// ---------------------------------------------------------------- 8. Rückblick über den Vorgang

func (a *App) vgOpenRecap() tea.Cmd {
	g := a.vg
	if g == nil {
		return nil
	}
	g.mode = vgRecap
	a.resizeAll()
	if len(g.events) == 0 {
		g.recapErr = i18n.T("Noch keine Befehle in den Fenstern des Vorgangs")
		return nil
	}
	wins, conns := a.vgWindows()
	ag, err := a.agentClientFor(conns...)
	if err != nil {
		g.recapErr = err.Error()
		return nil
	}
	g.recapBusy, g.recapErr, g.recap = true, "", nil
	evs := append([]agent.MultiEvent(nil), g.events...)
	goal := g.name
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		r, err := ag.RecapMulti(ctx, goal, evs, wins)
		return vgRecapMsg{r, err}
	}
}

// vgRecapText: Rückblick als Markdown (Change-Protokoll).
func (a *App) vgRecapText() string {
	g := a.vg
	var b strings.Builder
	name := orDefault(g.name, i18n.T("Vorgang"))
	end := time.Now()
	if n := len(g.events); n > 0 {
		end = g.events[n-1].At
	}
	fmt.Fprintf(&b, "# %s\n\n%s – %s · %s\n\n", name, g.started.Format("2006-01-02 15:04"), end.Format("15:04"),
		plural(len(g.events), i18n.T("Befehl"), i18n.T("Befehle")))
	b.WriteString("## " + i18n.T("Fenster") + "\n\n")
	for _, s := range a.vgSessions() {
		fmt.Fprintf(&b, "- %d %s (%s@%s)\n", a.winNo(s), s.tabName(), s.conn.Username, s.conn.Host)
	}
	if r := g.recap; r != nil {
		b.WriteString("\n## " + i18n.T("Zusammenfassung") + "\n\n")
		for _, p := range r.Points {
			b.WriteString("- " + p + "\n")
		}
	}
	b.WriteString("\n## " + i18n.T("Ablauf") + "\n\n")
	for _, e := range g.events {
		mark := ""
		if e.Failed {
			mark = " ×"
		}
		fmt.Fprintf(&b, "- %s · %s %d · `%s`%s\n", e.At.Format("15:04"), i18n.T("Fenster"), e.Win, e.Command, mark)
	}
	if r := g.recap; r != nil {
		if len(r.Undo) > 0 {
			b.WriteString("\n## " + i18n.T("Zurück, falls nötig") + "\n\n")
			for _, u := range r.Undo {
				fmt.Fprintf(&b, "- %s %d: `%s`\n", i18n.T("Fenster"), u.Win, u.Command)
			}
		}
		if len(r.Open) > 0 {
			b.WriteString("\n## " + i18n.T("Offen") + "\n\n")
			for _, o := range r.Open {
				b.WriteString("- " + o + "\n")
			}
		}
	}
	return agent.Redact(b.String())
}

// vgSaveRecap: Change-Protokoll als Markdown-Datei im Download-Ordner.
func (a *App) vgSaveRecap() {
	dir := exportDir()
	name := "sshtermy-vorgang-" + time.Now().Format("2006-01-02-1504") + ".md"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(a.vgRecapText()), 0o600); err != nil {
		a.flashErr(i18n.T("Nicht gespeichert: ") + err.Error())
		return
	}
	a.flash(i18n.T("Change-Protokoll gespeichert: ") + p)
}

// vgRecapRunbook: Ablauf als Runbook speichern. Jeder Schritt nennt sein
// Fenster als Kommentar, damit beim nächsten Mal klar ist, wohin er gehört.
func (a *App) vgRecapRunbook() {
	g := a.vg
	var steps []string
	for _, e := range g.events {
		if e.Failed {
			continue
		}
		name := ""
		if s := a.byWin(e.Win); s != nil {
			name = s.tabName()
		}
		steps = append(steps, fmt.Sprintf("%s  # %s %d %s", e.Command, i18n.T("Fenster"), e.Win, name))
	}
	if len(steps) == 0 {
		a.flash(i18n.T("Keine erfolgreichen Befehle zum Speichern"))
		return
	}
	goal := orDefault(g.name, i18n.T("Vorgang über mehrere Fenster"))
	rb := domain.Runbook{Name: goal, Goal: goal, Steps: steps}
	if _, err := a.st.UpsertRunbook(rb); err != nil {
		a.flashErr(i18n.T("Runbook nicht gespeichert: ") + err.Error())
		return
	}
	a.flash(i18n.T("Als Runbook gespeichert: ") + trunc(goal, 40))
}
