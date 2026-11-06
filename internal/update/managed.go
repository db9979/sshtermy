package update

import (
	"os"
	"runtime"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Managed: Befehl zum Aktualisieren, wenn sshTermy über eine Paketquelle
// installiert wurde (Homebrew, apt). Dann tauscht sshTermy sich nicht selbst
// aus: apt hält /usr/bin schreibgeschützt, und Homebrew würde eine fremd
// ausgetauschte Datei im Cellar nicht kennen. Leer = selbst aktualisieren.
// winget (portable) bleibt beim Selbst-Update, der Ordner gehört dem Nutzer.
func Managed() string {
	exe, err := Executable()
	if err != nil {
		return ""
	}
	return managedBy(exe, runtime.GOOS, fileExists)
}

func managedBy(exe, goos string, exists func(string) bool) string {
	p := strings.ReplaceAll(exe, "\\", "/")
	if goos != "windows" && (strings.Contains(p, "/Cellar/") || strings.HasPrefix(p, "/opt/homebrew/") || strings.HasPrefix(p, "/home/linuxbrew/")) {
		return "brew upgrade sshtermy"
	}
	if goos == "linux" && p == "/usr/bin/sshtermy" && exists("/var/lib/dpkg/info/sshtermy.list") {
		return "sudo apt update && sudo apt install --only-upgrade sshtermy"
	}
	return ""
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// ManagedError: Fehlermeldung für Apply bei Paketquellen-Installation.
type ManagedError struct{ Cmd string }

func (e ManagedError) Error() string {
	return i18n.T("über eine Paketquelle installiert, aktualisieren mit: ") + e.Cmd
}
