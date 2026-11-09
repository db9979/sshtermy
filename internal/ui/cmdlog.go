package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

// Befehle verfolgen: Nach Enter merkt sich die Sitzung den Befehl. Steht
// wieder ein leerer Prompt da und ist die Ausgabe zur Ruhe gekommen, gilt er
// als fertig: seine Ausgabe wird aus dem Terminal gelesen, ins
// Sitzungsprotokoll geschrieben und auf Fehler geprüft. Meldet die Shell den
// Exit-Code (OSC 133;D, z. B. mit VS-Code-/FinalTerm-Shell-Integration), zählt
// der; sonst schätzt agent.LooksFailed anhand typischer Fehlermeldungen.
//
// Bei einem Fehlschlag bietet die Vorschlagszeile „Warum? & Fix“ an (Alt+0):
// die KI bekommt Befehl, Exit-Code und Ausgabe und antwortet im Frage-Panel
// mit Ursache und einfügbaren Befehlen.

type cmdRun struct {
	cmd         string
	at          time.Time
	exitSeq     int    // Zähler der OSC-133-Meldungen beim Start
	private     bool   // Leerzeichen am Anfang: nicht protokollieren
	interrupted bool   // Strg+C während der Befehl lief
	learnFrom   string // korrigiert diesen KI-Befehl (learn.go)
	agent       bool   // vom Agent-Modus ausgeführt
}

// beginCommand: Enter mit einem Befehl am Prompt.
func (a *App) beginCommand(s *session, typed string) {
	a.finishCommand(s) // vorherigen abschließen, falls noch offen
	cmd := strings.TrimSpace(typed)
	if cmd == "" || s.term == nil {
		return
	}
	seq, _ := s.term.LastExit()
	s.run = &cmdRun{cmd: cmd, at: time.Now(), exitSeq: seq, private: strings.HasPrefix(typed, " ")}
	s.fail = nil
}

// commandTick (jede Sekunde): ist der laufende Befehl fertig?
func (a *App) commandTick(s *session) {
	r := s.run
	if r == nil || s.term == nil || s.state != stConnected {
		return
	}
	if time.Since(r.at) < 400*time.Millisecond || time.Since(s.lastOutput) < 600*time.Millisecond || !s.term.AtIdlePrompt() {
		return
	}
	a.finishCommand(s)
}

func (a *App) finishCommand(s *session) {
	r := s.run
	if r == nil {
		return
	}
	s.run = nil
	if s.term == nil {
		return
	}
	out, line, found := s.term.CommandOutput(r.cmd, 600)
	if found && line != r.cmd && len(line) <= len(r.cmd)+64 {
		r.cmd = line // letzte Zeichen waren beim Enter noch nicht zu sehen
	}
	f := agent.Failure{Command: r.cmd, Output: out}
	failed := false
	if seq, code := s.term.LastExit(); seq != r.exitSeq {
		c := code
		f.Exit = &c
		// 130 = Strg+C, 148 = Strg+Z: vom Menschen gewollt, kein Fehler
		failed = code != 0 && code != 130 && code != 148
		if failed {
			f.Reason = fmt.Sprintf(i18n.T("Exit-Code %d"), code)
			if _, why := agent.LooksFailed(out); why != "" {
				f.Reason += ", " + why
			}
		}
	} else if !r.interrupted {
		failed, f.Reason = agent.LooksFailed(out)
	}
	// Ziel-/Agent-Modus: Ausgabe und Exit-Code gehen an das Modell
	if g := &s.goal; g.active && len(g.steps) > 0 && strings.HasPrefix(r.cmd, g.steps[len(g.steps)-1].Command) {
		g.steps[len(g.steps)-1].Output, g.steps[len(g.steps)-1].Exit = sessionlog.Clip(out), f.Exit
	}
	if failed && !r.interrupted {
		s.fail = &f
		// Ziel-Modus: der Agent erfährt, dass sein Schritt nicht geklappt hat
		if g := &s.goal; len(g.steps) > 0 && strings.HasPrefix(r.cmd, g.steps[len(g.steps)-1].Command) {
			g.steps[len(g.steps)-1].ExitHint = orDefault(f.Reason, i18n.T("Fehlermeldung in der Ausgabe"))
		}
	}
	if r.learnFrom != "" && !failed && !r.interrupted {
		a.learnCorrection(s, r.learnFrom, r.cmd)
	}
	if !failed && !r.interrupted && !r.private {
		a.noteFavorite(s, r.cmd)
	}
	end := s.lastOutput // Prompt kam zuletzt; Befehle ohne Ausgabe: jetzt
	if !end.After(r.at) {
		end = time.Now()
	}
	a.noticeDone(s, r.cmd, end.Sub(r.at), failed, r.interrupted)
	dur := 0.0
	if s.lastOutput.After(r.at) {
		dur = s.lastOutput.Sub(r.at).Seconds()
	}
	a.auditCommand(s, r, failed, f.Exit)
	a.companionAfter(s, r, f, failed)
	if s.log != nil && !r.private {
		s.log.Add(sessionlog.Entry{At: r.at, Command: r.cmd, Output: out, Duration: dur,
			Failed: failed, Exit: f.Exit, Reason: f.Reason, Agent: r.agent})
		a.saveLog(s)
	}
}

// ---------------------------------------------------------------- Protokoll

func (a *App) logsEnabled() bool { return !a.st.Settings().NoSessionLog }

// logBook öffnet den Protokollordner (Schlüssel aus dem entsperrten Store).
func (a *App) logBook() (*sessionlog.Book, error) {
	if a.book != nil {
		return a.book, nil
	}
	key, err := a.st.SessionLogKey()
	if err != nil {
		return nil, err
	}
	defer wipeKey(key)
	b, err := sessionlog.Open(a.st.LogDir(), key)
	if err != nil {
		return nil, err
	}
	a.book = b
	return b, nil
}

// closeBook beim Sperren: Schlüssel aus dem Speicher.
func (a *App) closeBook() {
	if a.book != nil {
		a.book.Close()
		a.book = nil
	}
}

// startLog beim Verbinden.
func (a *App) startLog(s *session) {
	if !a.logsEnabled() || s.log != nil {
		return
	}
	l := sessionlog.New(s.conn.ID, s.conn.Alias, s.conn.Target())
	if s.client != nil {
		l.System = s.profileSummary()
	}
	s.log = l
}

// saveLog schreibt das Protokoll (erst ab dem ersten Befehl, leere Sitzungen
// hinterlassen keine Datei).
func (a *App) saveLog(s *session) {
	if s.log == nil || len(s.log.Entries) == 0 {
		return
	}
	b, err := a.logBook()
	if err == nil {
		err = b.Save(s.log)
	}
	if err != nil && !s.logErr {
		s.logErr = true
		a.flashErr(i18n.T("Sitzungsprotokoll: ") + err.Error())
	}
}

// endLog beim Schließen einer Sitzung.
func (a *App) endLog(s *session) {
	a.finishCommand(s)
	if s.log == nil {
		return
	}
	s.log.Ended = time.Now()
	a.saveLog(s)
	s.log = nil
}

// ---------------------------------------------------------------- Fehler erklären

// explainFailure öffnet das Frage-Panel und fragt die KI nach Ursache und Fix.
func (a *App) explainFailure(s *session) tea.Cmd {
	f := s.fail
	if f == nil {
		a.flash(i18n.T("Kein fehlgeschlagener Befehl"))
		return nil
	}
	a.panelOpened(withPanel(s, panelAsk))
	a.resize(s)
	q := &s.ask
	ag, err := a.agentClientFor(s.conn)
	if err != nil {
		q.err = err.Error()
		return nil
	}
	q.cancel()
	q.question = i18n.T("Warum ist „") + trunc(f.Command, 80) + i18n.T("“ fehlgeschlagen? Und wie behebe ich es?")
	q.input.SetValue("")
	q.answer.Reset()
	q.err, q.busy = "", true
	q.seq++
	q.input.Blur()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	q.cancelFn = cancel
	sid, seq, ev := s.id, q.seq, a.events
	c := a.agentContext(s, false) // Ausgabe des Befehls statt ganzem Bildschirm
	ff := *f
	if !a.st.LLM().SendScreen {
		ff.Output = "" // Nutzer will keinen Terminal-Inhalt an die KI schicken
	}
	return func() tea.Msg {
		defer cancel()
		ans, err := ag.Explain(ctx, ff, c)
		if err == nil {
			ev <- askDeltaMsg{sid, seq, ans}
		}
		return askDoneMsg{sid, seq, err}
	}
}

func wipeKey(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
