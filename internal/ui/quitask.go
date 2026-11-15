package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Beenden absichern: Strg+Q und das Schließen des eigenen Fensters trennen offene
// Verbindungen nicht mehr ohne Rückfrage. Ohne offene Verbindung (oder noch
// gesperrt) wird sofort beendet; in der Rückfrage beendet ein zweites Strg+Q.

// liveSessions: Tabs, deren Verbindung noch steht oder gerade aufgebaut wird.
func (a *App) liveSessions() []*session {
	var out []*session
	for _, s := range a.sessions {
		if s.state != stClosed {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) askQuit() tea.Cmd {
	live := a.liveSessions()
	if len(live) == 0 || a.scr == scrUnlock || a.intro != nil {
		return a.quit()
	}
	names := make([]string, 0, len(live))
	for i, s := range live {
		if i == 6 {
			names = append(names, i18n.F("… und %d weitere", len(live)-i))
			break
		}
		names = append(names, "  • "+s.tabName())
	}
	body := i18n.T("Diese Verbindungen werden getrennt:") + "\n" + strings.Join(names, "\n")
	if len(live) == 1 {
		body = i18n.T("Diese Verbindung wird getrennt:") + "\n" + strings.Join(names, "\n")
	}
	if a.Keeper() {
		// Hintergrunddienst: abkoppeln ist der Normalfall (keeper.go)
		body = i18n.T("a = abkoppeln: Fenster zu, alle Sitzungen laufen im Hintergrund weiter (wieder anhängen: sshtermy)") +
			"\n\n" + body + "\n\n" + i18n.F("j = trotzdem beenden (%s noch einmal = beenden) · n = abbrechen", a.keys.label(actQuit))
		m := confirmModal(i18n.T("sshTermy abkoppeln oder beenden?"), body, func() tea.Cmd { return a.quit() })
		m.quit = true
		m.alt = map[string]func() tea.Cmd{"a": a.keepDetach}
		a.modal = m
		return nil
	}
	body += "\n\n" + i18n.F("Trotzdem beenden? (%s noch einmal = beenden)", a.keys.label(actQuit))
	if !a.keepOn() {
		body += "\n" + i18n.T("Tipp: Mit dem Hintergrunddienst laufen Sitzungen weiter (Befehlspalette, „Hintergrunddienst“)")
	}
	m := confirmModal(i18n.T("sshTermy beenden?"), body, func() tea.Cmd { return a.quit() })
	m.quit = true
	a.modal = m
	return nil
}
