//go:build !windows && !darwin

package notify

import "os/exec"

func send(title, body string) error {
	return exec.Command("notify-send", "--app-name=sshTermy", title, body).Run()
}
