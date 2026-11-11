package ui

// 7. Zielfenster-Schutz: Bevor sshTermy einen Befehl in einem Fenster des
// Vorgangs vortippt (oder der Agent dort etwas Lesendes ausführt), prüft es,
// ob das Fenster noch das System ist, mit dem geplant wurde, und ob es
// bereit ist. Schlägt eine Prüfung fehl, wird nichts eingefügt.

import (
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

type vgCheck struct {
	ok   bool
	text string
}

// failed: hat mindestens eine Prüfung nicht bestanden?
func failedChecks(cs []vgCheck) bool {
	for _, c := range cs {
		if !c.ok {
			return true
		}
	}
	return false
}

var (
	promptUserHost = regexp.MustCompile(`([A-Za-z0-9_][A-Za-z0-9._-]*)@([A-Za-z0-9][A-Za-z0-9._-]*)`)
	promptAngle    = regexp.MustCompile(`^\s*<([A-Za-z0-9][A-Za-z0-9._-]*)>`) // Huawei/H3C
	promptDevice   = regexp.MustCompile(`^\s*\[?([A-Za-z0-9][A-Za-z0-9._-]*)(?:\([^)]*\))?\]?\s*[#>]`)
)

// promptHost: Rechnername aus einer Prompt-Zeile („user@web01:~$“,
// „core-sw1(config)#“), klein und ohne Domain. "" = nicht erkennbar.
func promptHost(line string) string {
	h := ""
	if m := promptUserHost.FindStringSubmatch(line); m != nil {
		h = m[2]
	} else if m := promptAngle.FindStringSubmatch(line); m != nil {
		h = m[1]
	} else if m := promptDevice.FindStringSubmatch(line); m != nil && !strings.HasPrefix(strings.TrimSpace(line), "PS ") {
		h = m[1]
	}
	h = strings.ToLower(strings.TrimRight(h, ".:-_"))
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	return h
}

// Befehle, die auf einem anderen System zu Hause sind. Die Liste ist
// absichtlich kurz: sie soll grobe Verwechslungen fangen (Linux-Befehl ins
// Switch-Fenster), nicht jede Syntax prüfen.
var (
	unixFirst = wordSet("sudo", "systemctl", "apt", "apt-get", "yum", "dnf", "zypper", "cat", "grep", "echo", "ls", "cd",
		"tail", "head", "journalctl", "docker", "podman", "psql", "mysql", "curl", "wget", "rm", "cp", "mv", "chmod",
		"chown", "vim", "vi", "nano", "bash", "sh", "sed", "awk", "find", "networkctl", "nmcli", "service", "kubectl")
	netFirst = wordSet("show", "configure", "conf", "interface", "int", "vlan", "end", "write", "display", "system-view",
		"undo", "commit", "tagged", "untagged")
	winOnly = wordSet("get-service", "get-process", "set-service", "restart-service", "get-netipaddress", "ipconfig", "netsh", "get-eventlog", "get-winevent")
)

func wordSet(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// fitsSystem: passt der Befehl grob zum System des Fensters?
func fitsSystem(cmd, kind string) (bool, string) {
	f := strings.Fields(strings.ToLower(strings.TrimSpace(cmd)))
	if len(f) == 0 {
		return true, ""
	}
	first := f[0]
	switch kind {
	case safety.Network:
		if unixFirst[first] || (first == "ip" && len(f) > 1 && strings.HasPrefix(f[1], "-")) || winOnly[first] {
			return false, i18n.T("Linux-/Windows-Befehl, das Fenster ist ein Netzwerkgerät")
		}
	case "windows":
		if first == "sudo" || first == "systemctl" || first == "apt" || first == "apt-get" || first == "yum" || first == "journalctl" || netFirst[first] && first != "end" {
			return false, i18n.T("Befehl passt nicht zu Windows")
		}
	default:
		if netFirst[first] && first != "end" && first != "write" || winOnly[first] {
			return false, i18n.T("Switch-/Windows-Befehl, das Fenster ist ein Linux/Unix-System")
		}
	}
	return true, ""
}

// vgGuard: alle Prüfungen für das Vortippen von cmd in Sitzung s.
func (a *App) vgGuard(s *session, cmd string) []vgCheck {
	g := a.vg
	n := itoa(a.winNo(s))
	var out []vgCheck
	add := func(ok bool, text string) { out = append(out, vgCheck{ok, text}) }
	m := g.member(s.id)
	if m == nil {
		add(false, i18n.F("Fenster %s gehört nicht zum Vorgang (Alt+V)", n))
		return out
	}
	if s.state != stConnected || s.term == nil {
		add(false, i18n.F("Fenster %s ist nicht verbunden", n))
		return out
	}
	add(true, i18n.F("Fenster %s ist offen und verbunden", n))
	// gleiches System: Host-Key, Prompt
	if s.client != nil && m.hostKey != "" {
		if s.client.HostKey != m.hostKey {
			add(false, i18n.T("Host-Key ist ein anderer als beim Verbinden des Vorgangs"))
		} else if !m.connAt.IsZero() && !s.connectedAt.Equal(m.connAt) {
			add(true, i18n.T("neu verbunden, Host-Key ist derselbe"))
		} else {
			add(true, i18n.T("gleiche Verbindung, gleicher Host-Key"))
		}
	}
	typed, atPrompt := s.term.TypedInput()
	now := ""
	if atPrompt {
		now = promptHost(s.term.CursorLine())
	}
	switch {
	case m.prompt != "" && now != "" && now != m.prompt:
		add(false, i18n.F("Prompt zeigt „%s“ statt „%s“ (anderer Rechner? su/ssh weiter?)", now, m.prompt))
	case m.prompt != "" && now == m.prompt:
		add(true, i18n.F("Prompt passt (%s)", now))
	}
	switch {
	case s.term.AltScreen():
		add(false, i18n.T("Vollbild-Programm läuft (vim, top, less …)"))
	case agent.LooksLikeSecretPrompt(s.term.CursorLine()):
		add(false, i18n.T("Passwortabfrage am Cursor"))
	case !atPrompt:
		add(false, i18n.T("kein Prompt zu sehen (läuft noch ein Befehl?)"))
	case strings.TrimSpace(typed) != "":
		add(false, i18n.T("am Prompt steht schon Text (erst löschen oder ausführen)"))
	default:
		add(true, i18n.T("Prompt ist frei, keine Passwortabfrage, kein Vollbild-Programm"))
	}
	if s.goal.active {
		add(false, i18n.T("in diesem Fenster läuft gerade der Ziel-/Agent-Modus"))
	}
	if s.detached() {
		add(true, i18n.T("Fenster läuft abgetrennt (eigenes Terminalfenster)"))
	}
	if ok, why := fitsSystem(cmd, a.safetyKind(s)); !ok {
		add(false, why)
	} else {
		add(true, i18n.T("Befehl passt zum System des Fensters"))
	}
	return out
}

// vgGuardModal: Halt-Dialog mit allen Prüfungen.
func (a *App) vgGuardModal(s *session, cmd string, cs []vgCheck) {
	st := a.s
	title := i18n.F("Halt – Fenster %d (%s) ist gerade nicht bereit", a.winNo(s), s.tabName())
	for _, c := range cs {
		if !c.ok && strings.Contains(c.text, "Prompt") || !c.ok && strings.Contains(c.text, "Host-Key") {
			title = i18n.F("Halt – Fenster %d ist gerade nicht %s", a.winNo(s), s.tabName())
		}
	}
	lines := []string{
		i18n.T("Vorgesehen für ") + a.winBadge(s, true) + ":",
		"  " + st.Accent.Render(trunc(cmd, 90)),
		"",
		st.Muted.Render(i18n.T("Vor jedem Vortippen prüft sshTermy:")),
	}
	for _, c := range cs {
		if c.ok {
			lines = append(lines, "  "+st.OK.Render("✓")+" "+c.text)
		} else {
			lines = append(lines, "  "+st.Danger.Render("×")+" "+c.text)
		}
	}
	lines = append(lines, "", st.Text.Bold(true).Render(i18n.T("Es wurde nichts eingefügt.")), "",
		st.Muted.Render(i18n.T("Prüfe das Fenster selbst. Danach im Vorgang ⏎ für einen neuen Versuch.")))
	m := textModal(title, lines)
	m.styled = true
	a.modal = m
}
