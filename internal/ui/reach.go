package ui

import (
	"context"
	"errors"
	"image/color"
	"math"
	"net"
	"sort"
	"strings"
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
	reachPar     = 16 // gleichzeitig, bei vielen Hosts mehr (reachParFor)
	reachParMax  = 64
	reachKickGap = 3 * time.Second // neue sichtbare Zeilen: höchstens so oft außer der Reihe
)

// reachParFor: bei 1000 Hosts dauert eine Runde mit 16 gleichzeitigen
// Prüfungen und 2 s Zeitlimit sonst über zwei Minuten.
func reachParFor(n int) int { return min(reachParMax, max(reachPar, n/16)) }

// reachPause: Pause bis zur nächsten Runde, mindestens eine Minute, bei
// langen Runden das Dreifache der Dauer (Netz und Firewalls nicht fluten).
func reachPause(took time.Duration) time.Duration { return max(reachEvery, 3*took) }

type reachResult struct {
	ok  bool
	rtt time.Duration
	err string
	at  time.Time
}

type reachState struct {
	res     map[string]reachResult     // Connection.ID → letzte Prüfung
	hist    map[string][]time.Duration // Antwortzeiten der letzten Prüfungen (Verlauf)
	downAt  map[string]time.Time       // gerade ausgefallen (Punkt blinkt)
	running bool
	next    time.Time
	kick    time.Time // letzte Runde (für sofortige Prüfung neuer Zeilen)
	started time.Time
}

type reachMsg struct{ res map[string]reachResult }

func (a *App) reachEnabled() bool { return !a.st.Settings().NoReachCheck }

// Umfang der Prüfung: aus, nur sichtbare Verbindungen (Standard), alle.
const (
	reachOff = iota
	reachVisible
	reachAll
)

func (a *App) reachMode() int {
	switch set := a.st.Settings(); {
	case set.NoReachCheck:
		return reachOff
	case set.ReachAll:
		return reachAll
	}
	return reachVisible
}

func reachLabel(mode int) string {
	return [...]string{i18n.T("aus"), i18n.T("sichtbare"), i18n.T("alle")}[mode]
}

// reachVisibleIDs: Verbindungen, die im Manager gerade als Zeile stehen
// (zugeklappte Ordner verbergen ihre, die Suche zeigt nur Treffer).
func (a *App) reachVisibleIDs() map[string]bool {
	ids := map[string]bool{}
	for _, r := range a.mgrRows() {
		if !r.isFolder() {
			ids[r.conn.ID] = true
		}
	}
	return ids
}

// reachTick: einmal pro Sekunde; startet eine Runde, wenn fällig. Steht
// eine sichtbare Verbindung noch ganz ohne Ergebnis da (Ordner gerade
// aufgeklappt, Suche, neu angelegt), kommt die Runde sofort.
func (a *App) reachTick() tea.Cmd {
	mode := a.reachMode()
	if a.scr != scrManager || a.intro != nil || mode == reachOff || a.reach.running {
		return nil
	}
	var vis map[string]bool
	if mode == reachVisible {
		vis = a.reachVisibleIDs()
	}
	var todo, via []domain.Connection
	fresh := false
	for _, c := range a.st.Connections() {
		if vis != nil && !vis[c.ID] {
			continue
		}
		switch {
		case a.connected(c.ID):
		case c.ViaRelay():
			via = append(via, c) // Online laut Vermittler (relay.go)
		case c.JumpHostID == "" && c.Host != "":
			todo = append(todo, c)
			if _, ok := a.reach.res[c.ID]; !ok {
				fresh = true
			}
		}
	}
	now := time.Now()
	if now.Before(a.reach.next) && !(fresh && now.Sub(a.reach.kick) >= reachKickGap) {
		return nil
	}
	a.reach.next = now.Add(reachEvery)
	if len(todo) == 0 && len(via) == 0 {
		return nil
	}
	a.reach.kick = now
	a.reach.running, a.reach.started = true, time.Now()
	return func() tea.Msg {
		res := checkReach(todo)
		if len(via) > 0 {
			for id, r := range a.relayReach(via) {
				res[id] = r
			}
		}
		return reachMsg{res}
	}
}

func checkReach(conns []domain.Connection) map[string]reachResult {
	out := make(map[string]reachResult, len(conns))
	// gleiche Adresse (mehrere Benutzer auf einem Host): nur einmal prüfen
	byAddr := map[string][]string{}
	var uniq []domain.Connection
	for _, c := range conns {
		if _, seen := byAddr[c.Addr()]; !seen {
			uniq = append(uniq, c)
		}
		byAddr[c.Addr()] = append(byAddr[c.Addr()], c.ID)
	}
	defer func() {
		for _, c := range uniq {
			for _, id := range byAddr[c.Addr()][1:] {
				out[id] = out[c.ID]
			}
		}
	}()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, reachParFor(len(uniq)))
	for _, c := range uniq {
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
	if !a.reach.started.IsZero() {
		a.reach.next = time.Now().Add(reachPause(time.Since(a.reach.started)))
	}
	if a.reach.res == nil {
		a.reach.res = map[string]reachResult{}
		a.reach.hist = map[string][]time.Duration{}
		a.reach.downAt = map[string]time.Time{}
	}
	var down []string
	for id, r := range m.res {
		if old, ok := a.reach.res[id]; ok && old.ok && !r.ok {
			a.reach.downAt[id] = time.Now()
			if c, ok := a.st.Connection(id); ok {
				down = append(down, c.Alias)
			}
		}
		a.reach.res[id] = r
		h := a.reach.hist[id]
		if r.ok {
			h = append(h, r.rtt)
		} else {
			h = append(h, -1) // Lücke im Verlauf
		}
		if len(h) > reachHist {
			h = h[len(h)-reachHist:]
		}
		a.reach.hist[id] = h
	}
	// war erreichbar, jetzt nicht mehr: einmal melden (nur, solange der
	// Manager offen ist, dort laufen die Prüfungen)
	if len(down) > 0 {
		sort.Strings(down)
		names := strings.Join(down[:min(3, len(down))], ", ")
		if len(down) > 3 {
			names += i18n.F(" und %d weitere", len(down)-3)
		}
		a.flashErr(i18n.T("Antwortet nicht mehr: ") + names)
	}
}

// reachHist: so viele Prüfungen zeigt der Verlauf (eine pro Minute).
const reachHist = 20

// reachSpark: Verlauf der Antwortzeiten, Ausfälle als ×.
func (a *App) reachSpark(id string) string {
	h := a.reach.hist[id]
	if len(h) < 2 {
		return ""
	}
	vals := make([]float64, 0, len(h))
	for _, d := range h {
		vals = append(vals, math.Max(0, float64(d.Microseconds())))
	}
	sp := []rune(sparkline(vals))
	var b strings.Builder
	for i, d := range h {
		if d < 0 {
			b.WriteString(a.s.Danger.Render("×"))
		} else {
			b.WriteString(a.s.Accent.Render(string(sp[i])))
		}
	}
	return b.String()
}

// reachDot: Punkt vor der Verbindung. ◉ verbunden, ● grün erreichbar,
// ● rot nicht erreichbar, ○ unbekannt.
func (a *App) reachDot(c domain.Connection) (string, color.Color) {
	t := a.s.T
	if a.connected(c.ID) {
		return "◉", t.OK
	}
	if c.Protocol == domain.ProtoLocal {
		return "●", t.OK
	}
	r, ok := a.reach.res[c.ID]
	switch {
	case !a.reachEnabled() || !ok:
		return "○", t.Muted
	case r.ok:
		if motionLevel() == 2 {
			// „atmet“ leicht, jeder Host etwas versetzt
			h := 0
			for _, ch := range c.ID {
				h = (h*31 + int(ch)) % 2400
			}
			ph := time.Duration(h) * time.Millisecond
			return "●", mixColor(mixColor(t.OK, t.Bg, 0.45), t.OK, pulse(time.Now().Add(ph), 2400*time.Millisecond))
		}
		return "●", t.OK
	case accessible:
		return "×", t.Danger // nicht nur die Farbe unterscheidet
	}
	if at, ok := a.reach.downAt[c.ID]; ok && motionLevel() > 0 && time.Since(at) < downBlink {
		if (time.Since(at)/(300*time.Millisecond))%2 == 0 {
			return "×", t.Danger // gerade ausgefallen: blinkt kurz
		}
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
	if c.Protocol == domain.ProtoLocal {
		return s.OK.Render(i18n.T("● dieser Rechner")) + s.Muted.Render(" · "+localShellLabel(c.LocalShell))
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

// cycleReach: aus → sichtbare → alle (Palette › Einstellungen).
func (a *App) cycleReach(dir int) tea.Cmd {
	next := cycleIndex(a.reachMode(), 3, dir)
	if err := a.st.UpdateSettings(func(s *domain.Settings) {
		s.NoReachCheck, s.ReachAll = next == reachOff, next == reachAll
	}); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	a.reach.next = time.Time{}
	switch next {
	case reachOff:
		a.flash(i18n.T("Erreichbarkeit aus: sshTermy baut keine Prüfverbindungen mehr auf."))
	case reachVisible:
		a.flash(i18n.T("Online-Punkt: nur sichtbare Verbindungen, zugeklappte Ordner ruhen."))
	default:
		a.flash(i18n.T("Online-Punkt: alle Verbindungen, Ausfälle melden sich auch aus zugeklappten Ordnern."))
	}
	return nil
}
