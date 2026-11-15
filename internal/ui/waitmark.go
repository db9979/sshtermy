package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/codeagent"
	"github.com/db9979/sshtermy/internal/i18n"
)

// „Wartet auf dich“ (Idee aus herdr): Ein verdeckter Tab, der an einer
// Eingabe hängt (Passwort/sudo, [Y/n], „Press any key“, --More--, less),
// bekommt eine gelbe Marke „?“ und meldet sich einmal (Einblendung, im
// Hintergrund auch als Desktop-Meldung). Alt+G springt zum nächsten Tab mit
// Marke, wartende zuerst. Was du gesehen hast, bevor du den Tab verlassen
// hast (z. B. ein offenes less), meldet sich nicht.
//
// KI-Coding-Agenten (codeagent): Läuft im Tab Claude Code, Codex, Gemini CLI
// & Co., zeigt der Tab, ob der Agent arbeitet (◆ violett) oder auf eine
// Freigabe wartet (◆ gelb). Wartet er im verdeckten Tab, kommt eine Meldung;
// ist er nach mindestens 10 s Arbeit fertig, ebenso.

// waitQuiet: so lange ohne Ausgabe, bis eine Rückfrage als „wartet“ gilt.
const waitQuiet = 1500 * time.Millisecond

// agentGone: so lange ohne Kennzeichen am Bildschirm, bis der Agent als
// beendet gilt.
const agentGone = 5 * time.Second

type waitState struct {
	kind string // agent.InputKind oder "secret"; "" = wartet nicht
	line string // Zeile mit der Rückfrage
	ack  string // diese Rückfrage hast du schon gesehen
}

type agentWatch struct {
	name     string // erkannter Agent, "" = keiner
	state    codeagent.State
	since    time.Time // Beginn des Zustands
	lastSeen time.Time // zuletzt am Bildschirm erkannt
	gen      uint64    // Terminal-Stand beim letzten Blick
}

// waitTick (aus onTick, je Tab).
func (a *App) waitTick(s *session, now time.Time) {
	if s.term == nil || s.state != stConnected || s.detached() {
		s.wait, s.agentW = waitState{}, agentWatch{}
		return
	}
	a.codeAgentTick(s, now)
	if s.agentW.name != "" || s.goal.active {
		s.wait.kind = "" // Agent oder Ziel-Modus melden sich selbst
		if s.notice == noticeWait {
			s.notice = noticeNone
		}
		return
	}
	if now.Sub(s.lastOutput) < waitQuiet {
		if s.wait.kind != "" && s.lastOutput.After(now.Add(-waitQuiet)) {
			s.wait.kind = "" // es geht weiter
			a.clearWaitNotice(s)
		}
		return
	}
	kind, line := waitKind(s)
	if kind == "" {
		if s.wait.kind != "" {
			s.wait.kind = ""
			a.clearWaitNotice(s)
		}
		s.wait.ack = ""
		return
	}
	if a.sessionVisible(s) && !a.blurred {
		s.wait.kind, s.wait.line, s.wait.ack = kind, line, line // du siehst es ja
		return
	}
	if line == s.wait.ack || (kind == s.wait.kind && line == s.wait.line) {
		s.wait.kind, s.wait.line = kind, line
		return // schon gesehen bzw. schon gemeldet
	}
	s.wait.kind, s.wait.line = kind, line
	if !a.notifyOn() {
		return
	}
	if !a.sessionVisible(s) {
		s.notice = noticeWait
	}
	msg := s.tabName() + ": " + waitLabel(kind)
	if kind != "secret" && line != "" {
		msg += " · " + trunc(line, 50)
	}
	a.pushToast(toastWarn, "? "+msg)
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() {
		body := waitLabel(kind)
		if a.lock == nil && kind != "secret" {
			body += ": " + trunc(line, 80)
		}
		a.sendDesktopFor(s, "sshTermy · "+s.tabName(), body)
	}
}

// waitKind: wartet die Cursorzeile auf eine Eingabe?
func waitKind(s *session) (string, string) {
	if s.term.AtIdlePrompt() {
		return "", ""
	}
	line := strings.TrimSpace(s.term.CursorLine())
	if agent.LooksLikeSecretPrompt(line) {
		return "secret", line
	}
	if k := agent.InputWait(line, s.term.AltScreen()); k != agent.InputNone {
		return string(k), line
	}
	return "", ""
}

func waitLabel(kind string) string {
	switch kind {
	case "secret":
		return i18n.T("wartet auf ein Passwort")
	case string(agent.InputYesNo), string(agent.InputConfirm):
		return i18n.T("wartet auf ja/nein")
	case string(agent.InputPager):
		return i18n.T("wartet im Pager (Leertaste/q)")
	case string(agent.InputAnyKey):
		return i18n.T("wartet auf eine Taste")
	}
	return i18n.T("wartet auf dich")
}

func (a *App) clearWaitNotice(s *session) {
	if s.notice == noticeWait || s.notice == noticeAgent {
		s.notice = noticeNone
	}
}

// codeAgentTick: KI-Coding-Agent im Tab erkennen und Zustandswechsel melden.
func (a *App) codeAgentTick(s *session, now time.Time) {
	w := &s.agentW
	gen := s.term.Gen()
	if gen == w.gen && w.name == "" {
		return // nichts Neues am Bildschirm
	}
	w.gen = gen
	screen := s.term.ScreenText()
	title := s.term.Title()
	if w.name == "" {
		name := ""
		if s.run != nil {
			name = codeagent.FromCommand(s.run.cmd)
		}
		if name == "" && !s.term.AtIdlePrompt() {
			name = codeagent.FromScreen(screen, title)
		}
		if name == "" {
			return
		}
		*w = agentWatch{name: name, since: now, lastSeen: now, gen: gen}
	}
	st, seen := codeagent.Detect(w.name, screen, title)
	if seen {
		w.lastSeen = now
	} else if now.Sub(w.lastSeen) > agentGone && (s.run == nil || s.term.AtIdlePrompt()) {
		*w = agentWatch{gen: gen} // beendet, zurück an der Shell
		a.clearWaitNotice(s)
		return
	}
	if st == codeagent.Unknown || st == w.state {
		return
	}
	prev, ran := w.state, now.Sub(w.since)
	w.state, w.since = st, now
	seenNow := a.sessionVisible(s) && !a.blurred
	switch {
	case st == codeagent.Blocked:
		if !seenNow {
			a.agentNotice(s, noticeAgent, i18n.F("%s wartet auf deine Freigabe", w.name))
		}
	case prev == codeagent.Working && st == codeagent.Idle && ran >= notifyAfter:
		if !seenNow {
			a.agentNotice(s, noticeOK, i18n.F("%s ist fertig (%s)", w.name, fmtDur(ran)))
		}
	default:
		if s.notice == noticeAgent {
			s.notice = noticeNone // Freigabe erteilt, es geht weiter
		}
	}
}

func (a *App) agentNotice(s *session, kind noticeKind, text string) {
	if !a.notifyOn() {
		return
	}
	if !a.sessionVisible(s) {
		s.notice = kind
	}
	if kind == noticeAgent {
		a.pushToast(toastWarn, "◆ "+s.tabName()+": "+text)
	} else {
		a.flashOK("◆ " + s.tabName() + ": " + text)
	}
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() {
		a.sendDesktopFor(s, "sshTermy · "+s.tabName(), text)
	}
}

// agentGlyph: Zustand des Agenten im Tab ("" = kein Agent).
func (a *App) agentGlyph(s *session) string {
	switch s.agentW.state {
	case codeagent.Working:
		return a.s.AI.Render("◆")
	case codeagent.Blocked:
		return a.s.Warn.Render("◆")
	}
	return ""
}

// tabStatus: Zustand als Wort für die Tab-Liste (Alt+T), "" = nichts Besonderes.
func (a *App) tabStatus(s *session) string {
	if w := s.agentW; w.name != "" {
		switch w.state {
		case codeagent.Working:
			return "◆ " + w.name + " " + i18n.T("arbeitet")
		case codeagent.Blocked:
			return "◆ " + w.name + " " + i18n.T("wartet auf Freigabe")
		}
		return "◆ " + w.name + " " + i18n.T("bereit")
	}
	if s.wait.kind != "" {
		return "? " + waitLabel(s.wait.kind)
	}
	return ""
}

// noticeRank: Reihenfolge für Alt+G (kleiner = zuerst), -1 = keine Marke.
func noticeRank(s *session) int {
	switch {
	case s.notice == noticeWait || s.notice == noticeAgent:
		return 0
	case s.notice == noticeFail || s.notice == noticeLog:
		return 1
	case s.notice != noticeNone:
		return 2
	}
	return -1
}

// jumpNextNotice (Alt+G): zum nächsten Tab mit Marke, Wartende zuerst; bei
// gleichem Rang der nächste rechts vom aktuellen.
func (a *App) jumpNextNotice() tea.Cmd {
	best, bestRank := -1, 99
	n := len(a.sessions)
	for k := 1; k <= n; k++ {
		i := (a.cur + k) % n
		if r := noticeRank(a.sessions[i]); r >= 0 && r < bestRank && i != a.cur {
			best, bestRank = i, r
		}
	}
	if best < 0 {
		a.flash(i18n.T("Kein Tab wartet und keiner hat eine Meldung"))
		return nil
	}
	a.focusSession(best)
	a.scr = scrTerminal
	return nil
}
