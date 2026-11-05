package sshx

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/crypto/ssh"
)

// RunResult: Ausgabe (stdout und stderr zusammen) und Exit-Code eines
// Befehls. Exit -1 heißt unbekannt (Netzwerkgerät, Abbruch, kein Status).
type RunResult struct {
	Output []byte
	Exit   int
}

// maxRunOutput: mehr wird pro Host nicht aufgehoben (der Anfang zählt).
const maxRunOutput = 512 << 10

// cappedBuf sammelt Ausgabe nebenläufig sicher und merkt sich, wann zuletzt
// etwas kam (für das Warten auf Ruhe in der Shell).
type cappedBuf struct {
	mu   sync.Mutex
	b    bytes.Buffer
	last time.Time
}

func (c *cappedBuf) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = time.Now()
	if room := maxRunOutput - c.b.Len(); room > 0 {
		if len(p) > room {
			c.b.Write(p[:room])
		} else {
			c.b.Write(p)
		}
	}
	return len(p), nil
}

func (c *cappedBuf) snapshot() ([]byte, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.b.Bytes()...), c.last
}

func (c *cappedBuf) touch() {
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
}

// Exec führt einen Befehl in einer eigenen Exec-Sitzung aus, unsichtbar für
// ein offenes Terminal. Nur für Server: Viele Switches legen bei Exec auf.
func (c *Client) Exec(ctx context.Context, cmd string) (RunResult, error) {
	s, err := c.NewSession()
	if err != nil {
		return RunResult{Exit: -1}, err
	}
	defer s.Close()
	var buf cappedBuf
	s.Stdout, s.Stderr = &buf, &buf
	if err := s.Start(cmd); err != nil {
		return RunResult{Exit: -1}, err
	}
	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = s.Signal(ssh.SIGTERM)
		s.Close()
		out, _ := buf.snapshot()
		return RunResult{Output: out, Exit: -1}, ctx.Err()
	}
	out, _ := buf.snapshot()
	if err == nil {
		return RunResult{Output: out, Exit: 0}, nil
	}
	var ee *ssh.ExitError
	if errors.As(err, &ee) {
		return RunResult{Output: out, Exit: ee.ExitStatus()}, nil
	}
	var em *ssh.ExitMissingError
	if errors.As(err, &em) {
		return RunResult{Output: out, Exit: -1}, nil
	}
	return RunResult{Output: out, Exit: -1}, err
}

// pagerRx: „weiter mit Taste“-Aufforderungen von Switch-CLIs.
var pagerRx = regexp.MustCompile(`(?i)(--\s*more\s*--|<-+\s*more\s*-+>|press any key|any key to continue)`)

// ShellTiming steuert RunInShell (Tests verkürzen die Zeiten).
type ShellTiming struct {
	Quiet   time.Duration // so lange ohne Ausgabe = Befehl fertig (Standard 1,5 s)
	Login   time.Duration // höchstens so lange auf den ersten Prompt warten (8 s)
	PerStep time.Duration // höchstens so lange pro Befehl (2 min)
}

// RunInShell tippt Befehle in eine interaktive Shell, wie ein Mensch: für
// Switches und Router, die keine Exec-Kanäle können. Fertig ist ein Befehl,
// wenn eine Weile nichts mehr kommt. Seiten-Pausen („-- MORE --“) werden
// mit Leertaste weitergeblättert. Einen Exit-Code gibt es hier nicht.
func (c *Client) RunInShell(ctx context.Context, cmds []string, tm ShellTiming) (RunResult, error) {
	if tm.Quiet <= 0 {
		tm.Quiet = 1500 * time.Millisecond
	}
	if tm.Login <= 0 {
		tm.Login = 8 * time.Second
	}
	if tm.PerStep <= 0 {
		tm.PerStep = 2 * time.Minute
	}
	buf := &cappedBuf{}
	sh, err := c.OpenShell(200, 50, buf)
	if err != nil {
		return RunResult{Exit: -1}, err
	}
	defer sh.Close()
	closed := false
	seen := 0 // bis hierhin wurden Seiten-Pausen schon beantwortet
	wait := func(limit time.Duration) error {
		deadline := time.Now().Add(limit)
		buf.touch()
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-sh.Done:
				closed = true
				return nil
			case <-t.C:
			}
			out, last := buf.snapshot()
			// Nur Text nach der letzten beantworteten Pause zählt.
			tail := out[max(seen, len(out)-160):]
			if pagerRx.Match([]byte(ansi.Strip(string(tail)))) {
				seen = len(out)
				_, _ = sh.Write([]byte(" "))
				buf.touch()
				continue
			}
			if time.Since(last) >= tm.Quiet || time.Now().After(deadline) {
				return nil
			}
		}
	}
	if err := wait(tm.Login); err != nil {
		return RunResult{Exit: -1}, err
	}
	before, _ := buf.snapshot()
	start := len(before)
	for _, cmd := range cmds {
		if closed {
			break
		}
		if _, err := sh.Write([]byte(cmd + "\r")); err != nil {
			break
		}
		if err := wait(tm.PerStep); err != nil {
			out, _ := buf.snapshot()
			return RunResult{Output: CleanShellOutput(out[start:]), Exit: -1}, err
		}
	}
	out, _ := buf.snapshot()
	return RunResult{Output: CleanShellOutput(out[start:]), Exit: -1}, nil
}

// CleanShellOutput macht aus einem rohen PTY-Strom lesbaren Text: ohne
// Steuersequenzen, Wagenrückläufe und Seiten-Pausen.
func CleanShellOutput(raw []byte) []byte {
	s := ansi.Strip(string(raw))
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		// Zeile, die mit \r überschrieben wurde: das Letzte zählt.
		if i := strings.LastIndex(line, "\r"); i >= 0 {
			rest := line[i+1:]
			if strings.TrimSpace(rest) == "" {
				rest = strings.TrimSpace(line[:i])
				if j := strings.LastIndex(rest, "\r"); j >= 0 {
					rest = rest[j+1:]
				}
			}
			line = rest
		}
		for strings.Contains(line, "\b") {
			i := strings.Index(line, "\b")
			if i > 0 {
				line = line[:i-1] + line[i+1:]
			} else {
				line = line[1:]
			}
		}
		line = strings.TrimRight(pagerRx.ReplaceAllString(line, ""), " \t")
		out = append(out, line)
	}
	return []byte(strings.Trim(strings.Join(out, "\n"), "\n"))
}
