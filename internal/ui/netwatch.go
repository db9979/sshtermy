package ui

import (
	"net"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/debuglog"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/netwatch"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Verbindungen überstehen Ruhezustand und Netzwechsel: netwatch meldet
// beides, dann prüft sshTermy sofort jede Leitung (statt bis zu 30 s auf
// den Keepalive zu warten), wartende Neuverbindungen starten gleich, und
// ohne Netz zählen Versuche nicht, bis wieder eine Adresse da ist.

const netWatchEvery = 3 * time.Second

// netWatchCheck ist austauschbar für Tests.
var netWatchCheck = func(c *sshx.Client) { c.Check(4 * time.Second) }

func (a *App) netWatchTick() tea.Cmd {
	if a.nw == nil {
		a.nw = netwatch.New()
	}
	if time.Since(a.nwAt) < netWatchEvery && !a.nwAt.IsZero() {
		return nil
	}
	a.nwAt = time.Now()
	ev, _ := a.nw.Check(time.Now())
	return a.onNetEvent(ev)
}

func (a *App) onNetEvent(ev netwatch.Event) tea.Cmd {
	if ev == netwatch.None || !a.autoReconnect() {
		return nil
	}
	what := "Netzwechsel"
	if ev == netwatch.Resumed {
		what = "Ruhezustand beendet"
	}
	debuglog.Printf("net", "%s: Verbindungen werden geprüft", what)
	a.auditAdd(audit.EvSettings, "", what)
	var checks []*sshx.Client
	waiting := 0
	for _, s := range a.sessions {
		switch {
		case s.state == stReconnect && !s.rc.dialing:
			s.rc.due, s.rc.attempts = time.Now(), 0 // neue Runde, sofort
			waiting++
		case s.state == stConnected && s.client != nil:
			checks = append(checks, s.client)
		}
	}
	if len(checks) == 0 && waiting == 0 {
		return nil
	}
	if ev == netwatch.Resumed {
		a.flash(i18n.T("Ruhezustand beendet: Verbindungen werden geprüft"))
	} else {
		a.flash(i18n.T("Netzwerk gewechselt: Verbindungen werden geprüft"))
	}
	return func() tea.Msg {
		for _, c := range checks {
			go netWatchCheck(c) // abgerissene melden sich als shellClosed → Auto-Reconnect
		}
		return nil
	}
}

// offlineWait: kein Netz und das Ziel liegt nicht auf diesem Rechner →
// Versuch verschieben, ohne ihn zu zählen.
func (a *App) offlineWait(s *session) bool {
	if a.nw == nil || a.nw.Online() || !needsNetwork(s.conn) {
		return false
	}
	s.rc.lastErr = i18n.T("kein Netz, wartet auf eine Verbindung …")
	s.rc.due = time.Now().Add(2 * time.Second)
	return true
}

func needsNetwork(c domain.Connection) bool {
	if c.Protocol == domain.ProtoLocal || c.Protocol == domain.ProtoSerial {
		return false
	}
	h := strings.ToLower(strings.Trim(c.Host, "[]"))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return false
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}
