package ui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Tab-Verwaltung
//
// Die Tab-Leiste oben (tabStrip) hat ein „+“ für einen neuen Tab, ein „×“ am
// aktiven Tab und ein Menü per Rechtsklick (Mittelklick schließt). Dazu
// kommt die Tab-Liste (Alt+T): alle offenen Tabs mit Status, Ordner und
// letztem Befehl, durchsuchbar, mit Tasten zum Springen, Schließen,
// Umbenennen, Duplizieren und Verschieben. Beides ruft dieselben Funktionen
// hier auf.

// tabName: eigener Name oder Alias.
func (s *session) tabName() string {
	if s.title != "" {
		return s.title
	}
	return s.conn.Alias
}

// closeTabAsk schließt einen Tab. Läuft die Verbindung noch, fragt
// sshTermy vorher nach.
func (a *App) closeTabAsk(s *session, after func()) tea.Cmd {
	done := func() tea.Cmd {
		a.closeTab(s)
		if after != nil {
			after()
		}
		return nil
	}
	if s.state != stConnected {
		return done()
	}
	if a.wantsRecap(s) { // Begleiter „Coach“: erst Rückblick, dann schließen
		return a.openRecap(s, true, func() { done() })
	}
	a.modal = confirmModal(i18n.T("Tab schließen?"),
		i18n.F("Die Verbindung zu %s wird getrennt.", s.tabName())+"\n"+i18n.T("Läuft dort noch etwas, bleibt es nur in tmux oder screen erhalten."), done)
	return nil
}

// closeOtherTabs schließt alle Tabs außer s (nach einer Rückfrage).
func (a *App) closeOtherTabs(s *session) tea.Cmd {
	n := len(a.sessions) - 1
	if n <= 0 {
		return nil
	}
	a.modal = confirmModal(i18n.T("Andere Tabs schließen?"),
		i18n.F("%d Tabs außer %s werden geschlossen und ihre Verbindungen getrennt.", n, s.tabName()), func() tea.Cmd {
			for _, x := range append([]*session(nil), a.sessions...) {
				if x != s {
					a.closeTab(x)
				}
			}
			a.focusSession(a.indexOf(s))
			return nil
		})
	return nil
}

// renameTab fragt nach einem eigenen Namen ("" = wieder der Alias).
func (a *App) renameTab(s *session, after func()) tea.Cmd {
	m := inputModal(i18n.T("Tab umbenennen"), i18n.T("Leer lassen für den Namen der Verbindung."), s.title, func(v string) tea.Cmd {
		if v == s.conn.Alias {
			v = ""
		}
		s.title = v
		if after != nil {
			after()
		}
		return nil
	})
	m.in.Placeholder = s.conn.Alias
	a.modal = m
	return m.in.Focus()
}

// duplicateTab öffnet dieselbe Verbindung noch einmal in einem neuen Tab
// (direkt rechts daneben).
func (a *App) duplicateTab(s *session) tea.Cmd {
	c := s.conn
	if fresh, ok := a.st.Connection(c.ID); ok {
		c = fresh
	}
	at := a.indexOf(s) + 1
	cmd := a.connect(c)
	a.moveTabTo(len(a.sessions)-1, at)
	return cmd
}

// moveTab verschiebt Tab s um d Stellen (−1 = nach links).
func (a *App) moveTab(s *session, d int) {
	i := a.indexOf(s)
	if i < 0 {
		return
	}
	a.moveTabTo(i, clamp(i+d, 0, len(a.sessions)-1))
}

// moveTabTo verschiebt den Tab an Stelle i nach j; der aktive bleibt aktiv.
// Geteilte Bereiche merken sich Sitzungs-IDs, sie bleiben unberührt.
func (a *App) moveTabTo(i, j int) {
	if i == j || i < 0 || j < 0 || i >= len(a.sessions) || j >= len(a.sessions) {
		return
	}
	cur := a.current()
	s := a.sessions[i]
	a.sessions = append(a.sessions[:i], a.sessions[i+1:]...)
	a.sessions = append(a.sessions[:j], append([]*session{s}, a.sessions[j:]...)...)
	if k := a.indexOf(cur); k >= 0 {
		a.cur = k
	}
}

// reopenTab verbindet einen getrennten Tab neu.
func (a *App) reopenTab(s *session) tea.Cmd {
	switch s.state {
	case stConnected, stConnecting:
		return nil
	case stReconnect:
		return a.reconnectNow(s)
	}
	if fresh, ok := a.st.Connection(s.conn.ID); ok {
		s.conn = fresh
	}
	s.state, s.err = stConnecting, ""
	return a.connectCmd(s)
}

// tabMenu: Kontextmenü zu einem Tab (Rechtsklick in der Leiste oder Liste).
func (a *App) tabMenu(s *session, x, y int, after func()) *modal {
	k := a.keys
	i := a.indexOf(s)
	items := []menuItem{
		{label: i18n.T("Umbenennen"), run: func() tea.Cmd { return a.renameTab(s, after) }},
		{label: i18n.T("Duplizieren"), run: func() tea.Cmd { return a.duplicateTab(s) }},
	}
	if s.state != stConnected && s.state != stConnecting {
		items = append(items, menuItem{label: i18n.T("Neu verbinden"), hint: "⏎", run: func() tea.Cmd { return a.reopenTab(s) }})
	}
	if s.state == stConnected {
		label := i18n.T("In eigenes Fenster")
		if s.detached() {
			label = i18n.T("Aus eigenem Fenster zurückholen")
		}
		items = append(items, menuItem{label: label, hint: k.label(actDetach), run: func() tea.Cmd { return a.toggleDetach(s) }})
	}
	if i > 0 {
		items = append(items, menuItem{label: i18n.T("Nach links"), run: func() tea.Cmd { a.moveTab(s, -1); return nil }})
	}
	if i < len(a.sessions)-1 {
		items = append(items, menuItem{label: i18n.T("Nach rechts"), run: func() tea.Cmd { a.moveTab(s, 1); return nil }})
	}
	items = append(items, menuItem{label: i18n.T("Tab schließen"), hint: k.label(actCloseTab), run: func() tea.Cmd { return a.closeTabAsk(s, after) }})
	if len(a.sessions) > 1 {
		items = append(items, menuItem{label: i18n.T("Andere Tabs schließen"), run: func() tea.Cmd { return a.closeOtherTabs(s) }})
	}
	return &modal{kind: mMenu, title: trunc(s.tabName(), 30), items: items, at: [2]int{x, y}, anchored: true}
}

// ---------------------------------------------------------------- Neuer Tab

// openNewTab: Auswahl einer Verbindung für einen neuen Tab. Offene und
// zuletzt benutzte stehen oben; ⏎ öffnet immer einen neuen Tab, auch wenn
// die Verbindung schon offen ist (zweite Shell auf demselben Server).
func (a *App) openNewTab() tea.Cmd {
	all := a.st.Connections()
	rank := func(c domain.Connection) int {
		switch {
		case a.connected(c.ID):
			return 0
		case !c.LastUsed.IsZero():
			return 1
		}
		return 2
	}
	sort.SliceStable(all, func(i, j int) bool {
		ri, rj := rank(all[i]), rank(all[j])
		if ri != rj {
			return ri < rj
		}
		return all[i].LastUsed.After(all[j].LastUsed)
	})
	byID := map[string]domain.Connection{}
	// oben: Shell dieses Rechners (immer ein neuer Tab)
	lc := localConn("")
	byID[lc.ID] = lc
	items := []pickItem{{value: lc.ID, label: ic("command") + " " + i18n.T("Lokale Shell"), detail: i18n.T("dieser Rechner"),
		hint: shortTarget(lc), search: "local lokal shell terminal powershell pwsh cmd zsh bash"}}
	for _, c := range all {
		byID[c.ID] = c
		icon, _ := a.devIcon(c)
		hint := shortTarget(c)
		if a.connected(c.ID) {
			hint = i18n.T("offen") + " · " + hint
		}
		items = append(items, pickItem{value: c.ID, label: icon + " " + c.Alias, detail: c.Group, hint: hint,
			search: c.Host + " " + strings.Join(c.Tags, " ")})
	}
	p := newPicker(i18n.T("Neuer Tab"), i18n.T("Name, Host, Ordner oder Tag"), items, "\x00", func(it pickItem) tea.Cmd {
		if c, ok := byID[it.value]; ok {
			return a.connect(c)
		}
		return nil
	})
	p.keys = [][2]string{{"⏎", i18n.T("öffnen")}, {"Esc", i18n.T("zurück")}, {"", i18n.T("tippen filtert")}}
	p.extra = func(q string) *pickItem {
		if q != "" {
			return nil
		}
		return &pickItem{value: "\x00new", label: ic("new") + " " + i18n.T("Neue Verbindung anlegen …")}
	}
	inner := p.onPick
	p.onPick = func(it pickItem) tea.Cmd {
		if it.value == "\x00new" {
			a.scr = scrManager
			return a.openConnForm(newConnDefaults())
		}
		return inner(it)
	}
	return a.openPicker(p)
}

// ---------------------------------------------------------------- Tab-Liste

func (a *App) tabAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d >= time.Hour {
		return i18n.F("vor %d h", int(d.Hours()))
	}
	return fmtAgo(d)
}

// tabListItems: eine Zeile pro Tab, Wert = Sitzungs-ID.
func (a *App) tabListItems() []pickItem {
	var items []pickItem
	for i, s := range a.sessions {
		st := a.s
		dot := map[sessState]string{stConnecting: st.Warn.Render("◌"), stConnected: st.OK.Render("●"), stClosed: st.Danger.Render("○"), stReconnect: st.Warn.Render("◌")}[s.state]
		if s.detached() {
			dot = st.AI.Render("↑")
		}
		var hint string
		switch {
		case s.state == stClosed:
			hint = i18n.T("getrennt")
			if s.err != "" {
				hint += ": " + trunc(s.err, 30)
			}
		case s.state == stConnecting:
			hint = i18n.T("verbindet …")
		case s.state == stReconnect:
			hint = i18n.T("verbindet neu …")
		default:
			if n := len(s.history); n > 0 {
				hint = trunc(oneLine(s.history[n-1]), 22)
			}
			if ago := a.tabAgo(s.lastOutput); ago != "" {
				if hint != "" {
					hint += " · "
				}
				hint += ago
			}
		}
		label := strconv.Itoa(i+1) + "  " + dot + " " + s.tabName()
		if s.title != "" {
			label += " (" + s.conn.Alias + ")"
		}
		items = append(items, pickItem{value: strconv.Itoa(s.id), label: label, detail: s.conn.Group, hint: hint,
			search: s.conn.Alias + " " + s.conn.Host})
	}
	return items
}

// openTabList zeigt alle offenen Tabs.
func (a *App) openTabList() tea.Cmd {
	if len(a.sessions) == 0 {
		return a.openNewTab()
	}
	var p *picker
	reload := func() {
		if a.pick != p {
			return
		}
		if len(a.sessions) == 0 {
			a.pick = nil
			return
		}
		p.items = a.tabListItems()
	}
	sessOf := func(it *pickItem) *session {
		if it == nil {
			return nil
		}
		id, _ := strconv.Atoi(it.value)
		return a.byID(id)
	}
	p = newPicker(i18n.T("Offene Tabs"), i18n.T("Name, Ordner oder Host"), a.tabListItems(), "\x00", func(it pickItem) tea.Cmd {
		if s := sessOf(&it); s != nil {
			a.focusSession(a.indexOf(s))
			a.scr = scrTerminal
		}
		return nil
	})
	if c := a.current(); c != nil {
		p.sel = a.indexOf(c)
	}
	p.reload = a.tabListItems
	p.keys = [][2]string{{"⏎", i18n.T("wechseln")}, {"1…9", i18n.T("springen")}, {"Strg+W", i18n.T("schließen")},
		{"Strg+R", i18n.T("umbenennen")}, {"Strg+D", i18n.T("duplizieren")}, {"⇧↑↓", i18n.T("verschieben")}, {"Esc", i18n.T("zurück")}}
	p.onMenu = func(it pickItem, m tea.Mouse) tea.Cmd {
		if s := sessOf(&it); s != nil {
			// Menü ersetzt die Liste; nach dem Umbenennen o. Ä. kommt sie wieder
			a.pick = nil
			a.modal = a.tabMenu(s, m.X, m.Y, nil)
		}
		return nil
	}
	p.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		s := sessOf(it)
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' && p.in.Value() == "" {
			if n := int(key[0] - '1'); n < len(a.sessions) {
				a.pick = nil
				a.focusSession(n)
				a.scr = scrTerminal
			}
			return nil, true
		}
		if s == nil {
			return nil, false
		}
		switch key {
		case "ctrl+w", "delete":
			return a.closeTabAsk(s, reload), true
		case "ctrl+r":
			return a.renameTab(s, reload), true
		case "ctrl+d":
			a.pick = nil
			return a.duplicateTab(s), true
		case "shift+up", "shift+down", "alt+up", "alt+down":
			d := 1
			if strings.HasSuffix(key, "up") {
				d = -1
			}
			a.moveTab(s, d)
			p.items = a.tabListItems()
			p.in.SetValue("")
			p.sel = a.indexOf(s)
			return nil, true
		}
		return nil, false
	}
	return a.openPicker(p)
}
