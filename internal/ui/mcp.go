package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/mcp"
	"github.com/db9979/sshtermy/internal/safety"
)

// sshTermy als MCP-Server (internal/mcp): Einstellungen › Sicherheit ›
// „MCP-Zugriff“ = aus / nur lesen / mit Rückfrage. Ein KI-Client startet
// „sshtermy mcp“; der reicht jeden Aufruf hierher. Gelesen wird nur, was
// der KI-Datenschutz erlaubt; Befehle, die etwas ändern könnten, laufen nur
// nach Rückfrage hier im Fenster.

const (
	mcpOff    = ""
	mcpRead   = "lesen"
	mcpAsk    = "rueckfrage"
	mcpMaxOut = 100_000
)

func mcpLabel(level string) string {
	switch level {
	case mcpRead:
		return i18n.T("nur lesen")
	case mcpAsk:
		return i18n.T("mit Rückfrage")
	}
	return i18n.T("aus")
}

func (a *App) mcpLevel() string { return a.st.Settings().MCP }

// mcpCallMsg: Aufruf aus dem Socket-Goroutine.
type mcpCallMsg struct {
	tool  string
	args  json.RawMessage
	reply func(string, error)
}

func (a *App) startMCP() {
	if a.mcpLevel() == mcpOff || a.mcpHost != nil {
		return
	}
	ev := a.events
	h, err := mcp.Listen(a.st.Dir, func(tool string, args json.RawMessage) (string, error) {
		type res struct {
			text string
			err  error
		}
		ch := make(chan res, 1)
		var once sync.Once
		ev <- mcpCallMsg{tool, args, func(t string, e error) { once.Do(func() { ch <- res{t, e} }) }}
		select {
		case r := <-ch:
			return r.text, r.err
		case <-time.After(10 * time.Minute):
			return "", errors.New("no answer from sshTermy (confirmation timed out)")
		}
	})
	if err != nil {
		a.flashErr("MCP: " + err.Error())
		return
	}
	a.mcpHost = h
}

func (a *App) stopMCP() {
	if a.mcpHost != nil {
		a.mcpHost.Close()
		a.mcpHost = nil
	}
}

func (a *App) cycleMCP(dir int) tea.Cmd {
	levels := []string{mcpOff, mcpRead, mcpAsk}
	cur := 0
	for i, l := range levels {
		if l == a.mcpLevel() {
			cur = i
		}
	}
	next := levels[cycleIndex(cur, len(levels), dir)]
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.MCP = next }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.auditAdd(audit.EvSettings, "", "MCP-Zugriff: "+next)
	if next == mcpOff {
		a.stopMCP()
	} else {
		a.startMCP()
	}
	a.flash(i18n.T("MCP-Zugriff: ") + mcpLabel(next))
	return nil
}

// onMCP läuft im Update-Goroutine.
func (a *App) onMCP(m mcpCallMsg) tea.Cmd {
	fail := func(f string, x ...any) tea.Cmd { m.reply("", fmt.Errorf(f, x...)); return nil }
	switch {
	case a.mcpLevel() == mcpOff:
		return fail("MCP access is off in sshTermy")
	case a.scr == scrUnlock || a.lock != nil:
		return fail("sshTermy is locked; unlock it first")
	case a.st.LLM().Privacy == llm.PrivacyOff:
		return fail("AI is turned off in sshTermy (Settings › AI › AI privacy)")
	}
	a.auditAdd("mcp", "", m.tool)
	switch m.tool {
	case "list_connections":
		var p struct{ Query string }
		_ = json.Unmarshal(m.args, &p)
		m.reply(a.mcpConnections(p.Query), nil)
	case "list_tabs":
		m.reply(a.mcpTabs(), nil)
	case "read_tab":
		var p struct{ Tab, Lines int }
		_ = json.Unmarshal(m.args, &p)
		s := a.byID(p.Tab)
		if s == nil || s.term == nil {
			return fail("no open tab %d (see list_tabs)", p.Tab)
		}
		if !mcpAllowed(a, s.conn) {
			return fail("AI access is off for this connection (privacy tag)")
		}
		n := p.Lines
		if n <= 0 {
			n = 100
		}
		m.reply(agent.Redact(s.term.PlainLines(min(n, 1000))), nil)
	case "run_command":
		return a.mcpRun(m)
	default:
		return fail("unknown tool %s", m.tool)
	}
	return nil
}

func mcpAllowed(a *App, c domain.Connection) bool { return a.aiLevel(c) == llm.PrivacyAll }

type mcpConn struct {
	ID       string   `json:"id"`
	Alias    string   `json:"alias"`
	Host     string   `json:"host,omitempty"`
	Port     int      `json:"port,omitempty"`
	User     string   `json:"user,omitempty"`
	Protocol string   `json:"protocol,omitempty"`
	Folder   string   `json:"folder,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Device   string   `json:"device,omitempty"`
	OS       string   `json:"os,omitempty"`
}

func (a *App) mcpConnections(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []mcpConn{}
	for _, c := range a.st.Connections() {
		if !mcpAllowed(a, c) {
			continue
		}
		mc := mcpConn{ID: c.ID, Alias: c.Alias, Host: c.Host, Port: c.Port, User: c.Username, Protocol: string(c.Protocol),
			Folder: c.Group, Tags: c.Tags, Device: string(c.Device)}
		if c.Profile != nil {
			mc.OS = c.Profile.OS
		}
		if q != "" && !strings.Contains(strings.ToLower(mc.Alias+" "+mc.Host+" "+mc.Folder+" "+strings.Join(mc.Tags, " ")), q) {
			continue
		}
		out = append(out, mc)
	}
	b, _ := json.MarshalIndent(out, "", " ")
	return string(b)
}

func (a *App) mcpTabs() string {
	type tab struct {
		Tab   int    `json:"tab"`
		Alias string `json:"alias"`
		Host  string `json:"host,omitempty"`
		State string `json:"state"`
	}
	out := []tab{}
	for _, s := range a.sessions {
		if !mcpAllowed(a, s.conn) {
			continue
		}
		st := "connected"
		switch s.state {
		case stReconnect:
			st = "reconnecting"
		case stConnected:
		default:
			st = "not connected"
		}
		out = append(out, tab{s.id, s.conn.Alias, s.conn.Host, st})
	}
	b, _ := json.MarshalIndent(out, "", " ")
	return string(b)
}

func (a *App) mcpFind(ref string) (domain.Connection, bool) {
	if c, ok := a.st.Connection(ref); ok {
		return c, true
	}
	for _, c := range a.st.Connections() {
		if strings.EqualFold(c.Alias, ref) {
			return c, true
		}
	}
	return domain.Connection{}, false
}

func mcpKind(c domain.Connection) string {
	net := c.Device == domain.DeviceNetwork
	os := ""
	if c.Profile != nil {
		net = net || c.Profile.Family == domain.FamilyNetwork
		os = c.Profile.OS
	}
	return safety.KindFor(os, net)
}

func (a *App) mcpRun(m mcpCallMsg) tea.Cmd {
	var p struct {
		Connection, Command string
		Timeout             int `json:"timeout_seconds"`
	}
	_ = json.Unmarshal(m.args, &p)
	cmd := strings.TrimSpace(p.Command)
	c, ok := a.mcpFind(strings.TrimSpace(p.Connection))
	switch {
	case cmd == "":
		m.reply("", errors.New("command is empty"))
		return nil
	case !ok:
		m.reply("", fmt.Errorf("no connection %q (see list_connections)", p.Connection))
		return nil
	case c.Protocol != "" && c.Protocol != domain.ProtoSSH:
		m.reply("", errors.New("only SSH connections can run commands via MCP"))
		return nil
	case !mcpAllowed(a, c):
		m.reply("", errors.New("AI access is off for this connection (privacy tag)"))
		return nil
	}
	kind := mcpKind(c)
	dangers := safety.Check(cmd, kind)
	secret := safety.Sensitive(cmd)
	// Nur Lesendes ohne sudo und ohne Geheimnisse läuft ohne Rückfrage: die
	// Ausgabe geht an einen fremden KI-Client.
	if safety.ReadOnly(cmd, kind) && len(dangers) == 0 && secret == "" && !safety.Elevated(cmd) {
		return a.mcpExec(c, cmd, p.Timeout, m.reply)
	}
	if a.mcpLevel() != mcpAsk && safety.ReadOnly(cmd, kind) && len(dangers) == 0 {
		m.reply("", errors.New("this command reads secrets or needs sudo; sshTermy only runs it after the user's confirmation (setting “MCP access: with confirmation”)"))
		return nil
	}
	if a.mcpLevel() != mcpAsk {
		m.reply("", errors.New("only read-only commands are allowed (sshTermy setting “MCP access: read only”); ask the user to run it or to allow confirmations"))
		return nil
	}
	body := fmt.Sprintf(i18n.T("Ein KI-Client möchte auf %s ausführen:\n\n  %s\n"), auditConn(c), cmd)
	for _, d := range dangers {
		body += "\n▲ " + d.Reason
	}
	if secret != "" {
		body += "\n◆ " + secret + i18n.T(": die Ausgabe geht an den KI-Client")
	}
	if len(dangers) > 0 {
		body += i18n.T("\n\nj zweimal = ausführen · n = ablehnen")
	} else {
		body += i18n.T("\n\nj = ausführen · n = ablehnen")
	}
	md := confirmModal(i18n.T("MCP: Befehl erlauben?"), body, func() tea.Cmd { return a.mcpExec(c, cmd, p.Timeout, m.reply) })
	md.onCancel = func() tea.Cmd {
		a.auditAdd("mcp", auditConn(c), "abgelehnt: "+cmd)
		m.reply("", errors.New("the user declined the command"))
		return nil
	}
	if a.modal != nil {
		m.reply("", errors.New("sshTermy is busy with another dialog; try again in a moment"))
		return nil
	}
	md.twice = len(dangers) > 0 // Gefahr: j zweimal
	a.showAsync(md)             // kam von selbst: erst nach einer Sekunde bedienbar
	return nil
}

func (a *App) mcpExec(c domain.Connection, cmd string, timeout int, reply func(string, error)) tea.Cmd {
	if timeout <= 0 {
		timeout = 60
	}
	timeout = min(timeout, 600)
	a.auditAdd(audit.EvAgent, auditConn(c), "MCP: "+cmd)
	a.flash(fmt.Sprintf(i18n.T("MCP: %s auf %s"), cmd, c.Alias))
	dial := a.bcDialer()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		rs := broadcast.Run(ctx, broadcast.Job{Conns: []domain.Connection{c}, Steps: []string{cmd}}, dial, func(int, broadcast.Result) {})
		r := rs[0]
		out := agent.Redact(r.Output)
		if len(out) > mcpMaxOut {
			out = out[len(out)-mcpMaxOut:]
		}
		switch {
		case r.Err != "":
			reply(out, errors.New(r.Err+"\n"+out))
		default:
			reply(fmt.Sprintf("%s\n[exit %d]", strings.TrimRight(out, "\n"), r.Exit), nil)
		}
		return nil
	}
}
