package ui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
)

const maxGoalSteps = 40

// ---------------------------------------------------------------- Zustände

// goalState: Ziel-Modus. Das Modell schlägt immer genau EINEN Befehl vor;
// ausgeführt wird erst nach Bestätigung (⏎), rote Befehle nach zweitem ⏎.
type goalState struct {
	input, edit, answer textinput.Model
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
}

func (g *goalState) cancel() {
	if g.cancelFn != nil {
		g.cancelFn()
		g.cancelFn = nil
	}
}

func (g *goalState) startWait() { g.waiting, g.waitSince = true, time.Now() }

type askState struct {
	input    textinput.Model
	inited   bool
	question string
	answer   strings.Builder
	busy     bool
	seq      int
	cancelFn context.CancelFunc
	err      string
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
			s.goal.input = newInput("", false)
			s.goal.input.Placeholder = "z. B. Installiere Docker und richte Nginx als Reverse Proxy ein"
			s.goal.edit, s.goal.answer = newInput("", false), newInput("", false)
			s.goal.inited = true
		}
		if !s.goal.active {
			return s.goal.input.Focus()
		}
	case panelAsk:
		if !s.ask.inited {
			s.ask.input = newInput("", false)
			s.ask.input.Placeholder = "Frage, z. B. Warum startet nginx nicht?"
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
		g.err = fmt.Sprintf("Ratenlimit: höchstens %d Schritte pro Minute. r = erneut versuchen", limit)
		return nil
	}
	if len(g.steps) >= maxGoalSteps {
		g.err = "Maximal 40 Schritte pro Ziel. Ziel neu setzen (x)."
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
	c := a.agentContext(s, true)
	return func() tea.Msg {
		defer cancel()
		st, err := ag.NextStep(ctx, goal, hist, ans, c)
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
	s.term.SendText(cmd + "\r")
	g.startWait()
}

// ---------------------------------------------------------------- Tasten im Panel

func (a *App) panelMsg(s *session, msg tea.Msg) tea.Cmd {
	k, isKey := msg.(tea.KeyPressMsg)
	if isKey && k.String() == "esc" {
		s.panelFocus = false
		if s.panel == panelTunnels || (s.panel == panelAsk && !s.ask.busy && s.ask.answer.Len() == 0) {
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
	// 1) noch kein Ziel: Eingabe
	if !g.active {
		if key == "enter" && strings.TrimSpace(g.input.Value()) != "" {
			return a.startGoal(s, g.input.Value())
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
				st.Reason = "von dir bearbeitet"
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
		g.active, g.busy, g.cur = false, false, nil
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

var codeBlock = regexp.MustCompile("(?s)```(?:sh|bash|shell|zsh|powershell|ps1|console)?\\s*\\n(.*?)```")

func extractCommands(ans string) []string {
	var out []string
	for _, m := range codeBlock.FindAllStringSubmatch(ans, 9) {
		c := strings.TrimSpace(m[1])
		c = strings.TrimPrefix(c, "$ ")
		if c != "" && !strings.Contains(c, "\n") {
			out = append(out, c)
		}
	}
	return out
}

func (a *App) askKey(s *session, msg tea.Msg) tea.Cmd {
	q := &s.ask
	k, isKey := msg.(tea.KeyPressMsg)
	if isKey && !q.input.Focused() {
		key := k.String()
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			cmds := extractCommands(q.answer.String())
			if i := int(key[0] - '1'); i < len(cmds) {
				s.term.SendText(cmds[i]) // nur einfügen, Enter drückt der Mensch
				s.panelFocus = false
			}
			return nil
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
				a.flash("Tunnel gestoppt: " + t.Name)
			} else if at, err := s.tunnels.Start(t); err != nil {
				a.flash("Tunnel: " + err.Error())
			} else if at.URL() != "" {
				a.flash("Web-Service bereit: " + at.URL())
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
			a.modal = confirmModal("Tunnel löschen", "„"+t.Name+"“ löschen?", func() tea.Cmd {
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
			return [][2]string{{"⏎", "Ziel starten"}, {"Esc", "Terminal"}, {"F2", "schließen"}}
		}
		if g.cur != nil && g.cur.Question != "" {
			return [][2]string{{"⏎", "antworten"}, {"Esc", "Terminal"}}
		}
		if g.editing {
			return [][2]string{{"⏎", "übernehmen"}, {"Esc", "Terminal"}}
		}
		return [][2]string{{"⏎", "ausführen"}, {"i", "nur einfügen"}, {"e", "bearbeiten"}, {"s", "überspringen"},
			{"r", "neu"}, {"x", "Ziel beenden"}, {"Esc", "Terminal"}}
	case panelAsk:
		return [][2]string{{"⏎", "fragen"}, {"1…9", "Befehl einfügen"}, {"n", "neue Frage"}, {"Esc", "Terminal"}}
	case panelTunnels:
		return [][2]string{{"⏎", "an/aus"}, {"o", "Browser"}, {"n", "neu"}, {"e", "bearbeiten"}, {"d", "löschen"}, {"Esc", "zurück"}}
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
	b.WriteString(st.PanelTitle.Render("◎ Ziel-Modus") + st.Muted.Render("  KI-Agent") + "\n\n")
	if !g.active && !g.done {
		b.WriteString(st.Text.Render("Was willst du auf "+s.conn.Alias+" erreichen?") + "\n\n")
		g.input.SetWidth(w - 2)
		b.WriteString(st.Accent.Render("› ") + g.input.View() + "\n\n")
		b.WriteString(wrap(st.Muted.Render("Die KI zerlegt das Ziel in einzelne Befehle, erklärt jeden kurz und wartet auf "+
			"deine Bestätigung. Sie tippt nie selbst."), w))
		return
	}
	b.WriteString(st.Muted.Render("Ziel") + "\n" + wrap(st.Text.Bold(true).Render(g.goal), w) + "\n\n")
	// Fortschritt
	for _, d := range g.steps {
		mark := st.OK.Render("✓ ")
		if d.Skipped {
			mark = st.Muted.Render("↷ ")
		}
		b.WriteString(mark + st.Muted.Render(trunc(d.Command, w-3)) + "\n")
	}
	if g.waiting {
		b.WriteString(st.Warn.Render("◌ läuft … ") + st.Muted.Render(fmt.Sprintf("%ds", int(time.Since(g.waitSince).Seconds()))) + "\n")
	}
	if g.manualPending {
		b.WriteString(st.Warn.Render("◌ eingefügt, drück im Terminal ⏎") + "\n")
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
		b.WriteString(st.OK.Render("✓ Ziel erreicht") + "\n" + wrap(st.Text.Render(g.doneReason), w) + "\n\n" + st.Muted.Render("F2 schließen · neues Ziel: x"))
		if s.panelFocus && g.done {
			g.active = false
		}
	case g.busy:
		b.WriteString(st.AI.Render("✦ denkt über den nächsten Schritt nach …"))
	case g.err != "":
		b.WriteString(wrap(st.Danger.Render("✗ "+g.err), w))
	case g.cur != nil && g.cur.Question != "":
		b.WriteString(st.AI.Render("✦ Rückfrage") + "\n" + wrap(st.Text.Render(g.cur.Question), w) + "\n\n")
		g.answer.SetWidth(w - 2)
		b.WriteString(st.Accent.Render("› ") + g.answer.View())
	case g.cur != nil:
		c := g.cur
		head := st.AI.Render("✦ Nächster Schritt")
		box := st.Panel
		if len(c.Danger) > 0 {
			head = st.Danger.Render("⚠ Vorsicht: " + c.Danger[0].Reason)
			box = st.DangerBox
		}
		cmdLine := c.Command
		if g.editing {
			g.edit.SetWidth(w - 6)
			cmdLine = g.edit.View()
		} else {
			cmdLine = st.Code.Render("$ " + cmdLine)
		}
		b.WriteString(head + "\n")
		b.WriteString(box.Width(w).Render(wrap(cmdLine, w-4)) + "\n")
		if c.Reason != "" {
			b.WriteString(wrap(st.Muted.Render(c.Reason), w) + "\n")
		}
		if c.Elevated {
			b.WriteString(st.Warn.Render("⬆ läuft mit erhöhten Rechten (sudo)") + "\n")
		}
		if g.armed {
			b.WriteString("\n" + st.Danger.Render("Nochmal ⏎ zum Ausführen. Esc/s zum Abbrechen.") + "\n")
		}
	}
}

func (a *App) viewAsk(b *strings.Builder, s *session, w int) {
	st := a.s
	q := &s.ask
	b.WriteString(st.PanelTitle.Render("✦ KI fragen") + st.Muted.Render("  mit Terminal-Kontext") + "\n\n")
	q.input.SetWidth(w - 2)
	if q.input.Focused() || q.question == "" {
		b.WriteString(st.Accent.Render("› ") + q.input.View() + "\n\n")
	} else {
		b.WriteString(st.Muted.Render("Frage") + "\n" + wrap(st.Text.Bold(true).Render(q.question), w) + "\n\n")
	}
	ans := q.answer.String()
	if ans != "" {
		// Codeblöcke nummerieren, damit 1…9 sie einfügen kann
		n := 0
		out := codeBlock.ReplaceAllStringFunc(ans, func(m string) string {
			sub := codeBlock.FindStringSubmatch(m)
			n++
			return st.Key.Render(fmt.Sprintf("%d", n)) + " " + st.Code.Render(strings.TrimSpace(sub[1]))
		})
		b.WriteString(wrap(st.Text.Render(out), w) + "\n")
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
	b.WriteString(st.PanelTitle.Foreground(st.T.Accent).Render("⇄ Tunnel & Web-Services") + "\n\n")
	list := a.st.Tunnels(s.conn.ID)
	if len(list) == 0 {
		b.WriteString(st.Muted.Render("Noch keine Tunnel für diese Verbindung.\nn = neuer Tunnel (z. B. -L 3000:localhost:3000)"))
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
		b.WriteString(dot + " " + name + "\n")
		b.WriteString("  " + st.Muted.Render(t.Spec()) + "\n")
		if at != nil {
			if u := at.URL(); u != "" {
				b.WriteString("  " + st.Accent.Underline(true).Render(u) + st.Muted.Render("  o öffnen") + "\n")
			} else {
				b.WriteString("  " + st.Text.Render("lauscht auf "+at.Addr) + "\n")
			}
			b.WriteString("  " + st.Muted.Render(fmt.Sprintf("%d offen · %d gesamt · ↓%s ↑%s", at.Conns.Load(), at.Total.Load(),
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
func (a *App) startGoal(s *session, text string) tea.Cmd {
	a.panelOpened(withPanel(s, panelGoal))
	g := &s.goal
	g.cancel()
	*g = goalState{input: g.input, edit: g.edit, answer: g.answer, inited: true,
		goal: strings.TrimSpace(text), active: true}
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
