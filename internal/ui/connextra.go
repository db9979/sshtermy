package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
	"github.com/db9979/sshtermy/internal/wol"
)

// Kleine Helfer je Verbindung: Startbefehl nach dem Anmelden, MAC-Adresse
// merken und Wake-on-LAN.

// ---------------------------------------------------------------- Startbefehl

// armStartCmd: nach dem Verbinden den Startbefehl vormerken. Nicht bei
// fester Sitzung (dort läuft die Shell in tmux/screen weiter) und nicht für
// die lokale Shell.
func (a *App) armStartCmd(s *session) {
	s.startPending = false
	c := s.conn
	if c.StartCmd == "" || c.Persist != "" || c.Protocol == domain.ProtoLocal || s.mux.kind != "" || s.muxPending {
		return
	}
	s.startPending = true
}

// runStartCmd tippt den Startbefehl, sobald der Prompt steht (aus dem
// Sekundentakt, wie resumeMux). Gefährliches wird nie von selbst getippt,
// auch wenn es über einen älteren Stand in die Verbindung gelangt ist.
func (a *App) runStartCmd(s *session) tea.Cmd {
	if !s.startPending || s.term == nil || s.shell == nil {
		return nil
	}
	ready := s.term.AtIdlePrompt() && time.Since(s.lastOutput) > 400*time.Millisecond
	if !ready && time.Since(s.connectedAt) < 6*time.Second {
		return nil
	}
	s.startPending = false
	cmd := s.conn.StartCmd
	if f := safety.Check(cmd, a.safetyKind(s)); len(f) > 0 {
		a.pushToast(toastWarn, i18n.F("Startbefehl nicht ausgeführt (%s). Bitte in der Verbindung ändern.", f[0].Reason))
		return nil
	}
	s.term.SendText(cmd + "\r")
	return nil
}

// ---------------------------------------------------------------- Wake-on-LAN

type macLearnedMsg struct {
	connID, mac string
}

type wakeDoneMsg struct {
	alias string
	sent  int
	err   error
}

// lookupMAC, wakeHost: Variablen für Tests.
var (
	lookupMAC = wol.Lookup
	wakeHost  = wol.Wake
)

// learnMAC: direkt nach dem Verbinden steht der Rechner in der ARP-Tabelle.
// Fehlt die MAC-Adresse noch, wird sie gemerkt, damit Aufwecken später ohne
// Eintippen geht. Nur für direkte Verbindungen im eigenen Netz.
func (a *App) learnMAC(s *session) tea.Cmd {
	c := s.conn
	if c.MAC != "" || c.ID == "" || c.Host == "" || c.JumpHostID != "" || c.ViaRelay() || c.Protocol.NoHost() {
		return nil
	}
	id, host := c.ID, c.Host
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if m := lookupMAC(ctx, host); m != "" {
			return macLearnedMsg{id, m}
		}
		return nil
	}
}

func (a *App) onMACLearned(m macLearnedMsg) {
	c, ok := a.st.Connection(m.connID)
	if !ok || c.MAC != "" {
		return
	}
	c.MAC = m.mac
	if _, err := a.st.UpsertConnection(c); err != nil {
		return
	}
	for _, s := range a.sessions {
		if s.conn.ID == m.connID {
			s.conn.MAC = m.mac
		}
	}
}

// canWake: Aufwecken ist möglich (MAC bekannt, direkte Verbindung).
func canWake(c domain.Connection) bool {
	return c.MAC != "" && c.JumpHostID == "" && !c.Protocol.NoHost()
}

// wake schickt das Magic Packet.
func (a *App) wake(c domain.Connection) tea.Cmd {
	if c.MAC == "" {
		a.flash(i18n.T("Aufwecken braucht die MAC-Adresse: einmal verbinden (sshTermy merkt sie sich) oder in der Verbindung unter „Erweitert“ eintragen."))
		return nil
	}
	alias, mac := c.Alias, c.MAC
	return func() tea.Msg {
		n, err := wakeHost(mac)
		return wakeDoneMsg{alias, n, err}
	}
}

func (a *App) onWakeDone(m wakeDoneMsg) {
	if m.err != nil {
		a.flashErr(fmt.Sprintf(i18n.T("Aufwecken von %s fehlgeschlagen: %v"), m.alias, m.err))
		return
	}
	a.flashOK(i18n.F("Weckruf an %s gesendet. Der Rechner braucht meist eine halbe bis zwei Minuten; der Punkt im Manager wird grün, sobald er erreichbar ist.", m.alias))
}
