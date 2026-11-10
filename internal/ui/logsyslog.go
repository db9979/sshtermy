package ui

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
)

// Syslog-Ziel am Gerät einrichten: sshTermy zeigt die Befehle für den
// Hersteller mit der eigenen IP und tippt sie nach Bestätigung sichtbar ein.
// Gespeichert wird am Gerät nichts (außer Junos commit), write memory bleibt
// beim Benutzer.

type syslogSetupMsg struct {
	sid int
	err error
}

// localIPFor: eigene Adresse, über die das Gerät erreichbar ist (UDP-Dial
// schickt nichts, wählt nur die Route).
var localIPFor = func(host string) string {
	c, err := net.DialTimeout("udp", net.JoinHostPort(host, "514"), 2*time.Second)
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.String()
	}
	return ""
}

func (a *App) askSyslogSetup(s *session) tea.Cmd {
	if s == nil || !s.netDevice() || s.state != stConnected {
		a.flash(i18n.T("Syslog einrichten geht nur bei Switches und Firewalls"))
		return nil
	}
	v := s.host.vendor
	if v == hostinfo.VendorUnknown {
		v = hostVendor(s.prof)
	}
	port := a.st.Settings().SyslogPort
	if port == 0 {
		port = 5514
	}
	ip := localIPFor(s.conn.Host)
	if ip == "" {
		a.flashErr(i18n.T("Eigene IP zum Gerät nicht gefunden (Verbindung über Jump-Host?)"))
		return nil
	}
	if configPrompt(s.term.CursorLine()) {
		a.flashErr(i18n.T("Der Switch ist im Konfigurationsmodus. Erst mit end bzw. exit zurück an den normalen Prompt."))
		return nil
	}
	cmds := logscan.SyslogSetup(v, ip, port)
	if len(cmds) == 0 {
		hint := i18n.T("Für dieses Gerät kennt sshTermy die Befehle nicht.")
		if v == hostinfo.VendorUniFi {
			hint = i18n.T("UniFi-Switches stellt man im UniFi-Controller ein: Einstellungen › System › Remote Logging (Syslog).")
		}
		a.modal = textModal(i18n.T("Syslog an sshTermy senden"), []string{hint, "", fmt.Sprintf(i18n.T("Ziel: %s, UDP-Port %d"), ip, port)})
		return nil
	}
	body := fmt.Sprintf(i18n.T("Das Gerät schickt danach Meldungen (ab Warnung) an %s:%d. sshTermy tippt diese Zeilen sichtbar ein:"), ip, port) +
		"\n\n  " + strings.Join(cmds, "\n  ") + "\n\n"
	if a.st.Settings().SyslogPort == 0 {
		body += fmt.Sprintf(i18n.T("Der Syslog-Empfang wird dabei auf UDP %d eingeschaltet."), port) + "\n"
	}
	body += i18n.T("Gespeichert wird am Gerät nichts: dauerhaft mit write memory (bzw. copy run start).") + "\n" +
		i18n.T("Läuft eine Firewall auf diesem Rechner, muss sie den UDP-Port durchlassen.")
	sid := s.id
	a.modal = confirmModal(i18n.T("Syslog an sshTermy senden"), body, func() tea.Cmd {
		if a.st.Settings().SyslogPort == 0 {
			_ = a.st.UpdateSettings(func(x *domain.Settings) { x.SyslogPort = port })
			a.watchTick()
		}
		return a.runSyslogSetup(sid, cmds, logscan.SyslogAbort(v))
	})
	return nil
}

func (a *App) runSyslogSetup(sid int, cmds, abort []string) tea.Cmd {
	s := a.byID(sid)
	if s == nil {
		return nil
	}
	if s.host.busy || s.term.AltScreen() || !hostinfo.IsPrompt(strings.TrimSpace(s.term.CursorLine())) {
		a.flashErr(i18n.T("Abfrage nur an einem leeren Prompt (z. B. switch#). Bitte erst Eingabe beenden oder ⏎ drücken."))
		return nil
	}
	a.scr = scrTerminal
	s.cap.hold() // bis alles getippt ist keine Tasten an diese Shell (shelllock.go)
	return func() tea.Msg {
		defer s.cap.release()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		// Fehler mitten in der Folge: nicht weitertippen, sondern verwerfen
		// und den Konfigurationsmodus verlassen.
		undo := func(err error) tea.Msg {
			if s.cap.aborted() {
				s.cap.hold() // Esc gilt der Folge, nicht dem Aufräumen
				defer s.cap.release()
			}
			uctx, ucancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer ucancel()
			for _, c := range abort {
				prompt := strings.TrimSpace(s.term.CursorLine())
				if _, e := shellRun(uctx, s, prompt, c, 10*time.Second); e != nil {
					break
				}
				waitPrompt(s, 3*time.Second)
			}
			return syslogSetupMsg{sid: sid, err: err}
		}
		for _, c := range cmds {
			// Der Prompt ändert sich im Konfigurationsmodus (switch(config)#).
			prompt := strings.TrimSpace(s.term.CursorLine())
			lines, err := shellRun(ctx, s, prompt, c, 20*time.Second)
			if err != nil {
				return undo(err)
			}
			if bad := cliError(lines); bad != "" {
				return undo(fmt.Errorf(i18n.T("Gerät meldet bei „%s“: %s (Einrichtung verworfen)"), c, bad))
			}
			waitPrompt(s, 3*time.Second)
		}
		return syslogSetupMsg{sid: sid}
	}
}

// waitPrompt: kurz warten, bis wieder ein Prompt dasteht.
func waitPrompt(s *session, limit time.Duration) {
	end := time.Now().Add(limit)
	for time.Now().Before(end) {
		if hostinfo.IsPrompt(strings.TrimSpace(s.term.CursorLine())) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (a *App) syslogSetupMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(syslogSetupMsg)
	if !ok {
		return nil, false
	}
	if m.err != nil {
		a.flashErr(i18n.T("Syslog einrichten: ") + m.err.Error())
		return nil, true
	}
	a.flashOK(fmt.Sprintf(i18n.T("Syslog eingerichtet (UDP %d). Mit write memory dauerhaft speichern."), a.st.Settings().SyslogPort))
	return nil, true
}
