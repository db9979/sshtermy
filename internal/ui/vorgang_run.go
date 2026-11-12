package ui

// Schritte im Vorgang ausführen: einzeln einfügen (3), Fahrplan (4) und
// Agent für Lesendes (5). Jeder Schritt geht durch den Zielfenster-Schutz
// (vorgang_guard.go). Vorgetippt heißt: der Befehl steht am Prompt des
// Zielfensters, ausgeführt wird er erst mit ⏎ dort. Dort greifen dann auch
// der Gefahren-Check und alle anderen Schutzregeln wie beim Tippen.

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// vgTypeInto: Befehl nach bestandenem Schutz im Fenster vortippen und das
// Fenster nach vorn holen. false = Schutz hat angehalten (Dialog offen).
func (a *App) vgTypeInto(s *session, cmd string) bool {
	cs := a.vgGuard(s, cmd)
	if failedChecks(cs) {
		a.vgGuardModal(s, cmd, cs)
		return false
	}
	a.vgFocus(s)
	s.term.SendText(cmd)
	s.noteAI(cmd, true)
	s.flashTab(a.winColor(s)) // Tab leuchtet kurz auf (motion.go)
	a.vg.focus = false
	a.flash(i18n.F("In Fenster %d (%s) vorgetippt · ⏎ dort führt aus, Strg+C verwirft", a.winNo(s), s.tabName()))
	return true
}

// vgFocus: Fenster nach vorn (Tab wechseln bzw. Bereich aktiv machen).
func (a *App) vgFocus(s *session) {
	if i := a.indexOf(s); i >= 0 && i != a.cur {
		a.focusSession(i)
	}
	s.panelFocus = false
	a.scr = scrTerminal
}

// stepTarget: Sitzung und ausgefüllter Befehl eines Schritts. need = Werte,
// die noch fehlen (erst erfragen).
func (a *App) stepTarget(st *vgStep) (s *session, cmd string, need []string) {
	cmd = agent.FillPlaceholders(st.Command, a.vg.vals)
	for _, p := range agent.Placeholders(cmd) {
		need = append(need, p)
	}
	if st.Win > 0 {
		s = a.byWin(st.Win)
	}
	return s, cmd, need
}

// rateStep: Gefahr und „nur lesen“ nach dem echten System des Fensters
// (die KI kann sich beim System irren, sshTermy prüft selbst).
func (a *App) rateStep(s *session, cmd string) (danger []string, readOnly bool) {
	kind := a.safetyKind(s)
	sg := agent.ParseStepCommand(cmd, kind)
	for _, d := range sg.Danger {
		danger = append(danger, d.Reason)
	}
	readOnly = len(danger) == 0 && !sg.Elevated && safety.ReadOnly(cmd, kind)
	return danger, readOnly
}

// askValues fragt fehlende {{Werte}} nacheinander ab, dann then().
func (a *App) askValues(need []string, then func() tea.Cmd) tea.Cmd {
	if len(need) == 0 {
		return then()
	}
	name := need[0]
	a.modal = inputModal(i18n.T("Wert fehlt"), i18n.F("Wert für {{%s}} (kam nicht aus einer Ausgabe)", name), a.vg.vals[name], func(v string) tea.Cmd {
		v = strings.TrimSpace(v)
		if v == "" || strings.ContainsAny(v, "\r\n;&|`$<>") {
			a.flash(i18n.T("Wert leer oder mit Sonderzeichen (; & | $ < > `) – nicht übernommen"))
			return nil
		}
		a.vg.vals[name] = v
		return a.askValues(need[1:], then)
	})
	a.modal.in.Placeholder = name
	return nil
}

// ---------------------------------------------------------------- 3. Einzeln einfügen

// vgInsertStep: Schritt i der Antwort in seinem Fenster vortippen. Gefahr
// braucht hier den zweiten Druck, dann noch ⏎ im Fenster.
func (a *App) vgInsertStep(i int) tea.Cmd {
	g := a.vg
	if g == nil || i < 0 || i >= len(g.steps) {
		return nil
	}
	st := &g.steps[i]
	if st.Win == 0 {
		return a.vgOpenMissing(st.System)
	}
	s, cmd, need := a.stepTarget(st)
	if s == nil {
		a.flash(i18n.F("Fenster %d ist nicht mehr offen", st.Win))
		return nil
	}
	if len(need) > 0 {
		return a.askValues(need, func() tea.Cmd { return a.vgInsertStep(i) })
	}
	if danger, _ := a.rateStep(s, cmd); len(danger) > 0 && g.armedIns != i {
		g.armedIns = i
		a.flash(i18n.F("▲ %s in Fenster %d. Nochmal %d zum Vortippen", danger[0], st.Win, i+1))
		return nil
	}
	g.armedIns = -1
	if a.vgTypeInto(s, cmd) {
		st.state, st.cmd, st.at = vsTyped, cmd, time.Now()
		g.stepSel = i
	}
	return nil
}

// ---------------------------------------------------------------- 4. Fahrplan

// vgStartRun: Fahrplan mit den (angewählten) Schritten starten.
func (a *App) vgStartRun(auto bool) tea.Cmd {
	g := a.vg
	if g == nil || len(g.steps) == 0 {
		a.flash(i18n.T("Keine Schritte für einen Fahrplan"))
		return nil
	}
	for i := range g.steps {
		if g.steps[i].off {
			g.steps[i].state = vsSkipped
		} else if g.steps[i].state != vsDone {
			g.steps[i].state = vsPending
		}
	}
	g.mode, g.cur, g.why, g.auto = vgRun, 0, "", auto
	a.resizeAll()
	return a.vgAdvance()
}

// vgCurStep: aktueller Schritt (nil = fertig).
func (g *vorgang) curStep() *vgStep {
	for g.cur < len(g.steps) && (g.steps[g.cur].state == vsDone || g.steps[g.cur].state == vsSkipped) {
		g.cur++
	}
	if g.cur >= len(g.steps) {
		return nil
	}
	return &g.steps[g.cur]
}

// vgAdvance: nächsten Schritt vorbereiten.
func (a *App) vgAdvance() tea.Cmd {
	g := a.vg
	if g == nil || g.mode != vgRun {
		return nil
	}
	if a.modal != nil {
		g.advanceDue = true // erst wenn der Dialog zu ist
		return nil
	}
	st := g.curStep()
	if st == nil {
		g.why = ""
		a.flash(i18n.T("Fahrplan fertig · r zeigt den Rückblick"))
		g.open, g.focus = true, true
		return nil
	}
	if st.state == vsTyped || st.state == vsRunning || st.state == vsArmed {
		return nil // wartet schon
	}
	if st.Win == 0 {
		g.why = i18n.F("%s ist in keinem Fenster offen · o öffnet es", st.System)
		g.focus = true
		return nil
	}
	s, cmd, need := a.stepTarget(st)
	if s == nil {
		g.why = i18n.F("Fenster %d ist nicht mehr offen · s überspringt", st.Win)
		g.focus = true
		return nil
	}
	if len(need) > 0 {
		return a.askValues(need, func() tea.Cmd { return a.vgAdvance() })
	}
	danger, readOnly := a.rateStep(s, cmd)
	st.cmd = cmd
	g.why = ""
	switch {
	case g.auto && readOnly:
		// Agent: Lesendes läuft allein, aber nur nach bestandenem Schutz
		cs := a.vgGuard(s, cmd)
		if failedChecks(cs) {
			a.vgGuardModal(s, cmd, cs)
			g.why = i18n.T("Schutz hat angehalten · ⏎ versucht es erneut")
			g.focus = true
			return nil
		}
		a.vgFocus(s)
		a.beginCommand(s, cmd)
		if s.run != nil {
			s.run.agent = true
		}
		a.sendLine(s, cmd)
		s.history = append(s.history, cmd)
		a.profileCommand(s, cmd)
		st.state, st.at, st.note = vsRunning, time.Now(), i18n.T("Agent (liest)")
	case len(danger) > 0:
		st.state = vsArmed
		st.note = danger[0]
		g.focus, g.open = true, true
		a.flash(i18n.F("▲ Schritt %d: %s · ⏎ im Fahrplan tippt vor, dann ⏎ in Fenster %d", g.cur+1, danger[0], st.Win))
	default:
		if a.vgTypeInto(s, cmd) {
			st.state, st.at = vsTyped, time.Now()
		} else {
			g.why = i18n.T("Schutz hat angehalten · ⏎ versucht es erneut")
			g.focus = true
		}
	}
	return nil
}

// vgConfirm: ⏎ im Fahrplan.
func (a *App) vgConfirm() tea.Cmd {
	g := a.vg
	st := g.curStep()
	if st == nil {
		return nil
	}
	switch st.state {
	case vsArmed:
		s, cmd, _ := a.stepTarget(st)
		if s == nil {
			return nil
		}
		if a.vgTypeInto(s, cmd) {
			st.state, st.cmd, st.at = vsTyped, cmd, time.Now()
		}
		return nil
	case vsFailed:
		st.state = vsDone // trotzdem weiter
		st.note = strings.TrimSpace(st.note + " " + i18n.T("(trotzdem weiter)"))
		g.cur++
		return a.vgAdvance()
	case vsTyped:
		s, cmd, _ := a.stepTarget(st)
		if s == nil {
			return nil
		}
		if typed, ok := s.term.TypedInput(); ok && strings.TrimSpace(typed) == "" {
			a.vgTypeInto(s, cmd) // Zeile wurde verworfen (Strg+C): noch einmal vortippen
			return nil
		}
		a.vgFocus(s) // zurück ins Zielfenster, dort ⏎
		g.focus = false
		return nil
	}
	g.why = ""
	return a.vgAdvance()
}

// vgSkip: Schritt überspringen (s).
func (a *App) vgSkip() tea.Cmd {
	g := a.vg
	st := g.curStep()
	if st == nil {
		return nil
	}
	st.state, st.note = vsSkipped, i18n.T("übersprungen")
	g.cur++
	return a.vgAdvance()
}

// vgRetry: fehlgeschlagenen Schritt noch einmal (w).
func (a *App) vgRetry() tea.Cmd {
	g := a.vg
	st := g.curStep()
	if st == nil || st.state != vsFailed {
		return nil
	}
	st.state, st.note = vsPending, ""
	return a.vgAdvance()
}

// vgEditStep: Befehl des aktuellen Schritts ändern (e).
func (a *App) vgEditStep(i int) tea.Cmd {
	g := a.vg
	if i < 0 || i >= len(g.steps) {
		return nil
	}
	st := &g.steps[i]
	a.modal = inputModal(i18n.T("Schritt ändern"), i18n.F("Befehl für Fenster %d", st.Win), st.Command, func(v string) tea.Cmd {
		v = strings.TrimSpace(v)
		if v == "" || strings.ContainsAny(v, "\r\n") {
			return nil
		}
		st.Command, st.note = v, i18n.T("von dir geändert")
		if st.state == vsArmed {
			st.state = vsPending
		}
		return nil
	})
	a.modal.in.SetWidth(70)
	return nil
}

// vgCommandDone (aus finishCommand): Befehl in einem Fenster fertig.
func (a *App) vgCommandDone(s *session, r *cmdRun, out string, failed bool, f agent.Failure) {
	g := a.vg
	if g == nil || !g.active || g.member(s.id) == nil || r.private {
		return
	}
	n := a.winNo(s)
	ev := agent.MultiEvent{At: r.at, Win: n, Command: r.cmd, Failed: failed}
	if failed {
		ev.Note = f.Reason
	}
	if g.mode == vgRun {
		if st := g.curStep(); st != nil && st.Win == n && (st.state == vsTyped || st.state == vsRunning) && sameProgram(st.cmd, r.cmd) {
			a.vgStepFinished(st, r, out, failed, f, &ev)
		}
	}
	g.events = append(g.events, ev)
	if len(g.events) > 400 {
		g.events = g.events[len(g.events)-400:]
	}
	if failed && !r.interrupted && a.vgCrossWanted(s) {
		g.crossDue = &vgCross{sid: s.id, f: f}
	}
}

func (a *App) vgStepFinished(st *vgStep, r *cmdRun, out string, failed bool, f agent.Failure, ev *agent.MultiEvent) {
	g := a.vg
	if strings.TrimSpace(r.cmd) != strings.TrimSpace(st.cmd) {
		st.note = i18n.T("von dir geändert ausgeführt")
	}
	st.cmd, st.at = r.cmd, time.Now()
	if failed || r.interrupted {
		st.state = vsFailed
		st.note = orDefault(f.Reason, i18n.T("abgebrochen"))
		g.why = i18n.T("Schritt fehlgeschlagen · ⏎ trotzdem weiter · w nochmal · s überspringen · f KI fragen")
		g.open, g.focus = true, true
		return
	}
	st.state = vsDone
	if st.Save != "" {
		if v, ok := st.Extract(out); ok {
			g.vals[st.Save] = v
			st.note = "{{" + st.Save + "}} = " + v
			ev.Note = st.note
		} else {
			st.note = i18n.F("Wert {{%s}} nicht in der Ausgabe gefunden", st.Save)
		}
	}
	g.cur++
	g.advanceDue = true // nächster Schritt beim nächsten Takt (finishCommand liefert kein Cmd)
}

// vgOpenMissing: System, das in keinem Fenster offen ist, öffnen und zum
// Vorgang nehmen (nach Rückfrage).
func (a *App) vgOpenMissing(name string) tea.Cmd {
	name = strings.TrimSpace(strings.SplitN(name, ":", 2)[0])
	var hit *domain.Connection
	for _, c := range a.st.Connections() {
		if strings.EqualFold(c.Alias, name) || strings.EqualFold(c.Host, name) {
			c := c
			hit = &c
			break
		}
	}
	if hit == nil {
		a.flash(i18n.F("Keine gespeicherte Verbindung „%s“ – im Manager anlegen oder öffnen, dann Alt+V", name))
		return nil
	}
	c := *hit
	a.modal = confirmModal(i18n.T("System öffnen?"), i18n.F("%s öffnen und zum Vorgang nehmen?\n\nDie KI sieht es danach wie die anderen Fenster.", c.Alias), func() tea.Cmd {
		s, cmd := a.spawn(c)
		a.vgJoin(s)
		n := a.winNo(s)
		for i := range a.vg.steps {
			if st := &a.vg.steps[i]; st.Win == 0 && strings.EqualFold(strings.TrimSpace(strings.SplitN(st.System, ":", 2)[0]), name) {
				st.Win, st.System = n, ""
			}
		}
		a.vg.why = ""
		a.flash(i18n.F("%s öffnet als Fenster %d", c.Alias, n))
		return cmd
	})
	return nil
}

// sameProgram: gehört der ausgeführte Befehl zum vorgetippten Schritt
// (gleiches erstes Wort)? Sonst war es etwas anderes im selben Fenster.
func sameProgram(step, ran string) bool {
	a, b := strings.Fields(step), strings.Fields(ran)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if a[0] == "sudo" && len(a) > 1 && b[0] == "sudo" && len(b) > 1 {
		return a[1] == b[1]
	}
	return a[0] == b[0]
}
