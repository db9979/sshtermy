package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// KI-Logdiagnose: Die KI wertet eine Markierung oder die letzten
// diagnoseLines Zeilen des Verlaufs aus (journalctl, show logging, dmesg …)
// und antwortet im Frage-Panel mit Lage, Auffälligem, Ursache und lesenden
// Prüfbefehlen als Karten.

const diagnoseLines = 400

// diagnoseText: markierter Text, sonst das Ende des Verlaufs.
func diagnoseText(s *session) (text, what string) {
	if s.sel != nil && !s.sel.Empty() {
		if t := strings.TrimSpace(s.term.Text(*s.sel)); t != "" {
			return t, plural(countLines(t), i18n.T("markierte Zeile"), i18n.T("markierte Zeilen"))
		}
	}
	t := s.term.PlainLines(diagnoseLines)
	return t, plural(countLines(t), i18n.T("Zeile"), i18n.T("Zeilen"))
}

func (a *App) startDiagnose(s *session) tea.Cmd {
	if s == nil || s.term == nil {
		return nil
	}
	if !a.st.LLM().SendScreen {
		a.flash(i18n.T("Terminal-Inhalt an die KI ist in den KI-Einstellungen ausgeschaltet"))
		return nil
	}
	text, what := diagnoseText(s)
	if strings.TrimSpace(text) == "" {
		a.flash(i18n.T("Nichts zum Auswerten im Verlauf"))
		return nil
	}
	return a.diagnoseWith(s, text, what)
}

// diagnoseWith: text im Frage-Panel auswerten lassen (what steht in der Frage).
func (a *App) diagnoseWith(s *session, text, what string) tea.Cmd {
	a.panelOpened(withPanel(s, panelAsk))
	a.resize(s)
	q := &s.ask
	ag, err := a.agentClientFor(s.conn)
	if err != nil {
		q.err = err.Error()
		return nil
	}
	q.cancel()
	q.question = i18n.T("Log-Diagnose: ") + what
	q.input.SetValue("")
	q.answer.Reset()
	q.err, q.busy = "", true
	q.sel, q.armed = 0, -1
	q.seq++
	q.input.Blur()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	q.cancelFn = cancel
	sid, seq, ev := s.id, q.seq, a.events
	c := a.agentContext(s, false)
	s.sel = nil
	return func() tea.Msg {
		defer cancel()
		err := ag.Diagnose(ctx, text, c, func(d string) {
			if d != "" {
				ev <- askDeltaMsg{sid, seq, d}
			}
		})
		return askDoneMsg{sid, seq, err}
	}
}
