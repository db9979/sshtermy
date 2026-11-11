package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Update-Kanal: sofort (jedes Release) oder stabil (erst nach einem Tag
// bzw. einer Woche). Gilt für den Hinweis beim Start und für „Nach Updates
// suchen“; `sshtermy -update` nimmt immer die neueste Version.

var updateWaits = []int{0, 24, 7 * 24}

func updateWaitLabel(h int) string {
	switch {
	case h <= 0:
		return i18n.T("sofort")
	case h == 24:
		return i18n.T("stabil: nach 1 Tag")
	case h%24 == 0:
		return fmt.Sprintf(i18n.T("stabil: nach %d Tagen"), h/24)
	}
	return fmt.Sprintf(i18n.T("stabil: nach %d Stunden"), h)
}

func (a *App) cycleUpdateWait(dir int) tea.Cmd {
	cur := a.st.Settings().UpdateWaitH
	i := 0
	for j, h := range updateWaits {
		if h == cur {
			i = j
		}
	}
	next := updateWaits[cycleIndex(i, len(updateWaits), dir)]
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.UpdateWaitH = next }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.rel = nil // ein schon gemeldetes Update passt vielleicht nicht mehr zum Kanal
	return a.checkUpdate()
}
