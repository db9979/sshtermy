package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Logwache: solange eine Sitzung offen ist, folgt sshTermy ihren Logs
// (Linux: journalctl -f, Windows: Abfrage jede Minute über WinRM) und
// empfängt auf Wunsch Syslog von Switches und Firewalls. Gemeldet wird nur,
// was neu ist (nicht in der Grundlinie des Log-Blicks, nicht als Rauschen
// ausgeblendet) oder kritisch, jedes Muster höchstens einmal pro Stunde.

const (
	watchMaxAlerts = 200
	watchRepeat    = time.Hour
	watchRetry     = time.Minute
	watchWinPoll   = time.Minute
)

type watchAlert struct {
	At     time.Time
	Host   string
	ConnID string
	Sev    logscan.Sev
	Title  string
	Line   string
	Key    string
	Src    string // journal, winrm, syslog
	New    bool
	Count  int
}

type logStream struct {
	cancel context.CancelFunc
	client *sshx.Client
	ended  time.Time // Strom ist abgerissen (nicht von uns beendet)
}

type logWatchState struct {
	streams  map[int]*logStream
	sysPort  int
	sysStop  context.CancelFunc
	sysErr   string
	resolved map[string]string // IP → Verbindungs-ID
	alerts   []watchAlert
	seen     map[string]int // ConnID|Key → Index in alerts
	base     map[string]map[string]bool
	unread   int
	burst    map[string]watchBurst      // gleiche Zeilen in kurzer Zeit (Fehlanmeldungen)
	syslog   map[string][]logscan.Entry // empfangenes Syslog je Verbindung (nur im Speicher)
	hostIdx  map[string]string          // Host aus der Verbindung → ID
	hostAt   time.Time
	// dropped: Syslog von unbekannten Absendern, verworfen (IP → Anzahl)
	dropped map[string]int
}

// watchSyslogKeep: so viele empfangene Zeilen je Gerät für den Log-Blick.
const watchSyslogKeep = 5000

type watchBurst struct {
	n    int
	from time.Time
}

// watchItem: eine Zeile aus einem Strom.
type watchItem struct {
	sid  int    // Sitzung (0 = Syslog)
	addr string // Absender beim Syslog
	kind logscan.Kind
	e    logscan.Entry
}

type watchBatchMsg struct {
	items []watchItem
}

type watchEndMsg struct {
	sid    int
	client *sshx.Client
	err    error
}

type watchSysErrMsg struct {
	port int
	err  error
}

type watchResolvedMsg struct{ m map[string]string }

// watchBatcher sammelt Zeilen und schickt sie höchstens einmal pro Sekunde
// an die Oberfläche (journalctl kann viel auf einmal ausgeben).
type watchBatcher struct {
	mu    sync.Mutex
	items []watchItem
}

func (b *watchBatcher) add(it watchItem) {
	b.mu.Lock()
	if len(b.items) < 500 {
		b.items = append(b.items, it)
	}
	b.mu.Unlock()
}

func (b *watchBatcher) run(ctx context.Context, ev chan<- tea.Msg) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		b.mu.Lock()
		items := b.items
		b.items = nil
		b.mu.Unlock()
		if len(items) == 0 {
			continue
		}
		select {
		case ev <- watchBatchMsg{items}:
		case <-ctx.Done():
			return
		}
	}
}

// watchLevel: 0 aus, 1 Fehler, 2 Fehler und Warnungen.
func (a *App) watchLevel() int {
	ls := a.st.Settings().LogScan
	switch {
	case !ls.Watch:
		return 0
	case ls.WatchWarn:
		return 2
	}
	return 1
}

func watchLabel(l int) string {
	return []string{i18n.T("aus"), i18n.T("Fehler"), i18n.T("Fehler + Warnungen")}[clamp(l, 0, 2)]
}

func (a *App) cycleWatch(dir int) tea.Cmd {
	l := (a.watchLevel() + dir + 3) % 3
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		s.LogScan.Watch, s.LogScan.WatchWarn = l > 0, l == 2
	})
	a.watchTick()
	return nil
}

var syslogPorts = []int{0, 5514, 1514, 514}

func syslogLabel(p int) string {
	if p == 0 {
		return i18n.T("aus")
	}
	return fmt.Sprintf("UDP %d", p)
}

func (a *App) cycleSyslog(dir int) tea.Cmd {
	cur := a.st.Settings().SyslogPort
	i := 0
	for j, p := range syslogPorts {
		if p == cur {
			i = j
		}
	}
	next := syslogPorts[(i+dir+len(syslogPorts))%len(syslogPorts)]
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.SyslogPort = next })
	a.lwatch.sysErr = ""
	a.watchTick()
	if next > 0 {
		a.flash(fmt.Sprintf(i18n.T("Syslog-Empfang auf UDP %d: am Switch/an der Firewall diesen Rechner als Syslog-Server eintragen"), next))
	}
	return nil
}

// watchWanted: soll die Sitzung beobachtet werden?
func (a *App) watchWanted(s *session) bool {
	if s.state != stConnected {
		return false
	}
	return s.isWinRM() || (a.hostServerOK(s) && s.client != nil)
}

// watchTick: Ströme starten und beenden, Syslog-Empfänger anpassen.
func (a *App) watchTick() {
	w := &a.lwatch
	on := a.watchLevel() > 0
	if w.streams == nil {
		w.streams = map[int]*logStream{}
	}
	alive := map[int]bool{}
	for _, s := range a.sessions {
		alive[s.id] = true
		st := w.streams[s.id]
		want := on && a.watchWanted(s)
		switch {
		case !want && st != nil:
			st.cancel()
			delete(w.streams, s.id)
		case want && st != nil && st.client != s.client:
			st.cancel()
			w.streams[s.id] = a.startWatch(s)
		case want && st != nil && !st.ended.IsZero() && time.Since(st.ended) > watchRetry:
			w.streams[s.id] = a.startWatch(s)
		case want && st == nil:
			w.streams[s.id] = a.startWatch(s)
		}
	}
	for id, st := range w.streams {
		if !alive[id] {
			st.cancel()
			delete(w.streams, id)
		}
	}
	if port := a.st.Settings().SyslogPort; port != w.sysPort {
		if w.sysStop != nil {
			w.sysStop()
			w.sysStop = nil
		}
		w.sysPort = port
		if port > 0 {
			w.sysStop = a.startSyslog(port)
		}
	}
}

func (a *App) startWatch(s *session) *logStream {
	ctx, cancel := context.WithCancel(context.Background())
	st := &logStream{cancel: cancel, client: s.client}
	ev := a.events
	b := &watchBatcher{}
	go b.run(ctx, ev)
	sid := s.id
	warn := a.watchLevel() == 2
	end := func(err error) {
		if ctx.Err() != nil {
			return
		}
		select {
		case ev <- watchEndMsg{sid: sid, client: st.client, err: err}:
		case <-ctx.Done():
		}
	}
	if s.isWinRM() {
		lvl := 2
		if warn {
			lvl = 3
		}
		go func() {
			since := time.Now().Add(-watchWinPoll).Unix()
			for {
				out, err := a.winrmRun(ctx, s, logscan.WindowsSinceScript(since, lvl))
				if ctx.Err() != nil {
					return
				}
				if err != nil && strings.TrimSpace(out) == "" {
					end(err)
					return
				}
				r := logscan.ParseWindows(out, 1, time.Now())
				for _, e := range r.Entries {
					if e.Time.Unix() >= since {
						b.add(watchItem{sid: sid, kind: logscan.KindWindows, e: e})
					}
				}
				since = r.At.Unix() + 1
				select {
				case <-ctx.Done():
					return
				case <-time.After(watchWinPoll):
				}
			}
		}()
		return st
	}
	cl := s.client
	go func() {
		err := cl.StreamScript(ctx, logscan.UnixWatchScript(warn), func(line string) {
			if e, ok := logscan.UnixWatchLine(line, time.Now()); ok {
				b.add(watchItem{sid: sid, kind: logscan.KindUnix, e: e})
			}
		})
		if err == nil {
			err = errors.New(i18n.T("Logstrom beendet"))
		}
		end(err)
	}()
	return st
}

// startSyslog: UDP-Empfänger auf allen Adressen.
func (a *App) startSyslog(port int) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	ev := a.events
	b := &watchBatcher{}
	pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		a.lwatch.sysErr = err.Error()
		a.flashErr(fmt.Sprintf(i18n.T("Syslog-Empfang auf UDP %d geht nicht: %v"), port, err))
		return cancel
	}
	go b.run(ctx, ev)
	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()
	go func() {
		buf := make([]byte, 8192)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				if ctx.Err() == nil {
					select {
					case ev <- watchSysErrMsg{port, err}:
					case <-ctx.Done():
					}
				}
				return
			}
			ip := addr.String()
			if h, _, e := net.SplitHostPort(ip); e == nil {
				ip = h
			}
			for _, line := range strings.Split(string(buf[:n]), "\n") {
				if e, ok := logscan.SyslogPacket(line, time.Now()); ok {
					b.add(watchItem{addr: ip, kind: logscan.KindNetwork, e: e})
				}
			}
		}
	}()
	go a.resolveHosts(ctx)
	return cancel
}

// resolveHosts: Hostnamen der Verbindungen zu IPs auflösen, damit Syslog
// vom Gerät der richtigen Verbindung zugeordnet wird (alle 10 Minuten neu).
func (a *App) resolveHosts(ctx context.Context) {
	ev := a.events
	for {
		m := map[string]string{}
		for _, c := range a.st.Connections() {
			h := strings.TrimSpace(c.Host)
			if h == "" || c.Protocol == domain.ProtoSerial || c.Protocol == domain.ProtoLocal {
				continue
			}
			if net.ParseIP(h) != nil {
				m[h] = c.ID
				continue
			}
			lctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			ips, err := net.DefaultResolver.LookupHost(lctx, h)
			cancel()
			if err == nil {
				for _, ip := range ips {
					if _, ok := m[ip]; !ok {
						m[ip] = c.ID
					}
				}
			}
		}
		select {
		case ev <- watchResolvedMsg{m}:
		case <-ctx.Done():
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Minute):
		}
	}
}

// watchMsg: Meldungen der Logwache verarbeiten.
func (a *App) watchMsg(msg tea.Msg) (tea.Cmd, bool) {
	w := &a.lwatch
	switch m := msg.(type) {
	case watchBatchMsg:
		for _, it := range m.items {
			if it.sid == 0 {
				if !a.syslogAccepted(it) {
					continue
				}
				a.keepSyslog(it)
			}
			a.watchEntry(it)
		}
		return a.listen(), true
	case watchEndMsg:
		if st := w.streams[m.sid]; st != nil && st.client == m.client {
			st.ended = time.Now()
		}
		return a.listen(), true
	case watchSysErrMsg:
		if m.port == w.sysPort {
			w.sysErr = m.err.Error()
		}
		return a.listen(), true
	case watchResolvedMsg:
		w.resolved = m.m
		return a.listen(), true
	}
	return nil, false
}

// watchConn: Verbindung zur Zeile (Sitzung oder Absender beim Syslog).
func (a *App) watchConn(it watchItem) (domain.Connection, bool) {
	if it.sid != 0 {
		if s := a.byID(it.sid); s != nil {
			return s.conn, true
		}
		return domain.Connection{}, false
	}
	w := &a.lwatch
	if w.hostIdx == nil || time.Since(w.hostAt) > 30*time.Second {
		w.hostIdx, w.hostAt = map[string]string{}, time.Now()
		for _, c := range a.st.Connections() {
			if _, ok := w.hostIdx[c.Host]; !ok && c.Host != "" {
				w.hostIdx[c.Host] = c.ID
			}
		}
	}
	if id, ok := w.hostIdx[it.addr]; ok {
		if c, ok := a.st.Connection(id); ok {
			return c, true
		}
	}
	if id, ok := a.lwatch.resolved[it.addr]; ok {
		if c, ok := a.st.Connection(id); ok {
			return c, true
		}
	}
	return domain.Connection{}, false
}

// syslogAccepted: Syslog nur von Absendern, die zu einer gespeicherten
// Verbindung gehören (oder alle, wenn eingeschaltet). Verworfenes wird je
// Absender gezählt; beim ersten Paket eines Absenders kommt ein Hinweis.
func (a *App) syslogAccepted(it watchItem) bool {
	if _, known := a.watchConn(it); known || a.st.Settings().SyslogAny {
		return true
	}
	w := &a.lwatch
	if w.dropped == nil {
		w.dropped = map[string]int{}
	}
	if _, seen := w.dropped[it.addr]; !seen && len(w.dropped) < 200 {
		a.pushToast(toastWarn, fmt.Sprintf(i18n.T("Syslog von %s verworfen: gehört zu keiner Verbindung. Verbindung anlegen oder in den Einstellungen „Syslog auch von unbekannten Absendern“ einschalten."), it.addr))
	}
	if len(w.dropped) < 200 || w.dropped[it.addr] > 0 {
		w.dropped[it.addr]++
	}
	return false
}

func (a *App) toggleSyslogAny() tea.Cmd {
	on := !a.st.Settings().SyslogAny
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.SyslogAny = on })
	if on {
		a.flash(i18n.T("Syslog von allen Absendern: auch Geräte ohne Verbindung landen in der Logwache (Absender lassen sich fälschen)."))
	} else {
		a.lwatch.dropped = nil
		a.flash(i18n.T("Syslog nur noch von gespeicherten Verbindungen."))
	}
	return nil
}

// keepSyslog: empfangene Zeile für den Log-Blick des Geräts aufheben.
func (a *App) keepSyslog(it watchItem) {
	c, ok := a.watchConn(it)
	if !ok {
		return
	}
	w := &a.lwatch
	if w.syslog == nil {
		w.syslog = map[string][]logscan.Entry{}
	}
	es := append(w.syslog[c.ID], it.e)
	if len(es) > watchSyslogKeep {
		es = append([]logscan.Entry(nil), es[len(es)-watchSyslogKeep:]...)
	}
	w.syslog[c.ID] = es
}

// watchBase: Musterschlüssel der letzten Grundlinie einer Verbindung.
func (a *App) watchBase(connID string) map[string]bool {
	w := &a.lwatch
	if w.base == nil {
		w.base = map[string]map[string]bool{}
	}
	if b, ok := w.base[connID]; ok {
		return b
	}
	b := map[string]bool{}
	if raw := a.st.LogBase(connID); len(raw) > 0 {
		var base logscan.Base
		if json.Unmarshal(raw, &base) == nil {
			for k := range base.Items {
				b[k] = true
			}
		}
	}
	w.base[connID] = b
	return b
}

func (a *App) watchEntry(it watchItem) {
	w := &a.lwatch
	minSev := logscan.SevErr
	if a.watchLevel() == 2 {
		minSev = logscan.SevWarn
	}
	if it.sid != 0 && a.watchLevel() == 0 {
		return
	}
	f, ok := logscan.Classify(it.kind, it.e)
	if !ok {
		return
	}
	c, known := a.watchConn(it)
	host, connID := it.addr, "ip:"+it.addr
	if known {
		connID = c.ID
	}
	// Fehlanmeldungen zählen in einem Zeitfenster mit.
	if w.burst == nil || len(w.burst) > 2000 {
		w.burst = map[string]watchBurst{}
	}
	bk := connID + "|" + f.Key
	b := w.burst[bk]
	if time.Since(b.from) > logscan.BurstWindow {
		b = watchBurst{from: time.Now()}
	}
	b.n++
	w.burst[bk] = b
	if b.n > 1 {
		f, _ = logscan.ClassifyBurst(it.kind, it.e, b.n)
	}
	if f.Sev < minSev {
		return
	}
	if known {
		host = c.Alias
	} else {
		c = domain.Connection{ID: connID}
	}
	if a.noiseFor(c)(f.Key) {
		return
	}
	id := connID + "|" + f.Key
	if w.seen == nil {
		w.seen = map[string]int{}
	}
	if i, ok := w.seen[id]; ok && i < len(w.alerts) && w.alerts[i].Key == f.Key && time.Since(w.alerts[i].At) < watchRepeat {
		w.alerts[i].Count++
		return
	}
	isNew := !known || !a.watchBase(connID)[f.Key]
	if !isNew && f.Sev < logscan.SevCrit {
		return
	}
	src := "journal"
	switch {
	case it.sid == 0:
		src = "syslog"
	case it.kind == logscan.KindWindows:
		src = "winrm"
	}
	al := watchAlert{At: time.Now(), Host: host, ConnID: connID, Sev: f.Sev, Title: f.Title, Line: it.e.Line(), Key: f.Key, Src: src, New: isNew, Count: 1}
	w.alerts = append(w.alerts, al)
	if len(w.alerts) > watchMaxAlerts {
		drop := len(w.alerts) - watchMaxAlerts
		w.alerts = append([]watchAlert(nil), w.alerts[drop:]...)
		for k, i := range w.seen {
			if i -= drop; i < 0 {
				delete(w.seen, k)
			} else {
				w.seen[k] = i
			}
		}
	}
	w.seen[id] = len(w.alerts) - 1
	w.unread++
	a.announceWatch(al, it.sid)
}

func (a *App) announceWatch(al watchAlert, sid int) {
	tag := i18n.T("neues Muster")
	if !al.New {
		tag = i18n.T("kritisch")
	}
	text := fmt.Sprintf(i18n.T("▲ Logwache %s: %s (%s)"), al.Host, trunc(al.Title, 70), tag)
	kind := toastWarn
	if al.Sev >= logscan.SevErr {
		kind = toastErr
	}
	a.pushToast(kind, text)
	if s := a.byID(sid); s != nil && !a.sessionVisible(s) {
		s.notice = noticeLog
	}
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() {
		a.sendDesktop("sshTermy · "+al.Host, al.Title)
	}
}

// watchStatus: Zustand der Logwache in einer Zeile.
func (a *App) watchStatus() string {
	w := &a.lwatch
	var parts []string
	if lvl := a.watchLevel(); lvl == 0 {
		parts = append(parts, i18n.T("aus (Einstellungen › Logwache)"))
	} else {
		parts = append(parts, fmt.Sprintf(i18n.T("%s · %d Sitzungen"), watchLabel(lvl), len(w.streams)))
	}
	if w.sysPort > 0 {
		if w.sysErr != "" {
			parts = append(parts, fmt.Sprintf(i18n.T("Syslog UDP %d: Fehler"), w.sysPort))
		} else {
			parts = append(parts, fmt.Sprintf("Syslog UDP %d", w.sysPort))
		}
		if n := len(w.dropped); n > 0 {
			parts = append(parts, plural(n, i18n.T("unbekannter Absender verworfen"), i18n.T("unbekannte Absender verworfen")))
		}
	}
	return strings.Join(parts, " · ")
}

// watchItems: Meldungen als Auswahlliste (neueste oben).
func (a *App) watchItems() []pickItem {
	w := &a.lwatch
	var out []pickItem
	for i := len(w.alerts) - 1; i >= 0; i-- {
		al := w.alerts[i]
		cnt := ""
		if al.Count > 1 {
			cnt = fmt.Sprintf(" ×%d", al.Count)
		}
		tag := ""
		if al.New {
			tag = i18n.T(" · neu")
		}
		out = append(out, pickItem{value: fmt.Sprint(i), label: al.Sev.Label() + "  " + al.Host + " · " + al.Title + cnt,
			detail: trunc(al.Line, 120), hint: al.At.Local().Format("02.01. 15:04") + tag, search: al.Src})
	}
	if len(out) == 0 {
		out = append(out, pickItem{value: "", label: i18n.T("Noch keine Meldungen. Gemeldet wird, was neu oder kritisch ist."),
			disabled: i18n.T("Noch keine Meldungen")})
	}
	return out
}

func (a *App) watchAlertAt(it *pickItem) (watchAlert, bool) {
	if it == nil {
		return watchAlert{}, false
	}
	var i int
	if _, err := fmt.Sscan(it.value, &i); err != nil || i < 0 || i >= len(a.lwatch.alerts) {
		return watchAlert{}, false
	}
	return a.lwatch.alerts[i], true
}

// openWatchList: Meldungen der Logwache. ⏎ öffnet den Log-Blick beim
// Muster (wenn der Tab offen ist), x blendet es als Rauschen aus.
func (a *App) openWatchList() tea.Cmd {
	a.lwatch.unread = 0
	var p *picker
	p = newPicker(i18n.T("Logwache: gesammelte Meldungen")+" · "+a.watchStatus(), i18n.T("Meldungen filtern …"), a.watchItems(), "", func(it pickItem) tea.Cmd {
		al, ok := a.watchAlertAt(&it)
		if !ok {
			return nil
		}
		return a.openWatchAlert(al)
	})
	p.reload = a.watchItems
	p.keys = [][2]string{{"⏎", i18n.T("Log-Blick beim Muster")}, {"x", i18n.T("als Rauschen ausblenden")}, {"Esc", i18n.T("schließen")}}
	p.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		if key != "x" || p.in.Value() != "" {
			return nil, false
		}
		al, ok := a.watchAlertAt(it)
		if !ok {
			return nil, true
		}
		c, known := a.st.Connection(al.ConnID)
		if !known {
			c = domain.Connection{ID: al.ConnID, Alias: al.Host}
		}
		a.addNoiseFor(c, logscan.Finding{Key: al.Key, Title: al.Title}, false)
		a.dropWatchAlerts(al.ConnID, al.Key)
		p.items = a.watchItems()
		p.sel = clamp(p.sel, 0, max(0, len(p.items)-1))
		return nil, true
	}
	a.pick = p
	return nil
}

// dropWatchAlerts: Meldungen eines ausgeblendeten Musters entfernen.
func (a *App) dropWatchAlerts(connID, key string) {
	w := &a.lwatch
	out := w.alerts[:0]
	for _, al := range w.alerts {
		if al.ConnID != connID || al.Key != key {
			out = append(out, al)
		}
	}
	w.alerts = out
	w.seen = map[string]int{}
	for i, al := range w.alerts {
		w.seen[al.ConnID+"|"+al.Key] = i
	}
}

// openWatchAlert: Log-Blick der Verbindung öffnen und das Muster wählen.
func (a *App) openWatchAlert(al watchAlert) tea.Cmd {
	for i, s := range a.sessions {
		if s.conn.ID != al.ConnID || s.state != stConnected {
			continue
		}
		a.focusSession(i)
		a.scr = scrTerminal
		if a.lscan.sid == s.id && a.lscan.rep != nil {
			a.lscan.focus = al.Key
			return a.openLogScan(s)
		}
		cmd := a.openLogScan(s)
		a.lscan.focus = al.Key
		return cmd
	}
	a.modal = textModal(al.Host+" · "+al.Title, []string{
		al.At.Local().Format("02.01.2006 15:04:05") + "  " + al.Sev.Label() + "  " + al.Src,
		"", al.Line, "",
		i18n.T("Kein offener Tab zu diesem Host: Verbindung öffnen, dann Alt+E für den Log-Blick."),
	})
	return nil
}

// stopWatch beim Beenden.
func (a *App) stopWatch() {
	w := &a.lwatch
	for id, st := range w.streams {
		st.cancel()
		delete(w.streams, id)
	}
	if w.sysStop != nil {
		w.sysStop()
		w.sysStop = nil
	}
	w.sysPort = 0
}

func (a *App) watchMenuLabel() string {
	if n := a.lwatch.unread; n > 0 {
		return fmt.Sprintf(i18n.T("Logwache: gesammelte Meldungen ansehen (%d neu)"), n)
	}
	return i18n.T("Logwache: gesammelte Meldungen ansehen")
}
