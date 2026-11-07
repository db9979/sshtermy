package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Offene Sitzungen über einen Update-Neustart retten: Vor dem Beenden merkt
// sich sshTermy, welche Tabs offen waren (nur Verweise, keine Geheimnisse),
// und öffnet sie nach dem Entsperren der neuen Version wieder. tmux/screen
// wird wie beim Auto-Reconnect wieder angehängt, laufende Arbeit bleibt so
// auf dem Server erhalten.

const restoreName = "restore-tabs.json"

// restoreMaxAge: ältere Dateien (Absturz, Neustart abgebrochen) ignorieren.
const restoreMaxAge = 30 * time.Minute

type restoreTab struct {
	ConnID  string             `json:"conn_id,omitempty"`
	Adhoc   *domain.Connection `json:"adhoc,omitempty"` // nicht gespeicherte Verbindung, ohne Geheimnisse
	Mux     string             `json:"mux,omitempty"`
	MuxName string             `json:"mux_name,omitempty"`
	Title   string             `json:"title,omitempty"` // umbenannter Tab
}

type restoreState struct {
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
		Persist: c.Persist, PersistName: c.PersistName, PersistView: c.PersistView,
		ForwardAgent: c.ForwardAgent, CertPath: c.CertPath,
	}
}

// saveRestore schreibt die offenen Tabs (vor dem Update-Neustart).
func (a *App) saveRestore() {
	if a.st == nil || a.st.Dir == "" {
		return
	}
	st := restoreState{Saved: time.Now()}
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
		_ = os.Remove(p)
		return
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o600)
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
	if json.Unmarshal(b, &st) != nil || len(st.Tabs) == 0 || now.Sub(st.Saved) > restoreMaxAge || st.Saved.After(now.Add(time.Minute)) {
		return nil
	}
	return &st
}

// restoreTabs öffnet die vor dem Neustart offenen Tabs wieder (nach dem Entsperren).
func (a *App) restoreTabs() tea.Cmd {
	st := loadRestore(a.st.Dir, time.Now())
	if st == nil {
		return nil
	}
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
		case t.Adhoc != nil && (t.Adhoc.Host != "" || t.Adhoc.Protocol == domain.ProtoSerial):
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
	a.flashOK(plural(n, i18n.T("Sitzung"), i18n.T("Sitzungen")) + i18n.T(" von vor dem Update wieder geöffnet"))
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
