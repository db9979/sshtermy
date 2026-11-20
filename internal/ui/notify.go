package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/notify"
)

// Meldungen aus verdeckten Tabs
//
// Läuft ein Befehl länger als notifyAfter und ist sein Tab beim Ende nicht
// zu sehen (anderer Tab, Manager, oder das sshTermy-Fenster ist nicht im
// Vordergrund), bekommt der Tab eine Markierung (✓ grün, × rot) und es
// erscheint eine Einblendung. Ist das Fenster im Hintergrund, kommt dazu
// eine Benachrichtigung des Betriebssystems. Die Glocke (BEL, z. B. von
// `make; tput bel` oder Chat-Programmen) markiert den Tab mit •.
//
// OSC 52: Programme auf dem Server (tmux mit set-clipboard, vim/neovim mit
// OSC-52-Plugin) können Text in die lokale Zwischenablage legen. Lesen
// können sie sie nie.

// notifyAfter: ab dieser Laufzeit meldet ein fertiger Befehl sich.
const notifyAfter = 10 * time.Second

type noticeKind int

const (
	noticeNone noticeKind = iota
	noticeOK
	noticeFail
	noticeBell
	noticeAlert // Farbregel mit „!“ hat angeschlagen (highlight.go)
	noticeLog   // Logwache: neues Muster (logwatch.go)
	noticeWait  // wartet auf eine Eingabe (waitmark.go)
	noticeAgent // KI-Coding-Agent wartet auf Freigabe (waitmark.go)
)

func (a *App) notifyOn() bool        { return !a.st.Settings().NoNotify }
func (a *App) desktopNotifyOn() bool { return !a.st.Settings().NoDesktopNotify }
func (a *App) osc52On() bool         { return !a.st.Settings().NoOSC52 }

func (a *App) toggleNotify() tea.Cmd {
	on := !a.notifyOn()
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.NoNotify = !on })
	return nil
}

func (a *App) toggleDesktopNotify() tea.Cmd {
	on := !a.desktopNotifyOn()
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.NoDesktopNotify = !on })
	if on {
		desktopSend(i18n.T("Desktop-Benachrichtigungen sind an"), i18n.T("So sieht eine Meldung aus, wenn ein langer Befehl fertig ist."), nil)
	}
	return nil
}

// Meldung bei langen Befehlen in einem Durchschalt-Eintrag:
// 0 = aus, 1 = nur in sshTermy, 2 = zusätzlich Desktop-Benachrichtigung.
func (a *App) notifyLevel() int {
	switch {
	case !a.notifyOn():
		return 0
	case !a.desktopNotifyOn():
		return 1
	}
	return 2
}

func notifyLabel(level int) string {
	return [...]string{i18n.T("aus"), i18n.T("in sshTermy"), i18n.T("+ Desktop")}[level]
}

func (a *App) cycleNotify(dir int) tea.Cmd {
	next := cycleIndex(a.notifyLevel(), 3, dir)
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		s.NoNotify = next == 0
		s.NoDesktopNotify = next < 2
	})
	if next == 2 {
		desktopSend(i18n.T("Desktop-Benachrichtigungen sind an"), i18n.T("So sieht eine Meldung aus, wenn ein langer Befehl fertig ist."), nil)
	}
	return nil
}

func (a *App) toggleOSC52() tea.Cmd {
	on := !a.osc52On()
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.NoOSC52 = !on })
	return nil
}

// sendDesktop: Benachrichtigung des Betriebssystems (in Tests abschaltbar).
// click läuft, wenn man die Meldung anklickt (nur wo das System es meldet).
var desktopSend = func(title, body string, click func()) {
	notify.SendClick(title, body, i18n.T("Öffnen"), click)
}

// Geht zusätzlich als Handy-Meldung raus, wenn eingerichtet und niemand am
// Rechner ist (pushnotify.go).
func (a *App) sendDesktop(title, body string) {
	if a.desktopAway() {
		desktopSend(title, body, nil)
	}
	a.pushSend(title, body)
}

// Klick auf eine Desktop-Meldung (oder zurück ins Fenster danach) führt in
// den Tab, um den es ging. Linux meldet den Klick (notify-send --action);
// Windows und macOS nicht: dort springt sshTermy beim nächsten Fokus hin,
// wenn seit der Meldung keine Taste gedrückt wurde.

// jumpWindow: so lange nach der Meldung gilt der Sprung beim Zurückkehren.
const jumpWindow = 30 * time.Minute

type notifyClickMsg struct{ sid int }

// sendDesktopFor: Meldung zu Tab s; merkt ihn für den Sprung.
func (a *App) sendDesktopFor(s *session, title, body string) {
	a.pushSend(title, body)
	if !a.desktopAway() {
		return
	}
	a.jumpSID, a.jumpAt = s.id, time.Now()
	ev, sid := a.events, s.id
	desktopSend(title, body, func() {
		select {
		case ev <- notifyClickMsg{sid}:
		default:
		}
	})
}

// jumpToNotified: in den gemeldeten Tab wechseln (Klick oder Fokus).
func (a *App) jumpToNotified(sid int, why string) bool {
	s := a.byID(sid)
	if s == nil || a.lock != nil || a.scr == scrUnlock || a.modal != nil {
		return false
	}
	for i, x := range a.sessions {
		if x == s {
			if a.cur == i && a.scr == scrTerminal {
				return false
			}
			a.focusSession(i)
			a.scr = scrTerminal
			a.flash("→ " + s.tabName() + " · " + why)
			return true
		}
	}
	return false
}

// onRefocus (tea.FocusMsg): seit der letzten Meldung nichts getippt? Dann
// dorthin, wo die Meldung herkam.
func (a *App) onRefocus() {
	sid, at := a.jumpSID, a.jumpAt
	a.jumpSID = 0
	if sid == 0 || time.Since(at) > jumpWindow || a.lastKey.After(at) {
		return
	}
	a.jumpToNotified(sid, i18n.T("von der Desktop-Meldung"))
}

// sessionVisible: Ist der Tab gerade auf dem Bildschirm (aktiv oder in
// einem Bereich der geteilten Ansicht)? Das Fenster selbst kann trotzdem im
// Hintergrund sein (a.blurred).
func (a *App) sessionVisible(s *session) bool {
	if a.scr != scrTerminal || a.lock != nil {
		return false
	}
	if s == a.current() {
		return true
	}
	return a.slotOf(s) >= 0
}

// clearNotices: sichtbare Tabs verlieren ihre Markierung.
func (a *App) clearNotices() {
	if a.blurred {
		return
	}
	for _, s := range a.sessions {
		if s.notice != noticeNone && a.sessionVisible(s) {
			s.notice = noticeNone
		}
	}
}

// noticeDone: Befehl r ist nach dur fertig (aus finishCommand).
func (a *App) noticeDone(s *session, cmd string, dur time.Duration, failed, interrupted bool) {
	if interrupted || dur < notifyAfter || !a.notifyOn() {
		return
	}
	seen := a.sessionVisible(s) && !a.blurred
	if seen {
		if a.pushAway() { // vor dem Bildschirm, aber lange keine Taste: nur aufs Handy
			a.pushSend("sshTermy · "+s.tabName(), trunc(cmd, 80)+" "+doneWord(failed)+" ("+fmtDur(dur)+")")
		}
		return
	}
	kind, word := noticeOK, i18n.T("fertig")
	if failed {
		kind, word = noticeFail, i18n.T("fehlgeschlagen")
	}
	if !a.sessionVisible(s) {
		s.notice = kind
		if failed {
			s.flashTab(a.s.T.Danger)
		}
	}
	msg := fmt.Sprintf("%s: %s %s (%s)", s.tabName(), trunc(cmd, 40), word, fmtDur(dur))
	if failed {
		a.flashErr(msg)
	} else {
		a.flashOK(msg)
	}
	if a.notifyAway() {
		body := trunc(cmd, 80) + " " + word
		if a.lock != nil {
			body = i18n.T("Befehl ") + word // gesperrt: keinen Befehl verraten
		}
		a.sendDesktopFor(s, "sshTermy · "+s.tabName(), body+" ("+fmtDur(dur)+")")
	}
}

func doneWord(failed bool) string {
	if failed {
		return i18n.T("fehlgeschlagen")
	}
	return i18n.T("fertig")
}

// noticeBell: BEL vom Server.
func (a *App) noticeBell(s *session) {
	if !a.notifyOn() || (a.sessionVisible(s) && !a.blurred) {
		return
	}
	if !a.sessionVisible(s) && s.notice == noticeNone {
		s.notice = noticeBell
	}
	if time.Since(s.bellAt) < 30*time.Second {
		return // läutet ein Programm dauernd, nicht jedes Mal melden
	}
	s.bellAt = time.Now()
	a.flash("♦ " + s.tabName() + ": " + i18n.T("Glocke (BEL) im Terminal"))
	if a.notifyAway() {
		a.sendDesktopFor(s, "sshTermy · "+s.tabName(), i18n.T("Glocke (BEL) im Terminal"))
	}
}

// noticeGlyph: Markierung im Tab statt des Verbindungspunkts ("" = keine).
func (a *App) noticeGlyph(s *session) string {
	switch s.notice {
	case noticeOK:
		return a.s.OK.Render("✓")
	case noticeFail:
		return a.s.Danger.Render("×")
	case noticeBell:
		return a.s.Warn.Render("•")
	case noticeAlert:
		return a.s.Warn.Render("!")
	case noticeLog:
		return a.s.Danger.Render("▲")
	case noticeWait:
		return a.s.Warn.Bold(true).Render("?")
	case noticeAgent:
		return a.s.Warn.Bold(true).Render("◆")
	}
	return ""
}

// takeClipboard: OSC 52 vom Server in die lokale Zwischenablage.
func (a *App) takeClipboard(s *session) tea.Cmd {
	if s.term == nil {
		return nil
	}
	txt := s.term.TakeClipboard()
	// Zuschauer-Tab: ein fremder Gastgeber darf nichts in die eigene
	// Zwischenablage legen (z. B. einen Befehl, den man dann einfügt).
	if txt == "" || !a.osc52On() || s.conn.Protocol == domain.ProtoWatch {
		return nil
	}
	clipRemember(txt, s.conn.Alias)
	go func() { _ = clipboard.WriteAll(txt) }()
	if n := countLines(txt); n > 1 {
		a.flash(i18n.T("Vom Server kopiert: ") + plural(n, i18n.T("Zeile"), i18n.T("Zeilen")))
	} else {
		a.flash(i18n.T("Vom Server kopiert: ") + trunc(txt, 50))
	}
	return tea.SetClipboard(txt)
}

func countLines(s string) int {
	n := 1
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '\n' {
			n++
		}
	}
	return n
}

// fmtDur: „12 s“, „3:05 min“, „1:02 h“.
func fmtDur(d time.Duration) string {
	sec := int(d.Round(time.Second).Seconds())
	switch {
	case sec < 60:
		return fmt.Sprintf("%d s", sec)
	case sec < 3600:
		return fmt.Sprintf("%d:%02d min", sec/60, sec%60)
	}
	return fmt.Sprintf("%d:%02d h", sec/3600, sec/60%60)
}
