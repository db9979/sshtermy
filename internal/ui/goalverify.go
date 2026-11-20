package ui

// Gegenprüfung nach Ziel- und Agent-Modus: Meldet die KI „Ziel erreicht“,
// fragt sshTermy eine zweite, unabhängige KI-Anfrage (ein Ersatz-Anbieter,
// wenn eingerichtet, sonst dasselbe Modell mit frischem Blick). Sie darf bis
// zu drei rein lesende Prüfbefehle vorschlagen, die sshTermy unsichtbar in
// einem eigenen SSH-Kanal ausführt (nicht im Terminal, nichts wird
// geändert), und urteilt dann: bestätigt oder Zweifel, mit Befund. Steht im
// Panel unter „Ziel erreicht“; Zweifel kommen zusätzlich als Meldung.
// Standard: an. Abschaltbar (Palette „Gegenprüfung nach Ziel und Agent“),
// g im fertigen Panel prüft (noch einmal) von Hand.

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
)

type verifyState struct {
	busy    bool
	model   string
	v       agent.Verdict
	checks  []string
	err     string
	seq     int
	started time.Time
}

type verifyMsg struct {
	sid, seq int
	checks   []string
	v        agent.Verdict
	err      error
}

func (a *App) verifyOn() bool { return a.st == nil || !a.st.Settings().NoVerify }

func (a *App) toggleVerify() tea.Cmd {
	on := !a.verifyOn()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoVerify = !on }); err != nil {
		a.flash(i18n.T("Fehler: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Gegenprüfung an: eine zweite KI prüft jedes erreichte Ziel nur lesend nach"))
	} else {
		a.flash(i18n.T("Gegenprüfung aus (g im fertigen Ziel prüft trotzdem)"))
	}
	return nil
}

// verifyAgent: Prüfer-KI. Ein eingerichteter Ersatz-Anbieter ist ein
// wirklich anderes Modell; sonst dasselbe Modell ohne den Verlauf des Ziels.
func (a *App) verifyAgent(s *session) (*agent.Agent, string, error) {
	base, err := a.agentClientFor(s.conn)
	if err != nil {
		return nil, "", err
	}
	cfg := a.st.LLM()
	if chain := cfg.Chain(); len(chain) > 1 {
		alt := chain[1]
		alt.Fallbacks, alt.FastModel = nil, ""
		if c, err := llm.NewWithPrivacy(alt, a.aiLevel(s.conn)); err == nil {
			return &agent.Agent{C: c, Rules: base.Rules}, alt.Model, nil
		}
	}
	return base, cfg.Model, nil
}

// startVerify: Gegenprüfung des gerade erreichten Ziels.
func (a *App) startVerify(s *session) tea.Cmd {
	g := &s.goal
	if !g.done || g.goal == "" || len(g.steps) == 0 {
		return nil
	}
	ag, model, err := a.verifyAgent(s)
	if err != nil {
		g.verify = &verifyState{err: err.Error()}
		return nil
	}
	seq := 1
	if g.verify != nil {
		seq = g.verify.seq + 1
	}
	g.verify = &verifyState{busy: true, model: model, seq: seq, started: time.Now()}
	sid := s.id
	goal, reason := g.goal, g.doneReason
	steps := append([]agent.DoneStep(nil), g.steps...)
	c := a.agentContext(s, false)
	cl := s.client
	canRun := cl != nil && s.state == stConnected && !s.netDevice() && s.conn.Protocol != domain.ProtoWinRM
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		var checks []string
		var res []agent.CheckResult
		if canRun {
			cmds, err := ag.VerifyChecks(ctx, goal, reason, steps, c)
			if err != nil && !errors.Is(err, agent.ErrUnreadable) {
				return verifyMsg{sid: sid, seq: seq, err: err}
			}
			checks = cmds
			for _, cmd := range cmds {
				cctx, ccancel := context.WithTimeout(ctx, 20*time.Second)
				r, err := cl.Exec(cctx, cmd)
				ccancel()
				out := string(r.Output)
				if err != nil && out == "" {
					out = err.Error()
				}
				res = append(res, agent.CheckResult{Command: cmd, Output: out, Exit: r.Exit})
			}
		}
		v, err := ag.VerifyJudge(ctx, goal, reason, steps, res, c)
		return verifyMsg{sid: sid, seq: seq, checks: checks, v: v, err: err}
	}
}

func (a *App) verifyDone(m verifyMsg) {
	s := a.byID(m.sid)
	if s == nil || s.goal.verify == nil || s.goal.verify.seq != m.seq {
		return
	}
	vs := s.goal.verify
	vs.busy, vs.checks = false, m.checks
	if m.err != nil {
		vs.err = m.err.Error()
		return
	}
	vs.v = m.v
	a.taskVerified(s, m.v.OK, m.v.Finding)
	if !m.v.OK {
		a.pushToast(toastWarn, i18n.T("Gegenprüfung hat Zweifel: ")+trunc(m.v.Finding, 80))
		if a.desktopNotifyOn() && a.blurred {
			a.sendDesktopFor(s, i18n.T("Gegenprüfung hat Zweifel"), s.conn.Alias+": "+trunc(m.v.Finding, 120))
		}
	}
}

// viewVerify: Zeile(n) unter „Ziel erreicht“.
func (a *App) viewVerify(b *strings.Builder, s *session, w int) {
	st := a.s
	vs := s.goal.verify
	if vs == nil {
		if !a.verifyOn() && s.goal.done {
			b.WriteString("\n" + st.Muted.Render(i18n.T("Gegenprüfung aus · g prüft jetzt")) + "\n")
		}
		return
	}
	b.WriteString("\n")
	who := ""
	if vs.model != "" {
		who = st.Muted.Render(" · " + trunc(vs.model, 24))
	}
	switch {
	case vs.busy:
		b.WriteString(st.AI.Render(i18n.T("◌ Gegenprüfung läuft (nur lesend) …")) + who + "\n")
	case vs.err != "":
		b.WriteString(st.Muted.Render(i18n.T("Gegenprüfung nicht möglich: ")+trunc(vs.err, max(20, w-30))) + "\n")
	case vs.v.OK:
		b.WriteString(st.OK.Render(i18n.T("✓ Gegenprüfung bestätigt")) + who + "\n")
	default:
		b.WriteString(st.Warn.Render(i18n.T("▲ Gegenprüfung hat Zweifel")) + who + "\n")
	}
	if !vs.busy && vs.err == "" && vs.v.Finding != "" {
		for _, l := range wrapText(vs.v.Finding, max(20, w-2)) {
			b.WriteString("  " + st.Text.Render(l) + "\n")
		}
	}
	if !vs.busy && len(vs.checks) > 0 {
		b.WriteString(st.Muted.Render("  "+i18n.T("geprüft: ")+trunc(strings.Join(vs.checks, " · "), max(20, w-12))) + "\n")
	}
}
