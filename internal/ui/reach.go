package ui

import (
	"context"
	"errors"
	"image/color"
	"net"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Online-Punkt im Manager: Solange der Manager offen ist, prüft sshTermy
// jede Minute per TCP-Verbindungsaufbau auf den SSH-Port, ob ein Host
// erreichbar ist. Es wird kein SSH-Handshake gemacht und nichts angemeldet,
// die Verbindung wird sofort wieder geschlossen. Hosts hinter einem
// Jump-Host bleiben „unbekannt“, weil sie von hier aus nicht direkt
// erreichbar sein müssen. Abschalten über die Befehlspalette.

const (
	reachEvery   = time.Minute
	reachTimeout = 2 * time.Second
	reachPar     = 16
)

type reachResult struct {
	ok  bool
	rtt time.Duration
	err string
	at  time.Time
}

type reachState struct {
	res     map[string]reachResult // Connection.ID → letzte Prüfung
	running bool
	next    time.Time
}

type reachMsg struct{ res map[string]reachResult }

func (a *App) reachEnabled() bool { return !a.st.Settings().NoReachCheck }

// reachTick: einmal pro Sekunde; startet eine Runde, wenn fällig.
func (a *App) reachTick() tea.Cmd {
	if a.scr != scrManager || a.intro != nil || !a.reachEnabled() || a.reach.running || time.Now().Before(a.reach.next) {
		return nil
	}
	var todo []domain.Connection
	for _, c := range a.st.Connections() {
		if c.JumpHostID == "" && c.Host != "" && !a.connected(c.ID) {
			todo = append(todo, c)
		}
	}
	a.reach.next = time.Now().Add(reachEvery)
	if len(todo) == 0 {
		return nil
	}
	a.reach.running = true
	return func() tea.Msg { return reachMsg{checkReach(todo)} }
}

func checkReach(conns []domain.Connection) map[string]reachResult {
	out := make(map[string]reachResult, len(conns))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, reachPar)
	for _, c := range conns {
		wg.Add(1)
		sem <- struct{}{}
		go func(c domain.Connection) {
			defer func() { <-sem; wg.Done() }()
			ctx, cancel := context.WithTimeout(context.Background(), reachTimeout)
			defer cancel()
			t0 := time.Now()
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", c.Addr())
			r := reachResult{at: time.Now(), rtt: time.Since(t0)}
			if err == nil {
				conn.Close()
				r.ok = true
			} else {
				r.err = reachReason(err)
			}
			mu.Lock()
			out[c.ID] = r
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	return out
}

// reachReason: kurzer, verständlicher Grund statt der Go-Fehlermeldung.
func reachReason(err error) string {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return "timeout"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	return "refused"
}

func (a *App) onReach(m reachMsg) {
	a.reach.running = false
	if a.reach.res == nil {
		a.reach.res = map[string]reachResult{}
	}
	for id, r := range m.res {
		a.reach.res[id] = r
	}
}

// reachDot: Punkt vor der Verbindung. ◉ verbunden, ● grün erreichbar,
// ● rot nicht erreichbar, ○ unbekannt.
func (a *App) reachDot(c domain.Connection) (string, color.Color) {
	t := a.s.T
	if a.connected(c.ID) {
		return "◉", t.OK
	}
	r, ok := a.reach.res[c.ID]
	switch {
	case !a.reachEnabled() || !ok:
		return "○", t.Muted
	case r.ok:
		return "●", t.OK
	}
	return "●", t.Danger
}

// reachText: Zeile für die Details rechts.
func (a *App) reachText(c domain.Connection) string {
	s := a.s
	if a.connected(c.ID) {
		return s.OK.Render(i18n.T("◉ verbunden"))
	}
	if c.JumpHostID != "" {
		return s.Muted.Render(i18n.T("○ über Jump-Host, nicht direkt geprüft"))
	}
	if c.Protocol == domain.ProtoSerial {
		return s.Muted.Render(i18n.T("○ serielle Konsole, Kabel wird beim Verbinden gesucht"))
	}
	if !a.reachEnabled() {
		return s.Muted.Render(i18n.T("○ Prüfung aus (Befehlspalette)"))
	}
	r, ok := a.reach.res[c.ID]
	if !ok {
		return s.Muted.Render(i18n.T("◌ wird geprüft …"))
	}
	ago := fmtAgo(time.Since(r.at))
	if r.ok {
		return s.OK.Render(i18n.T("● erreichbar")) + s.Muted.Render(" · "+fmtLatency(r.rtt)+" · "+ago)
	}
	why := map[string]string{
		"timeout": i18n.T("keine Antwort"),
		"dns":     i18n.T("Name nicht gefunden"),
		"refused": i18n.T("Port abgewiesen"),
	}[r.err]
	return s.Danger.Render(i18n.T("● nicht erreichbar")) + s.Muted.Render(" · "+why+" · "+ago)
}

func fmtAgo(d time.Duration) string {
	switch {
	case d < 10*time.Second:
		return i18n.T("gerade eben")
	case d < time.Minute:
		return i18n.F("vor %d s", int(d.Seconds()))
	}
	return i18n.F("vor %d min", int(d.Minutes()))
}

func (a *App) toggleReach() tea.Cmd {
	on := !a.reachEnabled()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoReachCheck = !on }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	a.reach.next = time.Time{}
	if on {
		a.flash(i18n.T("Erreichbarkeit an: der Manager zeigt pro Host einen Online-Punkt."))
	} else {
		a.flash(i18n.T("Erreichbarkeit aus: sshTermy baut keine Prüfverbindungen mehr auf."))
	}
	return nil
}

func reachLabel(on bool) string {
	if on {
		return i18n.T("Erreichbarkeit der Hosts nicht mehr prüfen")
	}
	return i18n.T("Erreichbarkeit der Hosts prüfen (Online-Punkt)")
}
