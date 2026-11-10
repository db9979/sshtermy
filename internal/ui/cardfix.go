package ui

// Antwortkarten passen sich nach Fehlern an. Schlägt ein Befehl aus einer
// Karte („KI fragen“) fehl, merkt die Karte das: Sie zeigt die Fehlermeldung
// und darunter einen korrigierten Befehl. Der kommt vom Begleiter, wenn der
// ohnehin zu dem Fehler etwas sagt, sonst aus einer kurzen eigenen Anfrage
// (agent.FixCard). Mit u wird der Fix zum Befehl der Karte; steckt dieselbe
// falsche Stelle (z. B. ein deutsches Datum) auch in anderen Karten, werden
// die gleich mitkorrigiert. Läuft der korrigierte Befehl danach ohne Fehler,
// merkt sich das Systemprofil die Korrektur (learn.go), damit die KI es auf
// diesem Gerät künftig gleich richtig vorschlägt.

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

// cardFix: Zustand einer Karte nach einem Fehlschlag.
type cardFix struct {
	failed  string        // der fehlgeschlagene Befehl
	errLine string        // kurze Fehlermeldung (letzte Zeile der Ausgabe)
	f       agent.Failure // für die eigene Anfrage
	fix     agent.Suggestion
	why     string
	want    bool      // eigene Anfrage beim nächsten Takt
	busy    bool      // eigene Anfrage läuft
	waitAt  time.Time // wartet auf den Begleiter (seit)
	none    bool      // kein Fix per Befehl möglich
	adopted bool      // Fix ist jetzt der Befehl der Karte
	from    string    // ursprünglicher Befehl (fürs Lernen)
	like    int       // >= 0: mitkorrigiert wie Karte like (Anzeige 1-basiert +1)
}

// Wie lange eine Karte auf den Begleiter wartet, bevor sie selbst fragt.
const cardFixWait = 50 * time.Second

type cardFixMsg struct {
	sid, seq, idx int
	r             agent.CardFix
	err           error
}

// cardText: der aktuelle Befehl einer Karte (nach u der Fix).
func (q *askState) cardText(i int, cd card) card {
	if c, ok := q.codes[i]; ok {
		cd.code = c
	}
	return cd
}

// failedCard: zu welcher Karte gehört der fehlgeschlagene Befehl?
func (q *askState) failedCard(cmd string) int {
	if q.busy || q.question == "" {
		return -1
	}
	cmd = oneLine(cmd)
	for i, cd := range answerCards(agent.VisibleAnswer(q.answer.String())) {
		cd = q.cardText(i, cd)
		if !cd.runnable() {
			continue
		}
		if c := oneLine(cd.code); c == cmd || (len(c) >= 8 && strings.HasPrefix(cmd, c)) {
			return i
		}
	}
	return -1
}

// errLineOf: die aussagekräftigste Zeile einer Fehlerausgabe.
func errLineOf(f agent.Failure) string {
	lines := strings.Split(strings.TrimSpace(f.Output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return oneLine(l)
		}
	}
	return f.Reason
}

// cardFailed (aus finishCommand): ein Befehl ist fehlgeschlagen. Stammt er
// aus einer Karte, wird sie markiert und ein Fix besorgt.
func (a *App) cardFailed(s *session, f agent.Failure) {
	q := &s.ask
	i := q.failedCard(f.Command)
	if i < 0 {
		return
	}
	if q.fixes == nil {
		q.fixes = map[int]*cardFix{}
	}
	old := q.fixes[i]
	cf := &cardFix{failed: f.Command, errLine: errLineOf(f), f: f, like: -1, from: f.Command}
	if old != nil && old.from != "" {
		cf.from = old.from // auch der Fix ging schief: gelernt wird vom Original
	}
	if !a.st.LLM().SendScreen {
		cf.f.Output = ""
	}
	q.fixes[i] = cf
	if fix, ok := s.learnedFix(f.Command); ok { // schon einmal korrigiert
		cf.fix = agent.ParseStepCommand(fix, a.safetyKind(s))
		cf.why = i18n.T("so hast du es auf diesem Gerät schon einmal korrigiert")
		return
	}
	if d := s.comp.due; d != nil && d.Kind == agent.EventFail && d.Command == f.Command {
		cf.waitAt = time.Now() // der Begleiter meldet sich gleich mit einem Fix
		return
	}
	cf.want = true
}

// cardFixFromCompanion: der Begleiter hat zu einem Fehler etwas gesagt.
func (a *App) cardFixFromCompanion(s *session, cmd string, r agent.Remark, err error) tea.Cmd {
	q := &s.ask
	for i, cf := range q.fixes {
		if cf.waitAt.IsZero() || cf.failed != cmd {
			continue
		}
		cf.waitAt = time.Time{}
		if err == nil && !r.Quiet && len(r.Commands) > 0 && oneLine(r.Commands[0].Command) != oneLine(cmd) {
			cf.fix, cf.why = r.Commands[0], r.Text
			if fix, ok := s.learnedFix(cf.fix.Command); ok {
				cf.fix = agent.ParseStepCommand(fix, a.safetyKind(s))
			}
			return nil
		}
		return a.requestCardFix(s, i)
	}
	return nil
}

// cardFixTick (jede Sekunde): Begleiter kam nicht dran oder braucht zu lange.
func (a *App) cardFixTick(s *session) tea.Cmd {
	c := &s.comp
	var cmds []tea.Cmd
	for i, cf := range s.ask.fixes {
		if cf.want {
			cf.want = false
			cmds = append(cmds, a.requestCardFix(s, i))
			continue
		}
		if cf.waitAt.IsZero() {
			continue
		}
		pending := c.busy || (c.due != nil && c.due.Command == cf.failed)
		if !pending || time.Since(cf.waitAt) > cardFixWait {
			cf.waitAt = time.Time{}
			cmds = append(cmds, a.requestCardFix(s, i))
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) requestCardFix(s *session, i int) tea.Cmd {
	q := &s.ask
	cf := q.fixes[i]
	if cf == nil {
		return nil
	}
	ag, err := a.agentClientFor(s.conn)
	if err != nil {
		cf.none, cf.why = true, err.Error()
		return nil
	}
	cf.busy = true
	sid, seq, question := s.id, q.seq, q.question
	c := a.agentContext(s, false)
	f := cf.f
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		r, err := ag.FixCard(ctx, question, f, c)
		return cardFixMsg{sid: sid, seq: seq, idx: i, r: r, err: err}
	}
}

func (a *App) cardFixDone(m cardFixMsg) {
	s := a.byID(m.sid)
	if s == nil || m.seq != s.ask.seq {
		return // inzwischen neue Frage
	}
	cf := s.ask.fixes[m.idx]
	if cf == nil {
		return
	}
	cf.busy = false
	if m.err != nil {
		cf.none, cf.why = true, i18n.T("KI-Fehler: ")+trunc(m.err.Error(), 100)
		return
	}
	cf.why = m.r.Reason
	if m.r.Command.Command == "" {
		cf.none = true
		return
	}
	cf.fix = m.r.Command
	if fix, ok := s.learnedFix(cf.fix.Command); ok {
		cf.fix = agent.ParseStepCommand(fix, a.safetyKind(s))
	}
	// Im Terminal gleich per Alt+1 greifbar, falls der Begleiter nichts anbietet
	if !s.compChips || len(s.chips) == 0 {
		s.chips, s.chipArmed, s.chipReplace, s.compChips = []agent.Suggestion{cf.fix}, -1, false, true
	}
}

// adoptCardFix (Taste u): der Fix wird zum Befehl der Karte, gleiche
// Fehler in anderen Karten werden mitkorrigiert.
func (a *App) adoptCardFix(s *session, i int) tea.Cmd {
	q := &s.ask
	cf := q.fixes[i]
	if cf == nil || cf.adopted || cf.fix.Command == "" {
		return nil
	}
	if q.codes == nil {
		q.codes = map[int]string{}
	}
	q.codes[i] = cf.fix.Command
	cf.adopted = true
	q.sel, q.armed = i, -1
	pairs := replacedRuns(cf.failed, cf.fix.Command)
	n := 0
	for j, cd := range answerCards(agent.VisibleAnswer(q.answer.String())) {
		if j == i {
			continue
		}
		cd = q.cardText(j, cd)
		if !cd.runnable() {
			continue
		}
		next := applyRuns(cd.code, pairs)
		if next == cd.code {
			continue
		}
		q.codes[j] = next
		if q.fixes[j] == nil {
			q.fixes[j] = &cardFix{from: cd.code, like: i, adopted: true}
		}
		n++
	}
	if n > 0 {
		a.flashOK(fmt.Sprintf(i18n.T("Fix übernommen · %d weitere Karten angepasst"), n))
	} else {
		a.flashOK(i18n.T("Fix übernommen"))
	}
	return nil
}

// learnSource: korrigiert der Befehl dieser Karte einen früheren? Dann
// lernt das Systemprofil daraus, sobald er ohne Fehler läuft.
func (q *askState) learnSource(i int) string {
	if cf := q.fixes[i]; cf != nil && cf.adopted {
		return cf.from
	}
	return ""
}

// ---------------------------------------------------------------- Mitziehen

// replacedRuns vergleicht zwei Befehle Wort für Wort (an Leerzeichen
// getrennt, Anführungszeichen bleiben am Wort) und liefert die ersetzten
// Wortfolgen: „--since "10.11. 12:00:00"“ → „--since "2026-11-10 12:00:00"“
// ergibt das Paar ["\"10.11."] → ["\"2026-11-10"]. Das Programm selbst
// (erstes Wort) und reine Einfügungen zählen nicht, die sind zu speziell.
func replacedRuns(from, to string) [][2][]string {
	a, b := strings.Fields(from), strings.Fields(to)
	if len(a) == 0 || len(b) == 0 || len(a) > 60 || len(b) > 60 {
		return nil
	}
	// längste gemeinsame Teilfolge
	l := make([][]int, len(a)+1)
	for i := range l {
		l[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				l[i][j] = l[i+1][j+1] + 1
			} else {
				l[i][j] = max(l[i+1][j], l[i][j+1])
			}
		}
	}
	var out [][2][]string
	seen := map[string]bool{}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if i < len(a) && j < len(b) && a[i] == b[j] {
			i++
			j++
			continue
		}
		si, sj := i, j
		for (i < len(a) || j < len(b)) && !(i < len(a) && j < len(b) && a[i] == b[j]) {
			if j >= len(b) || (i < len(a) && l[i+1][j] >= l[i][j+1]) {
				i++
			} else {
				j++
			}
		}
		old, nw := a[si:i], b[sj:j]
		key := strings.Join(old, " ") + "\x00" + strings.Join(nw, " ")
		if si == 0 || len(old) == 0 || len(nw) == 0 || len(old) > 3 || seen[key] || len(strings.Join(old, " ")) < 3 {
			continue
		}
		seen[key] = true
		out = append(out, [2][]string{old, nw})
		if len(out) == 3 {
			break
		}
	}
	return out
}

// applyRuns ersetzt die Wortfolgen in einem anderen Befehl (nicht am
// Programmnamen). Leerzeichen zwischen den Wörtern werden vereinheitlicht.
func applyRuns(cmd string, pairs [][2][]string) string {
	if len(pairs) == 0 {
		return cmd
	}
	w := strings.Fields(cmd)
	changed := false
	for _, p := range pairs {
		old, nw := p[0], p[1]
		var out []string
		for k := 0; k < len(w); {
			if k > 0 && k+len(old) <= len(w) && equalWords(w[k:k+len(old)], old) {
				out = append(out, nw...)
				k += len(old)
				changed = true
				continue
			}
			out = append(out, w[k])
			k++
		}
		w = out
	}
	if !changed {
		return cmd
	}
	return strings.Join(w, " ")
}

func equalWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cardFixView: Zeilen unter dem Befehl einer fehlgeschlagenen Karte.
func (a *App) cardFixView(cf *cardFix, w int) string {
	st := a.s
	out := []string{wrap(st.Danger.Render("× "+i18n.T("fehlgeschlagen: ")+trunc(cf.errLine, w*2)), w)}
	switch {
	case cf.fix.Command != "":
		out = append(out, st.OK.Render(i18n.T("Fix:"))+" "+wrap(highlightShell(cf.fix.Command, st), max(10, w-5)))
		if cf.why != "" {
			out = append(out, wrap(st.Muted.Render(trunc(cf.why, w*2)), w))
		}
	case cf.none:
		why := i18n.T("Kein Fix per Befehl")
		if cf.why != "" {
			why += ": " + cf.why
		}
		out = append(out, wrap(st.Muted.Render(trunc(why, w*2)+" · Alt+0 "+i18n.T("erklärt")), w))
	default:
		out = append(out, st.Muted.Render(i18n.T("KI sucht einen Fix …")))
	}
	return strings.Join(out, "\n")
}
