package ui

import (
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/localsh"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Lokale Shell: ein Tab mit der Shell dieses Rechners (PowerShell unter
// Windows über ConPTY, zsh/bash unter macOS/Linux über ein PTY). Sie läuft
// wie Telnet/Konsole ohne SSH-Client (s.client == nil), bekommt aber ein
// Systemprofil aus dem eigenen Rechner, damit KI, Gefahren-Check und
// Agent-Stufen den richtigen Befehlssatz (Windows/Unix) kennen.

// localQuickID: Kennung der Ad-hoc-Shell (Palette, neuer Tab, Alt+L).
const localQuickID = "quick:local"

// localConn: Ad-hoc-Verbindung für die Standard-Shell (oder eine bestimmte).
func localConn(shell string) domain.Connection {
	alias := i18n.T("Lokal")
	id := localQuickID
	if shell != "" {
		alias = i18n.T("Lokal") + " · " + localsh.ShortName(shell)
		id += ":" + shell
	}
	return domain.Connection{ID: id, Alias: alias, Protocol: domain.ProtoLocal, LocalShell: shell}
}

// openLocal: neuer Tab mit der lokalen Shell (immer ein neuer, mehrere
// lokale Tabs nebeneinander sind normal).
func (a *App) openLocal(shell string) tea.Cmd {
	return a.connect(localConn(shell))
}

func (s *session) isLocal() bool { return s.conn.Protocol == domain.ProtoLocal }

// localShellName: Name der laufenden (oder eingestellten) Shell.
func (s *session) localShellName() string {
	if st, ok := s.shell.(*localsh.Stream); ok {
		return st.Name()
	}
	return localShellLabel(s.conn.LocalShell)
}

func localShellLabel(shell string) string {
	if strings.TrimSpace(shell) == "" {
		shell = localsh.Default()
	}
	return localsh.ShortName(shell)
}

// localProfile: Systemprofil dieses Rechners für die gewählte Shell.
func localProfile(shellPath string) domain.SystemProfile {
	if strings.TrimSpace(shellPath) == "" {
		shellPath = localsh.Default()
	}
	p := domain.SystemProfile{Dialect: localsh.Dialect(runtime.GOOS, shellPath), Shell: shellPath}
	switch runtime.GOOS {
	case "windows":
		p.Family, p.OS = domain.FamilyWindows, "Windows"
	case "darwin":
		p.Family, p.OS = domain.FamilyMacOS, "macOS"
	case "linux":
		p.Family, p.OS = domain.FamilyLinux, "Linux"
	default:
		p.Family, p.OS = domain.FamilyBSD, runtime.GOOS
	}
	if p.Dialect == "posix" && runtime.GOOS == "windows" {
		p.OS = "Windows (WSL/Git-Bash)"
	}
	p.OS += " · " + i18n.T("dieser Rechner")
	return p
}

// localWindowsShell: PowerShell oder cmd (Regeln für Windows statt Unix).
func localWindowsShell(p domain.SystemProfile) bool {
	return p.Dialect == "powershell" || p.Dialect == "cmd"
}

// agentFS: Dateisystem, in dem der Agent seine Skripte ablegt (SFTP über
// SSH, bei der lokalen Shell die eigene Platte). close räumt auf.
// Aufruf im Hintergrund, daher mit vorher gelesenem Zustand (agentFSOf).
func agentFS(local bool, cl *sshx.Client) (fs sshx.FS, closeFn func(), err error) {
	if local {
		return sshx.LocalFS{}, func() {}, nil
	}
	if cl == nil {
		return nil, nil, errNoFiles
	}
	r, err := cl.OpenSFTP()
	if err != nil {
		return nil, nil, err
	}
	return r, func() { _ = r.Close() }, nil
}

func (s *session) agentFSOf() (bool, *sshx.Client) { return s.isLocal(), s.client }

var errNoFiles = i18n.NewError("Dateien gehen hier nicht")

// localPalette: Einträge „Lokale Shell öffnen“ in der Befehlspalette.
func (a *App) localPalette() []palItem {
	return []palItem{{group: pgConnect, icon: ic("command"), label: i18n.T("Lokale Shell öffnen"),
		hint: a.keys.label(actLocal), keys: "local lokal shell terminal powershell pwsh cmd zsh bash",
		run: func() tea.Cmd { return a.openLocal("") }}}
}

// parseQuickLocal: „lokal“, „local“, „shell“ oder ein Shell-Name („pwsh“,
// „cmd“, „zsh“, „wsl“ …) in der Palette.
func parseQuickLocal(q string) (domain.Connection, bool) {
	f := strings.Fields(strings.TrimSpace(q))
	if len(f) == 0 {
		return domain.Connection{}, false
	}
	switch w := strings.ToLower(f[0]); {
	case len(f) == 1 && (w == "lokal" || w == "local" || w == "shell" || w == "localhost"):
		return localConn(""), true
	case knownShell(w):
		return localConn(strings.TrimSpace(q)), true
	}
	return domain.Connection{}, false
}

func knownShell(w string) bool {
	switch strings.TrimSuffix(w, ".exe") {
	case "pwsh", "powershell", "cmd", "wsl", "zsh", "bash", "fish", "sh", "nu", "tcsh", "ksh":
		return true
	}
	return false
}

// quickLocalItems: Palette-Einträge für eine getippte Shell.
func (a *App) quickLocalItems(c domain.Connection) []palItem {
	return []palItem{
		{group: pgConnect, icon: ic("command"), label: i18n.T("Lokale Shell öffnen: ") + localShellLabel(c.LocalShell), hint: i18n.T("dieser Rechner"),
			run: func() tea.Cmd { return a.connect(c) }},
		{group: pgConnect, icon: ic("new"), label: i18n.T("Als Verbindung speichern: ") + c.Alias, hint: i18n.T("anlegen"),
			run: func() tea.Cmd {
				n := c
				n.ID = ""
				return a.openConnForm(n)
			}},
	}
}
