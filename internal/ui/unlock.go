package ui

import (
	"runtime"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/vault"
	"github.com/db9979/sshtermy/internal/version"
)

// unlockState: Ersteinrichtung oder Entsperren.
type unlockState struct {
	setup   bool
	pw, pw2 textinput.Model
	focus   int
	dpapi   bool
	err     string
	busy    bool
}

func newUnlock(st *store.Store) unlockState {
	u := unlockState{setup: !st.V.Exists(), pw: newInput("", true), pw2: newInput("", true), dpapi: vault.DPAPIAvailable()}
	u.pw.Placeholder = i18n.T("Master-Passwort")
	u.pw2.Placeholder = i18n.T("wiederholen")
	u.pw.Focus()
	return u
}

type unlockedMsg struct{ err error }

// init: unter Windows zuerst still per DPAPI entsperren.
func (u unlockState) init() tea.Cmd { return nil }

func (a *App) tryDPAPI() tea.Cmd {
	if a.unlock.setup || !a.st.V.HasDPAPI() {
		return nil
	}
	st := a.st
	return func() tea.Msg {
		if err := st.V.UnlockDPAPI(); err != nil {
			return unlockedMsg{err}
		}
		return unlockedMsg{st.AfterUnlock()}
	}
}

func (a *App) updateUnlock(msg tea.Msg) tea.Cmd {
	u := &a.unlock
	switch m := msg.(type) {
	case unlockedMsg:
		u.busy = false
		if m.err != nil {
			if !strings.Contains(m.err.Error(), "DPAPI") {
				u.err = m.err.Error()
			}
			return nil
		}
		a.scr = scrManager
		if u.setup {
			// Neuer Store: die Sprache des Entsperr-Bildschirms übernehmen.
			_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Language = i18n.Lang() })
		}
		a.lookSig = ""
		a.applyLook()
		a.mgr = newManager()
		a.showWhatsNew(u.setup)
		a.maybeTour(u.setup)
		return tea.Batch(checkUpdate(), a.runSync())
	case tea.KeyPressMsg:
		if u.busy {
			return nil
		}
		switch m.String() {
		case "esc":
			return a.quit()
		case "tab", "down", "up", "shift+tab":
			if u.setup {
				u.focus = (u.focus + 1) % 3
				u.pw.Blur()
				u.pw2.Blur()
				if u.focus == 0 {
					return u.pw.Focus()
				} else if u.focus == 1 {
					return u.pw2.Focus()
				}
			}
			return nil
		case "space":
			if u.setup && u.focus == 2 {
				u.dpapi = !u.dpapi && vault.DPAPIAvailable()
				return nil
			}
		case "enter":
			pw := []byte(u.pw.Value())
			st := a.st
			if u.setup {
				if u.focus == 0 {
					u.focus = 1
					u.pw.Blur()
					return u.pw2.Focus()
				}
				if u.pw.Value() != u.pw2.Value() {
					u.err = i18n.T("Passwörter stimmen nicht überein")
					return nil
				}
				dp := u.dpapi
				u.busy, u.err = true, ""
				return func() tea.Msg { return unlockedMsg{st.Init(pw, dp)} }
			}
			u.busy, u.err = true, ""
			return func() tea.Msg {
				if err := st.V.UnlockPassword(pw); err != nil {
					return unlockedMsg{err}
				}
				return unlockedMsg{st.AfterUnlock()}
			}
		}
	}
	var cmd tea.Cmd
	if u.focus == 0 {
		u.pw, cmd = u.pw.Update(msg)
	} else if u.focus == 1 {
		u.pw2, cmd = u.pw2.Update(msg)
	}
	return cmd
}

const logoArt = `
 ┌─┐┌─┐┬ ┬╔╦╗┌─┐┬─┐┌┬┐┬ ┬
 └─┐└─┐├─┤ ║ ├┤ ├┬┘│││└┬┘
 └─┘└─┘┴ ┴ ╩ └─┘┴└─┴ ┴ ┴ `

func (a *App) viewUnlock() string {
	u := a.unlock
	s := a.s
	var b strings.Builder
	b.WriteString(s.Accent.Render(logoArt) + "\n")
	b.WriteString(s.Muted.Render(" "+i18n.T(version.Slogan)+" · v"+version.Version) + "\n")
	b.WriteString(s.Muted.Italic(true).Render(" "+i18n.T(version.Idea)) + "\n\n")
	if u.setup {
		b.WriteString(s.Text.Render(i18n.T("Willkommen! Lege ein Master-Passwort für deinen verschlüsselten Store an.")) + "\n")
		b.WriteString(s.Muted.Render(i18n.T("Es wird nie gespeichert. Argon2id + AES-256-GCM schützen Passwörter, Keys und API-Keys.")) + "\n\n")
		b.WriteString(s.Label.Render(i18n.T("Master-Passwort")) + u.pw.View() + "\n")
		b.WriteString(s.Label.Render(i18n.T("Wiederholen")) + u.pw2.View() + "\n")
		mark := "○"
		if u.dpapi {
			mark = s.OK.Render("●")
		}
		lbl := i18n.T("Mit Windows-Anmeldung entsperren (DPAPI)")
		if !vault.DPAPIAvailable() {
			lbl = i18n.T("DPAPI nur unter Windows (") + runtime.GOOS + i18n.T(": nur Master-Passwort)")
		}
		focus := "  "
		if u.focus == 2 {
			focus = s.Accent.Render("▌ ")
		}
		b.WriteString(focus + mark + " " + s.Text.Render(lbl) + "\n")
	} else {
		b.WriteString(s.Text.Render(i18n.T("Store gesperrt.")) + "\n\n")
		b.WriteString(s.Label.Render(i18n.T("Master-Passwort")) + u.pw.View() + "\n")
	}
	if u.busy {
		b.WriteString("\n" + s.AI.Render(i18n.T("⠿ Schlüssel wird abgeleitet (Argon2id) …")))
	}
	if u.err != "" {
		b.WriteString("\n" + s.Danger.Render("✗ "+u.err))
	}
	b.WriteString("\n\n" + s.Muted.Render(i18n.T("⏎ weiter · Tab Feld · Esc beenden")))
	box := s.Modal.Width(min(86, a.w-4)).Render(b.String())
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, box)
}
