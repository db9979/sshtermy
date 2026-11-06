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
	"github.com/db9979/sshtermy/internal/i18n"
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
	w := a.termRect(s).w
	if w >= 110 {
		return 48
	}
	return w / 2
}

// termSize: Spalten und Zeilen des Terminals (Bereich minus Seitenpanel).
func (a *App) termSize(s *session) (int, int) {
	r := a.termRect(s)
	return max(20, r.w-a.panelWidth(s)), max(3, r.h)
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
		if s.search.open && !s.panelFocus && !s.files.open {
			return a.searchPaste(s, m)
		}
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
		// geteilte Ansicht: Strg+Pfeil wechselt den Bereich
		if dir, ok := paneDir(key); ok && a.split != splitOff {
			a.movePane(dir)
			return nil
		}
		if s.search.open && !s.panelFocus && !s.files.open {
			if cmd, ok := a.searchKey(s, m); ok {
				return cmd
			}
		}
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
				a.focusSession((a.cur + d + len(a.sessions)) % len(a.sessions))
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
		if a.keys.is(actSplit, key) {
			a.cycleSplit()
			return nil
		}
		if i, ok := a.quickIndex(key); ok {
			return a.runQuick(s, i)
		}
		switch id := a.keys.which(key, actGoal, actAsk, actTunnels, actHost); id {
		case actGoal, actAsk, actTunnels, actHost:
			if id == actTunnels && s.conn.Protocol.Raw() {
				a.flash(i18n.T("Dateien und Tunnel gibt es nur bei SSH-Verbindungen"))
				return nil
			}
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
		if a.keys.is(actSearch, key) {
			return a.openSearch(s)
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
				if fresh, ok := a.st.Connection(s.conn.ID); ok {
					s.conn = fresh // inzwischen geändert (z. B. Schlüsseldatei gewählt)
				}
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
	a.fixFocus()
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
				a.flash(i18n.T("Vorsicht: ") + c.Danger[0].Reason + i18n.T(". Nochmal ") + a.keys.label(id) + i18n.T(" zum Einfügen"))
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
			a.focusSession(i)
			return nil
		}))
	}
	top := a.header(tabs.String())

	// Sitzungszeile (statusline.go)
	bar := a.sessionBar(s)

	// Terminalfläche (eine Sitzung oder geteilt)
	var body string
	var cur *tea.Cursor
	if a.split != splitOff && a.slotOf(s) >= 0 {
		body, cur = a.viewSplit(s)
	} else {
		body, cur = a.paneBody(s, true)
	}
	bottom := a.chipBar(s)
	if s.search.open {
		bottom = a.searchBar(s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, top, bar, body, bottom, a.footer(a.terminalKeys(s))), cur
}

// paneBody zeichnet die Terminalfläche einer Sitzung (samt Panel/Dateien).
// Nur die aktive Sitzung bekommt Cursor und Ghost-Text.
func (a *App) paneBody(s *session, focused bool) (string, *tea.Cursor) {
	st := a.s
	cols, rows := a.termSize(s)
	tr := a.termRect(s)
	if s.search.open {
		a.refreshSearch(s, false)
	}
	var body string
	var cur *tea.Cursor
	switch {
	case s.state == stConnecting:
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center,
			st.AI.Render(i18n.T("⠿ Verbinde mit ")+s.conn.Alias+" ("+s.conn.Addr()+") …"))
	case s.state == stReconnect:
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, a.reconnectView(s))
	case s.state == stClosed:
		msg := st.Danger.Render("✗ " + orDefault(s.err, i18n.T("getrennt")))
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center,
			msg+"\n\n"+st.Muted.Render(i18n.T("⏎ neu verbinden · x Tab schließen · ")+a.keys.label(actManager)+" Manager"))
	case s.search.open && s.search.list:
		body = a.viewErrorList(s, cols, rows)
	case s.scroll > 0 || s.sel != nil || s.search.open:
		// Rückblick im Verlauf und/oder Markierung
		if sb := s.term.ScrollbackLen(); s.scroll > 0 && sb > s.sbSeen {
			s.scroll = min(sb, s.scroll+sb-s.sbSeen) // neue Ausgabe verschiebt die Ansicht nicht
		}
		s.scroll = min(s.scroll, s.term.ScrollbackLen())
		if s.search.open {
			body = s.term.RenderMarked(s.scroll, s.sel, a.searchMarks(s), a.markStyle())
		} else {
			body = s.term.Render(s.scroll, s.sel)
		}
		if x, y, vis := s.term.Cursor(); vis && focused && !s.panelFocus && s.scroll == 0 && !s.search.open {
			cur = tea.NewCursor(tr.x+x, tr.y+y)
			cur.Color = st.T.Accent
		}
	default:
		ghost := ""
		if focused {
			ghost = a.visibleGhost(s)
		}
		body = s.term.View(ghost, func(g string) string { return st.Ghost.Render(g) })
		if x, y, vis := s.term.Cursor(); vis && focused && !s.panelFocus {
			cur = tea.NewCursor(tr.x+x, tr.y+y)
			cur.Color = st.T.Accent
		}
	}
	if s.term != nil {
		s.sbSeen = s.term.ScrollbackLen()
	}
	if s.files.open {
		body, cur = a.viewFiles(s, tr.w, rows), nil
	} else if s.panel != panelNone {
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(cols).Render(body), a.viewPanel(s, a.panelWidth(s), rows))
	}

	return body, cur
}

func (a *App) chipBar(s *session) string {
	st := a.s
	var b strings.Builder
	if g := a.visibleGhost(s); g != "" {
		b.WriteString(a.z.click(st.AI.Render(" ✦ ")+st.Muted.Render(i18n.T("Tab übernimmt: "))+st.Ghost.Render(trunc(g, 60)),
			func(tea.Mouse, bool) tea.Cmd { s.panelFocus = false; return a.pressKey("Tab") }) + "  ")
	} else {
		if f := s.fail; f != nil {
			why := orDefault(f.Reason, i18n.T("fehlgeschlagen"))
			b.WriteString(a.z.click(st.Danger.Render(" ✗ "+trunc(why, 34))+" "+st.Key.Render("Alt+0")+st.AI.Render(i18n.T(" Warum? & Fix ")),
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
		b.WriteString(st.AI.Render(i18n.T(" ✦ Als Nächstes: ")))
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
	if s.search.open {
		if s.search.list {
			return [][2]string{{"↑/↓", i18n.T("wählen")}, {"⏎", i18n.T("im Verlauf zeigen")}, {"Tab", i18n.T("Liste/Verlauf")}, {"Strg+E", i18n.T("Filter aus")}, {"Esc", i18n.T("Suche schließen")}}
		}
		k := [][2]string{{"⏎/↑", i18n.T("älter")}, {"Shift+⏎/↓", i18n.T("neuer")}, {"Bild↑/Bild↓", i18n.T("blättern")}, {"Strg+E", i18n.T("Fehlerfilter")}}
		if s.search.errors {
			k = append(k, [2]string{"Tab", i18n.T("Fehlerliste")})
		}
		return append(k, [2]string{"Esc", i18n.T("Suche schließen")})
	}
	k := a.keys
	keys := [][2]string{{k.label(actPalette), "Palette"}}
	switch {
	case s.state == stClosed:
		keys = append(keys, [2]string{"⏎", i18n.T("neu verbinden")}, [2]string{"x", i18n.T("Tab schließen")})
	case s.state == stConnected:
		if s.scroll > 0 {
			keys = append(keys, [2]string{"Esc", i18n.T("zurück zur Eingabe")})
		}
		if a.visibleGhost(s) != "" {
			keys = append(keys, [2]string{"Tab", i18n.T("Vorschlag übernehmen")})
		} else if len(s.chips) > 0 {
			keys = append(keys, [2]string{i18n.T("Alt+1…") + strconv.Itoa(min(4, len(s.chips))), i18n.T("Vorschlag einfügen")})
		}
		keys = append(keys, [2]string{k.label(actGoal), i18n.T("Ziel")}, [2]string{k.label(actAsk), i18n.T("KI fragen")}, [2]string{k.label(actTunnels), "Tunnel"},
			[2]string{k.label(actFiles), i18n.T("Dateien")}, [2]string{k.label(actSearch), i18n.T("Suche")}, [2]string{k.label(actSplit), i18n.T("Teilen")}, [2]string{k.label(actHost), "Host"}, [2]string{k.label(actQuickMenu), i18n.T("Schnellbefehle")})
	}
	if len(a.sessions) > 1 {
		keys = append(keys, [2]string{k.label(actPrevTab) + "/" + k.label(actNextTab), "Tabs"})
	}
	keys = append(keys, [2]string{k.label(actManager), "Manager"},
		[2]string{k.label(actHelp), i18n.T("Hilfe")}, [2]string{k.label(actCheat), i18n.T("erklären")}, [2]string{k.label(actQuit), i18n.T("beenden")})
	keys = sortFKeys(keys)
	if s.fail != nil {
		keys = append([][2]string{{"Alt+0", i18n.T("Warum? & Fix")}}, keys...)
	}
	return keys
}
