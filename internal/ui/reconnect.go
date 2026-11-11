package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/psremote"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Auto-Reconnect: Reißt die Leitung ab (WLAN, VPN, Standby, Server-Neustart),
// verbindet sshTermy die Sitzung von selbst neu, mit wachsenden Pausen. Der
// Verlauf im Terminal bleibt stehen, laufende Tunnel starten wieder, und
// war tmux oder screen offen, wird die Sitzung wieder angehängt.
//
// Kein Neuverbinden, wenn die Sitzung gewollt endet: exit/logout/Strg+D,
// Exit-Status vom Server, oder Esc während des Wartens.
//
// Netzwerkgeräte (Aruba & Co.): keine Systemabfrage und kein tmux; der
// Switch braucht nach einem Abbruch ein paar Sekunden, bis er wieder
// annimmt, deshalb startet der erste Versuch dort später.

// reconnectWaits: Pause vor Versuch 1, 2, … (insgesamt gut drei Minuten).
var reconnectWaits = []time.Duration{
	1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second,
	30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
}

const netDeviceFirstWait = 4 * time.Second // Switch gibt die alte Sitzung erst frei

type reconnState struct {
	active   bool
	dialing  bool
	attempts int       // gestartete Versuche seit dem Abbruch
	due      time.Time // nächster Versuch
	since    time.Time // Abbruch
	cause    string    // warum die Leitung weg ist
	lastErr  string    // Fehler des letzten Versuchs
	tunnels  []string  // Tunnel, die vor dem Abbruch liefen
	netDev   bool
	alt      bool // Alt-Screen war an (tmux/screen/vim)
}

// muxState: tmux oder screen, in dem der Nutzer zuletzt war.
type muxState struct {
	kind string // "tmux" | "screen" | ""
	name string // Sitzungsname, leer = zuletzt benutzte
	// Feste Sitzung (persist.go): geteilt anhängen, ohne andere Zuschauer
	// abzuhängen; view = schreibgeschützt; create = screen-Sitzung anlegen.
	shared, view, create bool
}

func (m muxState) attachCmd() string {
	switch {
	case m.kind == "tmux" && m.view:
		return "tmux attach -r -t " + shellQuote(m.name)
	case m.kind == "tmux" && m.shared:
		return "tmux new -A -s " + shellQuote(m.name)
	case m.kind == "screen" && m.shared && m.create:
		return "screen -S " + shellQuote(m.name)
	case m.kind == "screen" && m.shared:
		return "screen -x " + shellQuote(m.name)
	}
	switch m.kind {
	case "tmux":
		if m.name != "" {
			return "tmux attach -d -t " + shellQuote(m.name)
		}
		return "tmux attach -d"
	case "screen":
		if m.name != "" {
			return "screen -d -r " + shellQuote(m.name)
		}
		return "screen -d -r"
	}
	return ""
}

func (m muxState) label() string {
	l := m.kind + i18n.T("-Sitzung")
	if m.name != "" {
		l = m.kind + i18n.T("-Sitzung „") + m.name + i18n.T("“")
	}
	if m.view {
		l += i18n.T(" (nur zuschauen)")
	}
	return l
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-:") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseMux erkennt tmux-/screen-Aufrufe in einem getippten Befehl. ok=false:
// kein Start/Anhängen (z. B. "tmux ls", "screen -ls").
func parseMux(cmd string) (m muxState, ok bool) {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return m, false
	}
	switch f[0] {
	case "tmux":
		m.kind = "tmux"
		args := f[1:]
		// globale Schalter vor dem Unterbefehl überspringen (-2, -u, -L x, -S x, -f x)
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			if (args[0] == "-L" || args[0] == "-S" || args[0] == "-f") && len(args) > 1 {
				args = args[1:]
			}
			args = args[1:]
		}
		if len(args) == 0 {
			return m, true // "tmux" = neue Sitzung
		}
		var flag string
		switch args[0] {
		case "new", "new-session", "new-s":
			flag = "-s"
		case "a", "at", "att", "attach", "attach-session":
			flag = "-t"
		default:
			return muxState{}, false
		}
		for i := 1; i < len(args); i++ {
			if args[i] == flag && i+1 < len(args) {
				m.name = args[i+1]
			} else if strings.HasPrefix(args[i], flag) && len(args[i]) > 2 {
				m.name = args[i][2:]
			}
		}
		return m, true
	case "screen":
		m.kind = "screen"
		args := f[1:]
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch {
			case a == "-ls" || a == "-list" || a == "-wipe" || a == "-v" || a == "-h" || a == "--help":
				return muxState{}, false
			case a == "-S" && i+1 < len(args):
				m.name = args[i+1]
				i++
			case strings.HasPrefix(a, "-") && strings.ContainsAny(a[1:], "rRxD") && !strings.HasPrefix(a, "--"):
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					m.name = args[i+1]
					i++
				}
			case !strings.HasPrefix(a, "-"):
				return m, true // ab hier Befehl, der in screen läuft
			}
		}
		return m, true
	}
	return m, false
}

// isQuitCommand: Befehle, mit denen man eine Sitzung absichtlich verlässt.
func isQuitCommand(cmd string) bool {
	f := strings.Fields(strings.ToLower(cmd))
	if len(f) == 0 {
		return false
	}
	switch f[0] {
	case "exit", "logout", "logoff", "quit", "bye", "disconnect", "lo", "q", "reboot", "shutdown", "poweroff", "halt":
		return true
	}
	return false
}

// noteCommand wertet einen abgeschickten Befehl aus: gewolltes Ende,
// tmux/screen gestartet oder verlassen.
func (s *session) noteCommand(cmd string) {
	if isQuitCommand(cmd) {
		s.quitAt = time.Now()
	}
	s.checkMuxGone()
	if m, ok := parseMux(cmd); ok {
		s.mux = m
	}
}

// muxGone: Meldungen, nach denen keine tmux/screen-Sitzung mehr offen ist
// (verlassen, beendet oder gar nicht erst gestartet).
var muxGone = []string{
	"[detached", "[exited]", "[screen is terminating]", "[lost tty]",
	"command not found", "Befehl nicht gefunden", "no sessions", "There is no screen to be",
	"can't find session", "no server running",
}

// checkMuxGone prüft die letzten Zeilen (vor dem nächsten Befehl und beim Abbruch).
func (s *session) checkMuxGone() {
	if s.mux.kind == "" || s.term == nil || s.term.AltScreen() {
		return
	}
	scr := s.term.PlainLines(6)
	for _, g := range muxGone {
		if strings.Contains(scr, g) {
			s.mux = muxState{}
			return
		}
	}
}

// wantedEnd: hat der Nutzer die Sitzung eben selbst beendet?
func (s *session) wantedEnd() bool { return time.Since(s.quitAt) < 20*time.Second }

func (a *App) autoReconnect() bool { return !a.st.Settings().NoAutoReconnect }

func (a *App) toggleAutoReconnect() tea.Cmd {
	on := !a.autoReconnect()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoAutoReconnect = !on }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Auto-Reconnect an: abgerissene Sitzungen verbinden sich neu."))
	} else {
		a.flash(i18n.T("Auto-Reconnect aus: nach einem Abbruch ⏎ zum Neuverbinden."))
	}
	return nil
}

// onShellClosed entscheidet: neu verbinden oder Sitzung beenden.
func (a *App) onShellClosed(s *session, m shellClosedMsg) {
	if m.shell != nil && s.shell != m.shell {
		return // alte Shell, Sitzung ist schon weiter
	}
	if m.dropped && a.autoReconnect() && !s.wantedEnd() {
		cause := i18n.T("Verbindung verloren")
		if m.err != nil && !errors.Is(m.err, context.Canceled) {
			if e := strings.TrimSpace(m.err.Error()); e != "" && !strings.Contains(e, "without exit status") {
				cause += " (" + e + ")"
			}
		}
		a.beginReconnect(s, cause)
		return
	}
	a.endLog(s)
	s.close()
	s.err = i18n.T("Sitzung beendet")
	if m.dropped {
		s.err = i18n.T("Verbindung verloren")
	}
}

func (a *App) beginReconnect(s *session, cause string) {
	if !s.rc.active {
		if time.Since(s.connectedAt) > time.Minute {
			s.rc.attempts = 0 // lief eine Weile stabil: neue Runde
		}
		s.rc.since = time.Now()
	}
	if s.rc.attempts >= len(reconnectWaits) {
		a.endLog(s)
		s.close()
		s.err = cause + i18n.T(" · mehrmals hintereinander abgebrochen, ⏎ verbindet neu")
		s.rc = reconnState{}
		return
	}
	s.rc.active, s.rc.dialing, s.rc.cause, s.rc.lastErr = true, false, cause, ""
	if s.client != nil || s.conn.Protocol.Raw() {
		s.rc.netDev = s.netDev()
	}
	if s.tunnels != nil {
		s.rc.tunnels = s.rc.tunnels[:0]
		for _, t := range s.tunnels.List() {
			s.rc.tunnels = append(s.rc.tunnels, t.Spec.ID)
		}
	}
	if s.term != nil {
		s.rc.alt = s.term.AltScreen()
		s.checkMuxGone()
	}
	s.suspend()
	wait := reconnectWaits[s.rc.attempts]
	if s.rc.netDev && s.rc.attempts == 0 {
		wait = netDeviceFirstWait
	}
	s.rc.due = time.Now().Add(wait)
	s.state = stReconnect
	s.flashTab(a.s.T.Danger)
	if s.term != nil {
		// Programme der alten Sitzung (tmux, vim, htop) sind weg: Alt-Screen,
		// Maus und Bracketed Paste zurücksetzen, dann eine Trennlinie.
		_, _ = s.term.Write([]byte("\x1b[?1049l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[?1l\x1b[?25h\x1b[0m" +
			"\r\n\x1b[33m── " + cause + " · " + time.Now().Format("15:04:05") + " · sshTermy verbindet neu ──\x1b[0m\r\n"))
	}
}

// suspend schließt die tote Verbindung, behält aber Terminal und Verlauf.
func (s *session) suspend() {
	if s.ghostCancel != nil {
		s.ghostCancel()
	}
	s.ghost, s.chips = "", nil
	s.goal.cancel()
	s.ask.cancel()
	if s.tunnels != nil {
		s.tunnels.StopAll()
		s.tunnels = nil
	}
	if s.files.xfer != nil {
		s.files.xfer.cancel()
	}
	if s.files.remote != nil {
		s.files.remote.Close()
		s.files.remote = nil
	}
	if s.editFS != nil {
		s.editFS.Close()
		s.editFS = nil
	}
	s.files.open, s.panelFocus = false, false
	if s.shell != nil {
		s.shell.Close()
		s.setShell(nil)
	}
	if s.client != nil {
		s.client.Close() // Zeiger bleibt für Anzeige (Remote-Info) bis zur neuen Verbindung
	}
}

// reconnectTick startet fällige Versuche (aus onTick, einmal pro Sekunde).
func (a *App) reconnectTick(s *session) tea.Cmd {
	if s.state == stConnected {
		return tea.Batch(a.resumeMux(s), a.runStartCmd(s))
	}
	if s.state != stReconnect || s.rc.dialing || time.Now().Before(s.rc.due) {
		return nil
	}
	if a.offlineWait(s) {
		return nil
	}
	s.rc.dialing = true
	s.rc.attempts++
	return a.connectCmd(s)
}

// reconnectNow: ⏎ im Wartebild.
func (a *App) reconnectNow(s *session) tea.Cmd {
	if s.rc.dialing {
		return nil
	}
	s.rc.due = time.Now()
	return a.reconnectTick(s)
}

// cancelReconnect: Esc im Wartebild.
func (a *App) cancelReconnect(s *session) {
	s.rc = reconnState{}
	a.endLog(s)
	s.close()
	s.err = i18n.T("Neuverbinden abgebrochen")
}

// onReconnectErr: Versuch fehlgeschlagen → nächsten planen oder aufgeben.
func (a *App) onReconnectErr(s *session, err error) {
	s.rc.dialing = false
	s.rc.lastErr = err.Error()
	if !retryable(err) || s.rc.attempts >= len(reconnectWaits) {
		msg := s.rc.cause + i18n.T(" · Neuverbinden fehlgeschlagen: ") + err.Error()
		s.rc = reconnState{}
		a.endLog(s)
		s.close()
		s.err, s.errv = msg, err
		return
	}
	s.rc.due = time.Now().Add(reconnectWaits[s.rc.attempts])
}

// onReconnected: Tunnel wieder starten, tmux/screen anhängen.
func (a *App) onReconnected(s *session) tea.Cmd {
	down := time.Since(s.rc.since).Round(time.Second)
	for _, id := range s.rc.tunnels {
		t, ok := a.tunnelByID(s.conn.ID, id)
		if !ok || t.AutoStart || s.tunnels.Get(id) != nil {
			continue // AutoStart-Tunnel startet connectedMsg schon
		}
		if _, err := s.tunnels.Start(t); err != nil {
			a.flashErr("Tunnel " + t.Name + ": " + err.Error())
		}
	}
	msg := fmt.Sprintf(i18n.T("%s wieder verbunden (%s getrennt)"), s.conn.Alias, down)
	var probe tea.Cmd
	if s.mux.kind == "" && s.rc.alt && s.client != nil && !s.client.NetDevice {
		// Vollbild lief, aber kein tmux-Aufruf gesehen (z. B. tmux startet aus
		// .bashrc): fragen, ob genau eine tmux-Sitzung noch angehängt ist.
		probe = probeMux(s.id, s.client)
	}
	if s.mux.kind != "" && !s.netDev() {
		s.rc.active, s.rc.dialing = false, false
		s.rc.tunnels = nil
		s.muxPending = true
		msg += i18n.T(", hänge ") + s.mux.label() + i18n.T(" wieder an …")
	} else {
		s.rc = reconnState{attempts: s.rc.attempts}
	}
	if s.term != nil {
		_, _ = s.term.Write([]byte("\x1b[32m── wieder verbunden · " + time.Now().Format("15:04:05") + " ──\x1b[0m\r\n"))
	}
	a.flash(msg)
	return probe
}

type muxProbeMsg struct {
	sid int
	m   muxState
}

func probeMux(sid int, c *sshx.Client) tea.Cmd {
	return func() tea.Msg {
		out, err := c.Output(`tmux list-sessions -F '#{session_attached} #{session_name}' 2>/dev/null`, 4*time.Second)
		if err != nil && out == "" {
			return muxProbeMsg{sid: sid}
		}
		return muxProbeMsg{sid, pickAttachedTmux(out)}
	}
}

// pickAttachedTmux: Nach einem Abbruch hält der Server die alte Verbindung
// oft noch für angehängt. Genau eine solche Sitzung = die des Nutzers.
func pickAttachedTmux(out string) muxState {
	var found []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		n, name, ok := strings.Cut(strings.TrimSpace(l), " ")
		if ok && n != "0" && name != "" {
			found = append(found, name)
		}
	}
	if len(found) != 1 {
		return muxState{}
	}
	return muxState{kind: "tmux", name: found[0]}
}

func (a *App) onMuxProbe(m muxProbeMsg) {
	s := a.byID(m.sid)
	if s == nil || m.m.kind == "" || s.state != stConnected || s.mux.kind != "" {
		return
	}
	s.mux, s.muxPending = m.m, true
	a.flash(i18n.T("Hänge ") + m.m.label() + i18n.T(" wieder an …"))
}

// resumeMux schickt den Befehl zum Anhängen, sobald der Prompt steht.
func (a *App) resumeMux(s *session) tea.Cmd {
	if !s.muxPending || s.term == nil || s.shell == nil {
		return nil
	}
	ready := s.term.AtIdlePrompt() && time.Since(s.lastOutput) > 400*time.Millisecond
	if !ready && time.Since(s.connectedAt) < 6*time.Second {
		return nil
	}
	s.muxPending = false
	s.term.SendText(s.mux.attachCmd() + "\r")
	s.mux.create = false // beim Neuverbinden gibt es sie schon
	return nil
}

func (a *App) tunnelByID(connID, id string) (domain.Tunnel, bool) {
	for _, t := range a.st.Tunnels(connID) {
		if t.ID == id {
			return t, true
		}
	}
	return domain.Tunnel{}, false
}

// retryable: Lohnt ein weiterer Versuch? Falsches Passwort, abgelehnter oder
// geänderter Host-Key und abgebrochene Rückfragen nicht.
func retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, sshx.ErrHostKeyChanged) || isAuthFailure(err) {
		return false
	}
	m := err.Error()
	for _, s := range []string{"Host-Key", "host key", "Keyfile", "Key file", "passphrase"} {
		if strings.Contains(m, s) {
			return false
		}
	}
	return true
}

func isAuthFailure(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "unable to authenticate") || errors.Is(err, psremote.ErrAuth))
}

// secretCache merkt sich Antworten auf Rückfragen (Passwort, Passphrase) für
// die Lebensdauer der Sitzung, damit das Neuverbinden ohne Nachfrage klappt.
// Nur im Arbeitsspeicher, nie im Store.
type secretCache struct {
	mu   sync.Mutex
	seen map[string]string
}

func (c *secretCache) wrap(ask sshx.SecretPrompt) sshx.SecretPrompt {
	return func(label string) (string, error) {
		c.mu.Lock()
		v, ok := c.seen[label]
		c.mu.Unlock()
		if ok {
			return v, nil
		}
		v, err := ask(label)
		if err == nil {
			c.mu.Lock()
			if c.seen == nil {
				c.seen = map[string]string{}
			}
			c.seen[label] = v
			c.mu.Unlock()
		}
		return v, err
	}
}

// get: Antwort auf eine bestimmte Rückfrage (für den Schlüsselbund).
func (c *secretCache) get(label string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.seen[label]
	return v, ok
}

func (c *secretCache) forget() {
	c.mu.Lock()
	c.seen = nil
	c.mu.Unlock()
}

// reconnectView: Wartebild mit Countdown.
func (a *App) reconnectView(s *session) string {
	st := a.s
	r := s.rc
	where := s.conn.Alias
	if !strings.Contains(where, s.conn.Host) {
		where += " (" + s.conn.ShowAddr() + ")"
	}
	head := st.Warn.Render("◌ " + r.cause + ": " + where)
	var line string
	if r.dialing {
		line = st.AI.Render(fmt.Sprintf(i18n.T("Verbinde neu … (Versuch %d von %d)"), r.attempts, len(reconnectWaits)))
	} else {
		left := max(0, int(time.Until(r.due).Round(time.Second)/time.Second))
		line = st.Text.Render(fmt.Sprintf(i18n.T("Neuer Versuch in %d s (Versuch %d von %d)"), left, r.attempts+1, len(reconnectWaits)))
	}
	out := head + "\n\n" + line
	if r.lastErr != "" {
		out += "\n" + st.Muted.Render(i18n.T("Letzter Fehler: ")+trunc(r.lastErr, 70))
	}
	if s.mux.kind != "" && !r.netDev {
		out += "\n" + st.OK.Render(i18n.T("Danach wird die ")+s.mux.label()+i18n.T(" wieder angehängt."))
	}
	if n := len(r.tunnels); n > 0 {
		out += "\n" + st.Muted.Render(plural(n, i18n.T("Tunnel startet"), i18n.T("Tunnel starten"))+i18n.T(" wieder."))
	}
	return out + "\n\n" + a.hints(st.Muted, [2]string{"⏎", i18n.T("jetzt versuchen")}, [2]string{"Esc", i18n.T("abbrechen")},
		[2]string{"x", i18n.T("Tab schließen")}, [2]string{a.keys.label(actManager), i18n.T("Manager")})
}
