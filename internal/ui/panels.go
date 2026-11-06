package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

const maxGoalSteps = 40

// ---------------------------------------------------------------- Zustände

// goalState: Ziel-Modus. Das Modell schlägt immer genau EINEN Befehl vor;
// ausgeführt wird erst nach Bestätigung (⏎), rote Befehle nach zweitem ⏎.
type goalState struct {
	input, edit, answer aiInput
	inited              bool
	goal                string
	active, done        bool
	doneReason, err     string
	steps               []agent.DoneStep
	plan                []string
	cur                 *agent.Step
	armed, editing      bool
	waiting             bool // Befehl läuft, wir warten auf den Prompt
	waitSince           time.Time
	manualPending       bool // "i": eingefügt, wartet auf Enter des Nutzers
	busy                bool
	seq                 int
	cancelFn            context.CancelFunc
	answers             []string
	times               []time.Time
	recipe              []string // Vorlage aus einem Runbook
	runbookID           string
	rbSel               int // gewähltes Runbook in der Liste (-1 = keins)
}

func (g *goalState) cancel() {
	if g.cancelFn != nil {
		g.cancelFn()
		g.cancelFn = nil
	}
}

func (g *goalState) startWait() { g.waiting, g.waitSince = true, time.Now() }

type askState struct {
	input     aiInput
	inited    bool
	question  string
	answer    strings.Builder
	busy      bool
	seq       int
	cancelFn  context.CancelFunc
	err       string
	sel       int  // gewählte Befehlskarte (aicards.go)
	armed     int  // Karte, deren gefährlicher Befehl auf den zweiten Druck wartet (-1 = keine)
	collapsed bool // Erklärung eingeklappt
}

func (q *askState) cancel() {
	if q.cancelFn != nil {
		q.cancelFn()
		q.cancelFn = nil
	}
}

// Nachrichten
type goalStepMsg struct {
	sid, seq int
	step     agent.Step
	err      error
}
type askDeltaMsg struct {
	sid, seq int
	text     string
}
type askDoneMsg struct {
	sid, seq int
	err      error
}

func (a *App) panelOpened(s *session) tea.Cmd {
	switch s.panel {
	case panelGoal:
		if !s.goal.inited {
			s.goal.input = newAIInput(i18n.T("z. B. Installiere Docker und richte Nginx als Reverse Proxy ein"))
			s.goal.edit, s.goal.answer = newCommandInput(), newAIInput("")
			s.goal.inited, s.goal.rbSel = true, -1
		}
		if !s.goal.active {
			return s.goal.input.Focus()
		}
	case panelHost:
		return a.hostOpened(s)
	case panelAsk:
		if !s.ask.inited {
			s.ask.input = newAIInput(i18n.T("Frage, z. B. Warum startet nginx nicht?"))
			s.ask.inited = true
		}
		return s.ask.input.Focus()
	}
	return nil
}

// sessionMsg verteilt asynchrone Ergebnisse an ihre Sitzung.
func (a *App) sessionMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case ghostTickMsg:
		if s := a.byID(m.sid); s != nil {
			return a.fireGhost(s, m.seq), true
		}
		return nil, true
	case ghostMsg:
		if s := a.byID(m.sid); s != nil && m.seq == s.ghostSeq {
			s.ghost, s.ghostFor = m.rest, m.typed
		}
		return nil, true
	case followMsg:
		if s := a.byID(m.sid); s != nil && m.seq == s.chipSeq {
			s.chips, s.chipArmed = m.chips, -1
		}
		return nil, true
	case goalStepMsg:
		s := a.byID(m.sid)
		if s == nil || m.seq != s.goal.seq {
			return nil, true
		}
		g := &s.goal
		g.busy = false
		if m.err != nil {
			g.err = m.err.Error()
			return nil, true
		}
		g.err = ""
		switch {
		case m.step.Done:
			g.done, g.active, g.doneReason, g.cur = true, false, m.step.Reason, nil
		case m.step.Question != "":
			st := m.step
			g.cur = &st
			g.answer.SetValue("")
			return g.answer.Focus(), true
		default:
			st := m.step
			g.cur, g.armed = &st, false
			if len(st.Plan) > 0 {
				g.plan = st.Plan
			}
		}
		return nil, true
	case askDeltaMsg:
		if s := a.byID(m.sid); s != nil && m.seq == s.ask.seq {
			s.ask.answer.WriteString(m.text)
		}
		return a.listen(), true
	case askDoneMsg:
		if s := a.byID(m.sid); s != nil && m.seq == s.ask.seq {
			s.ask.busy = false
			if m.err != nil {
				s.ask.err = m.err.Error()
			}
		}
		return nil, true
	}
	return nil, false
}

// ---------------------------------------------------------------- Ziel-Modus

func (a *App) requestStep(s *session) tea.Cmd {
	g := &s.goal
	cfg := a.st.LLM()
	limit := cfg.GoalStepsMin
	if limit <= 0 {
		limit = 6
	}
	now := time.Now()
	var recent []time.Time
	for _, t := range g.times {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	g.times = recent
	if len(recent) >= limit {
		g.err = fmt.Sprintf(i18n.T("Ratenlimit: höchstens %d Schritte pro Minute. r = erneut versuchen"), limit)
		return nil
	}
	if len(g.steps) >= maxGoalSteps {
		g.err = i18n.T("Maximal 40 Schritte pro Ziel. Ziel neu setzen (x).")
		return nil
	}
	ag, err := a.agentClient()
	if err != nil {
		g.err = err.Error()
		return nil
	}
	g.times = append(g.times, now)
	g.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	g.cancelFn = cancel
	g.seq++
	g.busy, g.cur, g.err = true, nil, ""
	sid, seq := s.id, g.seq
	goal, hist, ans := g.goal, append([]agent.DoneStep(nil), g.steps...), append([]string(nil), g.answers...)
	recipe := g.recipe
	c := a.agentContext(s, true)
	return func() tea.Msg {
		defer cancel()
		st, err := ag.NextStep(ctx, goal, recipe, hist, ans, c)
		return goalStepMsg{sid, seq, st, err}
	}
}

// goalTick: läuft ein bestätigter Befehl, wird der nächste Schritt erst
// angefragt, wenn die Ausgabe ruht und wieder ein Prompt steht.
func (a *App) goalTick(s *session) tea.Cmd {
	g := &s.goal
	if !g.active || !g.waiting || s.term == nil {
		return nil
	}
	quiet := time.Since(s.lastOutput) > 1500*time.Millisecond && time.Since(g.waitSince) > time.Second
	if quiet && s.term.AtIdlePrompt() {
		g.waiting = false
		return a.requestStep(s)
	}
	return nil
}

func (a *App) runProposal(s *session, cmd string, insertOnly bool) {
	g := &s.goal
	g.steps = append(g.steps, agent.DoneStep{Command: cmd})
	s.history = append(s.history, cmd)
	g.cur, g.armed, g.editing = nil, false, false
	if insertOnly {
		s.term.SendText(cmd) // Dry-Run: steht in der Zeile, Nutzer drückt selbst Enter
		g.manualPending = true
		s.panelFocus = false
		return
	}
	a.beginCommand(s, cmd)
	s.term.SendText(cmd + "\r")
	a.profileCommand(s, cmd)
	g.startWait()
}

// ---------------------------------------------------------------- Tasten im Panel

func (a *App) panelMsg(s *session, msg tea.Msg) tea.Cmd {
	k, isKey := msg.(tea.KeyPressMsg)
	if isKey && k.String() == "esc" {
		s.panelFocus = false
		if s.panel == panelTunnels || s.panel == panelHost || (s.panel == panelAsk && !s.ask.busy && s.ask.answer.Len() == 0) {
			s.panel = panelNone
			a.resize(s)
		}
		return nil
	}
	switch s.panel {
	case panelGoal:
		return a.goalKey(s, msg)
	case panelAsk:
		return a.askKey(s, msg)
	case panelTunnels:
		if isKey {
			return a.tunnelKey(s, k)
		}
	case panelHost:
		if isKey {
			return a.hostKey(s, k)
		}
	}
	return nil
}

func (a *App) goalKey(s *session, msg tea.Msg) tea.Cmd {
	g := &s.goal
	k, isKey := msg.(tea.KeyPressMsg)
	key := ""
	if isKey {
		key = k.String()
	}
	if key == "ctrl+s" {
		a.saveRunbook(s)
		return nil
	}
	// 1) noch kein Ziel: Eingabe, darunter die gespeicherten Runbooks
	if !g.active && g.done {
		if key == "enter" || key == "x" {
			g.done, g.rbSel, g.runbookID, g.recipe = false, -1, "", nil
			g.input.SetValue("")
			return g.input.Focus()
		}
		return nil
	}
	if !g.active {
		rbs := a.st.Runbooks(s.conn.ID)
		switch key {
		case "up", "down":
			if len(rbs) == 0 {
				return nil
			}
			if key == "down" {
				g.rbSel = min(len(rbs)-1, g.rbSel+1)
			} else {
				g.rbSel = max(-1, g.rbSel-1)
			}
			if g.rbSel >= 0 {
				g.input.SetValue(rbs[g.rbSel].Goal)
				g.input.CursorEnd()
			} else {
				g.input.SetValue("")
			}
			return nil
		case "ctrl+d":
			if g.rbSel >= 0 && g.rbSel < len(rbs) {
				rb := rbs[g.rbSel]
				a.modal = confirmModal(i18n.T("Runbook löschen?"), "„"+rb.Name+i18n.T("“ löschen?"), func() tea.Cmd {
					_ = a.st.DeleteRunbook(rb.ID)
					g.rbSel = -1
					g.input.SetValue("")
					return nil
				})
			}
			return nil
		case "enter":
			text := strings.TrimSpace(g.input.Value())
			if text == "" {
				return nil
			}
			if g.rbSel >= 0 && g.rbSel < len(rbs) && rbs[g.rbSel].Goal == text {
				return a.startRunbook(s, rbs[g.rbSel])
			}
			return a.startGoal(s, text)
		}
		var cmd tea.Cmd
		g.input, cmd = g.input.Update(msg)
		return cmd
	}
	// 2) Rückfrage des Modells beantworten
	if g.cur != nil && g.cur.Question != "" {
		if key == "enter" && strings.TrimSpace(g.answer.Value()) != "" {
			g.answers = append(g.answers, g.cur.Question+" → "+strings.TrimSpace(g.answer.Value()))
			g.answer.Blur()
			return a.requestStep(s)
		}
		if key == "x" && g.answer.Value() == "" {
			g.active = false
			return nil
		}
		var cmd tea.Cmd
		g.answer, cmd = g.answer.Update(msg)
		return cmd
	}
	// 3) Befehl bearbeiten
	if g.editing {
		switch key {
		case "enter":
			cmd := strings.TrimSpace(g.edit.Value())
			if cmd != "" {
				st := *g.cur
				st.Command = cmd
				st.Reason = i18n.T("von dir bearbeitet")
				*g.cur = st
				fresh := agent.ParseStepCommand(cmd, s.client.Remote.String())
				g.cur.Danger, g.cur.Elevated = fresh.Danger, fresh.Elevated
			}
			g.editing = false
			return nil
		}
		var cmd tea.Cmd
		g.edit, cmd = g.edit.Update(msg)
		return cmd
	}
	switch key {
	case "x":
		g.cancel()
		g.active, g.busy, g.cur, g.rbSel = false, false, nil, -1
		g.input.SetValue("")
		return g.input.Focus()
	case "r":
		return a.requestStep(s)
	}
	if g.cur == nil || g.busy {
		return nil
	}
	switch key {
	case "enter":
		if len(g.cur.Danger) > 0 && !g.armed {
			g.armed = true // erster Druck: nur scharf schalten
			return nil
		}
		a.runProposal(s, g.cur.Command, false)
	case "i":
		a.runProposal(s, g.cur.Command, true)
	case "s":
		g.steps = append(g.steps, agent.DoneStep{Command: g.cur.Command, Skipped: true})
		return a.requestStep(s)
	case "e":
		g.editing = true
		g.edit.SetValue(g.cur.Command)
		g.edit.CursorEnd()
		return g.edit.Focus()
	}
	return nil
}

func (a *App) askKey(s *session, msg tea.Msg) tea.Cmd {
	q := &s.ask
	k, isKey := msg.(tea.KeyPressMsg)
	if isKey && !q.input.Focused() {
		key := k.String()
		if cmd, ok := a.askCardKey(s, key); ok {
			return cmd
		}
		if key == "n" || key == "enter" {
			q.input.SetValue("")
			return q.input.Focus()
		}
		return nil
	}
	if isKey && k.String() == "enter" && strings.TrimSpace(q.input.Value()) != "" {
		return a.startAsk(s, q.input.Value())
	}
	var cmd tea.Cmd
	q.input, cmd = q.input.Update(msg)
	return cmd
}

func (a *App) tunnelKey(s *session, k tea.KeyPressMsg) tea.Cmd {
	list := a.st.Tunnels(s.conn.ID)
	switch k.String() {
	case "up", "k":
		s.tunSel = max(0, s.tunSel-1)
	case "down", "j":
		s.tunSel = min(len(list)-1, s.tunSel+1)
	case "enter", "space":
		if s.tunSel < len(list) && s.tunnels != nil {
			t := list[s.tunSel]
			if s.tunnels.Get(t.ID) != nil {
				s.tunnels.Stop(t.ID)
				a.flash(i18n.T("Tunnel gestoppt: ") + t.Name)
			} else if at, err := s.tunnels.Start(t); err != nil {
				a.flashErr(i18n.T("Tunnel: ") + err.Error())
			} else if at.URL() != "" {
				a.flash(i18n.T("Web-Service bereit: ") + at.URL())
			}
		}
	case "o":
		if s.tunSel < len(list) && s.tunnels != nil {
			if at := s.tunnels.Get(list[s.tunSel].ID); at != nil && at.URL() != "" {
				_ = sshx.OpenBrowser(at.URL())
			}
		}
	case "n":
		a.returnToTerminal = true
		return a.openTunnelForm(domain.Tunnel{ConnectionID: s.conn.ID, Kind: domain.TunnelLocal, TargetHost: "127.0.0.1"})
	case "e":
		if s.tunSel < len(list) {
			a.returnToTerminal = true
			return a.openTunnelForm(list[s.tunSel])
		}
	case "d":
		if s.tunSel < len(list) {
			t := list[s.tunSel]
			a.modal = confirmModal(i18n.T("Tunnel löschen"), "„"+t.Name+i18n.T("“ löschen?"), func() tea.Cmd {
				if s.tunnels != nil {
					s.tunnels.Stop(t.ID)
				}
				_ = a.st.DeleteTunnel(t.ID)
				return nil
			})
		}
	}
	return nil
}

func (a *App) panelKeys(s *session) [][2]string {
	switch s.panel {
	case panelGoal:
		g := s.goal
		if !g.active {
			if g.done {
				return [][2]string{{"Strg+S", i18n.T("als Runbook merken")}, {"⏎", i18n.T("neues Ziel")}, {"Esc", "Terminal"}, {a.keys.label(actGoal), i18n.T("schließen")}}
			}
			if len(a.st.Runbooks(s.conn.ID)) == 0 {
				return [][2]string{{"⏎", i18n.T("Ziel starten")}, {"Esc", "Terminal"}, {a.keys.label(actGoal), i18n.T("schließen")}}
			}
			return [][2]string{{"⏎", i18n.T("Ziel starten")}, {"↑↓", "Runbook"}, {"Strg+D", i18n.T("Runbook löschen")}, {"Esc", "Terminal"}, {a.keys.label(actGoal), i18n.T("schließen")}}
		}
		if g.cur != nil && g.cur.Question != "" {
			return [][2]string{{"⏎", i18n.T("antworten")}, {"Esc", "Terminal"}}
		}
		if g.editing {
			return [][2]string{{"⏎", i18n.T("übernehmen")}, {"Esc", "Terminal"}}
		}
		return [][2]string{{"⏎", i18n.T("ausführen")}, {"i", i18n.T("nur einfügen")}, {"e", i18n.T("bearbeiten")}, {"s", i18n.T("überspringen")},
			{"r", i18n.T("neu")}, {"x", i18n.T("Ziel beenden")}, {"Strg+S", "Runbook"}, {"Esc", "Terminal"}}
	case panelAsk:
		q := &s.ask
		switch {
		case q.busy:
			return [][2]string{{"Esc", "Terminal"}, {a.keys.label(actAsk), i18n.T("schließen")}}
		case q.question == "":
			return [][2]string{{"⏎", i18n.T("fragen")}, {"Esc", "Terminal"}, {a.keys.label(actAsk), i18n.T("schließen")}}
		}
		k := [][2]string{}
		if cards := answerCards(agent.VisibleAnswer(q.answer.String())); len(cards) > 0 {
			if cards[clamp(q.sel, 0, len(cards)-1)].runnable() {
				k = append(k, [2]string{"⏎", i18n.T("einfügen")}, [2]string{"a", i18n.T("ausführen")})
			}
			k = append(k, [2]string{"c", i18n.T("kopieren")})
			if len(cards) > 1 {
				k = append(k, [2]string{"↑↓", i18n.T("Befehl wählen")})
			}
			k = append(k, [2]string{"z", i18n.T("Erklärung")})
		}
		return append(k, [2]string{"n", i18n.T("neue Frage")}, [2]string{"Esc", "Terminal"}, [2]string{a.keys.label(actAsk), i18n.T("schließen")})
	case panelTunnels:
		list := a.st.Tunnels(s.conn.ID)
		if len(list) == 0 {
			return [][2]string{{"n", i18n.T("neuer Tunnel")}, {"Esc", i18n.T("zurück")}}
		}
		k := [][2]string{{"⏎", i18n.T("an/aus")}}
		if s.tunSel < len(list) && s.tunnels != nil {
			if at := s.tunnels.Get(list[s.tunSel].ID); at != nil && at.URL() != "" {
				k = append(k, [2]string{"o", "Browser"})
			}
		}
		return append(k, [2]string{"n", i18n.T("neu")}, [2]string{"e", i18n.T("bearbeiten")}, [2]string{"d", i18n.T("löschen")}, [2]string{"Esc", i18n.T("zurück")})
	case panelHost:
		return a.hostPanelKeys(s)
	}
	return nil
}

// ---------------------------------------------------------------- Ansicht

func (a *App) viewPanel(s *session, w, h int) string {
	st := a.s
	inner := w - 4
	var b strings.Builder
	switch s.panel {
	case panelGoal:
		a.viewGoal(&b, s, inner)
	case panelAsk:
		a.viewAsk(&b, s, inner)
	case panelTunnels:
		a.viewTunnels(&b, s, inner)
	case panelHost:
		a.viewHost(&b, s, inner)
	}
	style := st.Panel
	if s.panelFocus {
		style = st.PanelFocus
	}
	content := lipgloss.NewStyle().Width(inner).Render(b.String())
	lines := strings.Split(content, "\n")
	if len(lines) > h-2 {
		lines = lines[len(lines)-(h-2):] // Ende sichtbar halten
	}
	return style.Width(w).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n"))
}

func wrap(s string, w int) string { return lipgloss.NewStyle().Width(w).Render(s) }

func (a *App) viewGoal(b *strings.Builder, s *session, w int) {
	st := a.s
	g := &s.goal
	b.WriteString(st.PanelTitle.Render(i18n.T("◎ Ziel-Modus")) + st.Muted.Render(i18n.T("  KI-Agent")) + "\n\n")
	if !g.active && !g.done {
		b.WriteString(st.Text.Render(i18n.T("Was willst du auf ")+s.conn.Alias+i18n.T(" erreichen?")) + "\n\n")
		g.input.SetWidth(w - 2)
		b.WriteString(promptBlock(st.Accent.Render("› "), g.input.View()) + "\n\n")
		b.WriteString(wrap(st.Muted.Render(i18n.T("Die KI zerlegt das Ziel in einzelne Befehle, erklärt jeden kurz und wartet auf ")+
			i18n.T("deine Bestätigung. Sie tippt nie selbst.")), w))
		if rbs := a.st.Runbooks(s.conn.ID); len(rbs) > 0 {
			b.WriteString("\n\n" + st.Muted.Render(i18n.T("RUNBOOKS  ↑↓ wählen · ⏎ starten")) + "\n")
			for i, rb := range rbs {
				if i >= 8 {
					b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("  … %d weitere (Strg+Leertaste sucht)"), len(rbs)-8)) + "\n")
					break
				}
				meta := plural(len(rb.Steps), i18n.T("Schritt"), i18n.T("Schritte"))
				line := "▶ " + trunc(rb.Name, w-lipgloss.Width(meta)-4)
				gap := max(1, w-lipgloss.Width(line)-lipgloss.Width(meta))
				row := st.AI.Render("▶ ") + st.Text.Render(line[len("▶ "):]) + strings.Repeat(" ", gap) + st.Muted.Render(meta)
				if i == g.rbSel {
					row = st.Selected.Width(w).Render(line + strings.Repeat(" ", gap) + meta)
				}
				i, rb := i, rb
				b.WriteString(a.z.click(row, func(_ tea.Mouse, double bool) tea.Cmd {
					s.panelFocus = true
					g.rbSel = i
					g.input.SetValue(rb.Goal)
					g.input.CursorEnd()
					if double {
						return a.startRunbook(s, rb)
					}
					return g.input.Focus()
				}) + "\n")
			}
		}
		return
	}
	switch {
	case len(g.recipe) > 0:
		b.WriteString(st.Badge.Foreground(st.T.AI).Render(i18n.T("▶ Runbook")) + " " + st.Muted.Render(plural(len(g.recipe), i18n.T("Schritt"), i18n.T("Schritte"))+i18n.T(" als Vorlage")) + "\n")
	case g.runbookID != "":
		b.WriteString(st.Badge.Foreground(st.T.AI).Render(i18n.T("▶ als Runbook gespeichert")) + "\n")
	}
	b.WriteString(st.Muted.Render(i18n.T("Ziel")) + "\n" + wrap(st.Text.Bold(true).Render(g.goal), w) + "\n\n")
	// Fortschritt
	for _, d := range g.steps {
		mark := st.OK.Render("✓ ")
		if d.Skipped {
			mark = st.Muted.Render("↷ ")
		}
		b.WriteString(mark + st.Muted.Render(trunc(d.Command, w-3)) + "\n")
	}
	if g.waiting {
		b.WriteString(st.Warn.Render(i18n.T("◌ läuft … ")) + st.Muted.Render(fmt.Sprintf("%ds", int(time.Since(g.waitSince).Seconds()))) + "\n")
	}
	if g.manualPending {
		b.WriteString(st.Warn.Render(i18n.T("◌ eingefügt, drück im Terminal ⏎")) + "\n")
	}
	if len(g.plan) > 0 && !g.done {
		b.WriteString("\n" + st.Muted.Render("Plan") + "\n")
		for _, p := range g.plan {
			b.WriteString(st.Muted.Render("○ ") + st.Text.Render(trunc(p, w-3)) + "\n")
		}
	}
	b.WriteString("\n")
	switch {
	case g.done:
		b.WriteString(st.OK.Render(i18n.T("✓ Ziel erreicht")) + "\n" + wrap(st.Text.Render(g.doneReason), w) + "\n\n" + st.Muted.Render(i18n.T("Strg+S als Runbook merken · ⏎ neues Ziel · ")+a.keys.label(actGoal)+i18n.T(" schließen")))
		if s.panelFocus && g.done {
			g.active = false
		}
	case g.busy:
		b.WriteString(st.AI.Render(i18n.T("✦ denkt über den nächsten Schritt nach …")))
	case g.err != "":
		b.WriteString(wrap(st.Danger.Render("✗ "+g.err), w))
	case g.cur != nil && g.cur.Question != "":
		b.WriteString(st.AI.Render(i18n.T("✦ Rückfrage")) + "\n" + wrap(st.Text.Render(g.cur.Question), w) + "\n\n")
		g.answer.SetWidth(w - 2)
		b.WriteString(promptBlock(st.Accent.Render("› "), g.answer.View()))
	case g.cur != nil:
		c := g.cur
		head := st.AI.Render(i18n.T("✦ Nächster Schritt"))
		box := st.Panel
		if len(c.Danger) > 0 {
			head = st.Danger.Render(i18n.T("⚠ Vorsicht: ") + c.Danger[0].Reason)
			box = st.DangerBox
		}
		cmdLine := c.Command
		if g.editing {
			g.edit.SetWidth(w - 6)
			cmdLine = g.edit.View()
		} else {
			cmdLine = st.Muted.Render("$ ") + highlightShell(cmdLine, st)
		}
		b.WriteString(head + "\n")
		b.WriteString(box.Width(w).Render(wrap(cmdLine, w-4)) + "\n")
		if c.Reason != "" {
			b.WriteString(wrap(st.Muted.Render(c.Reason), w) + "\n")
		}
		if c.Elevated {
			b.WriteString(st.Warn.Render(i18n.T("⬆ läuft mit erhöhten Rechten (sudo)")) + "\n")
		}
		if g.armed {
			b.WriteString("\n" + st.Danger.Render(i18n.T("Nochmal ⏎ zum Ausführen. Esc/s zum Abbrechen.")) + "\n")
		}
	}
}

func (a *App) viewAsk(b *strings.Builder, s *session, w int) {
	st := a.s
	q := &s.ask
	b.WriteString(st.PanelTitle.Render(i18n.T("✦ KI fragen")) + st.Muted.Render(i18n.T("  mit Terminal-Kontext")) + "\n\n")
	q.input.SetWidth(w - 2)
	if q.input.Focused() || q.question == "" {
		b.WriteString(promptBlock(st.Accent.Render("› "), q.input.View()) + "\n\n")
	} else {
		b.WriteString(st.Muted.Render(i18n.T("Frage")) + "\n" + wrap(st.Text.Bold(true).Render(q.question), w) + "\n\n")
	}
	ans := agent.VisibleAnswer(q.answer.String())
	if ans == "" && q.busy && q.answer.Len() > 0 {
		b.WriteString(st.Muted.Render(i18n.T("denkt nach …")) + "\n")
	}
	if ans != "" {
		a.viewAnswer(b, s, ans, w) // Erklärung + Befehlskarten (aicards.go)
	}
	if q.busy {
		b.WriteString(st.AI.Render("▍"))
	}
	if q.err != "" {
		b.WriteString("\n" + wrap(st.Danger.Render("✗ "+q.err), w))
	}
}

func (a *App) viewTunnels(b *strings.Builder, s *session, w int) {
	st := a.s
	b.WriteString(st.PanelTitle.Foreground(st.T.Accent).Render(i18n.T("⇄ Tunnel & Web-Services")) + "\n\n")
	list := a.st.Tunnels(s.conn.ID)
	if len(list) == 0 {
		b.WriteString(st.Muted.Render(i18n.T("Noch keine Tunnel für diese Verbindung.\nn = neuer Tunnel (z. B. -L 3000:localhost:3000)")))
		return
	}
	for i, t := range list {
		var at *sshx.ActiveTunnel
		if s.tunnels != nil {
			at = s.tunnels.Get(t.ID)
		}
		dot := st.Muted.Render("○")
		if at != nil {
			dot = st.OK.Render("●")
		}
		name := trunc(t.Name, w-4)
		if i == s.tunSel {
			name = st.Selected.Render(name)
		}
		i := i
		b.WriteString(a.z.click(dot+" "+name, func(_ tea.Mouse, double bool) tea.Cmd {
			s.panelFocus, s.tunSel = true, i
			if double {
				return a.pressKey("⏎")
			}
			return nil
		}) + "\n")
		b.WriteString("  " + st.Muted.Render(t.Spec()) + "\n")
		if at != nil {
			if u := at.URL(); u != "" {
				b.WriteString("  " + a.z.click(st.Accent.Underline(true).Render(u)+st.Muted.Render(i18n.T("  o öffnen")),
					func(tea.Mouse, bool) tea.Cmd { _ = sshx.OpenBrowser(u); return nil }) + "\n")
			} else {
				b.WriteString("  " + st.Text.Render(i18n.T("lauscht auf ")+at.Addr) + "\n")
			}
			b.WriteString("  " + st.Muted.Render(fmt.Sprintf(i18n.T("%d offen · %d gesamt · ↓%s ↑%s"), at.Conns.Load(), at.Total.Load(),
				human(at.BytesIn.Load()), human(at.BytesOut.Load()))) + "\n")
			if e, _ := at.LastError.Load().(string); e != "" {
				b.WriteString("  " + st.Danger.Render(trunc(e, w-4)) + "\n")
			}
		}
		b.WriteString("\n")
	}
}

func human(n int64) string {
	switch {
	case n > 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n > 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n > 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// startGoal setzt ein neues Ziel und fragt den ersten Schritt an (Panel und Palette).
func (a *App) startGoal(s *session, text string) tea.Cmd { return a.startGoalWith(s, text, nil) }

// startRunbook startet ein gespeichertes Ziel; die damals ausgeführten
// Befehle gehen dem Agenten als Vorlage mit.
func (a *App) startRunbook(s *session, rb domain.Runbook) tea.Cmd {
	a.st.UseRunbook(rb.ID)
	return a.startGoalWith(s, rb.Goal, &rb)
}

func (a *App) startGoalWith(s *session, text string, rb *domain.Runbook) tea.Cmd {
	a.panelOpened(withPanel(s, panelGoal))
	g := &s.goal
	g.cancel()
	*g = goalState{input: g.input, edit: g.edit, answer: g.answer, inited: true,
		goal: strings.TrimSpace(text), active: true, rbSel: -1}
	if rb != nil {
		g.recipe, g.runbookID = rb.Steps, rb.ID
	}
	g.input.SetValue(g.goal)
	g.input.Blur()
	a.resize(s)
	return a.requestStep(s)
}

// startAsk stellt eine Frage mit Terminal-Kontext (Panel und Palette).
func (a *App) startAsk(s *session, text string) tea.Cmd {
	a.panelOpened(withPanel(s, panelAsk))
	a.resize(s)
	q := &s.ask
	ag, err := a.agentClient()
	if err != nil {
		q.err = err.Error()
		return nil
	}
	q.cancel()
	q.question = strings.TrimSpace(text)
	q.input.SetValue(q.question)
	q.answer.Reset()
	q.err, q.busy = "", true
	q.sel, q.armed = 0, -1
	q.seq++
	q.input.Blur()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	q.cancelFn = cancel
	sid, seq, ev := s.id, q.seq, a.events
	c := a.agentContext(s, true)
	question := q.question
	return func() tea.Msg {
		defer cancel()
		err := ag.Ask(ctx, question, c, func(d string) {
			if d != "" {
				ev <- askDeltaMsg{sid, seq, d}
			}
		})
		return askDoneMsg{sid, seq, err}
	}
}

func withPanel(s *session, k panelKind) *session {
	s.panel, s.panelFocus = k, true
	return s
}

// saveRunbook merkt sich das aktuelle (oder gerade erreichte) Ziel samt den
// ausgeführten Befehlen. Kam das Ziel aus einem Runbook, wird dieses
// aktualisiert statt dupliziert.
func (a *App) saveRunbook(s *session) {
	g := &s.goal
	running := g.active || g.done
	goal := strings.TrimSpace(g.input.Value())
	if running {
		goal = strings.TrimSpace(g.goal)
	}
	if goal == "" {
		a.flash(i18n.T("Kein Ziel zum Speichern"))
		return
	}
	var steps []string
	for _, d := range g.steps {
		if !running {
			break
		}
		if !d.Skipped {
			steps = append(steps, d.Command)
		}
	}
	rb := domain.Runbook{Name: goal, Goal: goal, Steps: steps}
	if s.client != nil {
		rb.OS = s.client.Remote.OS
	}
	for _, x := range a.st.Runbooks("") {
		if (running && x.ID == g.runbookID) || x.Goal == goal {
			rb.ID, rb.CreatedAt, rb.Uses, rb.LastUsed, rb.ConnectionID = x.ID, x.CreatedAt, x.Uses, x.LastUsed, x.ConnectionID
			if x.Goal == goal {
				rb.Name = x.Name
			}
			if len(steps) == 0 {
				rb.Steps = x.Steps
			}
			break
		}
	}
	saved, err := a.st.UpsertRunbook(rb)
	if err != nil {
		a.flashErr(i18n.T("Runbook: ") + err.Error())
		return
	}
	if running {
		g.runbookID = saved.ID
	}
	a.flash(i18n.T("Runbook gespeichert: ") + trunc(saved.Name, 40) + " (" + plural(len(saved.Steps), i18n.T("Schritt"), i18n.T("Schritte")) + ")")
}
