package ui

// F5 folgt dem Ordner des Terminals: Meldet die Shell ihren Ordner (OSC 7,
// wie bash/zsh/fish mit Shell-Integration oder vte.sh, oder VS-Codes
// 633;P;Cwd), öffnet die Serverseite dort. Sonst liest sshTermy den Ordner
// aus der Eingabeaufforderung, wenn sie wie üblich „user@host:/pfad$“ oder
// „…:~/pfad#“ aussieht. Klappt beides nicht oder gibt es den Ordner nicht,
// bleibt es beim Home-Ordner.

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/sshx"
)

// promptCwd: „user@host:/var/log$ “, „root@h:~/x# “, „user@h ~/x % “ (zsh).
var promptCwd = regexp.MustCompile(`^(?:\([^)]*\)\s*)?[\w.-]+@[\w.-]+[: ]((?:~|/)[^\s$#%>]*)\s*[$#%>]\s?`)

// cwdFromPrompt: Ordner aus der Prompt-Zeile ("" = nicht erkannt).
func cwdFromPrompt(line string) string {
	m := promptCwd.FindStringSubmatch(strings.TrimLeft(line, " "))
	if m == nil {
		return ""
	}
	return m[1]
}

// termCwd: Ordner des Terminals, aufgelöst gegen das Home des Servers.
func termCwd(s *session, fs sshx.FS) string {
	if s == nil || s.term == nil || fs == nil {
		return ""
	}
	p := s.term.Cwd()
	if p == "" && s.term.AtIdlePrompt() {
		p = cwdFromPrompt(s.term.CursorLine())
	}
	switch {
	case p == "":
		return ""
	case p == "~":
		return fs.Home()
	case strings.HasPrefix(p, "~/"):
		return fs.Join(fs.Home(), p[2:])
	}
	if _, local := fs.(sshx.LocalFS); local {
		p = filepath.FromSlash(p) // Windows: OSC 7 schickt C:/Users/…
	}
	return p
}

// startPath: Ordner des Terminals, falls lesbar, sonst Home.
func startPath(s *session, fs sshx.FS, hidden bool) *pane {
	if p := termCwd(s, fs); p != "" && p != fs.Home() {
		pn := &pane{fs: fs, path: p}
		pn.load(hidden)
		if pn.err == "" {
			return pn
		}
	}
	pn := &pane{fs: fs, path: fs.Home()}
	pn.load(hidden)
	return pn
}
