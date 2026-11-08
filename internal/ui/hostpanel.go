package ui

// Host-Übersicht (F6) und Config-Sicherung.
//
// Server: Beim Verbinden läuft ein kleines, nur lesendes sh-Skript in einer
// eigenen Exec-Sitzung (CPU, RAM, Platte, Laufzeit, offene Updates). Das
// Ergebnis steht kurz in der Kopfzeile und dauerhaft als Plakette in der
// Sitzungsleiste.
//
// Netzwerkgeräte: Exec-Kanäle sind tabu (Aruba/ProCurve beenden dann die
// ganze Verbindung). Abgefragt wird deshalb nur auf Abruf und sichtbar über
// die interaktive Shell: sshTermy tippt die show-Befehle am Prompt, blättert
// "-- MORE --" selbst weiter und liest die Ausgabe mit. "show running-config"
// wird verschlüsselt gesichert und mit dem letzten Stand verglichen.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
)

// hostState: Daten des F6-Panels einer Sitzung.
type hostState struct {
	busy      bool
	step      string
	err       string
	seq       int
	cancelFn  context.CancelFunc
	srv       *hostinfo.Server
	sw        *hostinfo.Switch
	vendor    hostinfo.Vendor
	pagingOff bool // "no page" o. Ä. in dieser Sitzung schon gesendet
	backups   []store.Backup
	sel       int
	note      string           // Ergebnis der letzten Sicherung
	viol      string           // verletzte Config-Regeln der letzten Sicherung
	alerts    []hostinfo.Alert // Kennzahlen über der Warnschwelle
	lastTry   time.Time        // letzte Server-Abfrage (auch fehlgeschlagene)
}

func (h *hostState) cancel() {
	if h.cancelFn != nil {
		h.cancelFn()
		h.cancelFn = nil
	}
}

// shellCap schneidet den PTY-Strom mit, solange eine Abfrage läuft.
type shellCap struct {
	mu   sync.Mutex
	on   bool
	buf  bytes.Buffer
	last time.Time
}

func (c *shellCap) feed(p []byte) {
	c.mu.Lock()
	if c.on && c.buf.Len() < 4<<20 {
		c.buf.Write(p)
		c.last = time.Now()
	}
	c.mu.Unlock()
}

func (c *shellCap) begin() {
	c.mu.Lock()
	c.on, c.last = true, time.Now()
	c.buf.Reset()
	c.mu.Unlock()
}

func (c *shellCap) end() {
	c.mu.Lock()
	c.on = false
	c.buf.Reset()
	c.mu.Unlock()
}

func (c *shellCap) peek() (string, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String(), c.last
}

// Nachrichten
type hostSrvMsg struct {
	sid, seq int
	srv      hostinfo.Server
	err      error
	mode     hostMode
}

// hostMode: warum der Server abgefragt wird.
type hostMode int

const (
	hostManual  hostMode = iota // r im Panel bzw. Panel geöffnet
	hostConnect                 // nach dem Verbinden: Zusammenfassung als Toast
	hostAuto                    // Überwachung im Hintergrund: nur Warnungen melden
)

type netResult struct {
	vendor    hostinfo.Vendor
	pagingOff bool
	sw        *hostinfo.Switch
	backup    *store.Backup
	unchanged bool
	prev      *store.Backup
	added     int
	removed   int
	model     string // aus der Konfiguration (ProCurve)
	viol      []hostinfo.Violation
	err       error
}

type hostNetMsg struct {
	sid, seq int
	res      netResult
}

// ---------------------------------------------------------------- Ablauf

// hostOnConnect: nach dem Verbinden. Server werden sofort abgefragt,
// bei Netzwerkgeräten nur die vorhandenen Sicherungen gelistet.
func (a *App) hostOnConnect(s *session) tea.Cmd {
	if s.netDev() {
		s.host.backups = a.st.Backups(s.conn.ID)
		return nil
	}
	if !a.hostServerOK(s) {
		return nil
	}
	return a.hostServerCmd(s, hostConnect)
}

// hostServerOK: darf der Server per Exec-Sitzung abgefragt werden? Nie bei
// Netzwerkgeräten (Aruba/ProCurve trennen sonst die Verbindung), auch nicht,
// wenn nur das Systemprofil sie als solche kennt, und nicht bei Windows
// (kein sh).
func (a *App) hostServerOK(s *session) bool {
	return s.client != nil && !s.netDevice() && s.prof.Family != domain.FamilyWindows
}

// monLimits: Warnschwellen aus den Einstellungen.
func (a *App) monLimits() hostinfo.Limits {
	c, r, d := a.st.Settings().Monitor.Limits()
	return hostinfo.Limits{CPU: c, RAM: r, Disk: d}
}

// monitorTick (jede Sekunde): Server im eingestellten Abstand still
// nachfragen. Netzwerkgeräte nie, dort tippt sshTermy sichtbar am Prompt.
func (a *App) monitorTick(s *session) tea.Cmd {
	h := &s.host
	every := a.st.Settings().Monitor.Every()
	if every <= 0 || h.busy || s.state != stConnected || !a.hostServerOK(s) || h.lastTry.IsZero() {
		return nil
	}
	if time.Since(h.lastTry) < every {
		return nil
	}
	return a.hostServerCmd(s, hostAuto)
}

func (a *App) hostServerCmd(s *session, mode hostMode) tea.Cmd {
	h := &s.host
	h.cancel()
	h.seq++
	h.lastTry = time.Now()
	h.busy, h.err, h.step = true, "", i18n.T("Kennzahlen abfragen …")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	h.cancelFn = cancel
	sid, seq, cl := s.id, h.seq, s.client
	return func() tea.Msg {
		defer cancel()
		out, err := cl.RunScript(ctx, hostinfo.ServerScript)
		if err != nil {
			return hostSrvMsg{sid: sid, seq: seq, err: err, mode: mode}
		}
		srv, err := hostinfo.ParseServer(out)
		return hostSrvMsg{sid: sid, seq: seq, srv: srv, err: err, mode: mode}
	}
}

// hostNetCmd fragt ein Netzwerkgerät über die sichtbare Shell ab: info =
// Übersicht, backup = running-config sichern.
func (a *App) hostNetCmd(s *session, info, backup bool) tea.Cmd {
	h := &s.host
	if h.busy {
		return nil
	}
	if s.shell == nil || s.term == nil {
		return nil
	}
	prompt := strings.TrimSpace(s.term.CursorLine())
	if s.term.AltScreen() || !hostinfo.IsPrompt(prompt) {
		h.err = i18n.T("Abfrage nur an einem leeren Prompt (z. B. switch#). Bitte erst Eingabe beenden oder ⏎ drücken.")
		return nil
	}
	h.seq++
	h.busy, h.err, h.note = true, "", ""
	if backup {
		h.viol = ""
	}
	h.step = i18n.T("Übersicht abfragen …")
	if backup && !info {
		h.step = i18n.T("show running-config sichern …")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	h.cancelFn = cancel
	sid, seq := s.id, h.seq
	vendor, pagingOff := h.vendor, h.pagingOff
	if vendor == hostinfo.VendorUnknown {
		vendor = hostVendor(s.prof) // aus dem Systemprofil
	}
	st := a.st
	connID := s.conn.ID
	rules := a.configRules()
	return func() tea.Msg {
		defer cancel()
		r := netResult{vendor: vendor, pagingOff: pagingOff}
		run := func(cmd string, limit time.Duration) (string, error) {
			lines, err := shellRun(ctx, s, prompt, cmd, limit)
			return strings.Join(lines, "\n"), err
		}
		fail := func(err error) tea.Msg { r.err = err; return hostNetMsg{sid, seq, r} }

		var ver string
		if info || r.vendor == hostinfo.VendorUnknown {
			out, err := run("show version", 20*time.Second)
			if err != nil {
				return fail(err)
			}
			ver = out
			if v := hostinfo.DetectVendor(out); v != hostinfo.VendorUnknown {
				r.vendor = v
			}
		}
		if p := r.vendor.PagingOff(); p != "" && !r.pagingOff {
			if _, err := run(p, 10*time.Second); err != nil {
				return fail(err)
			}
			r.pagingOff = true
		}
		if info {
			sysCmd, portCmd, vlanCmd := r.vendor.InfoCommands()
			outs := make([]string, 3)
			for i, c := range []string{sysCmd, portCmd, vlanCmd} {
				if c == "" {
					continue
				}
				out, err := run(c, 30*time.Second)
				if err != nil {
					return fail(err)
				}
				outs[i] = out
			}
			sw := hostinfo.ParseSwitch(r.vendor, ver, outs[0], outs[1], outs[2])
			r.sw = &sw
		}
		if backup {
			cfg, err := run("show running-config", 2*time.Minute)
			if err != nil {
				return fail(err)
			}
			if err := checkConfig(cfg); err != nil {
				return fail(err)
			}
			cfg = strings.TrimRight(cfg, "\n") + "\n"
			r.model = hostinfo.ModelFromConfig(cfg)
			r.viol = hostinfo.CheckConfig(cfg, rules)
			if list := st.Backups(connID); len(list) > 0 {
				prev := list[0]
				r.prev = &prev
				if old, err := st.LoadBackup(connID, prev.ID); err == nil {
					if hostinfo.SameConfig(old, cfg) {
						r.unchanged = true
						return hostNetMsg{sid, seq, r}
					}
					_, r.added, r.removed = hostinfo.Diff(hostinfo.ConfigLines(old), hostinfo.ConfigLines(cfg), 0)
				}
			}
			b, err := st.SaveBackup(connID, cfg)
			if err != nil {
				return fail(err)
			}
			r.backup = &b
		}
		return hostNetMsg{sid, seq, r}
	}
}

// checkConfig: hat das Gerät wirklich eine Konfiguration geliefert?
func checkConfig(cfg string) error {
	l := strings.ToLower(cfg)
	switch {
	case strings.Count(cfg, "\n") < 2:
		return errors.New(i18n.T("keine Konfiguration erhalten"))
	case strings.Contains(l, "invalid input") || strings.Contains(l, "unknown command") || strings.Contains(l, "invalid command"):
		return errors.New(i18n.T("Gerät kennt „show running-config“ nicht"))
	}
	return nil
}

// shellRun tippt einen Befehl in die Shell und liest bis zum nächsten Prompt
// mit. "-- MORE --" wird mit Leertaste beantwortet.
func shellRun(ctx context.Context, s *session, prompt, cmd string, limit time.Duration) ([]string, error) {
	s.cap.begin()
	defer s.cap.end()
	if _, err := s.shell.Write([]byte(cmd + "\r")); err != nil {
		return nil, err
	}
	start, answered := time.Now(), 0
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New(i18n.T("abgebrochen"))
		case <-t.C:
		}
		raw, last := s.cap.peek()
		if len(raw) > answered && hostinfo.HasMore(raw) {
			if _, err := s.shell.Write([]byte(" ")); err != nil {
				return nil, err
			}
			answered = len(raw)
			continue
		}
		lines, done := hostinfo.Output(raw, cmd, prompt)
		idle := time.Since(last)
		switch {
		case done && idle > 300*time.Millisecond:
			return lines, nil
		case raw == "" && time.Since(start) > 10*time.Second:
			return nil, fmt.Errorf(i18n.T("„%s“: keine Antwort"), cmd)
		case time.Since(start) > limit || (raw != "" && idle > 10*time.Second):
			return lines, fmt.Errorf(i18n.T("„%s“: Prompt kam nicht zurück"), cmd)
		}
	}
}

// hostMsg verarbeitet die Ergebnisse.
func (a *App) hostMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case hostSrvMsg:
		s := a.byID(m.sid)
		if s == nil || m.seq != s.host.seq {
			return nil, true
		}
		h := &s.host
		h.busy, h.cancelFn = false, nil
		if m.err != nil {
			h.err = i18n.T("Abfrage fehlgeschlagen: ") + m.err.Error()
			return nil, true
		}
		srv := m.srv
		h.srv = &srv
		fresh := a.setAlerts(s, srv.Alerts(a.monLimits()))
		switch {
		case len(fresh) > 0:
			a.pushToast(toastWarn, s.conn.Alias+": "+hostinfo.AlertsText(fresh))
		case m.mode == hostConnect && s == a.current() && a.scr == scrTerminal:
			a.flash(s.conn.Alias + ": " + srv.Summary())
		}
		return nil, true
	case hostNetMsg:
		s := a.byID(m.sid)
		if s == nil || m.seq != s.host.seq {
			return nil, true
		}
		h, r := &s.host, m.res
		h.busy, h.cancelFn = false, nil
		h.vendor, h.pagingOff = r.vendor, r.pagingOff
		if r.sw != nil {
			h.sw = r.sw
		}
		if h.sw != nil && h.sw.Model == "" && r.model != "" {
			h.sw.Model = r.model
		}
		a.profileFromSwitch(s, r.vendor, h.sw)
		if r.sw != nil {
			if fresh := a.setAlerts(s, r.sw.Alerts(a.monLimits())); len(fresh) > 0 {
				a.pushToast(toastWarn, s.conn.Alias+": "+hostinfo.AlertsText(fresh))
			}
		}
		switch {
		case r.err != nil:
			h.err = r.err.Error()
		case r.unchanged:
			h.note = i18n.T("Unverändert seit ") + localTime(r.prev.Taken) + i18n.T(", nichts Neues gespeichert.")
		case r.backup != nil && r.prev != nil:
			h.note = fmt.Sprintf(i18n.T("Gesichert: +%d −%d Zeilen gegenüber %s. d zeigt den Diff."), r.added, r.removed, localTime(r.prev.Taken))
		case r.backup != nil:
			h.note = i18n.T("Erste Sicherung gespeichert (") + plural(r.backup.Lines, i18n.T("Zeile"), i18n.T("Zeilen")) + ")."
		}
		if r.backup != nil || r.unchanged {
			h.backups, h.sel = a.st.Backups(s.conn.ID), 0
			h.viol = ""
			if len(r.viol) > 0 {
				h.viol = fmt.Sprintf(i18n.T("Regeln verletzt (%d): "), len(r.viol)) + violationText(r.viol)
				a.pushToast(toastWarn, s.conn.Alias+": "+h.viol)
			} else {
				a.flash(s.conn.Alias + ": " + firstSentence(h.note))
			}
		}
		return nil, true
	}
	return nil, false
}

// setAlerts übernimmt die aktuellen Warnungen und liefert die, die neu
// dazugekommen sind (eine Warnung meldet sich nur einmal, bis der Wert
// wieder unter die Schwelle fällt).
func (a *App) setAlerts(s *session, now []hostinfo.Alert) (fresh []hostinfo.Alert) {
	for _, al := range now {
		if !hostinfo.Over(s.host.alerts, al.Metric) {
			fresh = append(fresh, al)
		}
	}
	s.host.alerts = now
	return fresh
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func localTime(t time.Time) string {
	lt := t.Local()
	if y, m, d := lt.Date(); time.Now().Year() == y && time.Now().Month() == m && time.Now().Day() == d {
		return i18n.T("heute ") + lt.Format("15:04")
	}
	return lt.Format("02.01.06 15:04")
}

// ---------------------------------------------------------------- Tasten

func (a *App) hostOpened(s *session) tea.Cmd {
	if s.netDev() {
		s.host.backups = a.st.Backups(s.conn.ID)
		return nil
	}
	if !a.hostServerOK(s) {
		return nil
	}
	if s.host.srv == nil && !s.host.busy {
		return a.hostServerCmd(s, hostManual)
	}
	return nil
}

func (a *App) hostKey(s *session, k tea.KeyPressMsg) tea.Cmd {
	h := &s.host
	net := s.netDev()
	switch k.String() {
	case "x":
		if h.busy {
			h.cancel()
			h.busy, h.err = false, i18n.T("Abgebrochen.")
		}
	case "r", "enter":
		if h.busy {
			return nil
		}
		if net {
			s.panelFocus = false // Ausgabe läuft sichtbar im Terminal
			return a.hostNetCmd(s, true, false)
		}
		if !a.hostServerOK(s) {
			return nil
		}
		return a.hostServerCmd(s, hostManual)
	case "g":
		if !h.busy {
			return a.openMonitorForm()
		}
	case "b":
		if net && !h.busy {
			s.panelFocus = false
			return a.hostNetCmd(s, false, true)
		}
	case "up", "k":
		h.sel = max(0, h.sel-1)
	case "down", "j":
		h.sel = min(max(0, len(h.backups)-1), h.sel+1)
	case "d":
		a.showBackupDiff(s)
	case "v":
		a.showBackup(s)
	}
	return nil
}

// showBackupDiff: gewählter Stand gegen den davor.
func (a *App) showBackupDiff(s *session) {
	h := &s.host
	if h.sel < 0 || h.sel+1 >= len(h.backups) {
		h.err = i18n.T("Für einen Vergleich braucht es mindestens zwei Sicherungen.")
		return
	}
	cur, prev := h.backups[h.sel], h.backups[h.sel+1]
	newTxt, err1 := a.st.LoadBackup(s.conn.ID, cur.ID)
	oldTxt, err2 := a.st.LoadBackup(s.conn.ID, prev.ID)
	if err := errors.Join(err1, err2); err != nil {
		h.err = err.Error()
		return
	}
	lines, add, del := hostinfo.Diff(hostinfo.ConfigLines(oldTxt), hostinfo.ConfigLines(newTxt), 3)
	st := a.s
	var out []string
	for _, l := range lines {
		switch l.Op {
		case hostinfo.DiffHunk:
			out = append(out, st.AI.Render(l.Text))
		case hostinfo.DiffAdd:
			out = append(out, st.OK.Render("+ "+l.Text))
		case hostinfo.DiffDel:
			out = append(out, st.Danger.Render("- "+l.Text))
		default:
			out = append(out, st.Muted.Render("  "+l.Text))
		}
	}
	if len(out) == 0 {
		out = []string{st.Muted.Render(i18n.T("Keine inhaltlichen Änderungen."))}
	}
	title := fmt.Sprintf(i18n.T("Diff %s · %s → %s  "), s.conn.Alias, localTime(prev.Taken), localTime(cur.Taken)) +
		st.OK.Render(fmt.Sprintf("+%d", add)) + " " + st.Danger.Render(fmt.Sprintf("−%d", del))
	a.modal = textModal(title, out)
}

func (a *App) showBackup(s *session) {
	h := &s.host
	if h.sel < 0 || h.sel >= len(h.backups) {
		return
	}
	b := h.backups[h.sel]
	txt, err := a.st.LoadBackup(s.conn.ID, b.ID)
	if err != nil {
		h.err = err.Error()
		return
	}
	a.modal = textModal(i18n.T("Konfiguration ")+s.conn.Alias+" · "+localTime(b.Taken), strings.Split(strings.TrimRight(txt, "\n"), "\n"))
}

func (a *App) hostPanelKeys(s *session) [][2]string {
	if s.host.busy {
		return [][2]string{{"x", i18n.T("abbrechen")}, {"Esc", "Terminal"}, {a.keys.label(actHost), i18n.T("schließen")}}
	}
	if s.netDev() {
		k := [][2]string{{"⏎", i18n.T("abfragen")}, {"b", i18n.T("Config sichern")}}
		if len(s.host.backups) > 1 {
			k = append(k, [2]string{"d", "Diff"})
		}
		if len(s.host.backups) > 0 {
			k = append(k, [2]string{"↑↓", i18n.T("Stand")}, [2]string{"v", i18n.T("ansehen")})
		}
		return append(k, [2]string{"g", i18n.T("Grenzwerte")}, [2]string{"Esc", "Terminal"}, [2]string{a.keys.label(actHost), i18n.T("schließen")})
	}
	return [][2]string{{"r", i18n.T("aktualisieren")}, {"g", i18n.T("Grenzwerte")}, {"Esc", "Terminal"}, {a.keys.label(actHost), i18n.T("schließen")}}
}

// ---------------------------------------------------------------- Ansicht

// hostBadge: Plakette in der Sitzungsleiste (Klick öffnet F6).
func (a *App) hostBadge(s *session) string {
	st := a.s
	h := s.host
	txt := ""
	switch {
	case h.busy:
		txt = "▤ …"
	case h.srv != nil:
		txt = "▤ " + h.srv.Summary()
	case h.sw != nil:
		txt = "▤ " + h.sw.Summary()
	case s.netDev():
		txt = i18n.T("▤ F6 Übersicht")
	default:
		return ""
	}
	style := st.Badge
	if len(h.alerts) > 0 && !h.busy {
		style = style.Foreground(st.T.Warn)
		txt = "▲" + strings.TrimPrefix(txt, "▤")
	}
	return a.keyZone(a.keys.label(actHost), style.Render(trunc(txt, 46)))
}

func (a *App) viewHost(b *strings.Builder, s *session, w int) {
	st := a.s
	h := &s.host
	b.WriteString(st.PanelTitle.Foreground(st.T.Accent).Render(i18n.T("▤ Host-Übersicht")) + "\n\n")
	row := func(k, v string) {
		b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", k)) + st.Text.Render(trunc(v, w-10)) + "\n")
	}
	net := s.netDev()

	if h.busy {
		b.WriteString(st.Warn.Render("◌ "+h.step) + "\n")
		if net {
			b.WriteString(st.Muted.Render(i18n.T("Die Befehle laufen sichtbar im Terminal.")) + "\n")
		}
		b.WriteString("\n")
	}
	if h.err != "" {
		b.WriteString(st.Danger.Render(wrap(h.err, w)) + "\n\n")
	}

	if !net {
		srv := h.srv
		if srv == nil {
			if !h.busy {
				b.WriteString(st.Muted.Render(i18n.T("Noch keine Daten. r = abfragen")) + "\n")
			}
			return
		}
		if s.client != nil {
			row("System", s.client.Remote.OS)
		}
		lim := a.monLimits()
		cpu := fmt.Sprintf(i18n.T("%d Kerne"), srv.CPUs)
		if srv.HasLoad {
			cpu += fmt.Sprintf(i18n.T(" · Last %.2f %.2f %.2f"), srv.Load[0], srv.Load[1], srv.Load[2])
		}
		row("CPU", cpu)
		if srv.CPUPct >= 0 {
			b.WriteString(strings.Repeat(" ", 10) + a.bar(srv.CPUPct, w-10, lim.CPU) + "\n")
		}
		if srv.MemTotal > 0 {
			row("RAM", fmt.Sprintf("%s / %s", hostinfo.Bytes(srv.MemTotal-min(srv.MemFree, srv.MemTotal)), hostinfo.Bytes(srv.MemTotal)))
			b.WriteString(strings.Repeat(" ", 10) + a.bar(srv.MemPct(), w-10, lim.RAM) + "\n")
		}
		if srv.DiskTotal > 0 {
			row(i18n.T("Platte /"), fmt.Sprintf("%s / %s", hostinfo.Bytes(srv.DiskUsed), hostinfo.Bytes(srv.DiskTotal)))
			b.WriteString(strings.Repeat(" ", 10) + a.bar(srv.DiskPct(), w-10, lim.Disk) + "\n")
		}
		if srv.Uptime > 0 {
			row(i18n.T("Laufzeit"), hostinfo.Duration(srv.Uptime))
		}
		switch {
		case srv.UpdateTool == "":
			row("Updates", i18n.T("unbekannt"))
		case srv.Updates < 0:
			row("Updates", i18n.T("unbekannt (")+srv.UpdateTool+")")
		case srv.Updates == 0:
			b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", "Updates")) + st.OK.Render(i18n.T("aktuell")) + st.Muted.Render(" · "+srv.UpdateTool) + "\n")
		default:
			v := plural(srv.Updates, i18n.T("Paket"), i18n.T("Pakete"))
			if srv.Security > 0 {
				v += fmt.Sprintf(i18n.T(" (%d Sicherheit)"), srv.Security)
			}
			b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", "Updates")) + st.Warn.Render(v) + st.Muted.Render(" · "+srv.UpdateTool) + "\n")
		}
		if srv.RebootNeeded {
			b.WriteString(st.Warn.Render(i18n.T("◌ Neustart erforderlich")) + "\n")
		}
		a.viewAlerts(b, s, w)
		b.WriteString("\n" + st.Muted.Render(i18n.T("Stand ")+srv.At.Format("15:04:05")+i18n.T(" · r aktualisieren")) + "\n")
		b.WriteString(st.Muted.Render(wrap(a.monitorLine(false), w)) + "\n")
		if srv.UpdateTool == "apt" {
			b.WriteString(st.Muted.Render(wrap(i18n.T("Updates laut lokaler Paketliste (ohne apt update)."), w)) + "\n")
		}
		return
	}

	// Netzwerkgerät
	if sw := h.sw; sw != nil {
		row(i18n.T("Gerät"), sw.Vendor.Name())
		if sw.Name != "" {
			row("Name", sw.Name)
		}
		if sw.Model != "" {
			row(i18n.T("Modell"), sw.Model)
		}
		row("Firmware", orDefault(sw.Firmware, "?"))
		if sw.Uptime != "" {
			row(i18n.T("Laufzeit"), sw.Uptime)
		}
		if n := sw.PortsUp + sw.PortsDown; n > 0 {
			b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", "Ports")) + st.OK.Render(fmt.Sprintf(i18n.T("%d aktiv"), sw.PortsUp)) +
				st.Muted.Render(fmt.Sprintf(i18n.T(" · %d inaktiv"), sw.PortsDown)) + "\n")
		}
		lim := a.monLimits()
		if sw.CPUPct >= 0 {
			b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", "CPU")) + a.bar(sw.CPUPct, w-10, lim.CPU) + "\n")
		}
		if sw.MemPct >= 0 {
			b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", "RAM")) + a.bar(sw.MemPct, w-10, lim.RAM) + "\n")
		}
		if len(sw.VLANs) > 0 {
			var vs []string
			for _, v := range sw.VLANs {
				vs = append(vs, fmt.Sprintf("%d %s", v.ID, v.Name))
			}
			b.WriteString(st.Muted.Render(fmt.Sprintf("%-9s ", "VLANs")) + st.Text.Render(lipgloss.NewStyle().Width(w-10).Render(trunc(strings.Join(vs, ", "), 3*(w-10)))) + "\n")
		}
		a.viewAlerts(b, s, w)
		b.WriteString(st.Muted.Render(i18n.T("Stand ")+sw.At.Format("15:04:05")) + "\n")
		b.WriteString(st.Muted.Render(wrap(a.monitorLine(true), w)) + "\n")
	} else if !h.busy {
		b.WriteString(st.Text.Render(wrap(i18n.T("⏎ fragt Firmware, Ports und VLANs ab. sshTermy tippt dafür show-Befehle sichtbar am Prompt (keine versteckte Sitzung, die den Switch trennen könnte)."), w)) + "\n")
	}

	b.WriteString("\n" + st.PanelTitle.Foreground(st.T.Accent).Render(i18n.T("▣ Config-Sicherung")) + "\n")
	if h.note != "" {
		b.WriteString(st.OK.Render(wrap(h.note, w)) + "\n")
	}
	if h.viol != "" {
		b.WriteString(st.Warn.Render(wrap(h.viol, w)) + "\n")
	}
	if len(h.backups) == 0 {
		b.WriteString(st.Muted.Render(wrap(i18n.T("Noch keine Sicherung. b sichert „show running-config“ verschlüsselt auf diesem Rechner."), w)) + "\n")
		return
	}
	for i, bk := range h.backups {
		if i >= 8 {
			b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("  … %d ältere"), len(h.backups)-8)) + "\n")
			break
		}
		line := fmt.Sprintf("%s  %s", localTime(bk.Taken), plural(bk.Lines, i18n.T("Zeile"), i18n.T("Zeilen")))
		mark := "  "
		if i == h.sel {
			mark = st.Accent.Render("▸ ")
			line = st.Selected.Render(line)
		} else {
			line = st.Text.Render(line)
		}
		i := i
		b.WriteString(a.z.click(mark+line, func(_ tea.Mouse, double bool) tea.Cmd {
			s.panelFocus, h.sel = true, i
			if double {
				a.showBackupDiff(s)
			}
			return nil
		}) + "\n")
	}
}

// viewAlerts: Warnzeilen über der Schwelle.
func (a *App) viewAlerts(b *strings.Builder, s *session, w int) {
	for _, al := range s.host.alerts {
		b.WriteString(a.s.Warn.Render(wrap("▲ "+al.Text(), w)) + "\n")
	}
}

// monitorLine: eingestellte Schwellen und Prüfrhythmus.
func (a *App) monitorLine(net bool) string {
	m := a.st.Settings().Monitor
	c, r, d := m.Limits()
	var txt string
	if net {
		txt = fmt.Sprintf(i18n.T("Warnung ab CPU %d %% · RAM %d %% · nur auf Abruf (⏎)"), c, r)
	} else {
		txt = fmt.Sprintf(i18n.T("Warnung ab CPU %d %% · RAM %d %% · Platte %d %%"), c, r, d)
		if e := m.Every(); e > 0 {
			txt += fmt.Sprintf(i18n.T(" · prüft alle %d Min"), int(e.Minutes()))
		} else {
			txt += i18n.T(" · automatische Prüfung aus")
		}
	}
	return txt + i18n.T(" · g ändern")
}

// bar: Belegungsbalken, ab der Warnschwelle gelb.
func (a *App) bar(pct, w, limit int) string {
	w = max(4, min(w-6, 24))
	n := min(w, max(0, pct*w/100))
	style := a.s.OK
	if limit > 0 && pct >= limit {
		style = a.s.Warn
	}
	return style.Render(strings.Repeat("█", n)) + a.s.Muted.Render(strings.Repeat("░", w-n)) + a.s.Muted.Render(fmt.Sprintf(" %d%%", pct))
}

// ---------------------------------------------------------------- Textanzeige

// textModal: großer, blätterbarer Text (Diff, Konfiguration).
func textModal(title string, lines []string) *modal {
	return &modal{kind: mText, title: title, lines: lines}
}

func (m *modal) textView(s Styles, w, h int) string {
	width := dialog{maxW: 200}.innerMax(w)
	maxBody := max(3, h-2-dialogChrome)
	total := len(m.lines)
	m.scroll = max(0, min(m.scroll, total-maxBody))
	end := min(total, m.scroll+maxBody)
	var body []string
	for _, l := range m.lines[m.scroll:end] {
		body = append(body, trunc(l, width))
	}
	keys := [][2]string{{"Esc", i18n.T("schließen")}}
	info := ""
	if total > maxBody {
		info = fmt.Sprintf(i18n.T("Zeile %d–%d von %d"), m.scroll+1, end, total)
		keys = append([][2]string{{"↑↓", i18n.T("blättern")}, {"Bild↑", ""}, {"Bild↓", ""}, {"Pos1", ""}, {"Ende", ""}}, keys...)
	}
	return s.dialog(dialog{title: m.title, body: strings.Join(body, "\n"), info: info, keys: keys, width: width, maxW: 200}, w)
}

func (m *modal) textKey(key string) (closeIt bool) {
	switch key {
	case "up", "k":
		m.scroll = max(0, m.scroll-1)
	case "down", "j":
		m.scroll++
	case "pgup":
		m.scroll = max(0, m.scroll-20)
	case "pgdown", "space":
		m.scroll += 20
	case "home", "g":
		m.scroll = 0
	case "end", "G":
		m.scroll = len(m.lines)
	case "esc", "q", "enter":
		return true
	}
	return false
}
