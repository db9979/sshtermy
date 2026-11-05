// Package version: Programmname, Version und Credits an genau einer Stelle.
//
// Version, Commit und Date setzt der Build per -ldflags (siehe Makefile und
// .github/workflows/release.yml). Die Version zählt bei jedem Push auf main
// automatisch hoch: MAJOR.MINOR aus der Datei VERSION, PATCH = Anzahl der
// Commits (git rev-list --count HEAD). Lokale Builds ohne ldflags heißen "dev".
package version

import (
	"fmt"
	"runtime"
)

const (
	Name   = "sshTermy"
	Idea   = "Idee: Dominik Bornhäußer"
	Repo   = "db9979/sshtermy"
	Slogan = "SSH-Terminal mit KI-Copilot"
	// Coffee: Buy-Me-a-Coffee-Seite von Dominik (dieselbe wie bei speech-on-dgx-spark).
	Coffee = "https://buymeacoffee.com/zquu1xu570"
)

var (
	Version = "dev"     // z. B. "1.0.42"
	Commit  = "none"    // kurzer Git-Hash
	Date    = "unknown" // Build-Zeit (UTC, RFC 3339)
	// UpdatePubKey: Ed25519-Public-Key (hex) zum Prüfen der Release-Signatur.
	// Wird im Release-Build gesetzt; leer = Updates nur mit SHA-256-Prüfung.
	UpdatePubKey = ""
)

// Short: "sshTermy v1.0.42"
func Short() string { return fmt.Sprintf("%s v%s", Name, Version) }

// Long: für "sshtermy --version" und den Über-Dialog.
func Long() string {
	return fmt.Sprintf("%s v%s (%s, %s) %s/%s\n%s\n%s · https://github.com/%s",
		Name, Version, Commit, Date, runtime.GOOS, runtime.GOARCH, Slogan, Idea, Repo)
}
