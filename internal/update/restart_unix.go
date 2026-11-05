//go:build !windows

package update

import (
	"os"
	"syscall"
)

// Restart ersetzt den laufenden Prozess durch die (neue) Programmdatei mit
// denselben Argumenten. Kehrt nur bei einem Fehler zurück.
func Restart() error {
	exe, err := Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, append([]string{exe}, os.Args[1:]...), os.Environ())
}
