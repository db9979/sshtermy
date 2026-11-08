package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/version"
)

// Audit-Protokoll (Einstellungen › Sicherheit): wer hat wann was getan,
// verkettet und mit einem Schlüssel aus dem Store signiert (internal/audit).
// Passwörter und Bildschirminhalte stehen nie darin; Befehle mit
// führendem Leerzeichen erscheinen nur als „(privat)“.

func (a *App) auditOn() bool { return a.audit != nil }

// startAudit nach dem Entsperren.
func (a *App) startAudit() {
	if !a.st.Settings().Audit || a.audit != nil {
		return
	}
	if err := a.openAudit(); err != nil {
		a.flashErr(i18n.T("Audit-Protokoll: ") + err.Error())
		return
	}
	a.audit.Add(audit.EvStart, "", version.Long())
}

func (a *App) openAudit() error {
	key, err := a.st.V.DeriveKey("audit-log")
	if err != nil {
		return err
	}
	l, err := audit.Open(a.st.Dir, key)
	if err != nil {
		return err
	}
	a.audit = l
	llm.OnRequest(func(r llm.Request) {
		l.Add(audit.EvAI, "", fmt.Sprintf("%s %s %s", r.Provider, r.Host, r.Model))
	})
	return nil
}

func (a *App) toggleAudit() tea.Cmd {
	on := !a.auditOn()
	if !on {
		a.audit.Add(audit.EvSettings, "", "Audit-Protokoll aus")
	}
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Audit = on }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	if !on {
		a.audit = nil
		llm.OnRequest(nil)
		a.flash(i18n.T("Audit-Protokoll aus"))
		return nil
	}
	if err := a.openAudit(); err != nil {
		a.flashErr(i18n.T("Audit-Protokoll: ") + err.Error())
		return nil
	}
	a.audit.Add(audit.EvSettings, "", "Audit-Protokoll an")
	a.flash(i18n.T("Audit-Protokoll an: ") + a.audit.Dir())
	return nil
}

// auditConn: Verbindung als „Alias (user@host:port)“.
func auditConn(c domain.Connection) string {
	switch c.Protocol {
	case domain.ProtoLocal:
		return c.Alias + " (lokal)"
	case domain.ProtoSerial:
		return fmt.Sprintf("%s (%s)", c.Alias, c.SerialPort)
	}
	t := c.Host
	if c.Username != "" {
		t = c.Username + "@" + t
	}
	if c.Port != 0 && c.Port != 22 {
		t = fmt.Sprintf("%s:%d", t, c.Port)
	}
	return c.Alias + " (" + t + ")"
}

func (a *App) auditSess(sid int, ev, detail string) {
	if a.audit == nil {
		return
	}
	target := fmt.Sprintf("#%d", sid)
	if s := a.byID(sid); s != nil {
		target = auditConn(s.conn)
	}
	a.audit.Add(ev, target, detail)
}

// auditMsg: Ereignisse aus Nachrichten (vor a.update, wie debugMsg).
func (a *App) auditMsg(msg tea.Msg) {
	if a.audit == nil {
		return
	}
	switch m := msg.(type) {
	case connectedMsg:
		a.auditSess(m.sid, audit.EvConnect, "")
	case connectErrMsg:
		a.auditSess(m.sid, audit.EvConnFail, errText(m.err))
	case shellClosedMsg:
		d := ""
		if m.dropped {
			d = "abgerissen: " + errText(m.err)
		}
		a.auditSess(m.sid, audit.EvDisconnect, d)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// auditCommand aus finishCommand.
func (a *App) auditCommand(s *session, r *cmdRun, failed bool, exit *int) {
	if a.audit == nil {
		return
	}
	ev, cmd := audit.EvCommand, r.cmd
	if r.agent {
		ev = audit.EvAgent
	}
	if r.private {
		cmd = "(privat)"
	}
	switch {
	case r.interrupted:
		cmd += "  [abgebrochen]"
	case failed && exit != nil:
		cmd += fmt.Sprintf("  [Fehler, Exit %d]", *exit)
	case failed:
		cmd += "  [Fehler]"
	}
	a.audit.Add(ev, auditConn(s.conn), cmd)
}

func (a *App) auditBroadcast(targets []domain.Connection, steps []string) {
	if a.audit == nil {
		return
	}
	names := make([]string, len(targets))
	for i, c := range targets {
		names[i] = auditConn(c)
	}
	a.audit.Add(audit.EvBroadcast, strings.Join(names, ", "), strings.Join(steps, " ⏎ "))
}

func (a *App) auditAdd(ev, target, detail string) {
	if a.audit != nil {
		a.audit.Add(ev, target, detail)
	}
}

// exportAudit prüft die Kette und legt eine lesbare Fassung mit dem
// Prüfergebnis in Downloads.
func (a *App) exportAudit() tea.Cmd {
	if a.audit == nil {
		a.flash(i18n.T("Audit-Protokoll ist aus (Einstellungen › Sicherheit)"))
		return nil
	}
	r := a.audit.Verify()
	var b strings.Builder
	fmt.Fprintf(&b, "%s · %s\n", version.Long(), time.Now().Format(time.RFC3339))
	if r.OK() {
		fmt.Fprintf(&b, i18n.T("Prüfung: %d Einträge, Kette lückenlos und unverändert")+"\n\n", len(r.Entries))
	} else {
		fmt.Fprintf(&b, i18n.T("Prüfung: %d Auffälligkeiten")+"\n", len(r.Problems))
		for _, p := range r.Problems {
			b.WriteString("  × " + p + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(audit.Text(r.Entries))
	p := filepath.Join(exportDir(), "sshtermy-audit-"+time.Now().Format("2006-01-02-1504")+".txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		a.flashErr(i18n.T("Speichern fehlgeschlagen: ") + err.Error())
		return nil
	}
	if r.OK() {
		a.flashOK(fmt.Sprintf(i18n.T("Audit-Protokoll geprüft (%d Einträge, unverändert) und gespeichert: %s"), len(r.Entries), p))
	} else {
		a.flashErr(fmt.Sprintf(i18n.T("Audit-Protokoll: %d Auffälligkeiten! Bericht: %s"), len(r.Problems), p))
	}
	return nil
}

func (a *App) auditTransfer(s *session, paths []string, dst string, up bool) {
	if a.audit == nil {
		return
	}
	ev := audit.EvDownload
	if up {
		ev = audit.EvUpload
	}
	a.audit.Add(ev, auditConn(s.conn), strings.Join(paths, ", ")+" → "+dst)
}
