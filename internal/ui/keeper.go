package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/winstate"
)

// Hintergrunddienst (Idee aus herdr): Ist er eingeschaltet, läuft sshTermy
// als eigener Prozess ohne Terminal weiter, das Terminalfenster dockt nur an
// (cmd/sshtermy keep.go, internal/detach hub.go). Strg+Q fragt dann:
// a = abkoppeln (alles läuft weiter), j = wirklich beenden. Schließt man das
// Terminalfenster einfach, ist das wie Abkoppeln. „sshtermy“ hängt sich
// beim nächsten Start wieder an, auch aus einem anderen Terminal.

// SetKeeper: sshTermy läuft als Hintergrunddienst; detach entlässt das
// angedockte Fenster mit einer Meldung.
func (a *App) SetKeeper(detach func(msg string)) { a.keeperDetach = detach }

// Keeper: läuft sshTermy als Hintergrunddienst?
func (a *App) Keeper() bool { return a.keeperDetach != nil }

// keepDetach: Fenster abkoppeln, Sitzungen laufen weiter.
func (a *App) keepDetach() tea.Cmd {
	if a.keeperDetach == nil {
		return nil
	}
	a.saveRestoreAs(restoreSession) // falls der Rechner bis zum Wiederanhängen neu startet
	a.keeperDetach(i18n.F("sshTermy läuft im Hintergrund weiter (%s). Wieder anhängen: sshtermy",
		plural(len(a.liveSessions()), i18n.T("Sitzung"), i18n.T("Sitzungen"))))
	return nil
}

func (a *App) keepOn() bool { return a.st != nil && winstate.Load(a.st.Dir).Keep }

func (a *App) toggleKeep() tea.Cmd {
	s := winstate.Load(a.st.Dir)
	s.Keep = !s.Keep
	if err := winstate.Save(a.st.Dir, s); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	switch {
	case s.Keep && !a.Keeper():
		a.flashOK(i18n.T("Hintergrunddienst ab dem nächsten Start: Sitzungen laufen weiter, wenn du das Fenster schließt"))
	case !s.Keep && a.Keeper():
		a.flash(i18n.T("Hintergrunddienst aus ab dem nächsten Start (läuft gerade noch)"))
	case s.Keep:
		a.flash(i18n.T("Hintergrunddienst an"))
	default:
		a.flash(i18n.T("Hintergrunddienst aus"))
	}
	return nil
}
