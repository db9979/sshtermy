package ui

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// „Testen“ im Verbindungsformular (Strg+T): Erreichbarkeit, Host-Key und
// Anmeldung prüfen, bevor gespeichert wird. Gespeichert wird dabei nichts
// außer einem bestätigten Host-Key (wie beim ersten Verbinden).

type connTestMsg struct {
	conn domain.Connection
	err  error
	step int  // erreichter Schritt (sshx.StepConnect …)
	tcp  bool // nur Port geprüft (Telnet, WinRM)
}

func (a *App) startConnTest() tea.Cmd {
	c, err := a.connFromForm()
	if err != nil {
		a.form.err = err.Error()
		return nil
	}
	switch c.Protocol {
	case domain.ProtoLocal, domain.ProtoSerial, domain.ProtoWatch:
		a.flash(i18n.T("Für diese Verbindungsart gibt es keinen Test"))
		return nil
	}
	if strings.TrimSpace(c.Host) == "" && c.RelayID == "" {
		a.form.err = i18n.T("Host fehlt")
		return nil
	}
	a.form.err = ""
	a.modal = &modal{kind: mBusy, title: i18n.T("Verbindung testen"), body: i18n.T("Prüfe ") + connTarget(c) + " …"}
	if c.Protocol.Raw() {
		return func() tea.Msg {
			d := net.Dialer{Timeout: 8 * time.Second}
			conn, err := d.Dial("tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
			if err == nil {
				conn.Close()
			}
			return connTestMsg{conn: c, err: err, tcp: true}
		}
	}
	opts := a.dialOpts()
	var step atomic.Int32
	opts.Step = func(n int) { step.Store(int32(n)) }
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cl, err := sshx.Dial(ctx, c, opts)
		if err == nil {
			cl.Close()
			step.Store(int32(sshx.StepShell))
		}
		return connTestMsg{conn: c, err: err, step: int(step.Load())}
	}
}

func connTarget(c domain.Connection) string {
	if c.Port > 0 {
		return c.Host + ":" + strconv.Itoa(c.Port)
	}
	return c.Host
}

// connTestResult zeigt das Ergebnis als Liste mit Haken.
func (a *App) connTestResult(m connTestMsg) {
	ok, bad := "✓ ", "× "
	var lines []string
	if m.tcp {
		if m.err == nil {
			lines = append(lines, ok+i18n.F("%s erreichbar", connTarget(m.conn)), "", i18n.T("Die Anmeldung prüft sshTermy beim Verbinden."))
		}
	} else {
		names := []string{i18n.T("Erreichbar"), i18n.T("Host-Key"), i18n.T("Anmeldung")}
		reached := m.step
		if m.err == nil {
			reached = len(names)
		}
		for i, n := range names {
			switch {
			case i < reached:
				lines = append(lines, ok+n)
			case i == reached && m.err != nil:
				lines = append(lines, bad+n)
			default:
				lines = append(lines, "· "+n)
			}
		}
	}
	title := i18n.T("Test erfolgreich")
	if m.err != nil {
		title = i18n.T("Test fehlgeschlagen")
		p := explainConnErr(m.err, m.conn)
		if m.tcp {
			lines = append(lines, bad+p.title)
		}
		lines = append(lines, "", p.title)
		if p.hint != "" {
			lines = append(lines, p.hint)
		}
		lines = append(lines, "", m.err.Error())
	} else if !m.tcp {
		lines = append(lines, "", i18n.T("Alles bereit: Strg+S speichert."))
	}
	a.modal = &modal{kind: mInfo, title: title, body: strings.Join(lines, "\n")}
}
