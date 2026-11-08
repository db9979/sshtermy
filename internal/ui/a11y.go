package ui

// Barrierearm (Settings.Accessible): keine Animationen (Popups, Start),
// Formen statt nur Farben, z. B. × statt rotem Punkt für „nicht erreichbar“.
// Für Rot-Grün-Schwäche gibt es zusätzlich die Farbschemata „Farbenblind“.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// accessible: aktueller Stand (applyLook), für Stellen ohne App.
var accessible bool

func (a *App) toggleAccessible() tea.Cmd {
	on := !accessible
	err := a.st.UpdateSettings(func(s *domain.Settings) {
		s.Accessible = on
		if on {
			s.Intro = "aus" // die Startanimation gleich mit
		}
	})
	if err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	accessible = on
	if on {
		_ = syncIntroFile(a.st.Dir, "aus")
		a.fx = fadeFx{}
		a.flash(i18n.T("Barrierearm an: keine Animationen, × für nicht erreichbar. Farbschema „Farbenblind“ unter Aussehen."))
	} else {
		a.flash(i18n.T("Barrierearm aus"))
	}
	return nil
}
