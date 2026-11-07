package ui

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Absturzbericht (internal/crash): Ist sshTermy beim letzten Mal hart
// abgestürzt, entsteht der Bericht nach dem Entsperren, weil erst dann die
// Passwörter aus dem Store bekannt sind, die herausgefiltert werden.

// checkCrash: nach dem Entsperren aufrufen.
func (a *App) checkCrash() {
	raw := crash.Pending(a.st.Dir)
	if raw == "" {
		return
	}
	r, err := crash.Write(a.st.Dir, crash.KindFatal, raw, a.st.Sensitive())
	if err != nil {
		a.flash(i18n.T("Absturzbericht ließ sich nicht schreiben: ") + err.Error())
		return
	}
	crash.Resolve(a.st.Dir)
	a.modal = crashModal(r)
}

func crashModal(r *crash.Report) *modal {
	body := i18n.T("sshTermy ist beim letzten Mal abgestürzt. Das tut mir leid.\n\n") +
		i18n.T("Meldung: ") + trunc(strings.TrimPrefix(r.Title, "Crash: "), 70) + "\n" +
		i18n.T("Bericht: ") + r.Path + "\n\n" +
		i18n.T("Im Bericht stehen Version, System und die Stelle im Programm. Passwörter,\n") +
		i18n.T("Keys, Hosts und IP-Adressen sind entfernt. Beim Melden zeigt der Browser\n") +
		i18n.T("alles vor dem Absenden.")
	return &modal{kind: mCrash, title: i18n.T("Absturzbericht"), body: body, crash: r}
}

func (a *App) crashKey(key string) tea.Cmd {
	r := a.modal.crash
	switch key {
	case "g":
		a.modal = nil
		return a.openURL(crash.IssueURL(r))
	case "o":
		if err := sshx.OpenBrowser(r.Path); err != nil {
			a.flash(i18n.T("Ordner: ") + filepath.Dir(r.Path))
		}
	case "esc", "q", "enter":
		a.modal = nil
	}
	return nil
}

// CrashTest löst zum Ausprobieren einen Absturz aus (sshtermy -crash-test).
// ui: Panik beim nächsten Tick in Update, cmd: Panik in einem Kommando,
// fatal: Panik in einer eigenen Goroutine (Prozess endet sofort).
func (a *App) CrashTest(kind string) { a.crashTest = kind }

type crashTestMsg struct{}

func (a *App) crashTestTick() tea.Cmd {
	kind := a.crashTest
	if kind == "" || a.scr == scrUnlock {
		return nil
	}
	a.crashTest = ""
	switch kind {
	case "cmd":
		return func() tea.Msg { panic("sshTermy crash test (cmd) password=hunter2") }
	case "fatal":
		go func() { panic("sshTermy crash test (fatal) password=hunter2") }()
		return nil
	}
	return func() tea.Msg { return crashTestMsg{} }
}
