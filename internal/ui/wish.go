package ui

// Satz am Prompt wird zum Befehl (Idee aus lxask, dort Strg+G): Du tippst
// in eigenen Worten, was du willst („größte Ordner hier zuerst“), drückst
// Strg+G, und die Zeile wird durch einen Befehl ersetzt. Ausgeführt wird
// nichts, ⏎ drückst du. Der Befehl ist vorher geprüft (cmdcheck.go):
// Reparaturen sind schon drin, offene Einwände stehen in der Meldung.
// Geht mit der vollen KI und mit der kleinen Offline-KI.
//
// Befehl erklären (Alt+H): die getippte Zeile (leer: der letzte Befehl)
// Teil für Teil aus der man-Seite des Hosts, ganz ohne KI.

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/cmdcheck"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

type wishState struct {
	busy bool
	text string
	seq  int
}

type wishMsg struct {
	sid, seq  int
	text, cmd string
	res       cmdcheck.Result
	checked   bool
	docs      cmdcheck.Docs
	err       error
}

// startWish: Strg+G im Terminal.
func (a *App) startWish(s *session) tea.Cmd {
	if s.term == nil || s.state != stConnected || s.term.AltScreen() {
		return nil
	}
	typed, ok := s.term.TypedInput()
	text := strings.TrimSpace(typed)
	if !ok || text == "" {
		a.flash(fmt.Sprintf(i18n.T("Tippe am Prompt in eigenen Worten, was du willst, dann %s"), a.keys.label(actWish)))
		return nil
	}
	if s.wish != nil && s.wish.busy {
		return nil
	}
	ag, err := a.cmdAgent(s)
	if err != nil {
		a.flash(err.Error())
		return nil
	}
	seq := 1
	if s.wish != nil {
		seq = s.wish.seq + 1
	}
	s.wish = &wishState{busy: true, text: text, seq: seq}
	if ag.Small {
		a.pushToast(toastInfo, i18n.T("◌ Offline-KI schreibt den Befehl …"))
	} else {
		a.pushToast(toastInfo, i18n.T("◌ Satz wird zum Befehl …"))
	}
	sid := s.id
	c := a.agentContext(s, true)
	ensure := a.offlineEnsure(ag)
	check := a.cmdCheckOn() && a.checkable(s)
	sys := a.checkSys(s)
	var run func(context.Context, string) (string, error)
	if check && a.canProbe(s) {
		run = a.probeRunner(s)
	}
	known := cmdcheck.Docs{}
	known.Merge(s.docs)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		m := wishMsg{sid: sid, seq: seq, text: text}
		if err := ensure(ctx); err != nil {
			m.err = err
			return m
		}
		cmd, err := ag.WishCommand(ctx, text, c)
		if err != nil {
			m.err = err
			return m
		}
		m.cmd = cmd
		if check {
			m.res, m.docs = checkBackground(ctx, ag, text, cmd, sys, known, run, c)
			m.cmd, m.checked = m.res.Command, true
		}
		return m
	}
}

// checkBackground prüft einen Befehl im Hintergrund (Doku vom Host, wenn
// run gesetzt) und gibt der KI bei Einwänden einen zweiten Versuch.
func checkBackground(ctx context.Context, ag *agent.Agent, question, cmd string, sys cmdcheck.Sys, known cmdcheck.Docs,
	run func(context.Context, string) (string, error), c agent.Context) (cmdcheck.Result, cmdcheck.Docs) {
	got := cmdcheck.Docs{}
	docsFor := func(cmd string) cmdcheck.Docs {
		all := cmdcheck.Docs{}
		all.Merge(known)
		all.Merge(got)
		if run != nil {
			if miss := all.Missing(cmdcheck.Programs(cmd)); len(miss) > 0 {
				if d, err := probeDocs(run, miss); err == nil {
					got.Merge(d)
					all.Merge(d)
				}
			}
		}
		return all
	}
	st := cmdcheck.Static(question, cmd, sys)
	res := cmdcheck.Full(question, st.Command, sys, docsFor(st.Command))
	res.Repairs = append(st.Repairs, res.Repairs...)
	if res.OK() || ag == nil {
		return res, got
	}
	next, err := ag.RecheckCommand(ctx, question, res.Command, res.Texts(), c)
	if err != nil || next == "" {
		return res, got
	}
	st2 := cmdcheck.Static(question, next, sys)
	res2 := cmdcheck.Full(question, st2.Command, sys, docsFor(st2.Command))
	res2.Repairs = append(st2.Repairs, res2.Repairs...)
	if len(res2.Issues) < len(res.Issues) {
		return res2, got
	}
	return res, got
}

func (a *App) wishDone(m wishMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil || s.wish == nil || s.wish.seq != m.seq {
		return nil
	}
	s.wish.busy = false
	a.clearToasts()
	if s.docs == nil {
		s.docs = cmdcheck.Docs{}
	}
	s.docs.Merge(m.docs)
	if m.err != nil {
		a.flash(i18n.T("Satz → Befehl: ") + trunc(m.err.Error(), 120))
		return nil
	}
	if s.term == nil || s.state != stConnected {
		return nil
	}
	typed, _ := s.term.TypedInput()
	if strings.TrimSpace(typed) == m.text {
		s.term.SendText(a.clearLineSeq(s)) // Satz weg, Befehl hin
	} else if strings.TrimSpace(typed) != "" {
		// Inzwischen weitergetippt: nicht überschreiben, als Vorschlag anbieten
		s.chips, s.chipArmed, s.chipReplace, s.compChips = []agent.Suggestion{{Command: m.cmd}}, -1, true, true
		a.flash(fmt.Sprintf(i18n.T("Befehl bereit: %s fügt ihn ein"), a.keys.label(actChip1)))
		return nil
	}
	s.term.SendText(m.cmd)
	s.noteAI(m.cmd, true)
	switch {
	case m.checked && !m.res.OK():
		a.pushToast(toastWarn, i18n.T("unbestätigt: ")+trunc(strings.Join(m.res.Texts(), " · "), 140))
	case len(m.res.Repairs) > 0:
		a.flashOK(i18n.T("angepasst: ") + trunc(strings.Join(m.res.Repairs, " · "), 120))
	}
	if f := safety.Check(m.cmd, a.safetyKind(s)); len(f) > 0 {
		a.pushToast(toastWarn, i18n.T("Vorsicht: ")+f[0].Reason)
	}
	return nil
}

// clearLineSeq: Tasten, die die getippte Zeile leeren.
func (a *App) clearLineSeq(s *session) string {
	if a.safetyKind(s) == "windows" {
		return "\x1b" // PowerShell (PSReadLine): Esc leert die Zeile
	}
	return "\x05\x15" // Strg+E, Strg+U: ans Ende, alles davor löschen
}

// ---------------------------------------------------------------- Erklären

// startExplain: Alt+H im Terminal: getippte Zeile oder letzter Befehl.
func (a *App) startExplain(s *session) tea.Cmd {
	line := ""
	if s.term != nil {
		if t, ok := s.term.TypedInput(); ok {
			line = strings.TrimSpace(t)
		}
	}
	if line == "" && len(s.history) > 0 {
		line = s.history[len(s.history)-1]
	}
	if line == "" {
		a.flash(fmt.Sprintf(i18n.T("Tippe einen Befehl, dann %s erklärt ihn"), a.keys.label(actExplain)))
		return nil
	}
	return a.explainCommand(s, line, "")
}

// explainCommand zeigt die Erklärung (Doku vorher vom Host holen).
func (a *App) explainCommand(s *session, line, question string) tea.Cmd {
	if !a.checkable(s) {
		a.flash(i18n.T("Erklären aus der man-Seite geht auf Linux-, BSD- und macOS-Shells"))
		return nil
	}
	return a.fetchDocs(s, cmdcheck.Programs(line), func(s *session) tea.Cmd {
		a.modal = a.explainModal(s, line, question)
		return nil
	})
}

func (a *App) explainModal(s *session, line, question string) *modal {
	st := a.s
	var lines, plain []string
	add := func(styled, raw string) {
		lines = append(lines, styled)
		plain = append(plain, raw)
	}
	add(highlightShell(line, st), line)
	add("", "")
	parts := cmdcheck.Explain(line, s.docs)
	width := 0
	for _, p := range parts {
		width = max(width, len([]rune(p.Name)))
	}
	width = min(width, 18)
	for _, p := range parts {
		txt := p.Text
		if txt == "" {
			txt = i18n.T("(die Doku auf dem Host sagt dazu nichts)")
		}
		name := p.Name + strings.Repeat(" ", max(0, width-len([]rune(p.Name))))
		if p.Program {
			add(st.OK.Bold(true).Render(name)+"  "+st.Text.Render(txt), name+"  "+txt)
		} else {
			add("  "+st.Accent.Render(name)+"  "+st.Muted.Render(txt), "  "+name+"  "+txt)
		}
	}
	if len(parts) == 0 {
		add(st.Muted.Render(i18n.T("Zeile ließ sich nicht zerlegen (Anführungszeichen offen?)")), "")
	}
	add("", "")
	for _, f := range safety.Check(line, a.safetyKind(s)) {
		add(st.Danger.Render("▲ "+f.Reason), "! "+f.Reason)
	}
	res := cmdcheck.Full(question, line, a.checkSys(s), s.docs)
	for _, is := range res.Issues {
		add(st.Warn.Render("? "+i18n.T("unbestätigt: ")+is.Text), "? "+is.Text)
	}
	if len(res.Repairs) > 0 && res.Command != line {
		add(st.AI.Render("» "+i18n.T("Vorschlag: ")+res.Command), "» "+res.Command)
		add(st.Muted.Render("  "+strings.Join(res.Repairs, " · ")), "  "+strings.Join(res.Repairs, " · "))
	}
	src := i18n.T("aus der man-Seite bzw. --help des Hosts, ohne KI; was die Doku nicht sagt, steht nicht da")
	if !a.canProbe(s) {
		src = i18n.T("ohne Doku vom Host (über diese Verbindung nicht lesbar)")
	}
	add(st.Muted.Render(src), src)
	m := textModal(i18n.T("Befehl erklärt"), lines)
	m.styled, m.copy = true, strings.Join(plain, "\n")
	return m
}
