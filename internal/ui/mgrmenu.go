package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Rechtsklick-Menü im Manager: dieselben Aktionen wie die Tasten in der
// Fußzeile, nur zum Anklicken. Für Verbindungen und Ordner.

func (a *App) mgrMenu(r mgrRow, x, y int) *modal {
	if r.isFolder() {
		return a.mgrFolderMenu(r, x, y)
	}
	k := a.keys
	c := r.conn
	items := []menuItem{
		{label: i18n.T("Verbinden"), hint: "⏎", run: func() tea.Cmd { return a.connect(c) }},
		{label: i18n.T("Bearbeiten …"), hint: "e", sep: true, run: func() tea.Cmd { return a.openConnForm(c) }},
		{label: i18n.T("In Ordner verschieben …"), hint: k.label(actMoveTo), run: func() tea.Cmd { return a.moveToDialog(r) }},
	}
	if !c.Protocol.Raw() && !c.Protocol.NoHost() {
		items = append(items,
			menuItem{label: i18n.T("Tunnel …"), hint: "t", sep: true, run: func() tea.Cmd { return a.mgrNewTunnel(c) }},
			menuItem{label: i18n.T("SSH-Schlüssel erzeugen / übertragen …"), hint: k.label(actSSHKey), run: func() tea.Cmd { return a.openSSHKeyForm(c) }})
	}
	items = append(items, menuItem{label: i18n.T("Schnellbefehle …"), hint: k.label(actQuickEdit), run: func() tea.Cmd { return a.openQuickForm(c, "") }})
	if !c.Protocol.NoHost() && c.JumpHostID == "" && !a.connected(c.ID) {
		items = append(items, menuItem{label: i18n.T("Aufwecken (Wake-on-LAN)"), hint: orDefault(c.MAC, i18n.T("MAC fehlt")), run: func() tea.Cmd { return a.wake(c) }})
	}
	mark := i18n.T("Markieren (für Sammeländerung)")
	if a.mgr.marked[c.ID] {
		mark = i18n.T("Markierung aufheben")
	}
	items = append(items,
		menuItem{label: mark, hint: i18n.T("Leertaste"), sep: true, run: func() tea.Cmd { a.mgrToggleMark(c.ID); return nil }},
		menuItem{label: i18n.T("Löschen"), hint: "d", sep: true, run: func() tea.Cmd { a.mgrDeleteConn(c); return nil }})
	return &modal{kind: mMenu, title: trunc(c.Alias, 30), items: items, at: [2]int{x, y}, anchored: true}
}

func (a *App) mgrFolderMenu(r mgrRow, x, y int) *modal {
	k := a.keys
	n := r.node
	open := i18n.T("Zuklappen")
	if n.Collapsed {
		open = i18n.T("Aufklappen")
	}
	items := []menuItem{
		{label: open, hint: "⏎", run: func() tea.Cmd { a.toggleFolder(n.Path, !n.Collapsed); return nil }},
		{label: i18n.T("Neue Verbindung hier …"), hint: "n", run: func() tea.Cmd {
			c := newConnDefaults()
			c.Group = n.Path
			return a.openConnForm(c)
		}},
		{label: i18n.T("Alle als Raster öffnen"), hint: k.label(actFolderGrid), run: func() tea.Cmd { return a.openFolderGrid(n.Path) }},
		{label: i18n.T("Bearbeiten …"), hint: "e", sep: true, run: func() tea.Cmd { return a.openFolderForm(n.Path) }},
		{label: i18n.T("In Ordner verschieben …"), hint: k.label(actMoveTo), run: func() tea.Cmd { return a.moveToDialog(r) }},
		{label: i18n.T("Löschen …"), hint: "d", sep: true, run: func() tea.Cmd { a.deleteFolderDialog(n); return nil }},
	}
	return &modal{kind: mMenu, title: trunc(r.name(), 30), items: items, at: [2]int{x, y}, anchored: true}
}
