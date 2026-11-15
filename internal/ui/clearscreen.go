package ui

import (
	"github.com/db9979/sshtermy/internal/i18n"
)

// Bildschirm und Verlauf leeren, Eingabezeile bleibt (Alt+X): der Prompt
// samt halb getippter Eingabe rückt nach oben, alles andere ist weg. Anders
// als `clear` oder Strg+L geht nichts an den Server, laufende Programme
// merken nichts. Vollbildprogramme (vim, less, htop) bleiben unberührt.
func (a *App) clearKeepLine(s *session) {
	if s == nil || s.term == nil {
		return
	}
	if !s.term.ClearKeepLine() {
		a.flash(i18n.T("Leeren geht nicht, solange ein Vollbildprogramm läuft (vim, less, htop)"))
		return
	}
	// alles, was auf Zeilen im Verlauf zeigt, gilt nicht mehr
	s.scroll, s.sel, s.selecting = 0, nil, false
	s.blocks, s.blockNav, s.blockScroll = nil, 0, 0
	s.search = searchState{}
	s.viewMemo = termMemo{}
	a.flash(i18n.T("Bildschirm und Verlauf geleert"))
}
