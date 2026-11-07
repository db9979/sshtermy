package sshx

import (
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Shell ist eine interaktive PTY-Sitzung. Ausgabe (stdout+stderr, bei PTY
// ohnehin ein Strom) geht an Output; Write schickt Tastendrücke zum Server.
type Shell struct {
	sess  *ssh.Session
	stdin io.WriteCloser
	mu    sync.Mutex
	Done  chan error
}

// OpenShell fordert ein PTY an (xterm-256color, damit vim/htop/tmux voll
// funktionieren) und startet die Login-Shell. Die Ausgabe wird roh, also mit
// allen ANSI/VT100-Sequenzen, an out geschrieben; die Emulation übernimmt
// internal/termemu.
func (c *Client) OpenShell(cols, rows int, out io.Writer) (*Shell, error) {
	s, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	term := "xterm-256color"
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
		ssh.IUTF8:         1,
	}
	if c.NetDevice {
		// Switch-CLIs kennen oft nur vt100/xterm und verschlucken sich an
		// unbekannten Terminal-Modes; schlicht anfordern.
		term = "xterm"
		modes = ssh.TerminalModes{}
	}
	if err := s.RequestPty(term, rows, cols, modes); err != nil {
		s.Close()
		return nil, err
	}
	stdin, err := s.StdinPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	s.Stdout, s.Stderr = out, out
	if !c.NetDevice {
		_ = s.Setenv("COLORTERM", "truecolor") // viele Server verwerfen das (AcceptEnv), harmlos
	}
	if c.Conn.ForwardAgent && !c.NetDevice {
		if err := c.forwardAgent(s); err != nil && c.fwdErr == nil {
			c.fwdAsked = err
		}
	}
	if c.Conn.ForwardX11 && !c.NetDevice {
		c.requestX11(s)
	}
	if err := s.Shell(); err != nil {
		s.Close()
		return nil, err
	}
	sh := &Shell{sess: s, stdin: stdin, Done: make(chan error, 1)}
	go func() { sh.Done <- s.Wait() }()
	return sh, nil
}

// Write ist nebenläufig sicher (Tastatur, Agent und VT-Antworten schreiben).
func (s *Shell) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stdin.Write(p)
}

// Resize meldet dem Server die neue Fenstergröße (SIGWINCH dort).
func (s *Shell) Resize(cols, rows int) error { return s.sess.WindowChange(rows, cols) }

func (s *Shell) Close() error { return s.sess.Close() }
