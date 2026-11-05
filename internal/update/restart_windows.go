//go:build windows

package update

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// Restart startet die neue .exe im selben Konsolenfenster und wartet auf sie.
// Windows kennt kein exec(); würde der alte Prozess sofort enden, bekäme die
// aufrufende Shell (cmd/PowerShell) die Konsole zurück und stritte sich mit
// sshTermy um die Tastatur. Der alte Prozess (jetzt sshtermy.exe.old) tut
// darum nichts mehr außer warten und gibt am Ende den Exit-Code weiter.
// Kehrt nur bei einem Fehler zurück.
func Restart() error {
	exe, err := Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt) // Strg+C gehört dem neuen Prozess
	if err := cmd.Start(); err != nil {
		return err
	}
	err = cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		os.Exit(ee.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
