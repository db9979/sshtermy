package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/codeagent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// MCP zum Steuern (Idee aus herdr): Ein KI-Client (Claude Code, Claude
// Desktop …) kann über sshTermy sichtbare Tabs öffnen, hineintippen und
// warten, bis etwas passiert. Damit bedient er mehrere Server oder einen
// Coding-Agenten in einem anderen Tab, und du siehst alles mit.
//
// Regeln: nur bei „MCP-Zugriff: mit Rückfrage“. Am Shell-Prompt läuft ein
// lesender Befehl (ohne sudo, ohne Geheimnisse) direkt; alles andere fragt
// hier im Fenster (j, bei Gefahr j zweimal). Eingaben an laufende Programme
// (Rückfragen, Coding-Agenten) fragen immer. Passwörter tippt der KI-Client
// nie. Läuft im Tab ein Ziel oder der Agent-Modus, lehnt sshTermy ab.

const (
	mcpWaitDefault = 120 * time.Second
	mcpWaitMax     = 600 * time.Second
	mcpQuiet       = 2 * time.Second
)

type mcpWaiter struct {
	sid      int
	until    string
	text     string
	deadline time.Time
	since    time.Time
	reply    func(string, error)
}

// mcpStatus: Zustand eines Tabs in einfachen Worten (Englisch, für die KI).
func (a *App) mcpStatus(s *session) string {
	if s.term == nil || s.state != stConnected {
		return "not connected"
	}
	if w := s.agentW; w.name != "" {
		switch w.state {
		case codeagent.Working:
			return w.name + " working"
		case codeagent.Blocked:
			return w.name + " waiting for approval"
		}
		return w.name + " ready"
	}
	if kind, _ := waitKind(s); kind != "" {
		return map[string]string{"secret": "waiting for a password (the user types it)", string(agent.InputYesNo): "waiting for yes/no",
			string(agent.InputConfirm): "waiting for confirmation", string(agent.InputPager): "in a pager (space/q)", string(agent.InputAnyKey): "waiting for a key"}[kind]
	}
	if s.term.AtIdlePrompt() {
		return "at prompt"
	}
	return "running"
}

// mcpControl: open_tab, send_input, wait_tab. false = kein Werkzeug von hier.
func (a *App) mcpControl(m mcpCallMsg) (tea.Cmd, bool) {
	fail := func(f string, x ...any) (tea.Cmd, bool) { m.reply("", fmt.Errorf(f, x...)); return nil, true }
	switch m.tool {
	case "open_tab":
		if a.mcpLevel() != mcpAsk {
			return fail("opening tabs needs the sshTermy setting “MCP access: with confirmation”")
		}
		var p struct{ Connection string }
		_ = json.Unmarshal(m.args, &p)
		c, ok := a.mcpFind(strings.TrimSpace(p.Connection))
		switch {
		case !ok:
			return fail("no connection %q (see list_connections)", p.Connection)
		case !mcpAllowed(a, c):
			return fail("AI access is off for this connection (privacy tag)")
		}
		cur, scr := a.cur, a.scr
		cmd := a.connect(c)
		s := a.current()
		// nicht aus der Arbeit reißen: der neue Tab öffnet sich im Hintergrund
		if scr == scrTerminal && cur >= 0 && cur < len(a.sessions)-1 {
			a.focusSession(cur)
		}
		a.auditAdd(audit.EvAgent, auditConn(c), "MCP: Tab geöffnet")
		a.flash(i18n.F("MCP: Tab %s geöffnet", c.Alias))
		m.reply(fmt.Sprintf(`{"tab": %d, "alias": %q, "status": "connecting"}`, s.id, c.Alias), nil)
		return cmd, true
	case "send_input":
		return a.mcpSend(m), true
	case "wait_tab":
		return a.mcpWait(m), true
	}
	return nil, false
}

func (a *App) mcpTab(id int) (*session, error) {
	s := a.byID(id)
	switch {
	case s == nil || s.term == nil:
		return nil, fmt.Errorf("no open tab %d (see list_tabs)", id)
	case !mcpAllowed(a, s.conn):
		return nil, errors.New("AI access is off for this connection (privacy tag)")
	}
	return s, nil
}

func (a *App) mcpSend(m mcpCallMsg) tea.Cmd {
	var p struct {
		Tab   int
		Text  string
		Enter *bool
	}
	_ = json.Unmarshal(m.args, &p)
	enter := p.Enter == nil || *p.Enter
	text := strings.TrimRight(p.Text, "\r\n")
	s, err := a.mcpTab(p.Tab)
	switch {
	case a.mcpLevel() != mcpAsk:
		err = errors.New("typing into tabs needs the sshTermy setting “MCP access: with confirmation”")
	case err != nil:
	case text == "" && !enter:
		err = errors.New("text is empty")
	case s.state != stConnected:
		err = errors.New("the tab is not connected (wait_tab until=prompt after open_tab)")
	case s.goal.active:
		err = errors.New("sshTermy's own goal/agent mode is running in this tab; try again later")
	case s.detached():
		err = errors.New("this tab runs in its own window")
	case agent.LooksLikeSecretPrompt(s.term.CursorLine()):
		err = errors.New("the tab asks for a password; only the user types it")
	case strings.ContainsAny(text, "\x00\x03\x04\x1b"):
		err = errors.New("control characters are not allowed; send plain text")
	}
	if err != nil {
		m.reply("", err)
		return nil
	}
	atPrompt := s.term.AtIdlePrompt() && s.agentW.name == ""
	kind := mcpKind(s.conn)
	oneLine := !strings.Contains(text, "\n")
	var dangers []safety.Finding
	if atPrompt {
		dangers = safety.Check(text, kind)
	}
	direct := atPrompt && oneLine && enter && safety.ReadOnly(text, kind) && len(dangers) == 0 &&
		safety.Sensitive(text) == "" && !safety.Elevated(text)
	do := func() tea.Cmd {
		a.auditAdd(audit.EvAgent, auditConn(s.conn), "MCP tippt: "+text)
		if atPrompt && enter && oneLine {
			a.beginCommand(s, text)
			a.sendLine(s, text)
			s.history = append(s.history, text)
		} else {
			s.term.SendText(strings.ReplaceAll(text, "\n", "\r"))
			if enter {
				s.term.SendText("\r")
			}
		}
		s.lastOutput = time.Now() // wait_tab wartet ab jetzt
		m.reply(fmt.Sprintf("typed into tab %d (%s); status was: %s", s.id, s.conn.Alias, a.mcpStatus(s)), nil)
		return nil
	}
	if direct {
		a.flash(i18n.F("MCP tippt in %s: %s", s.tabName(), trunc(text, 50)))
		return do()
	}
	if a.modal != nil {
		m.reply("", errors.New("sshTermy is busy with another dialog; try again in a moment"))
		return nil
	}
	where := i18n.T("am Prompt")
	if !atPrompt {
		where = i18n.T("in das laufende Programm") + " (" + a.mcpStatus(s) + ")"
	}
	body := i18n.F("Ein KI-Client möchte in %s tippen, %s:", s.tabName(), where) + "\n\n  " + strings.ReplaceAll(trunc(text, 600), "\n", "\n  ")
	if enter {
		body += "  ⏎"
	}
	for _, d := range dangers {
		body += "\n▲ " + d.Reason
	}
	if len(dangers) > 0 {
		body += i18n.T("\n\nj zweimal = tippen · n = ablehnen")
	} else {
		body += i18n.T("\n\nj = tippen · n = ablehnen")
	}
	md := confirmModal(i18n.T("MCP: Eingabe erlauben?"), body, do)
	md.onCancel = func() tea.Cmd {
		a.auditAdd("mcp", auditConn(s.conn), "abgelehnt: "+text)
		m.reply("", errors.New("the user declined the input"))
		return nil
	}
	md.twice = len(dangers) > 0
	a.showAsync(md)
	return nil
}

func (a *App) mcpWait(m mcpCallMsg) tea.Cmd {
	var p struct {
		Tab     int
		Until   string
		Text    string
		Timeout int `json:"timeout_seconds"`
	}
	_ = json.Unmarshal(m.args, &p)
	s, err := a.mcpTab(p.Tab)
	if err != nil {
		m.reply("", err)
		return nil
	}
	until := strings.ToLower(strings.TrimSpace(p.Until))
	if until == "" {
		until = "prompt"
	}
	switch until {
	case "prompt", "input", "quiet":
	case "text":
		if strings.TrimSpace(p.Text) == "" {
			m.reply("", errors.New("until=text needs text"))
			return nil
		}
	default:
		m.reply("", fmt.Errorf("until must be prompt, input, text or quiet, not %q", p.Until))
		return nil
	}
	d := mcpWaitDefault
	if p.Timeout > 0 {
		d = min(time.Duration(p.Timeout)*time.Second, mcpWaitMax)
	}
	now := time.Now()
	w := &mcpWaiter{sid: s.id, until: until, text: p.Text, deadline: now.Add(d), since: now, reply: m.reply}
	if !a.mcpWaitDone(w, now) {
		a.mcpWaits = append(a.mcpWaits, w)
	}
	return nil
}

// mcpWaitTick (aus onTick): erfüllte oder abgelaufene Wartende beantworten.
func (a *App) mcpWaitTick(now time.Time) {
	keep := a.mcpWaits[:0]
	for _, w := range a.mcpWaits {
		if !a.mcpWaitDone(w, now) {
			keep = append(keep, w)
		}
	}
	a.mcpWaits = keep
}

// mcpWaitDone: Bedingung erfüllt (oder Tab weg, Zeit um)? Dann antworten.
func (a *App) mcpWaitDone(w *mcpWaiter, now time.Time) bool {
	s := a.byID(w.sid)
	if s == nil || s.term == nil {
		w.reply("", fmt.Errorf("tab %d was closed", w.sid))
		return true
	}
	quiet := now.Sub(s.lastOutput) >= mcpQuiet
	ok := false
	if s.state == stConnected {
		switch w.until {
		case "prompt":
			ok = quiet && s.term.AtIdlePrompt() && s.agentW.name == "" && now.Sub(w.since) >= 500*time.Millisecond
		case "input":
			st := s.agentW.state
			ok = (s.agentW.name != "" && (st == codeagent.Blocked || st == codeagent.Idle) && quiet) ||
				(s.agentW.name == "" && quiet && func() bool { k, _ := waitKind(s); return k != "" }())
		case "text":
			ok = strings.Contains(s.term.ScreenText(), w.text)
		case "quiet":
			ok = quiet
		}
	}
	if !ok && now.Before(w.deadline) {
		return false
	}
	head := "status: " + a.mcpStatus(s)
	if !ok {
		head = "timeout, " + head
	}
	w.reply(head+"\n\n"+agent.Redact(s.term.PlainLines(40)), nil)
	return true
}
