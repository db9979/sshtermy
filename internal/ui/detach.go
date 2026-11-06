package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/detach"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
	"github.com/db9979/sshtermy/internal/version"
)

// Tab in eigenes Fenster (⇧F10): Die Sitzung bleibt in diesem sshTermy, ein
// neues Terminalfenster dockt über einen lokalen Kanal an (internal/detach).
// Solange es offen ist, gehört ihm die Fenstergröße; der Emulator hier liest
// weiter mit (KI-Kontext, Suche, Aufnahme). Fenster schließen oder hier ⏎
// holt den Tab zurück.

type detachState struct {
	srv        *detach.Server
	conn       *detach.Conn // nil, solange das Fenster noch startet
	cols, rows int
	since      time.Time
}

type detachConnMsg struct {
	sid        int
	srv        *detach.Server
	conn       *detach.Conn
	cols, rows int
	err        error
}
type detachResizeMsg struct {
	sid        int
	conn       *detach.Conn
	cols, rows int
}
type detachEndMsg struct {
	sid  int
	conn *detach.Conn
}

// attachTexts: Meldungen im neuen Fenster in der eingestellten Sprache.
func attachTexts() {
	detach.MsgConnectFailed = i18n.T("Keine Verbindung zu sshTermy: %v")
	detach.MsgLost = i18n.T("Die Verbindung zu sshTermy ist weg (Hauptfenster geschlossen?).")
	detach.MsgPressKey = i18n.T("Taste drücken zum Schließen …")
}

// Attach läuft im neuen Fenster („sshtermy attach <ordner>“).
func Attach(dir string) int {
	attachTexts()
	return detach.Attach(dir)
}

func (s *session) detached() bool { return s.detach != nil }

// toggleDetach: abtrennen oder zurückholen.
func (a *App) toggleDetach(s *session) tea.Cmd {
	if s.detached() {
		a.reattach(s, i18n.T("Tab in sshTermy zurückgeholt."))
		return nil
	}
	if s.state != stConnected || s.term == nil {
		a.flash(i18n.T("Nur eine verbundene Sitzung lässt sich in ein eigenes Fenster abtrennen"))
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		a.flash(i18n.T("Eigenes Fenster geht nicht: ") + err.Error())
		return nil
	}
	srv, err := detach.Listen()
	if err != nil {
		a.flash(i18n.T("Eigenes Fenster geht nicht: ") + err.Error())
		return nil
	}
	title := version.Name + " · " + s.conn.Alias
	if err := openWindow(title, []string{exe, "attach", srv.Dir()}); err != nil {
		srv.Close()
		if errors.Is(err, detach.ErrNoTerminal) {
			a.flash(i18n.T("Kein Terminalprogramm gefunden. Eigenes festlegen mit SSHTERMY_TERMINAL, z. B. \"kitty {cmd}\""))
		} else {
			a.flash(i18n.T("Fenster ließ sich nicht öffnen: ") + err.Error())
		}
		return nil
	}
	s.detach = &detachState{srv: srv, since: time.Now()}
	s.panelFocus = false
	sid, ev := s.id, a.events
	go func() {
		c, cols, rows, err := srv.Accept(20 * time.Second)
		ev <- detachConnMsg{sid, srv, c, cols, rows, err}
	}()
	return nil
}

// openWindow: austauschbar für Tests.
var openWindow = detach.OpenWindow

func (a *App) onDetachConn(m detachConnMsg) {
	s := a.byID(m.sid)
	if s == nil || s.detach == nil || s.detach.srv != m.srv {
		if m.conn != nil {
			m.conn.Bye(i18n.T("Der Tab ist nicht mehr offen."))
		}
		m.srv.Close()
		return
	}
	if m.err != nil {
		s.detach = nil
		m.srv.Close()
		if errors.Is(m.err, detach.ErrTimeout) {
			a.flash(i18n.T("Das neue Fenster hat sich nicht gemeldet, der Tab bleibt hier"))
		} else {
			a.flash(i18n.T("Eigenes Fenster geht nicht: ") + m.err.Error())
		}
		return
	}
	d := s.detach
	d.conn, d.cols, d.rows = m.conn, max(20, m.cols), max(3, m.rows)
	_ = m.conn.Title(version.Name + " · " + s.conn.Alias)
	_, _ = m.conn.Write([]byte(replayScreen(s.term)))
	s.detOut.Store(m.conn)
	s.term.MuteReplies(true) // das echte Terminal dort antwortet selbst
	s.scroll, s.sel, s.selecting = 0, nil, false
	a.resize(s)
	a.flash("↑ " + s.conn.Alias + i18n.T(" läuft jetzt in eigenem Fenster"))

	// Tasten und Größe aus dem Fenster
	sid, ev, c := s.id, a.events, m.conn
	go func() {
		for {
			e, err := c.Read()
			if err != nil {
				ev <- detachEndMsg{sid, c}
				return
			}
			if e.Cols > 0 {
				ev <- detachResizeMsg{sid, c, e.Cols, e.Rows}
				continue
			}
			_, _ = shellWriter{s}.Write(e.Input) // wartet kurz, falls gerade neu verbunden wird
		}
	}()
}

func (a *App) onDetachResize(m detachResizeMsg) {
	s := a.byID(m.sid)
	if s == nil || s.detach == nil || s.detach.conn != m.conn {
		return
	}
	s.detach.cols, s.detach.rows = max(20, m.cols), max(3, m.rows)
	a.resize(s)
}

func (a *App) onDetachEnd(m detachEndMsg) {
	s := a.byID(m.sid)
	if s == nil || s.detach == nil || s.detach.conn != m.conn {
		return
	}
	a.reattach(s, "")
	a.flash("↓ " + s.conn.Alias + i18n.T(" ist zurück im Tab"))
}

// reattach: Fenster entlassen (mit Meldung) und Größe wieder übernehmen.
func (a *App) reattach(s *session, msg string) {
	s.endDetach(msg)
	a.resize(s)
}

// endDetach räumt ab; aufrufbar auch beim Schließen der Sitzung.
func (s *session) endDetach(msg string) {
	d := s.detach
	if d == nil {
		return
	}
	s.detach = nil
	s.detOut.Store(nil)
	if s.term != nil {
		s.term.MuteReplies(false)
	}
	if d.conn != nil {
		d.conn.Bye(msg)
	}
	d.srv.Close()
}

// detachNotice: Hinweiszeile ins abgetrennte Fenster (z. B. beim Neuverbinden).
func (s *session) detachNotice(text string) {
	if c := s.detOut.Load(); c != nil {
		_, _ = c.Write([]byte("\r\n\x1b[0;33m[" + version.Name + "] " + text + "\x1b[0m\r\n"))
	}
}

// replayScreen: aktueller Bildschirminhalt für das neue Fenster, damit es
// nicht leer startet. Danach sorgt die neue Größe beim Server für ein
// Neuzeichnen (Shell-Prompt, vim, htop …).
func replayScreen(t *termemu.Term) string {
	var b strings.Builder
	if t.AltScreen() {
		b.WriteString("\x1b[?1049h")
	}
	b.WriteString("\x1b[0m\x1b[H\x1b[2J")
	lines := strings.Split(t.Render(0, nil), "\n")
	for i, l := range lines {
		if i > 0 {
			b.WriteString("\x1b[0m\r\n")
		}
		b.WriteString(l)
	}
	b.WriteString("\x1b[0m")
	switch t.MouseTracking() {
	case termemu.MousePress, termemu.MouseButtons:
		b.WriteString("\x1b[?1000h\x1b[?1006h")
	case termemu.MouseDrag:
		b.WriteString("\x1b[?1002h\x1b[?1006h")
	case termemu.MouseAny:
		b.WriteString("\x1b[?1003h\x1b[?1006h")
	}
	x, y, vis := t.Cursor()
	fmt.Fprintf(&b, "\x1b[%d;%dH", y+1, x+1)
	if !vis {
		b.WriteString("\x1b[?25l")
	}
	return b.String()
}

// detachedView: Platzhalter im Hauptfenster.
func (a *App) detachedView(s *session, cols, rows int) string {
	st := a.s
	d := s.detach
	var head string
	if d.conn == nil {
		head = st.AI.Render(i18n.T("⠿ Öffne eigenes Fenster für ") + s.conn.Alias + " …")
	} else {
		head = st.AI.Render("↑ "+s.conn.Alias+i18n.T(" läuft in eigenem Fenster")) + "\n" +
			st.Muted.Render(fmt.Sprintf(i18n.T("seit %s · %d×%d · Fenster schließen holt den Tab zurück"), d.since.Format("15:04"), d.cols, d.rows))
	}
	hints := a.hints(st.Muted, [2]string{"⏎", i18n.T("hierher zurückholen")}, [2]string{a.keys.label(actManager), i18n.T("Manager")})
	return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, head, "", hints))
}

// detachedKey: Tasten im Hauptfenster, solange der Tab draußen ist.
func (a *App) detachedKey(s *session, key string) tea.Cmd {
	if key == "enter" || a.keys.is(actDetach, key) {
		a.reattach(s, i18n.T("Tab in sshTermy zurückgeholt."))
		a.flash("↓ " + s.conn.Alias + i18n.T(" ist zurück im Tab"))
	}
	return nil
}
