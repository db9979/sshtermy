package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/llm"
)

const (
	topRows    = 2 // Kopfzeile + Sitzungszeile
	bottomRows = 2 // Vorschläge + Tasten
)

func checkLLM(c domain.LLMConfig) error { return llm.Check(c) }

func (a *App) panelWidth(s *session) int {
	if s == nil || s.panel == panelNone {
		return 0
	}
	if a.w >= 110 {
		return 48
	}
	return a.w / 2
}

func (a *App) termSize(s *session) (int, int) {
	return max(20, a.w-a.panelWidth(s)), max(5, a.h-topRows-bottomRows)
}

func (a *App) resize(s *session) {
	if s.term == nil {
		return
	}
	c, r := a.termSize(s)
	if cw, rh := s.term.Size(); cw == c && rh == r {
		return
	}
	s.term.Resize(c, r)
	if s.shell != nil {
		_ = s.shell.Resize(c, r)
	}
}

func (a *App) agentClient() (*agent.Agent, error) {
	c, err := llm.New(a.st.LLM())
	if err != nil {
		return nil, err
	}
	return &agent.Agent{C: c}, nil
}

// ---------------------------------------------------------------- Tasten

func (a *App) updateTerminal(msg tea.Msg) tea.Cmd {
	s := a.current()
	if s == nil {
		a.scr = scrManager
		return nil
	}
	switch m := msg.(type) {
	case tea.PasteMsg:
		if s.files.open {
			if s.files.mkdir != nil {
				var cmd tea.Cmd
				*s.files.mkdir, cmd = s.files.mkdir.Update(msg)
				return cmd
			}
			return nil
		}
		if s.panelFocus {
			return a.panelMsg(s, msg)
		}
		return a.guardedPaste(s, m.Content)
	case tea.KeyPressMsg:
		key := m.String()
		// Tippen beendet Markierung und Rückblick (Esc nur das)
		if s.scroll > 0 && key == "esc" && !s.panelFocus && !s.files.open {
			s.scroll, s.sel = 0, nil
			return nil
		}
		s.sel, s.selecting = nil, false
		if !s.panelFocus && !s.files.open && !(m.Code >= tea.KeyF1 && m.Code <= tea.KeyF12) {
			s.scroll = 0
		}
		// globale Tasten der Terminal-Ansicht (frei belegbar, siehe keymap.go)
		switch a.keys.which(key, actHelp, actAbout, actManager, actPrevTab, actNextTab, actFiles, actCheat, actQuickMenu) {
		case actHelp:
			a.modal = a.helpModal()
			return nil
		case actAbout:
			a.modal = a.aboutModal()
			return nil
		case actManager:
			a.scr = scrManager
			return nil
		case actCheat:
			return a.toggleCheat()
		case actQuickMenu:
			if s.state == stConnected {
				a.modal = a.quickModal(s)
			}
			return nil
		case actPrevTab, actNextTab:
			if len(a.sessions) > 1 {
				d := 1
				if a.keys.is(actPrevTab, key) {
					d = -1
				}
				a.cur = (a.cur + d + len(a.sessions)) % len(a.sessions)
				a.resize(a.current())
			}
			return nil
		case actFiles:
			if s.state == stConnected {
				return a.toggleFiles(s)
			}
			return nil
		}
		if s.files.open {
			return a.filesKey(s, m)
		}
		if i, ok := a.quickIndex(key); ok {
			return a.runQuick(s, i)
		}
		switch id := a.keys.which(key, actGoal, actAsk, actTunnels, actHost); id {
		case actGoal, actAsk, actTunnels, actHost:
			kind := map[action]panelKind{actGoal: panelGoal, actAsk: panelAsk, actTunnels: panelTunnels, actHost: panelHost}[id]
			if s.panel == kind && s.panelFocus {
				s.panel, s.panelFocus = panelNone, false
			} else {
				s.panel, s.panelFocus = kind, true
			}
			a.resize(s)
			return a.panelOpened(s)
		}
		if s.panelFocus {
			return a.panelMsg(s, msg)
		}
		if s.state == stReconnect {
			switch key {
			case "enter", "r":
				return a.reconnectNow(s)
			case "esc":
				a.cancelReconnect(s)
			case "x":
				a.closeTab(s)
			}
			return nil
		}
		if s.state != stConnected {
			switch key {
			case "enter", "r":
				s.state, s.err = stConnecting, ""
				return a.connectCmd(s)
			case "x", "esc":
				a.closeTab(s)
			}
			return nil
		}
		return a.terminalKey(s, m)
	}
	return nil
}

func (a *App) closeTab(s *session) {
	a.endLog(s)
	s.close()
	for i, x := range a.sessions {
		if x == s {
			a.sessions = append(a.sessions[:i], a.sessions[i+1:]...)
		}
	}
	a.cur = max(0, min(a.cur, len(a.sessions)-1))
	if len(a.sessions) == 0 {
		a.scr = scrManager
	}
}

func (a *App) visibleGhost(s *session) string {
	if s.ghost == "" || s.term == nil {
		return ""
	}
	typed, ok := s.term.TypedInput()
	full := s.ghostFor + s.ghost
	if !ok || !strings.HasPrefix(typed, s.ghostFor) || !strings.HasPrefix(full, typed) || len(full) == len(typed) {
		return ""
	}
	return full[len(typed):]
}

func (a *App) terminalKey(s *session, k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	// Ghost-Text übernehmen
	if g := a.visibleGhost(s); g != "" && (key == "tab" || key == "right" || key == "end") {
		s.term.SendText(g)
		s.ghost = ""
		return nil
	}
	// fehlgeschlagener Befehl: Warum? & Fix
	if key == "alt+0" && s.fail != nil {
		return a.explainFailure(s)
	}
	// Folgebefehle einfügen (ohne Enter)
	if id := a.keys.which(key, actChip1, actChip2, actChip3, actChip4); id != "" {
		i := int(id[len(id)-1] - '1')
		if i < len(s.chips) {
			c := s.chips[i]
			if len(c.Danger) > 0 && s.chipArmed != i {
				s.chipArmed = i
				a.flash("Vorsicht: " + c.Danger[0].Reason + ". Nochmal " + a.keys.label(id) + " zum Einfügen")
				return nil
			}
			s.chipArmed = -1
			s.term.SendText(c.Command)
			s.chips = nil
		}
		return nil
	}
	if key == "ctrl+c" && s.run != nil {
		s.run.interrupted = true
	}
	if key == "enter" {
		if typed, ok := s.term.TypedInput(); ok && strings.TrimSpace(typed) != "" && !agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
			a.beginCommand(s, typed)
			s.history = append(s.history, strings.TrimSpace(typed))
			a.profileCommand(s, strings.TrimSpace(typed))
			s.noteCommand(strings.TrimSpace(typed))
			if len(s.history) > 200 {
				s.history = s.history[len(s.history)-200:]
			}
			s.followDue = time.Now().Add(1200 * time.Millisecond)
		}
		s.chips, s.chipArmed = nil, -1
		s.ghost = ""
		if s.goal.active && s.goal.manualPending {
			s.goal.manualPending = false
			s.goal.startWait()
		}
	}
	if key == "ctrl+d" && s.term.AtIdlePrompt() {
		s.quitAt = time.Now() // Strg+D am leeren Prompt = abmelden
	}
	s.term.SendKey(uv.KeyPressEvent(k))
	if key == "enter" || key == "ctrl+c" {
		return nil
	}
	return a.scheduleGhost(s)
}

// ---------------------------------------------------------------- Ghost-Text

type ghostTickMsg struct{ sid, seq int }
type ghostMsg struct {
	sid, seq    int
	typed, rest string
}

func (a *App) scheduleGhost(s *session) tea.Cmd {
	cfg := a.st.LLM()
	if !cfg.SuggestEnabled {
		return nil
	}
	s.ghostSeq++
	if s.ghostCancel != nil {
		s.ghostCancel()
		s.ghostCancel = nil
	}
	sid, seq := s.id, s.ghostSeq
	d := time.Duration(max(150, cfg.SuggestDelayMS)) * time.Millisecond
	return tea.Tick(d, func(time.Time) tea.Msg { return ghostTickMsg{sid, seq} })
}

func (a *App) fireGhost(s *session, seq int) tea.Cmd {
	if seq != s.ghostSeq || s.term == nil {
		return nil
	}
	typed, ok := s.term.TypedInput()
	if !ok || len(strings.TrimSpace(typed)) < 2 || agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
		return nil
	}
	ag, err := a.agentClient()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	s.ghostCancel = cancel
	c := a.agentContext(s, true)
	sid := s.id
	return func() tea.Msg {
		defer cancel()
		rest, _ := ag.Complete(ctx, typed, c)
		return ghostMsg{sid, seq, typed, rest}
	}
}

// ---------------------------------------------------------------- Folgebefehle

type followMsg struct {
	sid, seq int
	chips    []agent.Suggestion
}

func (a *App) followTick(s *session) tea.Cmd {
	if s.followDue.IsZero() || time.Now().Before(s.followDue) || s.term == nil || s.goal.active {
		return nil
	}
	if time.Since(s.lastOutput) < 800*time.Millisecond || !s.term.AtIdlePrompt() {
		return nil
	}
	s.followDue = time.Time{}
	if time.Since(s.lastFollow) < 5*time.Second {
		return nil
	}
	ag, err := a.agentClient()
	if err != nil {
		return nil
	}
	s.lastFollow = time.Now()
	s.chipSeq++
	sid, seq := s.id, s.chipSeq
	c := a.agentContext(s, true)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		v, _ := ag.FollowUps(ctx, c)
		return followMsg{sid, seq, v}
	}
}

// ---------------------------------------------------------------- Ansicht

func (a *App) viewTerminal() (string, *tea.Cursor) {
	s := a.current()
	st := a.s
	// Tabs rechts in der Kopfzeile
	var tabs strings.Builder
	for i, x := range a.sessions {
		dot := map[sessState]string{stConnecting: st.Warn.Render("◌"), stConnected: st.OK.Render("●"), stClosed: st.Danger.Render("○"), stReconnect: st.Warn.Render("⟳")}[x.state]
		label := " " + trunc(x.conn.Alias, 16) + " "
		tab := st.Tab.Render(dot + label)
		if i == a.cur {
			tab = st.TabActive.Render(dot + label)
		}
		i := i
		tabs.WriteString(a.z.click(tab, func(tea.Mouse, bool) tea.Cmd {
			if i < len(a.sessions) {
				a.cur = i
				a.resize(a.current())
			}
			return nil
		}))
	}
	top := a.header(tabs.String())

	// Sitzungszeile (statusline.go)
	bar := a.sessionBar(s)

	// Terminalfläche
	cols, rows := a.termSize(s)
	var body string
	var cur *tea.Cursor
	switch {
	case s.state == stConnecting:
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center,
			st.AI.Render("⠿ Verbinde mit "+s.conn.Alias+" ("+s.conn.Addr()+") …"))
	case s.state == stReconnect:
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, a.reconnectView(s))
	case s.state == stClosed:
		msg := st.Danger.Render("✗ " + orDefault(s.err, "getrennt"))
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center,
			msg+"\n\n"+st.Muted.Render("⏎ neu verbinden · x Tab schließen · "+a.keys.label(actManager)+" Manager"))
	case s.scroll > 0 || s.sel != nil:
		// Rückblick im Verlauf und/oder Markierung
		if sb := s.term.ScrollbackLen(); s.scroll > 0 && sb > s.sbSeen {
			s.scroll = min(sb, s.scroll+sb-s.sbSeen) // neue Ausgabe verschiebt die Ansicht nicht
		}
		s.scroll = min(s.scroll, s.term.ScrollbackLen())
		body = s.term.Render(s.scroll, s.sel)
		if x, y, vis := s.term.Cursor(); vis && !s.panelFocus && s.scroll == 0 {
			cur = tea.NewCursor(x, y+topRows)
			cur.Color = st.T.Accent
		}
	default:
		body = s.term.View(a.visibleGhost(s), func(g string) string { return st.Ghost.Render(g) })
		if x, y, vis := s.term.Cursor(); vis && !s.panelFocus {
			cur = tea.NewCursor(x, y+topRows)
			cur.Color = st.T.Accent
		}
	}
	if s.term != nil {
		s.sbSeen = s.term.ScrollbackLen()
	}
	if s.files.open {
		body, cur = a.viewFiles(s, a.w, rows), nil
	} else if s.panel != panelNone {
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(cols).Render(body), a.viewPanel(s, a.panelWidth(s), rows))
	}

	return lipgloss.JoinVertical(lipgloss.Left, top, bar, body, a.chipBar(s), a.footer(a.terminalKeys(s))), cur
}

func (a *App) chipBar(s *session) string {
	st := a.s
	var b strings.Builder
	if g := a.visibleGhost(s); g != "" {
		b.WriteString(a.z.click(st.AI.Render(" ✦ ")+st.Muted.Render("Tab übernimmt: ")+st.Ghost.Render(trunc(g, 60)),
			func(tea.Mouse, bool) tea.Cmd { s.panelFocus = false; return a.pressKey("Tab") }) + "  ")
	} else {
		if f := s.fail; f != nil {
			why := orDefault(f.Reason, "fehlgeschlagen")
			b.WriteString(a.z.click(st.Danger.Render(" ✗ "+trunc(why, 34))+" "+st.Key.Render("Alt+0")+st.AI.Render(" Warum? & Fix "),
				func(tea.Mouse, bool) tea.Cmd { s.panelFocus = false; return a.pressKey("alt+0") }) + " ")
		}
		a.chipList(s, &b)
	}
	out := b.String()
	if w := lipgloss.Width(out); w < a.w {
		out += strings.Repeat(" ", a.w-w)
	}
	return ansi.Truncate(out, a.w, "")
}

func (a *App) chipList(s *session, b *strings.Builder) {
	st := a.s
	if len(s.chips) > 0 {
		b.WriteString(st.AI.Render(" ✦ Als Nächstes: "))
		for i, c := range s.chips {
			style := st.Chip
			if len(c.Danger) > 0 {
				style = st.ChipDanger
			}
			id := []action{actChip1, actChip2, actChip3, actChip4}[min(i, 3)]
			lbl := a.keys.label(id)
			b.WriteString(a.z.click(st.Key.Render(lbl)+style.Render(trunc(c.Command, 34)),
				func(tea.Mouse, bool) tea.Cmd { s.panelFocus = false; return a.pressKey(lbl) }) + " ")
		}
		if len(s.chips) > 0 && s.chips[0].Reason != "" {
			b.WriteString(st.Muted.Render(trunc(s.chips[0].Reason, 40)))
		}
	}
}

func (a *App) terminalKeys(s *session) [][2]string {
	if s.files.open {
		return a.filesKeys(s)
	}
	if s.panelFocus {
		return a.panelKeys(s)
	}
	k := a.keys
	keys := [][2]string{{k.label(actPalette), "Palette"}}
	switch {
	case s.state == stClosed:
		keys = append(keys, [2]string{"⏎", "neu verbinden"}, [2]string{"x", "Tab schließen"})
	case s.state == stConnected:
		if s.scroll > 0 {
			keys = append(keys, [2]string{"Esc", "zurück zur Eingabe"})
		}
		if a.visibleGhost(s) != "" {
			keys = append(keys, [2]string{"Tab", "Vorschlag übernehmen"})
		} else if len(s.chips) > 0 {
			keys = append(keys, [2]string{"Alt+1…" + strconv.Itoa(min(4, len(s.chips))), "Vorschlag einfügen"})
		}
		keys = append(keys, [2]string{k.label(actGoal), "Ziel"}, [2]string{k.label(actAsk), "KI fragen"}, [2]string{k.label(actTunnels), "Tunnel"},
			[2]string{k.label(actFiles), "Dateien"}, [2]string{k.label(actHost), "Host"}, [2]string{k.label(actQuickMenu), "Schnellbefehle"})
	}
	if len(a.sessions) > 1 {
		keys = append(keys, [2]string{k.label(actPrevTab) + "/" + k.label(actNextTab), "Tabs"})
	}
	keys = append(keys, [2]string{k.label(actManager), "Manager"},
		[2]string{k.label(actHelp), "Hilfe"}, [2]string{k.label(actCheat), "erklären"}, [2]string{k.label(actQuit), "beenden"})
	keys = sortFKeys(keys)
	if s.fail != nil {
		keys = append([][2]string{{"Alt+0", "Warum? & Fix"}}, keys...)
	}
	return keys
}
