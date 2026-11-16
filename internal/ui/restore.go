package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Offene Sitzungen über einen Neustart retten: sshTermy merkt sich, welche
// Tabs offen waren (nur Verweise, keine Geheimnisse), samt geteilter Ansicht,
// und öffnet sie nach dem Entsperren wieder. tmux/screen wird wie beim
// Auto-Reconnect wieder angehängt, laufende Arbeit bleibt so auf dem Server.
//
// Zwei Anlässe: der Update-Neustart (Grund "", ohne Rückfrage) und jeder
// normale Start („Beim Start dort weitermachen“, Grund "session"). Für den
// zweiten schreibt sshTermy die Datei beim Beenden und alle 30 Sekunden neu,
// damit auch ein Absturz, Stromausfall oder das Herunterfahren des Rechners
// nichts kostet. Settings.RestoreTabs: fragen (Standard), immer, nie.

const restoreName = "restore-tabs.json"

// restoreMaxAge: ältere Dateien (Absturz, Neustart abgebrochen) ignorieren.
const restoreMaxAge = 30 * time.Minute

// restoreSessionMaxAge: „vom letzten Mal“ gilt zwei Wochen.
const restoreSessionMaxAge = 14 * 24 * time.Hour

// restoreEvery: so oft wird die Liste der offenen Tabs gesichert.
const restoreEvery = 30 * time.Second

const (
	restoreUpdate  = ""        // Update-Neustart
	restoreSession = "session" // Beenden, Absturz, Herunterfahren

	restoreAsk    = ""
	restoreAlways = "always"
	restoreNever  = "never"
)

type restoreTab struct {
	ConnID  string             `json:"conn_id,omitempty"`
	Adhoc   *domain.Connection `json:"adhoc,omitempty"` // nicht gespeicherte Verbindung, ohne Geheimnisse
	Mux     string             `json:"mux,omitempty"`
	MuxName string             `json:"mux_name,omitempty"`
	Title   string             `json:"title,omitempty"` // umbenannter Tab
}

type restoreState struct {
	Reason string       `json:"reason,omitempty"` // restoreUpdate oder restoreSession
	Saved  time.Time    `json:"saved"`
	Active int          `json:"active"`
	Tabs   []restoreTab `json:"tabs"`
	// geteilte Ansicht: Aufteilung, Größen, Tab-Nummer je Bereich (-1 = leer)
	Split string    `json:"split,omitempty"`
	FracX []float64 `json:"frac_x,omitempty"`
	FracY []float64 `json:"frac_y,omitempty"`
	Panes []int     `json:"panes,omitempty"`
	Zoom  bool      `json:"zoom,omitempty"`
}

func restorePath(dir string) string { return filepath.Join(dir, restoreName) }

// adhocCopy: nur, was zum Wiederverbinden nötig ist. Passwort, Key und
// Passphrase fragt sshTermy beim Verbinden wieder ab.
func adhocCopy(c domain.Connection) *domain.Connection {
	return &domain.Connection{
		Alias: c.Alias, Host: c.Host, Port: c.Port, Username: c.Username, Auth: c.Auth,
		KeyPath: c.KeyPath, JumpHostID: c.JumpHostID, RelayID: c.RelayID, RelaySystem: c.RelaySystem,
		Device: c.Device, Protocol: c.Protocol, SerialPort: c.SerialPort, Baud: c.Baud, Frame: c.Frame,
		LocalShell: c.LocalShell, LocalDir: c.LocalDir,
		Persist: c.Persist, PersistName: c.PersistName, PersistView: c.PersistView,
		ForwardAgent: c.ForwardAgent, ForwardX11: c.ForwardX11, CertPath: c.CertPath,
	}
}

// saveRestore schreibt die offenen Tabs (vor dem Update-Neustart).
func (a *App) saveRestore() { a.saveRestoreAs(restoreUpdate) }

// saveRestoreAs: Grund restoreUpdate oder restoreSession.
func (a *App) saveRestoreAs(reason string) {
	if a.st == nil || a.st.Dir == "" {
		return
	}
	st := restoreState{Reason: reason, Saved: time.Now()}
	tabOf := map[int]int{} // Sitzungs-ID → Nummer in Tabs
	for i, s := range a.sessions {
		if s.state == stClosed {
			continue
		}
		tabOf[s.id] = len(st.Tabs)
		s.checkMuxGone()
		t := restoreTab{Mux: s.mux.kind, MuxName: s.mux.name, Title: s.title}
		if _, ok := a.st.Connection(s.conn.ID); ok && s.conn.ID != "" {
			t.ConnID = s.conn.ID
		} else {
			t.Adhoc = adhocCopy(s.conn)
		}
		if i == a.cur {
			st.Active = len(st.Tabs)
		}
		st.Tabs = append(st.Tabs, t)
	}
	if a.split != splitOff {
		st.Split, st.FracX, st.FracY, st.Zoom = a.split.id(), a.fracX, a.fracY, a.zoom
		for _, id := range a.panes {
			n, ok := tabOf[id]
			if !ok {
				n = -1
			}
			st.Panes = append(st.Panes, n)
		}
	}
	p := restorePath(a.st.Dir)
	if len(st.Tabs) == 0 {
		if a.restoreSig != "-" {
			_ = os.Remove(p)
			a.restoreSig = "-"
		}
		return
	}
	// Nur schreiben, wenn sich an den Tabs etwas geändert hat (Zeit ausgenommen).
	sig, _ := json.Marshal(restoreState{Reason: st.Reason, Active: st.Active, Tabs: st.Tabs, Split: st.Split, FracX: st.FracX, FracY: st.FracY, Panes: st.Panes, Zoom: st.Zoom})
	if reason == restoreSession && string(sig) == a.restoreSig {
		return
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	// erst vollständig schreiben, dann ersetzen: ein Stromausfall hinterlässt
	// nie eine halbe Datei
	if os.WriteFile(p+".tmp", b, 0o600) == nil && os.Rename(p+".tmp", p) == nil {
		a.restoreSig = string(sig)
	}
}

// restoreMode: Einstellung „Tabs vom letzten Mal“ (fragen, immer, nie).
func (a *App) restoreMode() string { return a.st.Settings().RestoreTabs }

// restoreTick (aus onTick): Tabs regelmäßig sichern, damit auch ein Absturz
// oder das Herunterfahren nichts kostet. Erst nachdem die Tabs vom letzten
// Mal angeboten wurden, sonst würde die leere Liste sie löschen.
func (a *App) restoreTick(now time.Time) {
	if !a.restoreArmed || a.restart || a.restoreMode() == restoreNever || now.Sub(a.restoreAt) < restoreEvery {
		return
	}
	a.restoreAt = now
	a.saveRestoreAs(restoreSession)
}

// restoreOnQuit (aus quit): beim normalen Beenden die Tabs merken.
func (a *App) restoreOnQuit() {
	if !a.restoreArmed || a.restart {
		return // Update-Neustart hat seine Datei schon geschrieben
	}
	if a.restoreMode() == restoreNever {
		if a.st != nil && a.st.Dir != "" {
			_ = os.Remove(restorePath(a.st.Dir))
		}
		return
	}
	a.saveRestoreAs(restoreSession)
}

func restoreLabel(m string) string {
	switch m {
	case restoreAlways:
		return i18n.T("immer")
	case restoreNever:
		return i18n.T("nie")
	}
	return i18n.T("fragen")
}

var restoreModes = []string{restoreAsk, restoreAlways, restoreNever}

func (a *App) cycleRestore(dir int) tea.Cmd {
	m := restoreModes[cycleIndex(indexOf(restoreModes, a.restoreMode()), len(restoreModes), dir)]
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.RestoreTabs = m })
	if m == restoreNever && a.st.Dir != "" {
		_ = os.Remove(restorePath(a.st.Dir))
		a.restoreSig = ""
	}
	a.flash(i18n.T("Tabs vom letzten Mal beim Start: ") + restoreLabel(m))
	return nil
}

// loadRestore liest und löscht die Datei (nur einmal wiederherstellen).
func loadRestore(dir string, now time.Time) *restoreState {
	if dir == "" {
		return nil
	}
	p := restorePath(dir)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	_ = os.Remove(p)
	var st restoreState
	if json.Unmarshal(b, &st) != nil || len(st.Tabs) == 0 || st.Saved.After(now.Add(time.Minute)) {
		return nil
	}
	maxAge := restoreMaxAge
	if st.Reason == restoreSession {
		maxAge = restoreSessionMaxAge
	}
	if now.Sub(st.Saved) > maxAge {
		return nil
	}
	return &st
}

// restoreTabs öffnet die vor dem Neustart offenen Tabs wieder (nach dem
// Entsperren). Nach einem normalen Beenden fragt sshTermy erst (Einstellung).
func (a *App) restoreTabs() tea.Cmd {
	if a.restoreArmed {
		return nil // schon beim ersten Entsperren erledigt
	}
	a.restoreArmed = true
	st := loadRestore(a.st.Dir, time.Now())
	if st == nil || len(a.sessions) > 0 {
		return nil
	}
	if st.Reason != restoreSession {
		return a.openRestored(st)
	}
	switch a.restoreMode() {
	case restoreNever:
		return nil
	case restoreAlways:
		return a.openRestored(st)
	}
	var names []string
	for i, t := range st.Tabs {
		if i == 6 {
			names = append(names, i18n.F("… und %d weitere", len(st.Tabs)-i))
			break
		}
		names = append(names, "  • "+a.restoreName(t))
	}
	body := strings.Join(names, "\n")
	if st.Split != "" {
		body += "\n\n" + i18n.T("mit geteilter Ansicht")
	}
	body += "\n\n" + i18n.T("j = öffnen · n = nicht jetzt\nImmer oder nie: Befehlspalette, „Tabs vom letzten Mal“ suchen")
	a.showAsync(confirmModal(i18n.T("Tabs vom letzten Mal wieder öffnen?"), body, func() tea.Cmd { return a.openRestored(st) }))
	return nil
}

// restoreName: Anzeige eines gemerkten Tabs in der Rückfrage.
func (a *App) restoreName(t restoreTab) string {
	if t.Title != "" {
		return t.Title
	}
	if c, ok := a.st.Connection(t.ConnID); ok && t.ConnID != "" {
		return c.Alias
	}
	if t.Adhoc != nil {
		if t.Adhoc.Alias != "" {
			return t.Adhoc.Alias
		}
		return t.Adhoc.Host
	}
	return "?"
}

// openRestored: die gemerkten Tabs verbinden.
func (a *App) openRestored(st *restoreState) tea.Cmd {
	var cmds []tea.Cmd
	first := len(a.sessions)
	made := make([]*session, len(st.Tabs)) // Tab-Nummer → neue Sitzung
	for ti, t := range st.Tabs {
		var c domain.Connection
		switch {
		case t.ConnID != "":
			cc, ok := a.st.Connection(t.ConnID)
			if !ok {
				continue // inzwischen gelöscht (z. B. per Sync)
			}
			c = cc
		case t.Adhoc != nil && (t.Adhoc.Host != "" || t.Adhoc.Protocol.NoHost()):
			c = *t.Adhoc
		default:
			continue
		}
		cmd := a.connect(c)
		if s := a.current(); s != nil {
			s.title = t.Title
			made[ti] = s
		}
		// Feste Sitzung: hängt sich nach dem Verbinden selbst an (geteilt, ohne -d)
		if s := a.current(); s != nil && (t.Mux == "tmux" || t.Mux == "screen") && !c.Protocol.Raw() && c.Persist == "" {
			s.mux, s.muxPending = muxState{kind: t.Mux, name: t.MuxName}, true
		}
		cmds = append(cmds, cmd)
	}
	n := len(a.sessions) - first
	if n == 0 {
		return nil
	}
	a.focusSession(first + min(max(st.Active, 0), n-1))
	if m, ok := splitByID(st.Split); ok && m != splitOff && a.fits(m) {
		a.restoreSplit(m, st, made)
	}
	a.scr = scrTerminal
	if st.Reason == restoreSession {
		a.flashOK(plural(n, i18n.T("Sitzung"), i18n.T("Sitzungen")) + i18n.T(" vom letzten Mal wieder geöffnet"))
	} else {
		a.flashOK(plural(n, i18n.T("Sitzung"), i18n.T("Sitzungen")) + i18n.T(" von vor dem Update wieder geöffnet"))
	}
	return tea.Batch(cmds...)
}

// restoreSplit stellt die geteilte Ansicht von vor dem Neustart wieder her.
func (a *App) restoreSplit(m splitMode, st *restoreState, made []*session) {
	cur := a.current()
	a.split, a.fracX, a.fracY = m, st.FracX, st.FracY
	a.panes = make([]int, m.slots())
	for j, n := range st.Panes {
		if j < len(a.panes) && n >= 0 && n < len(made) && made[n] != nil {
			a.panes[j] = made[n].id
		}
	}
	if a.slotOf(cur) < 0 {
		a.fixFocus()
	}
	a.zoom = st.Zoom && a.slotOf(a.current()) >= 0
	a.resizeAll()
}
