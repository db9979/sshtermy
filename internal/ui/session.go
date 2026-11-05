package ui

import (
	"context"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/termemu"
)

type sessState int

const (
	stConnecting sessState = iota
	stConnected
	stClosed
)

type panelKind int

const (
	panelNone panelKind = iota
	panelGoal
	panelAsk
	panelTunnels
	panelHost
)

// session ist eine offene SSH-Sitzung (ein Tab).
type session struct {
	id      int
	conn    domain.Connection
	client  *sshx.Client
	shell   *sshx.Shell
	term    *termemu.Term
	tunnels *sshx.TunnelManager
	state   sessState
	err     string
	started time.Time

	pending    atomic.Bool // Ausgabe-Nachricht unterwegs (Koaleszieren)
	lastOutput time.Time

	// Ghost-Text
	ghost, ghostFor string
	ghostSeq        int
	ghostCancel     context.CancelFunc

	// eigene Befehle und Folgebefehle
	history    []string
	chips      []agent.Suggestion
	chipSeq    int
	chipArmed  int // Index eines roten Chips nach dem ersten Druck, sonst -1
	lastFollow time.Time
	followDue  time.Time

	panel      panelKind
	panelFocus bool
	goal       goalState
	ask        askState
	tunSel     int
	files      filesState
	host       hostState // F6: Host-Übersicht und Config-Sicherung
	cap        shellCap  // mitgeschnittene Shell-Ausgabe während einer Abfrage

	// Maus im Terminal
	scroll    int          // Zeilen zurück im Verlauf (0 = live)
	sbSeen    int          // Scrollback-Länge beim letzten Bild
	sel       *termemu.Sel // Markierung
	selecting bool         // linke Taste gedrückt, Markierung wächst
	fwdHeld   bool         // Taste gedrückt, Ereignisse gehen an den Server
}

// writer für den PTY-Strom: füttert den Emulator und meldet der Oberfläche,
// dass neu gezeichnet werden soll (höchstens eine Nachricht gleichzeitig).
type outWriter struct {
	s  *session
	ev chan<- tea.Msg
}

type outputMsg struct{ sid int }

func (w outWriter) Write(p []byte) (int, error) {
	w.s.cap.feed(p)
	n, err := w.s.term.Write(p)
	if w.s.pending.CompareAndSwap(false, true) {
		select {
		case w.ev <- outputMsg{w.s.id}:
		default:
			w.s.pending.Store(false)
		}
	}
	return n, err
}

type connectedMsg struct {
	sid    int
	client *sshx.Client
	shell  *sshx.Shell
}
type connectErrMsg struct {
	sid int
	err error
}
type shellClosedMsg struct {
	sid int
	err error
}

// hostKeyAskMsg / secretAskMsg: der Verbindungsaufbau wartet auf die Antwort.
type hostKeyAskMsg struct {
	host, fp, typ string
	reply         chan bool
}
type secretAskMsg struct {
	label string
	reply chan string
}

func (a *App) connectCmd(s *session) tea.Cmd {
	ev := a.events
	hk := a.hostKeys
	st := a.st
	cols, rows := a.termSize(s)
	return func() tea.Msg {
		opts := sshx.Options{
			HostKeys: hk,
			Resolve:  st.Connection,
			Prompt: func(host, fp, typ string) bool {
				r := make(chan bool, 1)
				ev <- hostKeyAskMsg{host, fp, typ, r}
				return <-r
			},
			AskSecret: func(label string) (string, error) {
				r := make(chan string, 1)
				ev <- secretAskMsg{label, r}
				v := <-r
				if v == "\x00cancel" {
					return "", context.Canceled
				}
				return v, nil
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		c, err := sshx.Dial(ctx, s.conn, opts)
		if err != nil {
			return connectErrMsg{s.id, err}
		}
		s.term = termemu.New(cols, rows, shellWriter{s})
		sh, err := c.OpenShell(cols, rows, outWriter{s, ev})
		if err != nil {
			c.Close()
			return connectErrMsg{s.id, err}
		}
		go func() { err := <-sh.Done; ev <- shellClosedMsg{s.id, err} }()
		return connectedMsg{s.id, c, sh}
	}
}

// shellWriter: vom Emulator kodierte Tasten → SSH (Shell kann kurz nil sein).
type shellWriter struct{ s *session }

func (w shellWriter) Write(p []byte) (int, error) {
	for i := 0; i < 100 && w.s.shell == nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if w.s.shell == nil {
		return len(p), nil
	}
	return w.s.shell.Write(p)
}

func (s *session) close() {
	if s.ghostCancel != nil {
		s.ghostCancel()
	}
	s.goal.cancel()
	s.ask.cancel()
	s.host.cancel()
	if s.tunnels != nil {
		s.tunnels.StopAll()
	}
	if s.files.xfer != nil {
		s.files.xfer.cancel()
	}
	if s.files.remote != nil {
		s.files.remote.Close()
	}
	if s.shell != nil {
		s.shell.Close()
	}
	if s.client != nil {
		s.client.Close()
	}
	if s.term != nil {
		s.term.Close()
	}
	s.state = stClosed
}

// context fürs Sprachmodell.
func (a *App) agentContext(s *session, withScreen bool) agent.Context {
	cfg := a.st.LLM()
	c := agent.Context{Host: s.conn.Alias, Commands: lastN(s.history, 10)}
	if s.client != nil {
		c.OS = s.client.Remote.String()
		c.Shell = s.client.Remote.Shell
	}
	if withScreen && cfg.SendScreen && s.term != nil {
		n := cfg.ContextLines
		if n <= 0 {
			n = 80
		}
		c.Screen = s.term.PlainLines(n)
	}
	return c
}

func lastN(x []string, n int) []string {
	if len(x) > n {
		return x[len(x)-n:]
	}
	return x
}

// rememberNetDevice: Der Rückfall hat ein Netzwerkgerät erkannt. Damit der
// nächste Aufbau gleich ohne Systemabfrage (und ohne Wartezeit) läuft, wird
// der Gerätetyp in der Verbindung gespeichert.
func (a *App) rememberNetDevice(s *session) {
	c, ok := a.st.Connection(s.conn.ID)
	if !ok || c.Device != domain.DeviceAuto {
		return
	}
	c.Device = domain.DeviceNetwork
	if saved, err := a.st.UpsertConnection(c); err == nil {
		s.conn = saved
		a.flash(c.Alias + " als Netzwerkgerät gemerkt (ohne Systemabfrage)")
	}
}
