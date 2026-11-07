// Package localsh startet die Shell des eigenen Rechners in einem
// Pseudo-Terminal: unter macOS/Linux ein klassisches PTY, unter Windows
// ConPTY (ab Windows 10 1809). sshTermy behandelt den Strom wie eine
// SSH-Shell (Emulator, KI, Protokoll), nur ohne Netz dazwischen.
package localsh

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ptyFile: Pseudo-Terminal je System (pty_unix.go, pty_windows.go).
type ptyFile interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// Stream: eine laufende lokale Shell.
type Stream struct {
	Shell string     // gestartetes Programm (Pfad)
	Done  chan error // nil = normal beendet (exit) oder von uns geschlossen

	pty    ptyFile
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed atomic.Bool
	once   sync.Once
}

// Start startet shell (leer = Standard-Shell des Systems) im Ordner dir
// (leer = Benutzerordner). Alles, was die Shell ausgibt, geht an out.
func Start(shell, dir string, cols, rows int, out io.Writer) (*Stream, error) {
	argv, err := Command(shell)
	if err != nil {
		return nil, err
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if runtime.GOOS == "darwin" && len(argv) == 1 {
		// Wie Terminal.app: Login-Shell (lädt .zprofile, Homebrew-Pfad …)
		cmd.Args[0] = "-" + filepath.Base(argv[0])
	}
	cmd.Dir = StartDir(dir)
	cmd.Env = Env(os.Environ())
	p, err := startPty(cmd, cols, rows)
	if err != nil {
		return nil, err
	}
	s := &Stream{Shell: argv[0], Done: make(chan error, 1), pty: p, cmd: cmd}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(out, p)
	}()
	go func() {
		err := waitProcess(cmd)
		// Nach dem Ende der Shell den Rest der Ausgabe noch abholen. Hält ein
		// Hintergrundprozess das Terminal offen (oder bei ConPTY, das erst
		// nach Close ein EOF liefert), nicht ewig warten.
		select {
		case <-readDone:
		case <-time.After(300 * time.Millisecond):
		}
		_ = p.Close()
		<-readDone
		if s.closed.Load() {
			err = nil
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			err = nil // exit 1 o. ä. ist ein normales Ende
		}
		s.Done <- err
	}()
	return s, nil
}

// Write: Tastendrücke an die Shell.
func (s *Stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pty.Write(p)
}

// Resize: Fenstergröße weitergeben.
func (s *Stream) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return s.pty.Resize(cols, rows)
}

// Close beendet die Shell (samt Kindprozessen, soweit das System sie mitnimmt).
func (s *Stream) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		_ = s.pty.Close()
	})
	return nil
}

// Name: kurzer Anzeigename der Shell („pwsh“, „zsh“).
func (s *Stream) Name() string { return ShortName(s.Shell) }

// ShortName: „C:\…\pwsh.exe“ → „pwsh“, „/bin/zsh“ → „zsh“.
func ShortName(shell string) string {
	b := filepath.Base(strings.ReplaceAll(shell, `\`, "/"))
	if i := strings.LastIndex(b, "/"); i >= 0 {
		b = b[i+1:]
	}
	return strings.TrimSuffix(strings.TrimSuffix(b, ".exe"), ".EXE")
}

// Env: Umgebung der Shell. TERM passt zum Emulator von sshTermy;
// SSHTERMY=1 erlaubt eigene Anpassungen im Profil.
func Env(base []string) []string {
	out := make([]string, 0, len(base)+3)
	for _, e := range base {
		k, _, _ := strings.Cut(e, "=")
		switch strings.ToUpper(k) {
		case "TERM", "COLORTERM", "SSHTERMY":
			continue
		}
		out = append(out, e)
	}
	return append(out, "TERM=xterm-256color", "COLORTERM=truecolor", "SSHTERMY=1")
}

// StartDir: gewünschter Ordner, falls es ihn gibt, sonst der Benutzerordner.
func StartDir(dir string) string {
	if dir = expandHome(strings.TrimSpace(dir)); dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[1:])
		}
	}
	return p
}

// lookPath ist austauschbar, damit Tests die Auswahl ohne echte Shells prüfen.
var lookPath = exec.LookPath

// Command zerlegt die Shell-Angabe in Programm und Argumente. Leer = Standard.
// Ein Pfad mit Leerzeichen („C:\Program Files\PowerShell\7\pwsh.exe“) geht
// ohne Anführungszeichen, wenn es die Datei gibt; sonst trennen Leerzeichen
// die Argumente, "…" hält zusammen.
func Command(shell string) ([]string, error) {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		d := Default()
		if d == "" {
			return nil, errors.New("keine Shell gefunden")
		}
		return []string{d}, nil
	}
	if st, err := os.Stat(expandHome(shell)); err == nil && !st.IsDir() {
		return []string{expandHome(shell)}, nil
	}
	argv := splitArgs(shell)
	p, err := lookPath(expandHome(argv[0]))
	if err != nil {
		return nil, err
	}
	argv[0] = p
	return argv, nil
}

func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	quote, have := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quote, have = !quote, true
		case (r == ' ' || r == '\t') && !quote:
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if have {
		out = append(out, cur.String())
	}
	return out
}

// Default: Standard-Shell. Windows: PowerShell 7 (pwsh), sonst Windows
// PowerShell, sonst cmd.exe. Unix: $SHELL, sonst zsh/bash/sh.
func Default() string { return defaultFor(runtime.GOOS, os.Getenv) }

func defaultFor(goos string, env func(string) string) string {
	if goos == "windows" {
		for _, c := range []string{"pwsh.exe", "powershell.exe"} {
			if p, err := lookPath(c); err == nil {
				return p
			}
		}
		if c := env("COMSPEC"); c != "" {
			return c
		}
		return "cmd.exe"
	}
	if sh := env("SHELL"); sh != "" {
		if _, err := os.Stat(sh); err == nil {
			return sh
		}
	}
	for _, c := range []string{"zsh", "bash", "sh"} {
		if p, err := lookPath(c); err == nil {
			return p
		}
	}
	return "/bin/sh"
}

// Dialect: Befehlssatz für die KI zur Shell („powershell“, „cmd“, „macos“,
// „posix“).
func Dialect(goos, shell string) string {
	n := strings.ToLower(ShortName(shell))
	switch {
	case n == "pwsh" || n == "powershell":
		return "powershell"
	case n == "cmd":
		return "cmd"
	case goos == "windows" && (n == "wsl" || n == "bash"):
		return "posix" // WSL oder Git-Bash
	case goos == "darwin":
		return "macos"
	case goos == "windows":
		return "powershell"
	case strings.HasSuffix(goos, "bsd"):
		return "bsd"
	}
	return "posix"
}
