package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Sprache der Oberfläche (Deutsch/Englisch). Sie steht in den Einstellungen
// (geht mit dem Sync mit) und zusätzlich unverschlüsselt im Datenordner,
// damit schon der Entsperr-Bildschirm sie kennt.

// applyLang übernimmt die Sprache aus den Einstellungen.
func (a *App) applyLang() {
	l := a.st.Settings().Language
	if l == "" {
		return // Store noch gesperrt: die Sprache vom Start (Datei/System) bleibt
	}
	if l != i18n.EN {
		l = i18n.DE
	}
	if l != i18n.Lang() {
		i18n.Set(l)
	}
	_ = i18n.Save(a.st.Dir, l)
}

// setLang wählt die Sprache und speichert sie.
func (a *App) setLang(l string) tea.Cmd {
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Language = l }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.applyLang()
	a.flash(i18n.T("Sprache: Deutsch"))
	return nil
}

// toggleLang wechselt zwischen Deutsch und Englisch.
func (a *App) toggleLang() tea.Cmd {
	if i18n.English() {
		return a.setLang(i18n.DE)
	}
	return a.setLang(i18n.EN)
}

// langLabel: Eintrag für Palette und Über-Dialog, in beiden Sprachen lesbar.
func langLabel() string {
	if i18n.English() {
		return "Language: English → Deutsch"
	}
	return "Sprache: Deutsch → English"
}

// yesKey: Taste für „ja“ in Dialogen (j und y gehen immer, angezeigt wird
// die der Sprache).
func yesKey() string {
	if i18n.English() {
		return "y"
	}
	return "j"
}

// langTarget: Beschriftung der Sprachtaste (die Sprache, zu der sie wechselt).
func langTarget() string {
	if i18n.English() {
		return "Deutsch"
	}
	return "English"
}
