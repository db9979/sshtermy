package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// Bildschirmsperre
//
// Nach Leerlauf (Einstellung „Automatisch sperren“) oder per Palette
// („Jetzt sperren“) verdeckt sshTermy alles und fragt das Master-Passwort.
// Ohne offene Sitzungen wird wie bisher der ganze Store gesperrt. Mit
// offenen Sitzungen bleibt der Store offen, damit Shells, Tunnel,
// Protokolle und Auto-Reconnect weiterlaufen; nur Bildschirm und Tastatur
// sind gesperrt. Benachrichtigungen nennen dann keinen Befehl.

type lockState struct {
	in    textinput.Model
	err   string
	busy  bool
	tries int
}

type lockDoneMsg struct{ err error }

// lockChoices: Minuten bis zur automatischen Sperre (0 = aus).
var lockChoices = []int{0, 5, 15, 30, 60}

func lockLabel(min int) string {
	if min <= 0 {
		return i18n.T("aus")
	}
	return fmt.Sprintf(i18n.T("nach %d min"), min)
}

// cycleLockAfter: nächste Stufe der automatischen Sperre.
func (a *App) cycleLockAfter() tea.Cmd {
	cur := a.st.Settings().LockAfterMin
	next := lockChoices[0]
	for i, c := range lockChoices {
		if c == cur {
			next = lockChoices[(i+1)%len(lockChoices)]
			break
		}
		if c > cur {
			next = c
			break
		}
	}
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.LockAfterMin = next })
	return nil
}

// lockNow: per Palette sperren.
func (a *App) lockNow() tea.Cmd {
	if len(a.sessions) == 0 {
		a.closeBook()
		a.st.Lock()
		a.unlock = newUnlock(a.st)
		a.scr = scrUnlock
		a.pal, a.modal, a.pick = nil, nil, nil
		return nil
	}
	return a.lockScreen()
}

// lockScreen: Bildschirm sperren, Sitzungen laufen weiter.
func (a *App) lockScreen() tea.Cmd {
	if a.lock != nil {
		return nil
	}
	in := newInput("", true)
	in.Placeholder = i18n.T("Master-Passwort")
	a.lock = &lockState{in: in}
	a.pal = nil // Palette zu; Rückfragen (Host-Key, Passwort) bleiben offen
	a.clearToasts()
	return a.lock.in.Focus()
}

// lockMsg fängt Tastatur und Einfügen ab, solange gesperrt ist.
func (a *App) lockMsg(msg tea.Msg) (tea.Cmd, bool) {
	if m, ok := msg.(lockDoneMsg); ok {
		if a.lock == nil {
			return nil, true
		}
		a.lock.busy = false
		if m.err != nil {
			a.lock.tries++
			a.lock.err = i18n.T("Falsches Passwort")
			a.lock.in.SetValue("")
			return nil, true
		}
		a.lock = nil
		a.lastKey = time.Now() // sonst sperrt der nächste Tick gleich wieder
		return nil, true
	}
	l := a.lock
	if l == nil {
		return nil, false
	}
	switch m := msg.(type) {
	case tea.KeyPressMsg:
		if l.busy {
			return nil, true
		}
		switch m.String() {
		case "enter":
			pw := []byte(l.in.Value())
			if len(pw) == 0 {
				return nil, true
			}
			l.busy, l.err = true, ""
			v := a.st.V
			return func() tea.Msg { return lockDoneMsg{v.CheckPassword(pw)} }, true
		case "esc":
			l.in.SetValue("")
			return nil, true
		}
		var cmd tea.Cmd
		l.in, cmd = l.in.Update(msg)
		return cmd, true
	case tea.PasteMsg:
		var cmd tea.Cmd
		l.in, cmd = l.in.Update(msg)
		return cmd, true
	case tea.KeyReleaseMsg:
		return nil, true
	}
	return nil, false
}

// viewLock: nur Logo, Hinweis und Passwortfeld; vom Inhalt ist nichts zu sehen.
func (a *App) viewLock() string {
	l := a.lock
	s := a.s
	var b strings.Builder
	b.WriteString(s.Accent.Render(logoArt) + "\n\n")
	b.WriteString(s.Text.Render(i18n.T("Bildschirm gesperrt.")) + "\n")
	running := 0
	for _, x := range a.sessions {
		if x.state == stConnected || x.state == stReconnect {
			running++
		}
	}
	b.WriteString(s.Muted.Render(plural(running, i18n.T("Sitzung läuft"), i18n.T("Sitzungen laufen"))+i18n.T(" im Hintergrund weiter.")) + "\n\n")
	b.WriteString(s.Label.Render(i18n.T("Master-Passwort")) + l.in.View() + "\n")
	if l.busy {
		b.WriteString("\n" + s.AI.Render(i18n.T("⠿ Passwort wird geprüft …")))
	}
	if l.err != "" {
		msg := l.err
		if l.tries > 1 {
			msg += fmt.Sprintf(" (%d×)", l.tries)
		}
		b.WriteString("\n" + s.Danger.Render("× "+msg))
	}
	keys := [][2]string{{"⏎", i18n.T("entsperren")}, {"Esc", i18n.T("Feld leeren")}}
	box := s.dialog(dialog{icon: "◆", title: version.Name, sub: i18n.T("gesperrt"), body: b.String(), keys: keys, width: min(64, a.w-8), noClose: true}, a.w)
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, box)
}
