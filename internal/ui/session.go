package ui

import (
	"context"
	"image/color"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/detach"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/termemu"
)

type sessState int

const (
	stConnecting sessState = iota
	stConnected
	stClosed
	stReconnect // Verbindung abgerissen, sshTermy verbindet gleich neu
)

type panelKind int

const (
	panelNone panelKind = iota
	panelGoal
	panelAsk
	panelTunnels
	panelHost
	panelCompanion // Begleiter-Leiste (companion_view.go)
)

// session ist eine offene SSH-Sitzung (ein Tab).
type session struct {
	connNote   string       // Hinweis während des Verbindens (z. B. Sicherheitsschlüssel berühren)
	connStep   int          // Schritt beim Verbinden (sshx.Step…), Karte in motion2.go
	connStepAt [4]time.Time // Beginn jedes Schritts
	revealAt   time.Time    // Shell erscheint: Zeilen blenden von oben ein
	panelSeen  panelKind    // Panel im letzten Bild (blendet beim Wechsel ein)
	panelAt    time.Time
	id         int
	conn       domain.Connection
	title      string // eigener Tab-Name (tabs.go), "" = Alias
	client     *sshx.Client
	shell      termStream // SSH-Shell, Telnet oder serielle Konsole (rawconn.go)
	term       *termemu.Term
	tunnels    *sshx.TunnelManager
	state      sessState
	err        string
	started    time.Time

	connectedAt time.Time                // letzter erfolgreicher Aufbau (Statuszeile: Dauer)
	quitAt      time.Time                // exit/logout getippt: Ende ist gewollt, kein Auto-Reconnect
	rc          reconnState              // Auto-Reconnect
	mux         muxState                 // tmux/screen, das nach dem Neuverbinden wieder angehängt wird
	secrets     *secretCache             // Antworten auf Passwort-Rückfragen, nur im Speicher
	muxPending  bool                     // tmux/screen nach dem Neuverbinden anhängen
	rootPrompt  bool                     // letzter Prompt endete auf „#“
	dangerOK    string                   // Gefahren-Check: diese Eingabe wurde bestätigt (dangerguard.go)
	rec         atomic.Pointer[recorder] // laufende Aufnahme (record.go)
	recStart    time.Time

	detach *detachState                // Tab läuft in eigenem Fenster (detach.go)
	detOut atomic.Pointer[detach.Conn] // dorthin geht die Ausgabe mit
	share  atomic.Pointer[shareOut]    // Sitzung live teilen (share.go)

	fixedCols, fixedRows int       // Zuschauer: Größe des Gastgebers (0 = Fenster)
	viewMemo             termMemo  // letztes Terminalbild (perf.go: termView)
	panelMemo            panelMemo // letzte Seitenleiste (panels.go: viewPanel)

	notice   noticeKind  // Markierung im Tab: Befehl fertig, Glocke (notify.go)
	unseen   bool        // neue Ausgabe, solange der Tab verdeckt war (motion.go)
	flashAt  time.Time   // Tab leuchtet kurz auf (motion.go)
	flashCol color.Color // … in dieser Farbe
	latAt    time.Time   // Latenzverlauf für die Hover-Karte
	latHist  []time.Duration
	bellAt   time.Time // letzte gemeldete Glocke

	editFS  *sshx.RemoteFS            // SFTP für Dateien im eigenen Editor (editor.go)
	alerts  atomic.Pointer[alertScan] // Farbregeln mit „!“ (highlight.go)
	alertAt time.Time                 // letzte Meldung einer Farbregel

	trz       atomic.Pointer[trzRun] // Dateiübertragung im Terminal (trzsz.go)
	trzTail   []byte                 // Ende der letzten Ausgabe (Auslöser über zwei Blöcke)
	trzLastID string                 // Auslöser schon behandelt

	pending    atomic.Bool // Ausgabe-Nachricht unterwegs (Koaleszieren)
	lastOutput time.Time

	// Ghost-Text
	ghost, ghostFor string
	ghostSeq        int
	ghostCancel     context.CancelFunc
	ghostMemo       map[string]ghostMemo // letzte Antworten je Eingabe (ghost.go)

	// eigene Befehle und Folgebefehle
	history     []string
	chips       []agent.Suggestion
	aiPend      *aiPending // zuletzt eingefügter/ausgeführter KI-Befehl (learn.go)
	chipSeq     int
	chipArmed   int            // Index eines roten Chips nach dem ersten Druck, sonst -1
	chipReplace bool           // Chips ersetzen die getippte Zeile (Begleiter: sicherere Variante)
	compChips   bool           // Chips stammen vom Begleiter (Folgebefehle überschreiben sie nicht)
	comp        companionState // KI-Begleiter (companion.go)
	quickArmed  int            // dasselbe für Schnellbefehle (⇧F1 …)
	lastFollow  time.Time
	followDue   time.Time

	// Systemprofil (siehe profile.go)
	prof    domain.SystemProfile
	profDue time.Time // nach Ruhe im Terminal mitlesen
	profCmd string    // zuletzt eingegebener Befehl (Ablehnung prüfen)

	panel      panelKind
	panelFocus bool
	goal       goalState
	ask        askState
	tunSel     int
	files      filesState
	search     searchState // Strg+F
	host       hostState   // F6: Host-Übersicht und Config-Sicherung
	cap        shellCap    // mitgeschnittene Shell-Ausgabe während einer Abfrage

	// laufender Befehl, letzter Fehlschlag, Sitzungsprotokoll (cmdlog.go)
	run    *cmdRun
	fail   *agent.Failure
	log    *sessionlog.Log
	logErr bool // Fehler beim Schreiben schon gemeldet

	// Maus im Terminal
	scroll    int          // Zeilen zurück im Verlauf (0 = live)
	sbSeen    int          // Scrollback-Länge beim letzten Bild
	sel       *termemu.Sel // Markierung
	selecting bool         // linke Taste gedrückt, Markierung wächst
	fwdHeld   bool         // Taste gedrückt, Ereignisse gehen an den Server

	// Bilder fürs äußere Terminal (pixels.go)
	pixMu sync.Mutex
	pixQ  []byte
}

// writer für den PTY-Strom: füttert den Emulator und meldet der Oberfläche,
// dass neu gezeichnet werden soll (höchstens eine Nachricht gleichzeitig).
type outWriter struct {
	s  *session
	ev chan<- tea.Msg
}

type outputMsg struct{ sid int }

func (w outWriter) Write(p []byte) (int, error) {
	full := len(p)
	outBytes.Add(int64(full)) // Ausgaberate für die Bildrate (perf.go)
	if p = w.s.trzScan(p, w.ev); len(p) == 0 {
		return full, nil // geht an die Dateiübertragung (trzsz.go)
	}
	w.s.cap.feed(p)
	if sc := w.s.alerts.Load(); sc != nil {
		sc.feed(p)
	}
	if r := w.s.rec.Load(); r != nil {
		r.Output(p)
	}
	_, err := w.s.term.Write(p)
	if c := w.s.detOut.Load(); c != nil {
		_, _ = c.Write(p)
	}
	if o := w.s.share.Load(); o != nil {
		o.feed(p)
	}
	if w.s.pending.CompareAndSwap(false, true) {
		select {
		case w.ev <- outputMsg{w.s.id}:
		default:
			w.s.pending.Store(false)
		}
	}
	return full, err
}

type connectedMsg struct {
	sid    int
	client *sshx.Client // nil bei Telnet und seriell
	shell  termStream
	note   string // Hinweis für die Statuszeile (z. B. anderes Konsolenkabel)
}
type connectErrMsg struct {
	sid int
	err error
}
type shellClosedMsg struct {
	sid     int
	shell   termStream
	err     error
	dropped bool // Leitung weggebrochen (nicht exit/logout)
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
	if s.secrets == nil {
		s.secrets = &secretCache{}
	}
	sc := s.secrets
	keepTerm := s.term != nil && s.rc.active // Auto-Reconnect: Verlauf bleibt stehen
	s.connStep, s.connStepAt = sshx.StepConnect, [4]time.Time{time.Now()}
	if s.conn.Protocol.Raw() {
		return a.connectRawCmd(s, cols, rows, keepTerm)
	}
	return func() tea.Msg {
		opts := sshx.Options{
			HostKeys: hk,
			Resolve:  st.Connection,
			Relay:    a.relayDialer(),
			Prompt: func(host, fp, typ string) bool {
				r := make(chan bool, 1)
				ev <- hostKeyAskMsg{host, fp, typ, r}
				return <-r
			},
			Notice: func(text string) { ev <- connectNoticeMsg{s.id, text} },
			Step:   func(n int) { ev <- connectStepMsg{s.id, n} },
			AskSecret: sc.wrap(func(label string) (string, error) {
				r := make(chan string, 1)
				ev <- secretAskMsg{label, r}
				v := <-r
				if v == "\x00cancel" {
					return "", context.Canceled
				}
				return v, nil
			}),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		c, err := sshx.Dial(ctx, s.conn, opts)
		if err != nil {
			if isAuthFailure(err) {
				sc.forget() // falsches Passwort nicht beim nächsten Versuch wiederholen
			}
			return connectErrMsg{s.id, err}
		}
		ev <- connectStepMsg{s.id, sshx.StepShell}
		if !keepTerm {
			s.term = termemu.New(cols, rows, shellWriter{s})
			s.pixelImages() // pixelgenaue Bilder, wenn das Terminal es kann (pixels.go)
		}
		sh, err := c.OpenShell(cols, rows, outWriter{s, ev})
		if err != nil {
			c.Close()
			return connectErrMsg{s.id, err}
		}
		go func() {
			err := <-sh.Done
			// exit/logout endet mit Exit-Status; ohne Status (oder mit Fehler)
			// prüfen, ob die ganze Leitung weg ist → Auto-Reconnect.
			dropped := err != nil && c.WaitDropped(1500*time.Millisecond)
			ev <- shellClosedMsg{s.id, sh, err, dropped}
		}()
		return connectedMsg{sid: s.id, client: c, shell: sh, note: joinNotes(c.AgentNote(), c.X11Note())}
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
	if o := s.share.Swap(nil); o != nil {
		o.end()
	}
	s.endDetach(i18n.T("Sitzung in sshTermy beendet."))
	if s.ghostCancel != nil {
		s.ghostCancel()
	}
	s.goal.cancel()
	s.ask.cancel()
	s.host.cancel()
	s.host.live.stop()
	if s.tunnels != nil {
		s.tunnels.StopAll()
	}
	if s.files.xfer != nil {
		s.files.xfer.cancel()
	}
	if s.files.remote != nil {
		s.files.remote.Close()
	}
	if s.editFS != nil {
		s.editFS.Close()
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
		c.OS = s.profileSummary()
		c.Shell = s.client.Remote.Shell
		c.Profile = s.prof.Prompt()
		c.Net = s.netDevice()
	} else if s.conn.Protocol.Raw() {
		c.OS = s.profileSummary()
		c.Profile = s.prof.Prompt()
		c.Net = s.netDevice()
		if s.isLocal() {
			c.Shell = s.localShellName()
		}
		if s.isWinRM() {
			c.Shell = "powershell (WinRM)"
		}
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
		a.flash(c.Alias + i18n.T(" als Netzwerkgerät gemerkt (ohne Systemabfrage)"))
	}
}

// joinNotes: mehrere Hinweise für die Einblendung nach dem Verbinden.
func joinNotes(ns ...string) string {
	var out []string
	for _, n := range ns {
		if n != "" {
			out = append(out, n)
		}
	}
	return strings.Join(out, " · ")
}

// connectNoticeMsg: Hinweis beim Verbinden (Kerberos, Sicherheitsschlüssel).
type connectStepMsg struct{ sid, step int }

type connectNoticeMsg struct {
	sid  int
	text string
}
