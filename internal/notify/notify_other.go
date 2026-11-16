//go:build !windows && !darwin

package notify

import (
	"os/exec"
	"strings"
)

// notifySend ist in Tests ersetzbar.
var notifySend = func(args ...string) ([]byte, error) { return exec.Command("notify-send", args...).Output() }

func send(title, body, action string, click func()) error {
	if click == nil {
		_, err := notifySend("--app-name=sshTermy", title, body)
		return err
	}
	// --wait hält, bis die Meldung angeklickt oder geschlossen wird, und
	// schreibt dann den Namen der Aktion. Älteres libnotify kennt --action
	// nicht: dann ohne Klick.
	out, err := notifySend("--app-name=sshTermy", "--action=default="+action, "--wait", title, body)
	if err != nil {
		_, err = notifySend("--app-name=sshTermy", title, body)
		return err
	}
	if strings.TrimSpace(string(out)) == "default" {
		click()
	}
	return nil
}
