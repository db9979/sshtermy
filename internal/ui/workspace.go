package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Arbeitsplätze
//
// Ein Arbeitsplatz merkt sich eine geteilte Ansicht: Aufteilung, Größen und
// welche Verbindung in welchem Bereich lief. Öffnen verbindet alles auf
// einmal (schon offene Tabs derselben Verbindung werden genommen statt neu
// verbunden). „Zuletzt“ schreibt sshTermy beim Beenden von selbst, solange
// die Ansicht geteilt war. Ein Ordner im Manager (g) öffnet alle seine
// Verbindungen als Raster. Arbeitsplätze gehen mit dem Sync mit, „Zuletzt“
// bleibt auf dem Rechner.

// currentWorkspace: die geteilte Ansicht als Arbeitsplatz (nur gespeicherte
// Verbindungen; Schnellverbindungen bleiben leere Bereiche).
func (a *App) currentWorkspace(name string) (domain.Workspace, bool) {
	w := domain.Workspace{Name: name, Layout: a.split.id(), Saved: time.Now()}
	ids := a.panes
	if a.split == splitOff {
		if s := a.current(); s != nil {
			ids = []int{s.id}
		}
	} else {
		w.FracX = append([]float64(nil), a.fracX...)
		w.FracY = append([]float64(nil), a.fracY...)
	}
	n := 0
	for _, id := range ids {
		cid := ""
		if s := a.byID(id); s != nil && s.conn.ID != "" {
			if _, ok := a.st.Connection(s.conn.ID); ok {
				cid, n = s.conn.ID, n+1
			}
		}
		w.Conns = append(w.Conns, cid)
	}
	return w, n > 0
}

// rememberLayout: beim Beenden „Zuletzt“ merken (nur geteilte Ansicht).
func (a *App) rememberLayout() {
	if a.st == nil || a.split == splitOff {
		return
	}
	w, ok := a.currentWorkspace(i18n.T("Zuletzt"))
	if !ok {
		return
	}
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.LastLayout = &w })
}

// spawn legt einen Tab für c an (ohne Fokus und Aufteilung zu ändern).
func (a *App) spawn(c domain.Connection) (*session, tea.Cmd) {
	a.nextSID++
	s := &session{id: a.nextSID, conn: c, state: stConnecting, started: time.Now(), chipArmed: -1, quickArmed: -1}
	a.sessions = append(a.sessions, s)
	return s, a.connectCmd(s)
}

// openConns öffnet die Verbindungen in der Aufteilung m (offene Tabs
// derselben Verbindung werden wiederverwendet). fx/fy: Größen.
func (a *App) openConns(conns []domain.Connection, m splitMode, fx, fy []float64) tea.Cmd {
	if len(conns) == 0 {
		return nil
	}
	if !a.fits(m) {
		// passt nicht: die größte Aufteilung, die passt
		for _, alt := range []splitMode{splitGrid6, splitGrid, splitCols3, splitMain, splitCols, splitRows, splitOff} {
			if alt.slots() <= m.slots() && a.fits(alt) {
				a.flash(i18n.T("Fenster zu klein für „") + m.label() + i18n.T("“, nehme „") + alt.label() + "“")
				m, fx, fy = alt, nil, nil
				break
			}
		}
	}
	var cmds []tea.Cmd
	used := map[int]bool{}
	var ids []int
	for _, c := range conns {
		if c.ID == "" {
			ids = append(ids, 0)
			continue
		}
		var s *session
		for _, x := range a.sessions {
			if x.conn.ID == c.ID && !used[x.id] && x.state != stClosed {
				s = x
				break
			}
		}
		if s == nil {
			var cmd tea.Cmd
			s, cmd = a.spawn(c)
			cmds = append(cmds, cmd)
		}
		used[s.id] = true
		ids = append(ids, s.id)
	}
	a.split, a.zoom, a.syncIn = m, false, false
	a.fracX, a.fracY = fx, fy
	first := 0
	for _, id := range ids {
		if id != 0 {
			first = id
			break
		}
	}
	if m == splitOff {
		a.panes = nil
	} else {
		a.panes = make([]int, m.slots())
		copy(a.panes, ids)
	}
	if s := a.byID(first); s != nil {
		a.cur = a.indexOf(s)
	}
	a.scr = scrTerminal
	a.resizeAll()
	if extra := len(ids) - len(a.panes); m != splitOff && extra > 0 {
		a.flash(i18n.F("%d weitere Verbindungen als Tabs geöffnet (F7/F8)", extra))
	}
	return tea.Batch(cmds...)
}

// openWorkspace öffnet einen gespeicherten Arbeitsplatz.
func (a *App) openWorkspace(w domain.Workspace) tea.Cmd {
	if w.Folder != "" {
		return a.openFolderGrid(w.Folder)
	}
	m, ok := splitByID(w.Layout)
	if !ok {
		m = splitOff
	}
	var conns []domain.Connection
	missing := 0
	for _, id := range w.Conns {
		c, ok := a.st.Connection(id)
		if id != "" && !ok {
			missing++
		}
		if !ok {
			c = domain.Connection{}
		}
		conns = append(conns, c)
	}
	if missing == len(w.Conns) {
		a.flashErr(i18n.T("Die Verbindungen dieses Arbeitsplatzes gibt es nicht mehr"))
		return nil
	}
	if missing > 0 {
		a.flash(i18n.F("%d Verbindungen gibt es nicht mehr, ihre Bereiche bleiben leer", missing))
	}
	return a.openConns(conns, m, w.FracX, w.FracY)
}

// layoutFor: passende Aufteilung für n Verbindungen.
func layoutFor(n int) splitMode {
	switch {
	case n <= 1:
		return splitOff
	case n == 2:
		return splitCols
	case n == 3:
		return splitMain
	case n == 4:
		return splitGrid
	}
	return splitGrid6
}

// folderConns: Verbindungen im Ordner, erst die direkten, dann die der
// Unterordner (Reihenfolge wie im Manager).
func (a *App) folderConns(path string) []domain.Connection {
	var find func(n *domain.TreeNode) *domain.TreeNode
	find = func(n *domain.TreeNode) *domain.TreeNode {
		if n.Path == path {
			return n
		}
		for _, f := range n.Folders {
			if r := find(f); r != nil {
				return r
			}
		}
		return nil
	}
	node := find(a.st.Tree())
	if node == nil {
		return nil
	}
	var out []domain.Connection
	var walk func(n *domain.TreeNode)
	walk = func(n *domain.TreeNode) {
		out = append(out, n.Conns...)
		for _, f := range n.Folders {
			walk(f)
		}
	}
	walk(node)
	return out
}

// openFolderGrid: alle Verbindungen eines Ordners als Raster öffnen.
func (a *App) openFolderGrid(path string) tea.Cmd {
	conns := a.folderConns(path)
	if len(conns) == 0 {
		a.flash(i18n.T("Im Ordner liegen keine Verbindungen"))
		return nil
	}
	return a.openConns(conns, layoutFor(len(conns)), nil, nil)
}

// workspaceItems: „Zuletzt“ und die gespeicherten Arbeitsplätze.
func (a *App) workspaceItems() []pickItem {
	set := a.st.Settings()
	var items []pickItem
	desc := func(w domain.Workspace) (string, string) {
		m, _ := splitByID(w.Layout)
		if w.Folder != "" {
			return i18n.T("Ordner „") + w.Folder + "“", i18n.T("Raster")
		}
		var names []string
		for _, id := range w.Conns {
			if c, ok := a.st.Connection(id); ok {
				names = append(names, c.Alias)
			}
		}
		return strings.Join(names, " · "), m.label()
	}
	if w := set.LastLayout; w != nil {
		d, h := desc(*w)
		items = append(items, pickItem{value: "last", label: i18n.T("Zuletzt") + " (" + w.Saved.Format("02.01. 15:04") + ")", detail: d, hint: h, search: d})
	}
	for i, w := range set.Workspaces {
		d, h := desc(w)
		items = append(items, pickItem{value: strconv.Itoa(i), label: w.Name, detail: d, hint: h, search: d})
	}
	return items
}

func (a *App) workspaceOf(it *pickItem) (domain.Workspace, int, bool) {
	if it == nil {
		return domain.Workspace{}, -1, false
	}
	set := a.st.Settings()
	if it.value == "last" {
		if set.LastLayout != nil {
			return *set.LastLayout, -1, true
		}
		return domain.Workspace{}, -1, false
	}
	i, err := strconv.Atoi(it.value)
	if err != nil || i < 0 || i >= len(set.Workspaces) {
		return domain.Workspace{}, -1, false
	}
	return set.Workspaces[i], i, true
}

// saveWorkspace speichert die aktuelle Ansicht unter name (gleicher Name =
// ersetzen).
func (a *App) saveWorkspace(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	w, ok := a.currentWorkspace(name)
	if !ok {
		a.flash(i18n.T("Erst Verbindungen öffnen, dann die Ansicht speichern"))
		return nil
	}
	err := a.st.UpdateSettings(func(s *domain.Settings) {
		for i, x := range s.Workspaces {
			if strings.EqualFold(x.Name, name) {
				s.Workspaces[i] = w
				return
			}
		}
		s.Workspaces = append(s.Workspaces, w)
	})
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.flashOK(i18n.T("Arbeitsplatz „") + name + i18n.T("“ gespeichert"))
	return nil
}

// saveWorkspaceAsk fragt nach dem Namen.
func (a *App) saveWorkspaceAsk() tea.Cmd {
	if _, ok := a.currentWorkspace(""); !ok {
		a.flash(i18n.T("Erst Verbindungen öffnen, dann die Ansicht speichern"))
		return nil
	}
	a.pick = nil
	a.modal = inputModal(i18n.T("Ansicht als Arbeitsplatz speichern"), i18n.T("Name, z. B. „Web-Cluster“. Gleicher Name ersetzt den alten."), "", a.saveWorkspace)
	return nil
}

// openWorkspaces: Liste der Arbeitsplätze.
func (a *App) openWorkspaces() tea.Cmd {
	var p *picker
	p = newPicker(i18n.T("Arbeitsplätze"), i18n.T("Name oder Verbindung"), a.workspaceItems(), "\x00", func(it pickItem) tea.Cmd {
		if w, _, ok := a.workspaceOf(&it); ok {
			return a.openWorkspace(w)
		}
		return nil
	})
	p.help = i18n.T("⏎ öffnet Aufteilung und Verbindungen. Ordner im Manager: ") + a.keys.label(actFolderGrid) + i18n.T(" = als Raster")
	p.reload = a.workspaceItems
	p.keys = [][2]string{{"⏎", i18n.T("öffnen")}, {"Strg+S", i18n.T("aktuelle Ansicht speichern")}, {"Strg+R", i18n.T("umbenennen")}, {"Strg+D", i18n.T("löschen")}, {"Esc", i18n.T("zurück")}}
	if _, ok := a.currentWorkspace(""); ok {
		p.extra = func(q string) *pickItem {
			if strings.TrimSpace(q) == "" {
				return nil
			}
			return &pickItem{value: "save:" + q, label: i18n.T("+ Aktuelle Ansicht speichern als „") + q + "“"}
		}
		inner := p.onPick
		p.onPick = func(it pickItem) tea.Cmd {
			if name, ok := strings.CutPrefix(it.value, "save:"); ok {
				return a.saveWorkspace(name)
			}
			return inner(it)
		}
	}
	p.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		switch key {
		case "ctrl+s":
			return a.saveWorkspaceAsk(), true
		}
		w, i, ok := a.workspaceOf(it)
		if !ok {
			return nil, false
		}
		switch key {
		case "ctrl+r":
			if i < 0 {
				return nil, true
			}
			a.pick = nil
			a.modal = inputModal(i18n.T("Arbeitsplatz umbenennen"), "", w.Name, func(v string) tea.Cmd {
				v = strings.TrimSpace(v)
				if v != "" {
					_ = a.st.UpdateSettings(func(s *domain.Settings) {
						if i < len(s.Workspaces) {
							s.Workspaces[i].Name = v
						}
					})
				}
				return a.openWorkspaces()
			})
			return nil, true
		case "ctrl+d", "delete":
			_ = a.st.UpdateSettings(func(s *domain.Settings) {
				if i < 0 {
					s.LastLayout = nil
				} else if i < len(s.Workspaces) {
					s.Workspaces = append(s.Workspaces[:i], s.Workspaces[i+1:]...)
				}
			})
			p.items = a.workspaceItems()
			p.sel = clamp(p.sel, 0, max(0, len(p.items)-1))
			a.flash(i18n.T("Arbeitsplatz „") + w.Name + i18n.T("“ gelöscht"))
			return nil, true
		}
		return nil, false
	}
	return a.openPicker(p)
}
