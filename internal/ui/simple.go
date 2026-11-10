package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/winstate"
)

// Einfacher Modus (Settings.Simple): Profi-Funktionen verschwinden aus der
// Übersicht der Palette, den Einstellungen und der Tastenzeile. Nichts wird
// abgeschaltet: Tippen in der Palette findet alles, die Tasten bleiben.

func (a *App) simple() bool { return a.st != nil && a.st.Settings().Simple }

func (a *App) toggleSimple() tea.Cmd {
	on := !a.simple()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Simple = on }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Einfacher Modus an: Profi-Funktionen findest du weiter über die Suche in der Palette."))
	} else {
		a.flash(i18n.T("Einfacher Modus aus: alle Funktionen sichtbar"))
	}
	return nil
}

// simpleProAreas: Bereiche der Palette, die im einfachen Modus fehlen.
var simpleProAreas = map[string]bool{"multi": true, "net": true, "team": true}

// simpleHide blendet Profi-Einträge aus (nur ohne Suchtext sichtbar weg).
func (a *App) simpleHide(items []palItem) []palItem {
	if !a.simple() {
		return items
	}
	for i := range items {
		if items[i].pro || simpleProAreas[items[i].area] {
			items[i].hidden = true
			items[i].top = false
		}
	}
	return items
}

// simpleKeys: Tastenzeile ohne Profi-Tasten.
func (a *App) simpleKeys(keys [][2]string) [][2]string {
	if !a.simple() {
		return keys
	}
	drop := map[string]bool{}
	for _, act := range []action{actBroadcast, actScan, actWorkspace, actFolderGrid, actTagFilter, actSchedule, actTunnels, actQuickEdit} {
		drop[a.keys.label(act)] = true
	}
	drop["t"] = true // Tunnel im Manager
	out := keys[:0:0]
	for _, k := range keys {
		if !drop[k[0]] {
			out = append(out, k)
		}
	}
	return out
}

// ---------------------------------------------------------------- Fenster

func (a *App) winStart() string { return winstate.Load(a.st.Dir).Mode() }

func winStartLabel(m string) string {
	switch m {
	case winstate.StartMax:
		return i18n.T("maximiert")
	case winstate.StartLast:
		return i18n.T("wie zuletzt")
	}
	return i18n.T("Vollbild")
}

func (a *App) cycleWinStart(dir int) tea.Cmd {
	modes := []string{winstate.StartFull, winstate.StartMax, winstate.StartLast}
	cur := -1
	for i, m := range modes {
		if m == a.winStart() {
			cur = i
		}
	}
	m := modes[cycleIndex(cur, len(modes), dir)]
	if err := winstate.SetStart(a.st.Dir, m); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	a.flash(i18n.T("Fenster beim Start: ") + winStartLabel(m) + i18n.T(" (F11 schaltet Vollbild um)"))
	return nil
}
