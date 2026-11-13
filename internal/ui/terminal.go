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
	topRows    = 1 // Kopfzeile mit Tabs und Sitzungszustand
	bottomRows = 2 // Vorschläge + Tasten
)

func checkLLM(c domain.LLMConfig) error { return llm.Check(c) }

func (a *App) panelWidth(s *session) int {
	if s == nil || s.panel == panelNone {
		return 0
	}
	w := a.termRect(s).w
	if w < panelNarrow {
		return panelRailW
	}
	if w >= 110 {
		return 48
	}
	return w / 2
}

// Schmale Fenster: Unter panelNarrow Spalten wäre neben dem Panel kaum noch
// Terminal übrig. Dann klappt das Panel zu einer schmalen Leiste am rechten
// Rand ein. Hat es den Fokus (Taste, Klick auf die Leiste), klappt es über
// dem Terminal auf; Esc klappt es wieder ein. Die Terminalgröße bleibt dabei
// gleich, damit der Server nicht bei jedem Fokuswechsel neu umbricht.
const (
	panelNarrow = 100
	panelRailW  = 3
)

func (a *App) panelCollapsed(s *session) bool {
	return s != nil && s.panel != panelNone && a.termRect(s).w < panelNarrow
}

// panelShown: Breite, die das Panel gerade auf dem Bildschirm belegt.
func (a *App) panelShown(s *session) int {
	if !a.panelCollapsed(s) || !s.panelFocus {
		return a.panelWidth(s)
	}
	w := a.termRect(s).w
	return max(panelRailW, min(w-8, max(44, w*2/3)))
}

// panelRail: eingeklappte Leiste mit Symbol und Taste des Panels.
func (a *App) panelRail(s *session, h int) string {
	st := a.s
	icon, act := "◎", actGoal
	switch s.panel {
	case panelAsk:
		icon, act = "◆", actAsk
	case panelTunnels:
		icon, act = "↔", actTunnels
	case panelHost:
		icon, act = "▤", actHost
	case panelCompanion:
		icon, act = "◇", actCompanionBar
	}
	// Zustand: KI arbeitet (◌) oder ein Vorschlag wartet auf dich (●)
	state := ""
	switch {
	case (s.panel == panelGoal && s.goal.busy) || (s.panel == panelAsk && s.ask.busy):
		state = st.AI.Render("◌")
	case s.panel == panelGoal && s.goal.cur != nil:
		state = st.Warn.Render("●")
	}
	cells := []string{st.PanelTitle.Render(icon), state}
	for _, r := range i18n.Keys(a.keys.label(act)) {
		cells = append(cells, st.Accent.Render(string(r)))
	}
	cells = append(cells, "", st.Muted.Render("◂"))
	side := lipgloss.NewStyle().Foreground(st.T.Border).Render("│")
	lines := make([]string, h)
	for i := range lines {
		c := " "
		if i < len(cells) && cells[i] != "" {
			c = cells[i]
		}
		lines[i] = side + ansi.Truncate(c, 1, "") + " "
	}
	return a.z.mark(strings.Join(lines, "\n"), zoneAction{block: true, click: func(tea.Mouse, bool) tea.Cmd {
		s.panelFocus = true
		return nil
	}})
}

// termSize: Spalten und Zeilen des Terminals (Bereich minus Seitenpanel).
func (a *App) termSize(s *session) (int, int) {
	if d := s.detach; d != nil && d.conn != nil {
		return d.cols, d.rows // eigenes Fenster bestimmt die Größe (detach.go)
	}
	return a.paneSize(s)
}

// paneSize: Platz für das Terminal hier im Fenster.
func (a *App) paneSize(s *session) (int, int) {
	r := a.termRect(s)
	return max(20, r.w-a.panelWidth(s)), max(3, r.h)
}

func (a *App) resize(s *session) {
	if s.term == nil {
		return
	}
	c, r := a.termSize(s)
	if s.fixedCols > 0 {
		c, r = s.fixedCols, s.fixedRows // Zuschauer: so groß wie beim Gastgeber
	}
	if cw, rh := s.term.Size(); cw == c && rh == r {
		return
	}
	s.term.Resize(c, r)
	s.resizeRecording(c, r)
	if s.shell != nil {
		_ = s.shell.Resize(c, r)
	}
	a.shareResized(s)
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
		if s.detached() && !s.panelFocus {
			return nil // Einfügen geht im eigenen Fenster
		}
		if s.search.open && !s.panelFocus && !s.files.open {
			return a.searchPaste(s, m)
		}
		if g := a.vg; g != nil && g.focus && g.open {
			return a.vgPaste(m)
		}
		if s.files.open {
			return a.filesPaste(s, m)
		}
		if s.panelFocus {
			return a.panelMsg(s, msg)
		}
		// Dateien aus dem Explorer/Finder ins Fenster gezogen: hochladen?
		if s.client != nil && s.state == stConnected && s.term != nil && !s.term.AltScreen() {
			if paths := droppedPaths(m.Content); paths != nil {
				a.dropMenu(s, m.Content, paths)
				return nil
			}
		}
		if tg := a.syncTargets(s); len(tg) > 0 && s.state == stConnected && s.term != nil {
			return a.syncPaste(s, tg, m.Content)
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
		if !s.panelFocus && !s.files.open && s.term != nil {
			switch a.keys.which(key, actPrevCmd, actNextCmd, actCmdList) { // Befehle als Blöcke (cmdblocks.go)
			case actPrevCmd:
				a.jumpBlock(s, -1)
				return nil
			case actNextCmd:
				a.jumpBlock(s, 1)
				return nil
			case actCmdList:
				return a.openBlocks(s)
			}
		}
		if !s.panelFocus && !s.files.open && s.term != nil && !s.term.AltScreen() {
			if act := a.keys.which(key, actScrollUp, actScrollDown, actScrollTop, actScrollBottom); act != "" {
				a.scrollKey(s, act)
				return nil
			}
		}
		s.sel, s.selecting = nil, false
		if !s.panelFocus && !s.files.open && !(m.Code >= tea.KeyF1 && m.Code <= tea.KeyF12) {
			s.scroll = 0
		}
		if a.keys.is(actVorgang, key) {
			return a.toggleVorgang()
		}
		if g := a.vg; g != nil && g.focus && g.open && !(m.Code >= tea.KeyF1 && m.Code <= tea.KeyF12) {
			return a.vgKey(m) // Leiste „Vorgang“ hat die Tasten (vorgang_view.go)
		}
		// globale Tasten der Terminal-Ansicht (frei belegbar, siehe keymap.go)
		switch a.keys.which(key, actHelp, actAbout, actManager, actPrevTab, actNextTab, actFiles, actCheat, actQuickMenu, actNewTab, actCloseTab, actTabList, actSearchAll) {
		case actSearchAll:
			return a.openSearchAll()
		case actNewTab:
			return a.openNewTab()
		case actCloseTab:
			return a.closeTabAsk(s, nil)
		case actTabList:
			return a.openTabList()
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
			if s.state == stConnected && !s.detached() {
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
		if s.detached() && !s.panelFocus {
			return a.detachedKey(s, key)
		}
		if a.keys.is(actDetach, key) && s.state == stConnected {
			return a.toggleDetach(s)
		}
		if a.keys.is(actSplit, key) {
			return a.openLayout()
		}
		switch a.keys.which(key, actZoom, actGrowLeft, actGrowRight, actGrowUp, actGrowDown) {
		case actZoom:
			a.toggleZoom()
			return nil
		case actGrowLeft, actGrowRight, actGrowUp, actGrowDown:
			if a.split != splitOff {
				a.resizeFocused(map[action]string{actGrowLeft: "left", actGrowRight: "right", actGrowUp: "up", actGrowDown: "down"}[a.keys.which(key, actGrowLeft, actGrowRight, actGrowUp, actGrowDown)])
				return nil
			}
		}
		if a.keys.is(actSyncInput, key) {
			a.toggleSyncInput()
			return nil
		}
		if i, ok := a.quickIndex(key); ok {
			return a.runQuick(s, i)
		}
		if a.keys.is(actLogScan, key) {
			return a.openLogScan(s)
		}
		switch a.keys.which(key, actCompanion, actCompanionBar) {
		case actCompanion:
			return a.cycleCompanion(1)
		case actCompanionBar:
			return a.toggleCompanionBar(s)
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
			if cmd, ok := a.closedKey(s, key); ok {
				return cmd
			}
			switch key {
			case "enter", "r":
				if fresh, ok := a.st.Connection(s.conn.ID); ok {
					s.conn = fresh // inzwischen geändert (z. B. Schlüsseldatei gewählt)
				}
				s.state, s.err, s.errv = stConnecting, "", nil
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
	a.vgForget(s)
	a.endLog(s)
	a.stopRecording(s)
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
	a.agentTyped(s) // Tippen hält einen laufenden Agenten an
	if s.trzKey(k.String()) {
		return nil // Dateiübertragung läuft: nur Strg+C/Esc (bricht ab)
	}
	if tg := a.syncTargets(s); len(tg) > 0 {
		return a.syncKey(s, tg, k)
	}
	return a.terminalKeyOne(s, k)
}

// terminalKeyOne: Taste für genau eine Sitzung.
func (a *App) terminalKeyOne(s *session, k tea.KeyPressMsg) tea.Cmd {
	if a.shellBusyKey(s, k) {
		return nil // sshTermy tippt gerade selbst (shelllock.go)
	}
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
			if s.chipReplace {
				s.term.SendText("\x15") // Strg+U: getippte Zeile durch den Vorschlag ersetzen
				s.chipReplace = false
			}
			s.term.SendText(c.Command)
			s.noteAI(c.Command, true)
			s.chips = nil
		}
		return nil
	}
	if key == "ctrl+c" && s.run != nil {
		s.run.interrupted = true
	}
	if key == "enter" && a.ttyFix(s) {
		return nil // docker exec ohne -it: Zeile ergänzt, erst nächstes ⏎ führt aus (ttyhint.go)
	}
	if key == "enter" && a.dangerCheck(s, k) {
		return nil // Gefahren-Check: erst bestätigen (dangerguard.go)
	}
	if key == "enter" {
		if typed, ok := s.term.TypedInput(); ok && strings.TrimSpace(typed) != "" && !agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
			a.finishCommand(s) // Ergebnis des vorigen Befehls (Fehlschlag?) zuerst
			from := s.correctionFrom(typed)
			a.beginCommand(s, typed)
			if from != "" && s.run != nil {
				s.run.learnFrom = from
			}
			s.history = append(s.history, strings.TrimSpace(typed))
			a.profileCommand(s, strings.TrimSpace(typed))
			s.noteCommand(strings.TrimSpace(typed))
			if len(s.history) > 200 {
				s.history = s.history[len(s.history)-200:]
			}
			s.followDue = time.Now().Add(1200 * time.Millisecond)
		}
		s.chips, s.chipArmed, s.chipReplace, s.compChips = nil, -1, false, false
		s.ghost = ""
		s.comp.typedSeen, s.comp.remark = "", nil // Bemerkung galt dem vorigen Befehl
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
	return tea.Batch(a.scheduleGhost(s), a.companionTyped(s))
}

// ---------------------------------------------------------------- Ghost-Text

type ghostTickMsg struct{ sid, seq int }
type ghostMsg struct {
	sid, seq    int
	typed, rest string
	ok          bool // Antwort kam (auch „nichts Passendes“), kein Fehler
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
	// Sparen: Der vorhandene Vorschlag passt noch (weitergetippt, wie
	// vorgeschlagen), oder dieselbe Eingabe wurde eben schon beantwortet.
	if a.visibleGhost(s) != "" {
		return nil
	}
	if rest, hit := s.ghostLookup(typed, time.Now()); hit {
		s.ghost, s.ghostFor = rest, typed
		return nil
	}
	ag, err := a.agentClientFor(s.conn)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	s.ghostCancel = cancel
	c := a.agentContext(s, true)
	sid := s.id
	return func() tea.Msg {
		defer cancel()
		rest, err := ag.Complete(ctx, typed, c)
		return ghostMsg{sid, seq, typed, rest, err == nil}
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
	if a.compLevel() >= domain.CompanionChatty && a.companionOff(s) == "" {
		s.followDue = time.Time{}
		return nil // der Begleiter kommentiert ohnehin jeden Befehl und bringt Vorschläge mit
	}
	if time.Since(s.lastOutput) < 800*time.Millisecond || !s.term.AtIdlePrompt() {
		return nil
	}
	s.followDue = time.Time{}
	if time.Since(s.lastFollow) < 5*time.Second {
		return nil
	}
	ag, err := a.agentClientFor(s.conn)
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
	top := a.termHeader(s) // Logo, Tabs, Zustand (statusline.go)

	// Terminalfläche (eine Sitzung oder geteilt)
	var body string
	var cur *tea.Cursor
	if a.split != splitOff && a.slotOf(s) >= 0 {
		body, cur = a.viewSplit(s)
	} else {
		body, cur = a.paneBody(s, true)
		if a.framed() {
			ar := a.areaRect()
			body = a.frameBody(body, ar.w, ar.h, s, true, false, "")
		}
	}
	bottom := a.chipBar(s)
	if s.search.open {
		bottom = a.searchBar(s)
	}
	if line := a.companionLine(s); line != "" {
		bottom = line + "\n" + bottom
	}
	if dock := a.viewVorgang(); dock != "" {
		body += "\n" + dock // Vorgang über mehrere Fenster (vorgang_view.go)
		if a.vg.focus {
			cur = nil
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, top, body, bottom, a.footer(a.simpleKeys(a.terminalKeys(s)))), cur
}

// paneBody zeichnet die Terminalfläche einer Sitzung (samt Panel/Dateien).
// Nur die aktive Sitzung bekommt Cursor und Ghost-Text.
func (a *App) paneBody(s *session, focused bool) (string, *tea.Cursor) {
	st := a.s
	cols, rows := a.paneSize(s)
	tr := a.termRect(s)
	if s.search.open {
		a.refreshSearch(s, false)
	}
	var body string
	var cur *tea.Cursor
	switch {
	case s.detached():
		body = a.detachedView(s, cols, rows)
	case s.state == stConnecting:
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, a.connectCard(s, cols-4)) // motion2.go
	case s.state == stReconnect:
		body = lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, a.reconnectView(s))
	case s.state == stClosed:
		body = a.closedView(s, cols, rows) // connerr.go
	case s.search.open && s.search.list:
		body = a.viewErrorList(s, cols, rows)
	case s.scroll > 0 || s.sel != nil || s.search.open:
		// Rückblick im Verlauf und/oder Markierung
		if sb := s.term.ScrollbackLen(); s.scroll > 0 && sb > s.sbSeen {
			s.scroll = min(sb, s.scroll+sb-s.sbSeen) // neue Ausgabe verschiebt die Ansicht nicht
		}
		s.scroll = min(s.scroll, s.term.ScrollbackLen())
		if s.search.open {
			body = s.term.RenderMarked(s.scroll, s.sel, a.searchMarks(s), a.markStyle(), a.termHL())
		} else {
			body = s.term.RenderHL(s.scroll, s.sel, a.termHL())
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
		body = a.termView(s, ghost)
		body = revealLines(s, body, rows, time.Now())
		if x, y, vis := s.term.Cursor(); vis && focused && !s.panelFocus {
			cur = tea.NewCursor(tr.x+x, tr.y+y)
			cur.Color = st.T.Accent
		}
	}
	if s.term != nil {
		s.sbSeen = s.term.ScrollbackLen()
	}
	if s.fixedCols > 0 {
		body = cropView(body, cols, rows)
	}
	if s.files.open {
		body, cur = a.viewFiles(s, tr.w, rows), nil
	} else if s.panel == panelHost && s.host.live.wide && s.host.live.hasLive() {
		body, cur = a.viewPanel(s, tr.w, rows), nil // F6 groß: über dem Terminal, ohne dessen Größe zu ändern
	} else if a.panelCollapsed(s) {
		if s.panelFocus {
			pw := a.panelShown(s)
			vis := cols + panelRailW - pw
			lines := strings.Split(body, "\n")
			for i, l := range lines {
				l = ansi.Truncate(l, vis, "")
				lines[i] = l + strings.Repeat(" ", max(0, vis-lipgloss.Width(l)))
			}
			body, cur = lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(lines, "\n"), a.viewPanel(s, pw, rows)), nil
		} else {
			body = beside(body, cols, a.panelRail(s, rows)) // perf.go
		}
	} else if s.panel != panelNone {
		body = beside(body, cols, a.viewPanel(s, a.panelWidth(s), rows))
	}

	return body, cur
}

func (a *App) chipBar(s *session) string {
	st := a.s
	var b strings.Builder
	if g := a.visibleGhost(s); g != "" {
		b.WriteString(a.z.click(st.AI.Render(" ◆ ")+st.Muted.Render(i18n.T("Tab übernimmt: "))+st.Ghost.Render(trunc(g, 60)),
			func(tea.Mouse, bool) tea.Cmd { s.panelFocus = false; return a.pressKey("Tab") }) + "  ")
	} else {
		if f := s.fail; f != nil {
			why := orDefault(f.Reason, i18n.T("fehlgeschlagen"))
			b.WriteString(a.z.click(st.Danger.Render(" × "+trunc(why, 34))+" "+st.Key.Render("Alt+0")+st.AI.Render(i18n.T(" Warum? & Fix ")),
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
		b.WriteString(st.AI.Render(i18n.T(" ◆ Als Nächstes: ")))
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
	if g := a.vg; g != nil && g.focus && g.open {
		return a.vgKeys()
	}
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
	keys := [][2]string{{k.label(actPalette), i18n.T("Befehle")}}
	switch {
	case s.state == stClosed:
		keys = append(keys, [2]string{"⏎", i18n.T("neu verbinden")}, [2]string{"x", i18n.T("Tab schließen")})
	case s.detached():
		keys = append(keys, [2]string{"⏎", i18n.T("zurückholen")})
	case s.state == stConnected:
		if s.scroll > 0 {
			keys = append(keys, [2]string{"Esc", i18n.T("zurück zur Eingabe")})
		}
		if a.visibleGhost(s) != "" {
			keys = append(keys, [2]string{"Tab", i18n.T("Vorschlag übernehmen")})
		} else if len(s.chips) > 0 {
			keys = append(keys, [2]string{i18n.T("Alt+1…") + strconv.Itoa(min(4, len(s.chips))), i18n.T("Vorschlag einfügen")})
		}
		keys = append(keys, [2]string{k.label(actGoal), i18n.T("Ziel")}, [2]string{k.label(actAsk), i18n.T("KI")}, [2]string{k.label(actTunnels), "Tunnel"},
			[2]string{k.label(actFiles), i18n.T("Dateien")}, [2]string{k.label(actHost), "Host"}, [2]string{k.label(actQuickMenu), i18n.Pick("Schnell", "Quick")})
	}
	if len(a.sessions) > 1 {
		keys = append(keys, [2]string{k.label(actPrevTab) + "/" + k.label(actNextTab), "Tabs"})
	}
	keys = append(keys, [2]string{k.label(actManager), "Manager"},
		[2]string{k.label(actHelp), i18n.T("Hilfe")}, [2]string{k.label(actCheat), i18n.T("erklären")})
	if s.state == stConnected && !s.detached() {
		// weniger wichtig: fällt in schmalen Fenstern vor den F-Tasten weg
		keys = append(keys, [2]string{k.label(actSearch), i18n.T("Suche")}, [2]string{k.label(actSplit), i18n.T("Teilen")},
			[2]string{k.label(actDetach), i18n.T("Fenster")})
	}
	if len(a.sessions) > 1 {
		keys = append(keys, [2]string{k.label(actTabList), i18n.T("Tab-Liste")})
	}
	if s.state == stConnected && !s.detached() {
		keys = append(keys, [2]string{k.label(actNewTab), i18n.T("neuer Tab")})
	}
	keys = append(keys, [2]string{k.label(actQuit), i18n.T("beenden")})
	// Anzeige-Reihenfolge (Palette, F1 … F12, ⇧F, Rest) macht footer().
	if s.fail != nil {
		keys = append([][2]string{{"Alt+0", i18n.T("Warum? & Fix")}}, keys...)
	}
	return keys
}

// tabLabel: Fensternummer (vorgang.go) und Name, auf l Zeichen gekürzt.
func (a *App) tabLabel(i, l int) string {
	x := a.sessions[i]
	if l == 0 {
		return " " + strconv.Itoa(a.winNo(x))
	}
	return " " + strconv.Itoa(a.winNo(x)) + " " + trunc(x.tabName(), l) + goalTabCount(x) // Schritte des Ziels
}

// tabWidth: Streifen, Punkt, Text, Leerraum, Innenabstand, × am aktiven.
func (a *App) tabWidth(i, l int) int {
	w := 1 + lipgloss.Width(a.tabLabel(i, l)) + 3 + 1   // + Lücke danach
	if i == a.cur || a.tabColor(a.sessions[i]) != nil { // Streifen bzw. ▶
		w++
	}
	if i == a.cur {
		w++ // „×“ statt des Leerzeichens am Ende
	}
	return w
}

// tabsNatural: Breite der Tab-Leiste mit ungekürzten Namen (samt „+“).
func (a *App) tabsNatural() int {
	if len(a.sessions) == 0 {
		return 0
	}
	w := 3
	for i := range a.sessions {
		w += a.tabWidth(i, 16)
	}
	return w
}

// tabStrip: Sitzungs-Tabs für die Kopfzeile. Wird es eng, kürzen sich erst
// die Namen (16 → 4 Zeichen), dann bleibt nur Punkt und Nummer; reicht auch
// das nicht, zeigt die Leiste einen Ausschnitt um den aktiven Tab mit ‹ ›.
func (a *App) tabStrip(room int) string {
	st := a.s
	n := len(a.sessions)
	if n == 0 {
		return ""
	}
	plus := st.Tab.Foreground(st.T.Accent).Bold(true).Render("+")
	room -= lipgloss.Width(plus)
	tabW := a.tabWidth
	label := a.tabLabel
	fit := 0
	for _, l := range []int{16, 12, 8, 4, 0} {
		fit = l
		total := 0
		for i := 0; i < n; i++ {
			total += tabW(i, l)
		}
		if total <= room {
			break
		}
	}
	// Ausschnitt um den aktiven Tab
	from, to := 0, n
	total := func() int {
		t := 0
		for i := from; i < to; i++ {
			t += tabW(i, fit)
		}
		if from > 0 {
			t += 2
		}
		if to < n {
			t += 2
		}
		return t
	}
	for total() > room && to-from > 1 {
		if a.cur-from > to-1-a.cur {
			from++
		} else {
			to--
		}
	}
	var b strings.Builder
	if from > 0 {
		b.WriteString(st.Header.Foreground(st.T.Muted).Render("‹ "))
	}
	now := time.Now()
	for i := from; i < to; i++ {
		x := a.sessions[i]
		if a.sessionVisible(x) {
			x.unseen = false
			if !a.blurred && x.goal.done {
				x.goal.doneSeen = true
			}
		}
		dot := a.tabDot(x, now) // Spinner, Puls bei neuer Ausgabe (motion.go)
		if x.detached() {
			dot = st.AI.Render("↑") // läuft in eigenem Fenster
		}
		if g := a.noticeGlyph(x); g != "" {
			dot = g // Befehl fertig / Glocke, solange der Tab verdeckt war
		}
		if x.share.Load() != nil {
			dot = st.Danger.Bold(true).Render("◉") // live geteilt: andere sehen zu
		}
		if g := a.goalGlyph(x); g != "" {
			dot = g // Ziel läuft / wartet auf dich / fertig (goalux.go)
		}
		// Aktiver Tab: Fläche in der Tab-Farbe, „▶“ vorn, fett; die anderen
		// liegen gedämpft auf eigener Fläche. Ohne Farbe bleiben ▶, Fettdruck
		// und × am aktiven (activetab.go).
		ts := st.Tab
		stripe, text := "", label(i, fit)
		if i == a.cur {
			fill := a.activeFill(x)
			ink := inkOn(st.T, fill)
			ts = st.TabActive.Background(tabFlashBg(x, fill, now)).Foreground(ink)
			dot = lipgloss.NewStyle().Foreground(ink).Background(ts.GetBackground()).Render(ansi.Strip(dot))
			// ▶ steht statt des linken Innenabstands, danach ein Leerzeichen
			ts = ts.PaddingLeft(0)
			stripe = ts.UnsetPadding().Render(activeMark + " ")
			text = ts.UnsetPadding().Render(text)
		} else {
			ts = ts.Background(tabFlashBg(x, ts.GetBackground(), now))
			dot = lipgloss.NewStyle().Background(ts.GetBackground()).Render(dot) // Punkt auf dem Tab-Hintergrund
			// Akzentfarbe der Verbindung als Streifen links im Tab (accent.go)
			if col := a.tabColor(x); col != nil {
				stripe = glyph("▌", col, ts.GetBackground())
				if a.framed() { // Name gedämpft in der Rahmenfarbe (tabframe.go)
					text = " " + lipgloss.NewStyle().Foreground(mixColor(ts.GetBackground(), col, 0.85)).Background(ts.GetBackground()).Render(strings.TrimPrefix(text, " "))
				}
			}
		}
		i := i
		end := " "
		if i == a.cur {
			end = ""
		}
		tab := a.z.mark(ts.Render(stripe+dot+ts.UnsetPadding().Render(text+end)), zoneAction{
			tab: i + 1,
			click: func(m tea.Mouse, _ bool) tea.Cmd {
				a.focusSession(i)
				a.tabDrag = &tabDrag{id: x.id, x: m.X} // Ziehen sortiert um (tabs.go)
				return nil
			},
			menu: func(m tea.Mouse) tea.Cmd {
				if m.Button == tea.MouseMiddle {
					return a.closeTabAsk(x, nil)
				}
				a.modal = a.tabMenu(x, m.X, m.Y, nil)
				return nil
			},
			tip: func() string { return a.tabTip(x) },
		})
		if i == a.cur {
			// × schließt den aktiven Tab (bei laufender Verbindung mit Rückfrage)
			close := a.z.click(ts.Padding(0, 1, 0, 0).UnsetBold().Render("×"), func(tea.Mouse, bool) tea.Cmd {
				return a.closeTabAsk(x, nil)
			})
			tab += close
		}
		b.WriteString(tab + st.Header.Render(" ")) // Lücke: jeder Tab ein eigener Knopf
	}
	if to < n {
		b.WriteString(st.Header.Foreground(st.T.Muted).Render(" ›"))
	}
	b.WriteString(a.z.click(plus, func(tea.Mouse, bool) tea.Cmd { return a.openNewTab() }))
	return b.String()
}
