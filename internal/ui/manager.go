package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

type managerState struct {
	sel       int
	top       int // erste sichtbare Zeile (Blättern bei vielen Servern)
	listH     int // sichtbare Zeilen der Liste (letztes Bild)
	search    textinput.Model
	searching bool
	tag       string // aktiver Tag-Filter ("" = keiner)
	want      string // nach einer Änderung diese Zeile wieder auswählen (mgrRow.key)
	drag      *mgrDrag
	rows      []mgrRow // Zeilen des letzten Aufrufs von mgrRows
	tree      *domain.TreeNode
}

func newManager() managerState {
	in := newInput("", false)
	in.Placeholder = "Suchen (Name, Host, Ordner, #tag)"
	return managerState{search: in}
}

// managerKeys: nur Tasten, die in der aktuellen Lage etwas tun (Auswahl
// Ordner/Verbindung, leere Liste, Suche, Filter, offene Sitzungen).
func (a *App) managerKeys() [][2]string {
	km := a.keys
	m := &a.mgr
	if m.searching {
		return [][2]string{{"⏎", "fertig"}, {"↑↓", "zur Liste"}, {"Esc", "Suche löschen"}, {km.label(actHelp), "Hilfe"}}
	}
	rows := a.mgrRows()
	var cur mgrRow
	have := m.sel < len(rows)
	if have {
		cur = rows[m.sel]
	}
	k := [][2]string{{km.label(actPalette), "Palette"}}
	if len(a.sessions) > 0 {
		k = append(k, [2]string{km.label(actManager), "zum Terminal"})
	}
	switch {
	case have && cur.isFolder():
		open := "zuklappen"
		if cur.node.Collapsed {
			open = "aufklappen"
		}
		k = append(k, [2]string{"⏎", open}, [2]string{"n", "neu hier"}, [2]string{"e", "umbenennen"}, [2]string{"d", "löschen"},
			[2]string{km.label(actMoveTo), "verschieben"})
	case have:
		k = append(k, [2]string{"⏎", "verbinden"}, [2]string{"n", "neu"}, [2]string{"e", "bearbeiten"}, [2]string{"d", "löschen"},
			[2]string{km.label(actMoveTo), "verschieben"}, [2]string{"t", "Tunnel"}, [2]string{km.label(actQuickEdit), "Schnellbefehle"})
	default:
		k = append(k, [2]string{"n", "neue Verbindung"}, [2]string{km.label(actImport), "Import"}, [2]string{km.label(actScan), "Scan"})
	}
	if m.tag != "" || m.search.Value() != "" {
		k = append(k, [2]string{"Esc", "Filter aus"})
	}
	if len(rows) > 1 {
		k = append(k, [2]string{"/", "suchen"})
		if have && !a.mgrFiltering() {
			k = append(k, [2]string{km.label(actMoveUp) + "/" + keyLabel("down"), "sortieren"})
		}
	}
	k = append(k, [2]string{km.label(actFolderNew), "Ordner"})
	if a.hasTags() {
		k = append(k, [2]string{km.label(actTagFilter), "Tags"})
	}
	if have {
		k = append(k, [2]string{km.label(actBroadcast), "Broadcast"}, [2]string{km.label(actScan), "Scan"}, [2]string{km.label(actImport), "Import"})
	}
	k = append(k, [2]string{"s", "KI"}, [2]string{"y", "Sync"}, [2]string{"l", "Protokolle"},
		[2]string{km.label(actHelp), "Hilfe"}, [2]string{km.label(actCheat), "erklären"}, [2]string{km.label(actAbout), "Über"}, [2]string{km.label(actQuit), "beenden"})
	return sortFKeys(k)
}

func (a *App) hasTags() bool {
	for _, c := range a.st.Connections() {
		if len(c.Tags) > 0 {
			return true
		}
	}
	return false
}

func (a *App) updateManager(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	m := &a.mgr
	rows := a.mgrRows()
	if m.searching {
		switch k.String() {
		case "esc":
			m.searching = false
			m.search.SetValue("")
			m.search.Blur()
		case "enter":
			m.searching = false
			m.search.Blur()
		case "up", "down", "pgup", "pgdown":
			m.searching = false
			m.search.Blur()
			return a.updateManager(k)
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(k)
			m.sel, m.top = 0, 0
			return cmd
		}
		return nil
	}
	var cur mgrRow
	have := m.sel < len(rows)
	if have {
		cur = rows[m.sel]
	}
	switch a.keys.which(k.String(), actManager, actHelp, actAbout, actImport, actNextLook, actKeys, actBroadcast, actScan, actCheat, actQuickEdit,
		actFolderNew, actRename, actMoveTo, actMoveUp, actMoveDown, actTagFilter, actFoldAll, actUnfoldAll) {
	case actCheat:
		return a.toggleCheat()
	case actQuickEdit:
		if have && !cur.isFolder() {
			return a.openQuickForm(cur.conn, "")
		}
		return nil
	case actManager:
		if len(a.sessions) > 0 {
			a.scr = scrTerminal
		}
		return nil
	case actHelp:
		a.modal = a.helpModal()
		return nil
	case actAbout:
		a.modal = a.aboutModal()
		return nil
	case actImport:
		return a.openImport()
	case actNextLook:
		return a.cycleTheme()
	case actKeys:
		return a.openKeys()
	case actBroadcast:
		return a.openBroadcast()
	case actScan:
		return a.openScan()
	case actFolderNew:
		return a.newFolderDialog()
	case actRename:
		if have && cur.isFolder() {
			return a.renameFolderDialog(cur.node.Path)
		}
		if have {
			return a.openConnForm(cur.conn)
		}
		return nil
	case actMoveTo:
		if have {
			return a.moveToDialog(cur)
		}
		return nil
	case actMoveUp, actMoveDown:
		if have {
			d := 1
			if a.keys.is(actMoveUp, k.String()) {
				d = -1
			}
			a.moveStep(cur, d)
		}
		return nil
	case actTagFilter:
		return a.tagDialog()
	case actFoldAll, actUnfoldAll:
		collapse := a.keys.is(actFoldAll, k.String())
		if err := a.st.SetCollapsed(collapse); err != nil {
			a.flash(err.Error())
		}
		if have && collapse {
			// Auswahl auf den obersten Ordner der aktuellen Zeile
			if parts := domain.PathParts(cur.path()); len(parts) > 0 {
				m.want = "f:" + parts[0]
			}
		} else if have {
			m.want = cur.key()
		}
		return nil
	}
	page := max(1, m.listH-1)
	switch k.String() {
	case "up", "k":
		m.sel = max(0, m.sel-1)
	case "down", "j":
		m.sel = min(len(rows)-1, m.sel+1)
	case "pgup":
		m.sel = max(0, m.sel-page)
	case "pgdown":
		m.sel = min(len(rows)-1, m.sel+page)
	case "home":
		m.sel = 0
	case "end":
		m.sel = max(0, len(rows)-1)
	case "left":
		if !have {
			return nil
		}
		if cur.isFolder() && !cur.node.Collapsed && !a.mgrFiltering() {
			a.toggleFolder(cur.node.Path, true)
			return nil
		}
		// zum übergeordneten Ordner springen
		parent := cur.conn.Group
		if cur.isFolder() {
			parent = domain.PathParent(cur.node.Path)
		}
		if parent != "" {
			m.want = "f:" + parent
		}
	case "right":
		if have && cur.isFolder() {
			if cur.node.Collapsed {
				a.toggleFolder(cur.node.Path, false)
			} else if m.sel+1 < len(rows) && rows[m.sel+1].depth > cur.depth {
				m.sel++
			}
		}
	case "space":
		if have && cur.isFolder() {
			a.toggleFolder(cur.node.Path, !cur.node.Collapsed)
		}
	case "esc":
		if m.tag != "" || m.search.Value() != "" {
			m.tag = ""
			m.search.SetValue("")
			if have {
				m.want = cur.key()
			}
		}
	case "/":
		m.searching = true
		return m.search.Focus()
	case "n":
		c := newConnDefaults()
		c.Group = a.mgrContext()
		return a.openConnForm(c)
	case "e":
		if have && cur.isFolder() {
			return a.renameFolderDialog(cur.node.Path)
		}
		if have {
			return a.openConnForm(cur.conn)
		}
	case "d":
		if have && cur.isFolder() {
			a.deleteFolderDialog(cur.node)
			return nil
		}
		if have {
			c := cur.conn
			a.modal = confirmModal("Verbindung löschen", "„"+c.Alias+"“ und ihre Tunnel wirklich löschen?", func() tea.Cmd {
				if err := a.st.DeleteConnection(c.ID); err != nil {
					a.flash(err.Error())
				}
				m.sel = max(0, m.sel-1)
				return nil
			})
		}
	case "t":
		if have && !cur.isFolder() {
			return a.openTunnelForm(domain.Tunnel{ConnectionID: cur.conn.ID, Kind: domain.TunnelLocal, TargetHost: "127.0.0.1"})
		}
	case "s":
		return a.openSettings()
	case "y":
		return a.openSync()
	case "l":
		return a.openLogs()
	case "enter":
		if have && cur.isFolder() {
			a.toggleFolder(cur.node.Path, !cur.node.Collapsed)
			return nil
		}
		if have {
			return a.connect(cur.conn)
		}
	}
	return nil
}

func (a *App) connect(c domain.Connection) tea.Cmd {
	a.nextSID++
	s := &session{id: a.nextSID, conn: c, state: stConnecting, started: time.Now(), chipArmed: -1, quickArmed: -1}
	a.sessions = append(a.sessions, s)
	a.focusSession(len(a.sessions) - 1)
	a.scr = scrTerminal
	return a.connectCmd(s)
}

func (a *App) connected(id string) bool {
	for _, ss := range a.sessions {
		if ss.conn.ID == id && ss.state == stConnected {
			return true
		}
	}
	return false
}

// connectedIn: verbundene Sitzungen in einem Ordner (samt Unterordnern).
func (a *App) connectedIn(path string) int {
	n := 0
	seen := map[string]bool{}
	for _, ss := range a.sessions {
		if ss.state == stConnected && !seen[ss.conn.ID] && domain.InFolder(ss.conn.Group, path) {
			seen[ss.conn.ID] = true
			n++
		}
	}
	return n
}

func (a *App) viewManager() string {
	s := a.s
	m := &a.mgr
	rows := a.mgrRows()
	h := a.h - 2
	leftW := min(52, max(32, a.w/3))
	rightW := a.w - leftW - 1

	// linke Spalte: Baum aus Ordnern und Verbindungen
	var lb strings.Builder
	total := 0
	if m.tree != nil {
		total = m.tree.Total
	}
	switch {
	case m.drag != nil && m.drag.moved:
		desc, _ := a.dropPlan(rows, m.drag.from, m.drag.over)
		if desc == "" {
			desc = "hier nicht ablegen"
		}
		name := ""
		if m.drag.from < len(rows) {
			name = rows[m.drag.from].name()
		}
		lb.WriteString(" " + s.Warn.Render("⇅ „"+trunc(name, 16)+"“ "+trunc(desc, leftW-24)) + "\n")
	case m.searching || m.search.Value() != "":
		m.search.SetWidth(leftW - 6)
		lb.WriteString(" " + s.Accent.Render("/") + " " + m.search.View() + "\n")
	default:
		title := s.PanelTitle.Foreground(s.T.Accent).Render(" Verbindungen") + s.Muted.Render(fmt.Sprintf("  %d", total))
		// Hinweise nur, solange sie in die Zeile passen
		for _, h := range [][2]string{{"/", "/ suchen"}, {a.keys.label(actTagFilter), a.keys.label(actTagFilter) + " Tags"}} {
			if lipgloss.Width(title)+2+lipgloss.Width(h[1]) > leftW-1 {
				break
			}
			title += "  " + a.keyZone(h[0], s.Muted.Render(h[1]))
		}
		lb.WriteString(title + "\n")
	}
	// zweite Zeile: aktiver Filter bzw. Position
	info := ""
	if m.tag != "" {
		info = a.z.click(s.Accent.Render(" #"+m.tag+" ✕"), func(tea.Mouse, bool) tea.Cmd { m.tag = ""; return nil })
	}
	if a.mgrFiltering() {
		n := 0
		for _, r := range rows {
			if !r.isFolder() {
				n++
			}
		}
		info += s.Muted.Render(fmt.Sprintf("  %d Treffer · Esc zeigt alle", n))
	}
	listH := max(1, h-2)
	m.listH = listH
	if m.sel < m.top {
		m.top = m.sel
	}
	if m.sel >= m.top+listH {
		m.top = m.sel - listH + 1
	}
	m.top = clamp(m.top, 0, max(0, len(rows)-listH))
	if len(rows) > listH {
		info += s.Muted.Render(fmt.Sprintf("  %d–%d von %d", m.top+1, min(len(rows), m.top+listH), len(rows)))
	}
	lb.WriteString(info + "\n")

	for i := m.top; i < len(rows) && i < m.top+listH; i++ {
		r := rows[i]
		indent := strings.Repeat("  ", min(r.depth, 6))
		var plain, styled string
		if r.isFolder() {
			n := r.node
			arrow := "▾"
			if n.Collapsed && !a.mgrFiltering() {
				arrow = "▸"
			}
			cnt := fmt.Sprintf("%d", n.Total)
			if c := a.connectedIn(n.Path); c > 0 {
				cnt = fmt.Sprintf("●%d · %d", c, n.Total)
			}
			nw := max(6, leftW-4-len(indent)-len([]rune(cnt)))
			name := trunc(n.Name, nw)
			plain = fmt.Sprintf(" %s%s %s", indent, arrow, name)
			pad := strings.Repeat(" ", max(1, leftW-2-len([]rune(plain))-len([]rune(cnt))))
			styled = " " + indent + s.Accent.Render(arrow) + " " + s.PanelTitle.Render(name) + pad + s.Muted.Render(cnt)
			plain += pad + cnt
		} else {
			c := r.conn
			dot := s.Muted.Render("○")
			pdot := "○"
			if a.connected(c.ID) {
				dot, pdot = s.OK.Render("●"), "●"
			}
			aw := max(8, min(18-2*min(r.depth, 5), (leftW-6-len(indent))*11/20))
			hw := max(4, leftW-6-len(indent)-aw)
			alias := fmt.Sprintf("%-*s", aw, trunc(c.Alias, aw))
			target := trunc(c.Username+"@"+c.Host, hw)
			plain = fmt.Sprintf(" %s%s %s %s", indent, pdot, alias, target)
			styled = " " + indent + dot + " " + alias + " " + s.Muted.Render(target)
		}
		line := styled
		switch {
		case i == m.sel:
			line = s.Selected.Width(leftW - 1).Render(plain)
		case m.drag != nil && m.drag.moved && i == m.drag.over:
			line = lipgloss.NewStyle().Foreground(s.T.Warn).Underline(true).Width(leftW - 1).Render(plain)
		}
		i, r := i, r
		line = a.z.mark(line, zoneAction{row: i + 1, click: func(_ tea.Mouse, double bool) tea.Cmd {
			m.sel = i
			m.drag = &mgrDrag{from: i, over: i, dbl: double}
			if double && !r.isFolder() {
				m.drag = nil
				return a.connect(r.conn)
			}
			return nil
		}})
		lb.WriteString(line + "\n")
	}
	if len(rows) == 0 {
		switch {
		case a.mgrFiltering():
			lb.WriteString(s.Muted.Render("  Nichts gefunden.\n  Esc = Filter aufheben"))
		default:
			lb.WriteString(s.Muted.Render("  Noch keine Verbindung.\n  n = neue Verbindung anlegen\n  o = Ordner anlegen"))
		}
	}
	listBox := a.z.mark(lb.String(), zoneAction{block: true, wheel: func(up bool) tea.Cmd {
		if up {
			m.sel = max(0, m.sel-1)
		} else {
			m.sel = min(len(rows)-1, m.sel+1)
		}
		return nil
	}})
	lb.Reset()
	lb.WriteString(listBox)
	left := lipgloss.NewStyle().Width(leftW).Height(h).Render(clipLines(lb.String(), leftW))
	sep := lipgloss.NewStyle().Foreground(s.T.Border).Render(strings.Repeat("│\n", max(0, h-1)) + "│")

	// rechte Spalte: Details
	var rb strings.Builder
	if m.sel < len(rows) && rows[m.sel].isFolder() {
		rb.WriteString(a.viewFolderDetails(rows[m.sel].node, rightW))
	} else if m.sel < len(rows) {
		c := rows[m.sel].conn
		head := s.Accent.Render(" " + trunc(c.Alias, max(8, rightW-4)))
		if g := c.Group; g != "" && lipgloss.Width(head)+2+len([]rune(g)) < rightW-1 {
			head += "  " + s.Muted.Render(g)
		}
		if acts := a.keyZone("⏎", s.Key.Render("⏎")+s.KeyDesc.Render("verbinden")) + " " + a.keyZone("e", s.Key.Render("e")+s.KeyDesc.Render("bearbeiten")); lipgloss.Width(head)+3+lipgloss.Width(acts) < rightW {
			head += "   " + acts
		}
		rb.WriteString(head + "\n\n")
		label := detailLabel(s, rightW)
		textW := max(10, rightW-2-label.GetWidth())
		row := func(k, v string) {
			rb.WriteString(" " + label.Render(k) + s.Text.Render(ansi.Truncate(v, textW, "…")) + "\n")
		}
		row("Ziel", fmt.Sprintf("%s@%s", c.Username, c.Addr()))
		auth := map[domain.AuthMethod]string{domain.AuthPassword: "Passwort (verschlüsselt)", domain.AuthKeyFile: "Keyfile", domain.AuthAgent: "SSH-Agent"}[c.Auth]
		if c.Auth == domain.AuthKeyFile {
			if c.KeyPEM.IsSet() {
				auth += " (eingebettet, verschlüsselt)"
			} else {
				auth += " " + c.KeyPath
			}
		}
		row("Anmeldung", auth)
		if c.JumpHostID != "" {
			row("Jump-Host", a.jumpChainText(c))
		}
		if len(c.Tags) > 0 {
			row("Tags", strings.Join(c.Tags, ", "))
		}
		if c.Profile != nil && !c.Profile.Empty() {
			sum := c.Profile.Summary()
			if c.Profile.Manual {
				sum += "  (von Hand)"
			}
			row("System", trunc(sum, max(10, rightW-22)))
			if c.Profile.Notes != "" {
				row("Notizen", trunc(c.Profile.Notes, max(10, rightW-22)))
			}
			if n := len(c.Profile.Failed); n > 0 {
				row("Gelernt", plural(n, "abgelehnter Befehl", "abgelehnte Befehle")+" · schlägt die KI nicht mehr vor")
			}
		} else {
			row("System", s.Muted.Render("wird beim ersten Verbinden erkannt"))
		}
		if !c.LastUsed.IsZero() {
			row("Zuletzt", c.LastUsed.Format("02.01.2006 15:04"))
		}
		tuns := a.st.Tunnels(c.ID)
		rb.WriteString("\n " + s.PanelTitle.Foreground(s.T.Accent).Render("Tunnel") + "  " + a.keyZone("t", s.Muted.Render("t = neuer Tunnel")) + "\n")
		if len(tuns) == 0 {
			rb.WriteString(s.Muted.Render("  keine") + "\n")
		}
		for _, t := range tuns {
			icon := "⇄"
			if t.IsWebService() {
				icon = "🌐"
			}
			auto := ""
			if t.AutoStart {
				auto = s.OK.Render(" auto")
			}
			rb.WriteString(fmt.Sprintf("  %s %-20s %s%s\n", icon, trunc(t.Name, 20), s.Muted.Render(t.Spec()), auto))
		}
		cfg := a.st.LLM()
		rb.WriteString("\n " + s.PanelTitle.Render("KI-Copilot") + "\n")
		rb.WriteString("  " + s.Muted.Render(trunc(cfg.Model+" @ "+cfg.BaseURL, max(10, rightW-3))) + "\n")
	}
	if len(rows) > 0 || a.st.SyncConfig().Enabled() {
		rb.WriteString("\n " + s.PanelTitle.Render("Sync") + "\n  " + a.syncStatus() + "\n")
	}
	right := lipgloss.NewStyle().Width(rightW).Height(h).Render(clipLines(rb.String(), rightW))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, sep, right)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// ---------------------------------------------------------------- Formulare

func (a *App) openConnForm(c domain.Connection) tea.Cmd {
	f := &form{title: "Neue Verbindung"}
	if c.ID != "" {
		f.title = "Verbindung bearbeiten: " + c.Alias
	}
	f.text("alias", "Alias", c.Alias, "Anzeigename, z. B. „DGX Spark“")
	f.text("host", "Host", c.Host, "Name oder IP")
	f.number("port", "Port", c.Port, "Standard 22")
	f.text("user", "Benutzer", c.Username, "")
	f.choice("auth", "Anmeldung", []string{"agent", "keyfile", "password"}, string(c.Auth), "agent = ssh-agent / Windows OpenSSH-Agent (empfohlen)")
	isAuth := func(v string) func(*form) bool { return func(f *form) bool { return f.sel("auth") != v } }
	f.secret("password", "Passwort", c.Password.Reveal(), "wird mit AES-256-GCM verschlüsselt gespeichert").hidden = isAuth("password")
	f.text("keypath", "Keyfile", c.KeyPath, "z. B. ~/.ssh/id_ed25519 (leer lassen bei eingebettetem Key)").hidden = isAuth("keyfile")
	f.secret("keypass", "Passphrase", c.KeyPassphrase.Reveal(), "optional; leer = beim Verbinden fragen").hidden = isAuth("keyfile")
	f.pick("group", "Ordner", domain.NormPath(c.Group), func(v string) string {
		if v == "" {
			return a.s.Muted.Render("— oberste Ebene")
		}
		return "▸ " + v
	}, func() tea.Cmd { return a.pickFolderField() }, "Auswahl mit Suche; neuen Namen tippen legt einen Ordner an („A / B“ verschachtelt)")
	f.text("tags", "Tags", strings.Join(c.Tags, ", "), "durch Komma getrennt, z. B. prod, linux · im Manager filtern mit #")
	self := c
	f.pick("jump", "Jump-Host", c.JumpHostID, func(v string) string { return a.jumpFieldText(self, v) },
		func() tea.Cmd { return a.pickJumpField(self) }, "wie ssh -J · ⏎ öffnet die Liste aller Verbindungen mit Suche")
	f.choice("device", "Gerätetyp", deviceLabels, deviceLabel(c.Device), "Netzwerkgerät = Switch/Router (Aruba, Cisco …): keine Systemabfrage")
	var pr domain.SystemProfile
	if c.Profile != nil {
		pr = *c.Profile
	}
	f.text("p_os", "System", pr.OS, "erkannt beim Verbinden; eigene Angabe hat Vorrang, z. B. „ArubaOS-CX 10.13“")
	f.choice("p_dialect", "Befehlssatz", dialectLabels(), dialectLabel(pr.Dialect), "welche Befehle die KI vorschlägt (automatisch = erkennen)")
	f.text("p_notes", "KI-Notizen", pr.Notes, "eigene Hinweise, z. B. „Core-Switch, VLAN 99 = Management, nie reload“")
	if len(pr.Failed) > 0 {
		f.toggle("p_forget", "Gelerntes löschen", false, plural(len(pr.Failed), "abgelehnter Befehl", "abgelehnte Befehle")+" gemerkt")
	}
	a.form, a.formID, a.scr = f, c.ID, scrConnForm
	a.formBase = c
	return f.setFocus(0)
}

func (a *App) openTunnelForm(t domain.Tunnel) tea.Cmd {
	f := &form{title: "Tunnel / Port-Forwarding"}
	f.text("name", "Name", t.Name, "z. B. „Grafana“")
	f.choice("kind", "Art", []string{"local", "remote", "dynamic"}, string(t.Kind), "local = -L · remote = -R · dynamic = -D (SOCKS5)")
	f.text("bindhost", "Bind-Adresse", orDefault(t.BindHost, "127.0.0.1"), "127.0.0.1 = nur dieser Rechner (empfohlen)")
	f.number("bindport", "Port", t.BindPort, "local/dynamic: lokaler Port (0 = frei wählen) · remote: Port am Server")
	notDyn := func(f *form) bool { return f.sel("kind") == "dynamic" }
	f.text("thost", "Ziel-Host", t.TargetHost, "local: vom Server aus gesehen · remote: von hier aus gesehen").hidden = notDyn
	f.number("tport", "Ziel-Port", t.TargetPort, "").hidden = notDyn
	onlyLocal := func(f *form) bool { return f.sel("kind") != "local" }
	f.choice("web", "Web-Service", []string{"—", "http", "https"}, orDefault(t.WebScheme, "—"), "One-Click: zeigt die lokale URL und öffnet den Browser").hidden = onlyLocal
	f.text("path", "Pfad", t.WebPath, "z. B. /admin").hidden = func(f *form) bool { return f.sel("kind") != "local" || f.sel("web") == "—" }
	f.toggle("autoopen", "Browser öffnen", t.AutoOpen, "").hidden = func(f *form) bool { return f.sel("web") == "—" || f.sel("kind") != "local" }
	f.toggle("autostart", "Autostart", t.AutoStart, "mit der Verbindung starten")
	a.form, a.formID, a.scr = f, t.ID, scrTunnelForm
	a.formTunnel = t
	return f.setFocus(0)
}

func (a *App) openSettings() tea.Cmd {
	c := a.st.LLM()
	f := &form{title: "KI-Copilot (OpenAI-kompatibel: OpenAI, vLLM, SGLang, Ollama, LiteLLM, LocalAI)"}
	f.text("base", "Base-URL", c.BaseURL, "z. B. http://dgx-spark:8000/v1 · Ollama: http://localhost:11434/v1")
	f.secret("key", "API-Key", c.APIKey.Reveal(), "verschlüsselt gespeichert; leer für lokale Server")
	f.text("model", "Modell", c.Model, "Strg+L: Liste vom Endpunkt · Strg+T: Verbindung testen")
	f.number("tokens", "Max-Tokens", c.MaxTokens, "")
	f.text("temp", "Temperatur", fmt.Sprintf("%.2f", c.Temperature), "0.0–1.0; für Befehle niedrig (0.1–0.3)")
	f.number("timeout", "Timeout (s)", c.TimeoutSec, "")
	f.toggle("suggest", "Ghost-Text", c.SuggestEnabled, "Vorschläge während des Tippens")
	f.number("delay", "Verzögerung ms", c.SuggestDelayMS, "Pause nach dem letzten Tastendruck")
	f.toggle("screen", "Bildschirm senden", c.SendScreen, "letzte Zeilen als Kontext (Geheimnisse werden entfernt)")
	f.number("lines", "Kontext-Zeilen", c.ContextLines, "")
	f.number("steps", "Ziel: Schritte/min", c.GoalStepsMin, "Ratenlimit im Ziel-Modus")
	f.toggle("insecure", "http mit API-Key", c.AllowInsecure, "nur wenn du weißt, was du tust")
	a.form, a.scr = f, scrSettings
	a.llm = llmUI{}
	return f.setFocus(0)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (a *App) updateForm(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok && a.scr == scrSettings {
		if cmd, done := a.settingsKey(k); done {
			return cmd
		}
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && a.scr == scrQuickForm && a.quickFormKey(k) {
		return nil
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
		a.form = nil
		a.scr = scrManager
		if a.returnToTerminal {
			a.scr, a.returnToTerminal = scrTerminal, false
		}
		return nil
	}
	submit, cmd := a.form.update(msg)
	if !submit {
		return cmd
	}
	var err error
	switch a.scr {
	case scrConnForm:
		err = a.saveConn()
	case scrTunnelForm:
		err = a.saveTunnel()
	case scrSettings:
		err = a.saveSettings()
	case scrQuickForm:
		err = a.saveQuick()
	}
	if err != nil {
		a.form.err = err.Error()
		return nil
	}
	a.form = nil
	a.scr = scrManager
	if a.returnToTerminal {
		a.scr, a.returnToTerminal = scrTerminal, false
	}
	a.flash("Gespeichert (verschlüsselt)")
	return nil
}

func (a *App) saveConn() error {
	f := a.form
	c := a.formBase
	c.Alias, c.Host, c.Port, c.Username = f.str("alias"), f.str("host"), f.num("port"), f.str("user")
	if c.Port == 0 {
		c.Port = 22
	}
	c.Auth = domain.AuthMethod(f.sel("auth"))
	c.Password, c.KeyPath, c.KeyPassphrase = "", "", ""
	switch c.Auth {
	case domain.AuthPassword:
		c.Password = domain.Secret(f.raw("password"))
	case domain.AuthKeyFile:
		c.KeyPath, c.KeyPassphrase = f.str("keypath"), domain.Secret(f.raw("keypass"))
		c.KeyPEM = a.formBase.KeyPEM
	}
	c.Group = f.val("group")
	c.Tags = nil
	for _, t := range strings.Split(f.str("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			c.Tags = append(c.Tags, t)
		}
	}
	c.Device = deviceFromLabel(f.sel("device"))
	c.Profile = profileFromForm(f, a.formBase.Profile)
	c.JumpHostID = f.val("jump")
	if c.JumpHostID != "" {
		if _, ok := a.st.Connection(c.JumpHostID); !ok {
			return fmt.Errorf("Jump-Host gibt es nicht mehr")
		}
		if c.ID != "" && domain.JumpCreatesCycle(c.ID, c.JumpHostID, a.st.Connection) {
			return fmt.Errorf("Jump-Host würde einen Kreis bilden")
		}
	}
	saved, err := a.st.UpsertConnection(c)
	if err == nil {
		a.mgr.want = "c:" + saved.ID
		a.expandTo(saved.Group)
		// Offene Sitzungen arbeiten ab sofort mit dem bearbeiteten Profil
		// (sonst schriebe die nächste Erkennung die alten Notizen zurück).
		for _, s := range a.sessions {
			if s.conn.ID == saved.ID {
				s.prof = domain.SystemProfile{}
				if saved.Profile != nil {
					s.prof = *saved.Profile
				}
			}
		}
	}
	return err
}

func (a *App) saveTunnel() error {
	f := a.form
	t := a.formTunnel
	t.Name, t.Kind = f.str("name"), domain.TunnelKind(f.sel("kind"))
	t.BindHost, t.BindPort = f.str("bindhost"), f.num("bindport")
	t.TargetHost, t.TargetPort = f.str("thost"), f.num("tport")
	t.WebScheme, t.WebPath = "", ""
	if t.Kind == domain.TunnelLocal && f.sel("web") != "—" {
		t.WebScheme, t.WebPath = f.sel("web"), f.str("path")
	}
	t.AutoOpen, t.AutoStart = f.flag("autoopen"), f.flag("autostart")
	if t.Name == "" {
		t.Name = t.Spec()
	}
	if t.Kind != domain.TunnelDynamic && (t.TargetHost == "" || t.TargetPort == 0) {
		return fmt.Errorf("Ziel-Host und Ziel-Port fehlen")
	}
	if t.Kind == domain.TunnelRemote && t.BindPort == 0 {
		return fmt.Errorf("Remote-Port fehlt")
	}
	_, err := a.st.UpsertTunnel(t)
	return err
}

func (a *App) saveSettings() error {
	c := a.formLLM()
	if err := checkLLM(c); err != nil {
		return err
	}
	return a.st.SetLLM(c)
}

func (a *App) viewForm() string {
	if a.form == nil {
		return ""
	}
	w := min(100, a.w-4)
	body := a.form.view(a.s, w-4, &a.z)
	if a.scr == scrSettings {
		body += a.viewLLMExtra(w - 4)
	}
	return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, a.s.Panel.Width(w).Render(body))
}

var _ = store.Dir

func newConnDefaults() domain.Connection { return domain.Connection{Port: 22, Auth: domain.AuthAgent} }

var deviceLabels = []string{"automatisch", "Server", "Netzwerkgerät"}

func deviceLabel(d domain.DeviceKind) string {
	switch d {
	case domain.DeviceServer:
		return deviceLabels[1]
	case domain.DeviceNetwork:
		return deviceLabels[2]
	}
	return deviceLabels[0]
}

func deviceFromLabel(l string) domain.DeviceKind {
	switch l {
	case deviceLabels[1]:
		return domain.DeviceServer
	case deviceLabels[2]:
		return domain.DeviceNetwork
	}
	return domain.DeviceAuto
}

// ---------------------------------------------------------------- Systemprofil

const dialectAuto = "automatisch"

func dialectLabels() []string {
	l := []string{dialectAuto}
	for _, d := range domain.Dialects {
		l = append(l, d.Name)
	}
	return l
}

func dialectLabel(id string) string {
	if d, ok := domain.DialectByID(id); ok {
		return d.Name
	}
	return dialectAuto
}

// profileFromForm übernimmt System, Befehlssatz und Notizen aus dem
// Formular. Weicht System oder Befehlssatz von der Erkennung ab, gilt das
// Profil als „von Hand“ und wird nicht mehr automatisch überschrieben;
// „automatisch“ bei leerem System gibt es wieder frei.
func profileFromForm(f *form, base *domain.SystemProfile) *domain.SystemProfile {
	var p domain.SystemProfile
	if base != nil {
		p = *base
		p.Failed = append([]string(nil), base.Failed...)
	}
	osText, notes := f.str("p_os"), f.str("p_notes")
	dia := ""
	if d, ok := domain.DialectByName(f.sel("p_dialect")); ok {
		dia = d.ID
	}
	if osText != p.OS || dia != p.Dialect {
		p.OS = osText
		if dia != "" {
			p.Dialect = dia
			p.Family = domain.FamilyOf(dia)
		}
		p.Manual = dia != "" || osText != ""
	}
	if dia == "" && osText == "" {
		p.Manual, p.Dialect = false, ""
	}
	p.Notes = notes
	if fl := f.get("p_forget"); fl != nil && fl.on {
		p.Failed = nil
	}
	if p.Empty() && p.Banner == "" {
		return nil
	}
	return &p
}

// clipLines kürzt jede Zeile auf w Zellen, statt sie umbrechen zu lassen
// (ein Umbruch würde die Spalten im schmalen Fenster durcheinanderbringen).
func clipLines(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = ansi.Truncate(l, w, "…")
		}
	}
	return strings.Join(lines, "\n")
}
