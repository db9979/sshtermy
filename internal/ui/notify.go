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
		a.sendDesktop(i18n.T("Desktop-Benachrichtigungen sind an"), i18n.T("So sieht eine Meldung aus, wenn ein langer Befehl fertig ist."))
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
		a.sendDesktop(i18n.T("Desktop-Benachrichtigungen sind an"), i18n.T("So sieht eine Meldung aus, wenn ein langer Befehl fertig ist."))
	}
	return nil
}

func (a *App) toggleOSC52() tea.Cmd {
	on := !a.osc52On()
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.NoOSC52 = !on })
	return nil
}

// sendDesktop: Benachrichtigung des Betriebssystems (in Tests abschaltbar).
var desktopSend = notify.Send

func (a *App) sendDesktop(title, body string) { desktopSend(title, body) }

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
		return
	}
	kind, word := noticeOK, i18n.T("fertig")
	if failed {
		kind, word = noticeFail, i18n.T("fehlgeschlagen")
	}
	if !a.sessionVisible(s) {
		s.notice = kind
	}
	msg := fmt.Sprintf("%s: %s %s (%s)", s.tabName(), trunc(cmd, 40), word, fmtDur(dur))
	if failed {
		a.flashErr(msg)
	} else {
		a.flashOK(msg)
	}
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() {
		body := trunc(cmd, 80) + " " + word
		if a.lock != nil {
			body = i18n.T("Befehl ") + word // gesperrt: keinen Befehl verraten
		}
		a.sendDesktop("sshTermy · "+s.tabName(), body+" ("+fmtDur(dur)+")")
	}
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
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() {
		a.sendDesktop("sshTermy · "+s.tabName(), i18n.T("Glocke (BEL) im Terminal"))
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
