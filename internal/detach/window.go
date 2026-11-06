package detach

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// EnvTerminal: eigener Befehl für das neue Fenster, z. B.
//
//	SSHTERMY_TERMINAL="kitty --title {title} {cmd}"
//
// {cmd} wird durch das sshTermy-Kommando ersetzt (fehlt es, kommt es ans Ende).
const EnvTerminal = "SSHTERMY_TERMINAL"

// ErrNoTerminal: kein Terminalprogramm gefunden, das ein Fenster öffnen kann.
var ErrNoTerminal = errors.New("detach: kein Terminalprogramm gefunden")

// launcher ist ein Kandidat für das neue Fenster.
type launcher struct {
	name string   // Programm (muss im PATH liegen)
	args []string // {title} und {cmd} werden ersetzt
}

// OpenWindow startet argv in einem neuen Terminalfenster.
func OpenWindow(title string, argv []string) error {
	cands := candidates(os.Getenv)
	if t := strings.TrimSpace(os.Getenv(EnvTerminal)); t != "" {
		f := strings.Fields(t)
		cands = append([]launcher{{f[0], f[1:]}}, cands...)
	}
	var last error = ErrNoTerminal
	for _, l := range cands {
		path, err := exec.LookPath(l.name)
		if err != nil {
			continue
		}
		cmd := command(path, expand(l.args, title, argv))
		if err := cmd.Start(); err != nil {
			last = err
			continue
		}
		go cmd.Wait() //nolint:errcheck // nur aufräumen
		return nil
	}
	if err := openFallback(title, argv); err != errNoFallback {
		return err
	}
	return last
}

var errNoFallback = errors.New("kein Rückfall")

// expand ersetzt {title} und {cmd}; ohne {cmd} kommt das Kommando ans Ende.
func expand(args []string, title string, argv []string) []string {
	var out []string
	hasCmd := false
	for _, a := range args {
		switch {
		case a == "{cmd}":
			out = append(out, argv...)
			hasCmd = true
		case a == "{shcmd}":
			out = append(out, shellJoin(argv))
			hasCmd = true
		default:
			out = append(out, strings.ReplaceAll(a, "{title}", title))
		}
	}
	if !hasCmd {
		out = append(out, argv...)
	}
	return out
}

// shellJoin: Kommando für eine POSIX-Shell (einfache Anführungszeichen).
func shellJoin(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}
