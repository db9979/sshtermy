package ui

import (
	"path"
	"strings"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

// docker exec ohne -it
//
// „docker exec gitea vi /data/app.ini“ startet vi ohne Terminal: vi zeigt
// kurz ein paar Zeilen und endet sofort, danach steht die Antwort auf vis
// Cursor-Abfrage (^[[61;189R) am Prompt. Das passiert in jedem Terminal,
// nicht nur in sshTermy. Weil es fast immer ein Versehen ist, ergänzt
// sshTermy beim Enter das fehlende -it (oder -t / -i) in der Zeile, führt sie
// aber noch nicht aus: ein zweites ⏎ startet den korrigierten Befehl. Wer die
// Zeile wirklich ohne -it will, tippt sie noch einmal; dann geht sie durch.
//
// Gilt für docker/podman/nerdctl exec und run sowie kubectl exec, wenn das
// Programm ein Editor, Pager, Monitor oder eine Shell ohne Befehl ist.

// ttyPrograms: brauchen ein Terminal, um überhaupt zu laufen.
var ttyPrograms = map[string]bool{
	"vi": true, "vim": true, "nvim": true, "view": true, "vim.tiny": true, "vim.basic": true,
	"nano": true, "pico": true, "emacs": true, "mcedit": true, "joe": true, "micro": true, "ne": true, "jed": true,
	"less": true, "more": true, "most": true, "man": true,
	"top": true, "htop": true, "btop": true, "atop": true, "iotop": true, "nmtui": true, "mc": true,
	"tmux": true, "screen": true, "watch": true,
}

// ttyShells: brauchen ein Terminal, wenn sie ohne -c/Skript starten.
var ttyShells = map[string]bool{
	"sh": true, "bash": true, "ash": true, "dash": true, "zsh": true, "fish": true, "ksh": true,
	"/bin/sh": true, "/bin/bash": true, "/bin/ash": true, "/bin/zsh": true,
	"mysql": true, "mariadb": true, "psql": true, "redis-cli": true, "mongosh": true, "mongo": true, "sqlite3": true, "python": true, "python3": true,
}

// dockerValueFlags: Schalter von docker exec/run mit eigenem Wert.
var dockerValueFlags = map[string]bool{
	"-u": true, "--user": true, "-e": true, "--env": true, "-w": true, "--workdir": true,
	"--env-file": true, "--detach-keys": true, "--name": true, "-v": true, "--volume": true,
	"-p": true, "--publish": true, "--network": true, "--entrypoint": true, "-h": true, "--hostname": true,
	"--mount": true, "-l": true, "--label": true, "--platform": true, "--pull": true, "-m": true, "--memory": true,
}

// kubectlValueFlags: Schalter von kubectl exec mit eigenem Wert.
var kubectlValueFlags = map[string]bool{
	"-n": true, "--namespace": true, "-c": true, "--container": true, "--context": true,
	"--kubeconfig": true, "-f": true, "--filename": true, "--pod-running-timeout": true,
}

// ttyFixLine: fehlt dem Befehl -it, obwohl das Programm ein Terminal braucht?
// Ergebnis: die korrigierte Zeile und das Programm, sonst "".
func ttyFixLine(line string) (fixed, prog string) {
	if strings.ContainsAny(line, "|<`") || strings.Contains(line, "$(") {
		return "", "" // Eingabe kommt aus einer Pipe/Datei: -i reicht, -t stört
	}
	f := strings.Fields(line)
	i := 0
	for i < len(f) && (f[i] == "sudo" || f[i] == "doas" || strings.Contains(f[i], "=") && !strings.HasPrefix(f[i], "-")) {
		i++
	}
	if i+1 >= len(f) {
		return "", ""
	}
	tool := path.Base(f[i])
	sub := i + 1
	values := dockerValueFlags
	switch tool {
	case "docker", "podman", "nerdctl":
		if f[sub] == "container" && sub+1 < len(f) {
			sub++
		}
		if f[sub] != "exec" && f[sub] != "run" {
			return "", ""
		}
	case "kubectl", "oc":
		if f[sub] != "exec" {
			return "", ""
		}
		values = kubectlValueFlags
	default:
		return "", ""
	}
	hasI, hasT := false, false
	var rest []string // Container/Image, Programm, Argumente
	for j := sub + 1; j < len(f); j++ {
		w := f[j]
		if len(rest) > 0 || w == "--" {
			if w != "--" || len(rest) > 0 {
				rest = append(rest, w)
			}
			continue
		}
		switch {
		case w == "-i" || w == "--interactive" || w == "--interactive=true":
			hasI = true
		case w == "-t" || w == "--tty" || w == "--tty=true":
			hasT = true
		case w == "-d" || w == "--detach":
			return "", "" // läuft im Hintergrund
		case strings.HasPrefix(w, "--"):
			if values[w] {
				j++
			}
		case strings.HasPrefix(w, "-") && len(w) > 1:
			// zusammengesetzt: -it, -itu 0, -u0
		short:
			for k, r := range w[1:] {
				switch {
				case r == 'i':
					hasI = true
				case r == 't':
					hasT = true
				case r == 'd':
					return "", ""
				case values["-"+string(r)]:
					if k == len(w)-2 {
						j++ // Wert im nächsten Wort
					}
					break short
				}
			}
		default:
			rest = append(rest, w)
		}
	}
	if hasI && hasT {
		return "", ""
	}
	// kubectl: Programm nach „--“, docker: nach dem Container
	if tool == "kubectl" || tool == "oc" {
		k := -1
		for j, w := range f {
			if w == "--" {
				k = j
				break
			}
		}
		if k < 0 || k+1 >= len(f) {
			return "", ""
		}
		rest = append([]string{""}, f[k+1:]...)
	}
	if len(rest) < 2 {
		return "", "" // docker run ohne Befehl: Image-Standard, unbekannt
	}
	p, args := rest[1], rest[2:]
	switch {
	case ttyPrograms[path.Base(p)]:
	case ttyShells[p] || ttyShells[path.Base(p)]:
		if len(args) > 0 {
			return "", "" // sh -c …, bash skript.sh, mysql -e …
		}
	default:
		return "", ""
	}
	flag := "-it"
	switch {
	case hasI:
		flag = "-t"
	case hasT:
		flag = "-i"
	}
	out := append(append(append([]string{}, f[:sub+1]...), flag), f[sub+1:]...)
	return strings.Join(out, " "), path.Base(p)
}

// ttyFlagOf: der ergänzte Schalter (für die Meldung).
func ttyFlagOf(orig, fixed string) string {
	o, f := strings.Fields(orig), strings.Fields(fixed)
	for i := range o {
		if o[i] != f[i] {
			return f[i]
		}
	}
	return f[len(f)-1]
}

// ttyFix: Enter am Prompt. true = Zeile korrigiert, Enter zurückgehalten.
func (a *App) ttyFix(s *session) bool {
	if s.term == nil || s.term.AltScreen() || s.netDevice() || a.safetyKind(s) != "unix" {
		return false
	}
	typed, ok := s.term.TypedInput()
	cmd := strings.TrimSpace(typed)
	if !ok || cmd == "" || agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
		return false
	}
	if s.ttyFixed != "" && s.ttyFixed == cmd {
		s.ttyFixed = "" // bewusst noch einmal ohne -it getippt
		return false
	}
	s.ttyFixed = ""
	fixed, prog := ttyFixLine(cmd)
	if fixed == "" {
		return false
	}
	s.ttyFixed = cmd
	s.term.SendText("\x15" + fixed) // Strg+U: Zeile ersetzen, noch ohne Enter
	a.flash(i18n.F("%s braucht ein Terminal: %s ergänzt. ⏎ führt aus (ohne: Zeile neu tippen)", prog, ttyFlagOf(cmd, fixed)))
	return true
}
