package ui

import (
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/keychain"
	"github.com/db9979/sshtermy/internal/pwsource"
	"github.com/db9979/sshtermy/internal/sshx"
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
	memo      mgrMemo   // mgrtree.go: Zeilen, solange Daten und Filter gleich sind
	selAt     time.Time // Auswahl zuletzt gewechselt (Einblenden, Balken)
	selSeen   int       // Auswahl im letzten Bild
}

func newManager() managerState {
	in := newInput("", false)
	in.Placeholder = i18n.T("Suchen (Name, Host, Ordner, #tag)")
	return managerState{search: in}
}

// managerKeys: nur Tasten, die in der aktuellen Lage etwas tun (Auswahl
// Ordner/Verbindung, leere Liste, Suche, Filter, offene Sitzungen).
func (a *App) managerKeys() [][2]string {
	km := a.keys
	m := &a.mgr
	if m.searching {
		return [][2]string{{"⏎", i18n.T("fertig")}, {"↑↓", i18n.T("zur Liste")}, {"Esc", i18n.T("Suche löschen")}, {km.label(actHelp), i18n.T("Hilfe")}}
	}
	rows := a.mgrRows()
	var cur mgrRow
	have := m.sel < len(rows)
	if have {
		cur = rows[m.sel]
	}
	k := [][2]string{{km.label(actPalette), i18n.T("Befehle")}}
	if len(a.sessions) > 0 {
		k = append(k, [2]string{km.label(actManager), i18n.T("zum Terminal")})
	}
	switch {
	case have && cur.isFolder():
		open := i18n.T("zuklappen")
		if cur.node.Collapsed {
			open = i18n.T("aufklappen")
		}
		k = append(k, [2]string{"⏎", open}, [2]string{"n", i18n.T("neu hier")}, [2]string{"e", i18n.T("bearbeiten")}, [2]string{"d", i18n.T("löschen")},
			[2]string{km.label(actMoveTo), i18n.T("verschieben")}, [2]string{km.label(actFolderGrid), i18n.T("als Raster öffnen")})
	case have:
		k = append(k, [2]string{"⏎", i18n.T("verbinden")}, [2]string{"n", i18n.T("neu")}, [2]string{"e", i18n.T("bearbeiten")}, [2]string{"d", i18n.T("löschen")},
			[2]string{km.label(actMoveTo), i18n.T("verschieben")}, [2]string{"t", "Tunnel"}, [2]string{km.label(actQuickEdit), i18n.T("Schnellbefehle")},
			[2]string{km.label(actSSHKey), i18n.T("SSH-Key")})
	default:
		k = append(k, [2]string{"n", i18n.T("neue Verbindung")}, [2]string{km.label(actImport), "Import"}, [2]string{km.label(actScan), "Scan"})
	}
	if m.tag != "" || m.search.Value() != "" {
		k = append(k, [2]string{"Esc", i18n.T("Filter aus")})
	}
	if len(rows) > 1 {
		k = append(k, [2]string{"/", i18n.T("suchen")})
		if have && !a.mgrFiltering() {
			k = append(k, [2]string{km.label(actMoveUp) + "/" + keyLabel("down"), i18n.T("sortieren")})
		}
	}
	k = append(k, [2]string{km.label(actFolderNew), i18n.T("Ordner")})
	if a.hasTags() {
		k = append(k, [2]string{km.label(actTagFilter), "Tags"})
	}
	if set := a.st.Settings(); len(set.Workspaces) > 0 || set.LastLayout != nil {
		k = append(k, [2]string{km.label(actWorkspace), i18n.T("Arbeitsplätze")})
	}
	if have {
		k = append(k, [2]string{km.label(actBroadcast), "Broadcast"}, [2]string{km.label(actScan), "Scan"}, [2]string{km.label(actImport), "Import"})
	}
	k = append(k, [2]string{"s", i18n.T("KI")}, [2]string{"y", "Sync"}, [2]string{"l", i18n.T("Protokolle")},
		[2]string{km.label(actHelp), i18n.T("Hilfe")}, [2]string{km.label(actCheat), i18n.T("erklären")}, [2]string{km.label(actAbout), i18n.T("Über")}, [2]string{km.label(actQuit), i18n.T("beenden")})
	return k
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
	switch a.keys.which(k.String(), actUndo, actManager, actHelp, actAbout, actImport, actNextLook, actKeys, actBroadcast, actScan, actCheat, actQuickEdit, actSSHKey,
		actFolderNew, actRename, actMoveTo, actMoveUp, actMoveDown, actTagFilter, actFoldAll, actUnfoldAll, actWorkspace, actFolderGrid, actSchedule) {
	case actUndo:
		return a.doUndo()
	case actSchedule:
		return a.openSchedules()
	case actWorkspace:
		return a.openWorkspaces()
	case actFolderGrid:
		if have && cur.isFolder() {
			return a.openFolderGrid(cur.node.Path)
		}
		if have {
			return a.openFolderGrid(cur.conn.Group)
		}
		return nil
	case actCheat:
		return a.toggleCheat()
	case actQuickEdit:
		if have && !cur.isFolder() {
			return a.openQuickForm(cur.conn, "")
		}
		return nil
	case actSSHKey:
		if have && !cur.isFolder() {
			return a.openSSHKeyForm(cur.conn)
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
		return a.openThemes()
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
			return a.openFolderForm(cur.node.Path)
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
			a.flashErr(err.Error())
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
			return a.openFolderForm(cur.node.Path)
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
			tr, err := a.st.DeleteConnectionTrash(c.ID)
			if err != nil {
				a.flashErr(err.Error())
				return nil
			}
			a.auditAdd(audit.EvSettings, auditConn(c), "Verbindung gelöscht")
			m.sel = max(0, m.sel-1)
			a.offerUndo(i18n.T("„")+c.Alias+i18n.T("“ gelöscht"), tr, func() {
				if c.Keychain {
					sshx.ForgetKeychain(c.ID)
				}
			}, func() {
				a.auditAdd(audit.EvSettings, auditConn(c), "Verbindung wiederhergestellt")
				a.mgr.want = "c:" + c.ID
				a.expandTo(c.Group)
			})
		}
	case "t":
		if have && !cur.isFolder() && cur.conn.Protocol.Raw() {
			a.flash(i18n.T("Dateien und Tunnel gibt es nur bei SSH-Verbindungen"))
			return nil
		}
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
			desc = i18n.T("hier nicht ablegen")
		}
		name := ""
		if m.drag.from < len(rows) {
			name = rows[m.drag.from].name()
		}
		lb.WriteString(" " + s.Warn.Render("↕ "+i18n.T("„")+trunc(name, 16)+i18n.T("“")+" "+trunc(desc, leftW-24)) + "\n")
	case m.searching || m.search.Value() != "":
		m.search.SetWidth(leftW - 6)
		lb.WriteString(" " + s.Accent.Render("/") + " " + m.search.View() + "\n")
	default:
		title := s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T(" Verbindungen")) + s.Muted.Render(fmt.Sprintf("  %d", total))
		// Hinweise nur, solange sie in die Zeile passen
		for _, h := range [][2]string{{"/", i18n.T("/ suchen")}, {a.keys.label(actTagFilter), a.keys.label(actTagFilter) + " Tags"}} {
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
		info = a.z.click(s.Accent.Render(" #"+m.tag+" ×"), func(tea.Mouse, bool) tea.Cmd { m.tag = ""; return nil })
	}
	if a.mgrFiltering() {
		n := 0
		for _, r := range rows {
			if !r.isFolder() {
				n++
			}
		}
		info += s.Muted.Render(fmt.Sprintf(i18n.T("  %d Treffer · Esc zeigt alle"), n))
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
	now := time.Now()
	if m.sel != m.selSeen {
		m.selSeen, m.selAt = m.sel, now // Auswahl blendet ein, Balken füllen sich
	}
	selK := 1.0
	if motionLevel() > 0 {
		selK = easeOut(float64(now.Sub(m.selAt)) / float64(selFadeDur))
	}
	if len(rows) > listH {
		info += s.Muted.Render(fmt.Sprintf(i18n.T("  %d–%d von %d"), m.top+1, min(len(rows), m.top+listH), len(rows)))
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
			pdot, dotC := a.reachDot(c)
			picon, iconC := a.devIcon(c)
			stripe, _ := a.stripe(c, nil)
			aw := max(8, min(18-2*min(r.depth, 5), (leftW-8-len(indent))*11/20))
			hw := max(4, leftW-8-len(indent)-aw)
			alias := fmt.Sprintf("%-*s", aw, trunc(c.Alias, aw))
			target := trunc(shortTarget(c), hw)
			plain = fmt.Sprintf("%s%s %s %s %s", indent, pdot, picon, alias, target)
			styled = stripe + indent + glyph(pdot, dotC, nil) + " " + glyph(picon, iconC, nil) + " " + alias + " " + s.Muted.Render(target)
			if i == m.sel {
				// Auswahl: Hintergrund über die ganze Zeile, Punkt und
				// Symbol behalten ihre Farbe (rot = nicht erreichbar).
				bg := mixColor(s.T.Bg, s.T.Surface2, selK)
				sel := s.Selected.Background(bg)
				stripeSel, _ := a.stripe(c, bg)
				body := sel.Render(indent) + glyph(pdot, dotC, bg) + sel.Render(" ") + glyph(picon, iconC, bg) + sel.Render(" "+alias+" "+target)
				styled = stripeSel + body + sel.Render(strings.Repeat(" ", max(0, leftW-2-lipgloss.Width(body))))
			}
			plain = " " + plain
		}
		line := styled
		switch {
		case i == m.sel && r.isFolder():
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
			lb.WriteString(a.mgrNoMatch(leftW))
		default:
			lb.WriteString(a.mgrEmpty(leftW))
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
	left := frameBlock(clipLines(lb.String(), leftW), leftW, h) // stamp.go: ohne lipgloss-Umbruch
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
		if acts := a.keyZone("⏎", s.Key.Render("⏎")+s.KeyDesc.Render(i18n.T("verbinden"))) + " " + a.keyZone("e", s.Key.Render("e")+s.KeyDesc.Render(i18n.T("bearbeiten"))); lipgloss.Width(head)+3+lipgloss.Width(acts) < rightW {
			head += "   " + acts
		}
		rb.WriteString(head + "\n\n")
		label := detailLabel(s, rightW)
		textW := max(10, rightW-2-label.GetWidth())
		row := func(k, v string) {
			rb.WriteString(" " + label.Render(k) + s.Text.Render(ansi.Truncate(v, textW, "…")) + "\n")
		}
		row(i18n.T("Ziel"), c.Target())
		row(i18n.T("Status"), a.reachText(c))
		if sp := a.reachSpark(c.ID); sp != "" && !a.connected(c.ID) {
			row(i18n.T("Verlauf"), sp+s.Muted.Render(i18n.T("  je Minute")))
		}
		a.mgrLoad(&rb, c, rightW, now)
		if d := deviceClass(c); d != devUnknown {
			ic, col := a.devIcon(c)
			row(i18n.T("Gerät"), glyph(ic, col, nil)+" "+devName(d))
		}
		if acc := a.accentOf(c); acc != nil {
			lbl := envLabel(envKind(c))
			if c.Color != accentAuto || lbl == "" {
				lbl = i18n.T(accentLabel(connAccent(c)))
			}
			row(i18n.T("Farbe"), glyph("▌", acc, nil)+" "+lbl)
		}
		auth := map[domain.AuthMethod]string{domain.AuthPassword: i18n.T("Passwort (verschlüsselt)"), domain.AuthKeyFile: "Keyfile", domain.AuthAgent: i18n.T("SSH-Agent"), domain.AuthKerberos: "Kerberos"}[c.Auth]
		if c.Auth == domain.AuthKeyFile {
			if c.KeyPEM.IsSet() {
				auth += i18n.T(" (eingebettet, verschlüsselt)")
			} else {
				auth += " " + c.KeyPath
			}
		}
		if c.Keychain && c.Auth == domain.AuthPassword {
			auth = i18n.T("Passwort") + " · " + i18n.T(keychainName())
		} else if c.Keychain && c.Auth == domain.AuthKeyFile {
			auth += " · " + i18n.T("Passphrase") + " " + i18n.T(keychainName())
		}
		if c.Protocol.Raw() {
			auth = i18n.T("von Hand am Gerät")
			if c.Username != "" || c.Password.IsSet() {
				auth = i18n.T("automatisch, sobald das Gerät fragt")
			}
		}
		if c.Protocol == domain.ProtoWinRM {
			auth = winrmAuthText(c)
		}
		if c.Protocol == domain.ProtoLocal {
			if c.LocalDir != "" {
				row(i18n.T("Startordner"), c.LocalDir)
			}
		} else {
			row(i18n.T("Anmeldung"), auth)
		}
		if c.JumpHostID != "" {
			row(i18n.T("Jump-Host"), a.jumpChainText(c))
		}
		if len(c.Tags) > 0 {
			row("Tags", strings.Join(c.Tags, ", "))
		}
		if c.Profile != nil && !c.Profile.Empty() {
			sum := c.Profile.Summary()
			if c.Profile.Manual {
				sum += i18n.T("  (von Hand)")
			}
			row("System", trunc(sum, max(10, rightW-22)))
			if c.Profile.Notes != "" {
				row(i18n.T("Notizen"), trunc(c.Profile.Notes, max(10, rightW-22)))
			}
			if n := len(c.Profile.Failed); n > 0 {
				row(i18n.T("Gelernt"), plural(n, i18n.T("abgelehnter Befehl"), i18n.T("abgelehnte Befehle"))+i18n.T(" · schlägt die KI nicht mehr vor"))
			}
			if n := len(c.Profile.Learned); n > 0 {
				last := c.Profile.Learned[n-1]
				row(i18n.T("Korrigiert"), trunc(plural(n, i18n.T("Korrektur"), i18n.T("Korrekturen"))+" · "+last.AI+" → "+last.Fixed, max(10, rightW-22)))
			}
		} else {
			row("System", s.Muted.Render(i18n.T("wird beim ersten Verbinden erkannt")))
		}
		if !c.LastUsed.IsZero() {
			row(i18n.T("Zuletzt"), c.LastUsed.Format("02.01.2006 15:04"))
		}
		tuns := a.st.Tunnels(c.ID)
		rb.WriteString("\n " + s.PanelTitle.Foreground(s.T.Accent).Render("Tunnel") + "  " + a.keyZone("t", s.Muted.Render(i18n.T("t = neuer Tunnel"))) + s.Muted.Render(i18n.T(" · Klick = bearbeiten/löschen")) + "\n")
		if len(tuns) == 0 {
			rb.WriteString(s.Muted.Render(i18n.T("  noch keine · z. B. Weboberfläche oder Datenbank sicher auf diesen Rechner holen")) + "\n")
		}
		for _, t := range tuns {
			icon := "↔"
			if t.IsWebService() {
				icon = "@"
			}
			auto := ""
			if t.AutoStart {
				auto = s.OK.Render(i18n.T(" auto"))
			}
			t := t
			line := fmt.Sprintf("  %s %-20s %s%s", icon, trunc(t.Name, 20), s.Muted.Render(t.Spec()), auto)
			rb.WriteString(a.z.click(line, func(tea.Mouse, bool) tea.Cmd { return a.openTunnelForm(t) }) + "\n")
		}
		cfg := a.st.LLM()
		rb.WriteString("\n " + s.PanelTitle.Render(i18n.T("KI-Copilot")) + "\n")
		rb.WriteString("  " + s.Muted.Render(trunc(cfg.Model+" @ "+cfg.BaseURL, max(10, rightW-3))) + "\n")
	}
	if len(rows) == 0 && !a.mgrFiltering() {
		rb.WriteString(a.mgrLegend(rightW))
	}
	if len(rows) > 0 || a.st.SyncConfig().Enabled() {
		rb.WriteString("\n " + s.PanelTitle.Render("Sync") + "\n  " + a.syncStatus() + "\n")
	}
	right := frameBlock(clipLines(rb.String(), rightW), rightW, h)
	return beside(beside(left, leftW, sep), leftW+1, right) // perf.go
}

// mgrLoad: Auslastung einer verbundenen Verbindung (aus der Host-Übersicht
// F6), Balken füllen sich beim Auswählen.
func (a *App) mgrLoad(rb *strings.Builder, c domain.Connection, w int, now time.Time) {
	var srv *hostinfo.Server
	for _, x := range a.sessions {
		if x.conn.ID == c.ID && x.host.srv != nil {
			srv = x.host.srv
			break
		}
	}
	if srv == nil {
		return
	}
	st := a.s
	k := 1.0
	if motionLevel() > 0 {
		k = easeOut(float64(now.Sub(a.mgr.selAt)) / float64(barFillDur))
	}
	cpuL, ramL, diskL := a.monLimits().CPU, a.monLimits().RAM, a.monLimits().Disk
	label := detailLabel(st, w)
	barW := max(8, min(30, w-label.GetWidth()-10))
	line := func(name string, pct, limit int) {
		if pct < 0 {
			return
		}
		col := st.T.OK
		switch {
		case limit > 0 && pct >= limit:
			col = st.T.Danger
		case limit > 0 && pct >= limit*8/10:
			col = st.T.Warn
		}
		rb.WriteString(" " + label.Render(name) + smoothBar(float64(pct)/100*k, barW, col, st.T.Surface2) +
			lipgloss.NewStyle().Foreground(col).Bold(true).Render(fmt.Sprintf(" %3d %%", int(float64(pct)*k+0.5))) + "\n")
	}
	line("CPU", srv.CPUPct, cpuL)
	if srv.MemTotal > 0 {
		line("RAM", srv.MemPct(), ramL)
	}
	disks := srv.DiskList()
	if len(disks) == 1 {
		line(i18n.T("Platte"), disks[0].Pct(), diskL)
		return
	}
	shown, hidden := pickDisks(disks, diskL)
	if len(shown) > 4 {
		shown, hidden = shown[:4], hidden+len(shown)-4
	}
	nameW := max(4, label.GetWidth()-2)
	for _, d := range shown {
		line(truncLeft(d.Mount, nameW), d.Pct(), diskL)
	}
	if hidden > 0 {
		rb.WriteString(" " + label.Render("") + st.Muted.Render(fmt.Sprintf(i18n.T("+%d weitere Platten · F6"), hidden)) + "\n")
	}
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
	f := &form{title: i18n.T("Neue Verbindung")}
	if c.ID != "" {
		f.title = i18n.T("Verbindung bearbeiten: ") + c.Alias
	}
	f.section("s_conn", i18n.T("Verbindung"), func(t Theme) color.Color { return t.Accent })
	f.text("alias", "Alias", c.Alias, i18n.T("Anzeigename, z. B. „DGX Spark“"))
	f.choice("proto", i18n.T("Verbindungsart"), protoLabels, protoLabel(c.Protocol), i18n.T("Telnet = unverschlüsselt (ältere Switches) · Seriell = USB-Konsolenkabel · Lokal = Shell auf diesem Rechner · PowerShell (WinRM) = Windows-Server wie Enter-PSSession")).value = protoLabel(c.Protocol)
	isProto := func(p domain.Protocol) bool { return protoFromLabel(f.sel("proto")) == p }
	raw := func(*form) bool { return protoFromLabel(f.sel("proto")).Raw() }
	notSerial := func(*form) bool { return !isProto(domain.ProtoSerial) }
	notLocal := func(*form) bool { return !isProto(domain.ProtoLocal) }
	noHost := func(*form) bool { return protoFromLabel(f.sel("proto")).NoHost() }
	f.text("host", "Host", c.Host, i18n.T("Name oder IP")).hidden = noHost
	f.number("port", "Port", c.Port, i18n.T("Standard 22 · Telnet 23 · WinRM 5985 (HTTP) / 5986 (HTTPS)")).hidden = noHost
	notWinRM := func(*form) bool { return !isProto(domain.ProtoWinRM) }
	f.toggle("winrm_https", "HTTPS", c.WinRMHTTPS, i18n.T("Port 5986 mit TLS · aus = 5985, dann verschlüsselt NTLM jede Nachricht")).hidden = notWinRM
	f.toggle("winrm_insecure", i18n.T("Zertifikat nicht prüfen"), c.WinRMInsecure, i18n.T("nur für selbst signierte Zertifikate im eigenen Netz")).hidden = func(f *form) bool {
		return notWinRM(f) || !f.flag("winrm_https")
	}
	f.text("lshell", "Shell", c.LocalShell, i18n.T("leer = Standard dieses Rechners (jetzt: ")+localShellLabel("")+i18n.T(") · z. B. pwsh, cmd, wsl, zsh, bash -l")).hidden = notLocal
	f.text("ldir", i18n.T("Startordner"), c.LocalDir, i18n.T("leer = Benutzerordner · z. B. ~/projekte")).hidden = notLocal
	sports, scur := serialPortOptions(c.SerialPort)
	f.choice("sport", i18n.T("Schnittstelle"), sports, scur, serialPortHelp()).hidden = notSerial
	f.choice("baud", "Baud", baudOpts(c.SerialBaud()), strconv.Itoa(c.SerialBaud()), i18n.T("Aruba/HPE/Cisco: 9600 · viele neuere Geräte: 115200")).hidden = notSerial
	f.choice("frame", i18n.T("Datenformat"), frameOpts(c.SerialFrame()), c.SerialFrame(), i18n.T("Datenbits, Parität, Stoppbits · fast immer 8N1")).hidden = notSerial
	self := c
	f.pick("jump", i18n.T("Jump-Host"), c.JumpHostID, func(v string) string { return a.jumpFieldText(self, v) },
		func() tea.Cmd { return a.pickJumpField(self) }, i18n.T("wie ssh -J · ⏎ öffnet die Liste aller Verbindungen mit Suche")).hidden = raw
	a.addRelayField(f, c)

	f.section("s_auth", i18n.T("Anmeldung"), func(t Theme) color.Color { return t.Warn }).hidden = func(*form) bool { return isProto(domain.ProtoLocal) }
	uf := f.text("user", i18n.T("Benutzer"), c.Username, "")
	uf.help = i18n.T("Telnet/Konsole: optional, wird eingetippt, sobald das Gerät fragt · WinRM: DOMÄNE\\name oder name@domäne")
	uf.hidden = func(*form) bool { return isProto(domain.ProtoLocal) }
	f.choice("auth", i18n.T("Verfahren"), []string{"agent", "keyfile", "password", "kerberos"}, string(c.Auth), i18n.T("agent = ssh-agent / Windows OpenSSH-Agent (empfohlen) · kerberos = Ticket der Firmen-Domäne (Windows-Anmeldung oder kinit)")).hidden = raw
	f.choice("winrm_auth", i18n.T("Verfahren"), []string{"negotiate", "kerberos", "basic"}, orDefault(c.WinRMAuth, "negotiate"),
		i18n.T("negotiate = NTLM, auch Domänenkonten (Standard) · kerberos = Domäne ohne NTLM, Rechnername statt IP · basic = nur lokale Konten und nur über HTTPS")).hidden = func(f *form) bool { return !isProto(domain.ProtoWinRM) }
	winrm := func(*form) bool { return isProto(domain.ProtoWinRM) }
	isAuth := func(v string) func(*form) bool {
		return func(f *form) bool { return raw(f) || f.sel("auth") != v }
	}
	pw, pp := c.Password.Reveal(), c.KeyPassphrase.Reveal()
	a.formKC = [2]string{}
	if c.Keychain && c.ID != "" {
		a.formKC = [2]string{sshx.KeychainSecret(c.ID, sshx.SecretPassword), sshx.KeychainSecret(c.ID, sshx.SecretPassphrase)}
		pw, pp = a.formKC[0], a.formKC[1]
	}
	f.secret("password", i18n.T("Passwort"), pw, i18n.T("wird mit AES-256-GCM verschlüsselt gespeichert")).hidden = func(f *form) bool {
		return isProto(domain.ProtoLocal) || !raw(f) && f.sel("auth") != "password"
	}
	f.text("keypath", "Keyfile", c.KeyPath, i18n.T("z. B. ~/.ssh/id_ed25519 (leer lassen bei eingebettetem Key)")).hidden = isAuth("keyfile")
	f.secret("keypass", "Passphrase", pp, i18n.T("optional; leer = beim Verbinden fragen")).hidden = isAuth("keyfile")
	f.text("certpath", i18n.T("Zertifikat"), c.CertPath, i18n.T("optional, von einer CA signiert · leer = <Keyfile>-cert.pub, falls vorhanden")).hidden = isAuth("keyfile")
	if keychain.Available() {
		f.toggle("keychain", i18n.T("Im Schlüsselbund"), c.Keychain, keychainHelp()).hidden = func(f *form) bool {
			if winrm(f) {
				return false
			}
			return raw(f) || f.sel("auth") != "password" && f.sel("auth") != "keyfile"
		}
	}

	f.text("pwsrc", i18n.T("Passwort aus"), c.PassSource,
		i18n.T("Passwortmanager statt gespeichertem Passwort: op://Tresor/Eintrag/password · bw:Eintrag · keepassxc:Datei.kdbx:Eintrag · pass:Name · cmd:Befehl")).hidden = func(f *form) bool {
		return !winrm(f) && (raw(f) || f.sel("auth") != "password")
	}
	f.get("pwsrc").in.Placeholder = i18n.T("leer = Passwort oben bzw. fragen")

	f.section("s_org", i18n.T("Einordnung"), func(t Theme) color.Color { return t.OK })
	f.pick("group", i18n.T("Ordner"), domain.NormPath(c.Group), func(v string) string {
		if v == "" {
			return a.s.Muted.Render(i18n.T("— oberste Ebene"))
		}
		return "▸ " + v
	}, func() tea.Cmd { return a.pickFolderField() }, i18n.T("Auswahl mit Suche; neuen Namen tippen legt einen Ordner an („A / B“ verschachtelt)"))
	f.text("tags", "Tags", strings.Join(c.Tags, ", "), i18n.T("durch Komma getrennt, z. B. prod, linux · im Manager filtern mit #"))
	f.choice("color", i18n.T("Farbe"), accentLabels(), accentLabel(c.Color), i18n.T("Streifen in Tab, Statuszeile und Liste · automatisch: prod rot, test grün, labor blau")).swatch = func(t Theme, opt string) color.Color {
		if id := accentFromLabel(opt); id != accentAuto && id != accentNone {
			return accentColor(t, id)
		}
		return nil
	}

	f.toggle("guard", i18n.T("Schutz"), !c.GuardOff,
		i18n.T("Standard an: riskante Befehle fragen vor ⏎, der Agent fragt bei Änderungen · aus nur für Test- und Laborsysteme"))

	f.section("s_dev", i18n.T("Gerät & KI"), func(t Theme) color.Color { return t.AI })
	f.choice("device", i18n.T("Gerätetyp"), deviceLabels, deviceLabel(c.Device), i18n.T("Netzwerkgerät = Switch/Router (Aruba, Cisco …): keine Systemabfrage"))
	var pr domain.SystemProfile
	if c.Profile != nil {
		pr = *c.Profile
	}
	f.text("p_os", "System", pr.OS, i18n.T("erkannt beim Verbinden; eigene Angabe hat Vorrang, z. B. „ArubaOS-CX 10.13“"))
	f.choice("p_dialect", i18n.T("Befehlssatz"), dialectLabels(), dialectLabel(pr.Dialect), i18n.T("welche Befehle die KI vorschlägt (automatisch = erkennen)"))
	f.text("p_notes", i18n.T("KI-Notizen"), pr.Notes, i18n.T("eigene Hinweise, z. B. „Core-Switch, VLAN 99 = Management, nie reload“"))
	if len(pr.Failed) > 0 || len(pr.Learned) > 0 {
		var parts []string
		if len(pr.Failed) > 0 {
			parts = append(parts, plural(len(pr.Failed), i18n.T("abgelehnter Befehl"), i18n.T("abgelehnte Befehle")))
		}
		if len(pr.Learned) > 0 {
			parts = append(parts, plural(len(pr.Learned), i18n.T("Korrektur"), i18n.T("Korrekturen")))
		}
		f.toggle("p_forget", i18n.T("Gelerntes löschen"), false, strings.Join(parts, ", ")+i18n.T(" gemerkt"))
	}

	netDev := func(f *form) bool { return raw(f) || deviceFromLabel(f.sel("device")) == domain.DeviceNetwork }
	f.folded("s_adv", i18n.T("Erweitert"), false, connAdvSummary,
		i18n.T("Agent-/X11-Weiterleitung, feste tmux-/screen-Sitzung und 2FA-Code · ⏎ klappt auf"))
	f.toggle("fwdagent", i18n.T("Agent weiterreichen"), c.ForwardAgent,
		i18n.T("wie ssh -A: git, ssh, scp auf dem Server mit deinen lokalen Keys · nur bei vertrauten Servern")).hidden = netDev
	f.toggle("fwdx11", i18n.T("X11 weiterleiten"), c.ForwardX11,
		i18n.T("wie ssh -X: grafische Programme vom Server hier zeigen · braucht lokal einen X-Server (XQuartz, VcXsrv, WSLg)")).hidden = netDev
	addPersistFields(f, c, raw)
	f.text("otpsrc", i18n.T("2FA-Code aus"), c.OTPSource,
		i18n.T("für Server, die nach einem Einmalcode fragen · op://Tresor/Eintrag · bw:Eintrag · keepassxc:… · leer = nachfragen")).hidden = raw
	f.get("otpsrc").in.Placeholder = i18n.T("leer = beim Verbinden fragen")
	f.get("tags").in.Placeholder = i18n.T("keine")
	f.get("p_os").in.Placeholder = i18n.T("noch nicht erkannt · wird beim Verbinden ermittelt")
	f.get("p_notes").in.Placeholder = i18n.T("keine · z. B. „VLAN 99 = Management, nie reload“")
	f.get("port").in.Placeholder = "22"
	a.form, a.formID, a.scr = f, c.ID, scrConnForm
	a.formBase = c
	return f.setFocus(0)
}

// connAdvSummary: was im zugeklappten Abschnitt „Erweitert“ eingeschaltet
// ist, damit nichts versteckt wirkt.
func connAdvSummary(f *form) string {
	var on []string
	if fl := f.get("fwdagent"); fl.on && (fl.hidden == nil || !fl.hidden(f)) {
		on = append(on, i18n.T("Agent weiterreichen"))
	}
	if fl := f.get("fwdx11"); fl.on && (fl.hidden == nil || !fl.hidden(f)) {
		on = append(on, i18n.T("X11"))
	}
	if fl := f.get("otpsrc"); fl != nil && strings.TrimSpace(fl.in.Value()) != "" && (fl.hidden == nil || !fl.hidden(f)) {
		on = append(on, i18n.T("2FA aus ")+orDefault(pwsource.Label(strings.TrimSpace(fl.in.Value())), "?"))
	}
	if fl := f.get("persist"); fl.options != nil && (fl.hidden == nil || !fl.hidden(f)) && persistFromLabel(f.sel("persist")) != "" {
		on = append(on, i18n.T("Feste Sitzung: ")+i18n.T(f.sel("persist")))
	}
	if len(on) == 0 {
		return i18n.T("alles aus")
	}
	return strings.Join(on, " · ")
}

func (a *App) openTunnelForm(t domain.Tunnel) tea.Cmd {
	f := &form{title: i18n.T("Tunnel / Port-Forwarding")}
	f.text("name", "Name", t.Name, i18n.T("z. B. „Grafana“"))
	f.choice("kind", i18n.T("Art"), []string{"local", "remote", "dynamic"}, string(t.Kind), i18n.T("local = -L · remote = -R · dynamic = -D (SOCKS5)"))
	f.text("bindhost", i18n.T("Bind-Adresse"), orDefault(t.BindHost, "127.0.0.1"), i18n.T("127.0.0.1 = nur dieser Rechner (empfohlen)"))
	f.number("bindport", "Port", t.BindPort, i18n.T("local/dynamic: lokaler Port (0 = frei wählen) · remote: Port am Server"))
	notDyn := func(f *form) bool { return f.sel("kind") == "dynamic" }
	f.text("thost", i18n.T("Ziel-Host"), t.TargetHost, i18n.T("local: vom Server aus gesehen · remote: von hier aus gesehen")).hidden = notDyn
	f.number("tport", i18n.T("Ziel-Port"), t.TargetPort, "").hidden = notDyn
	onlyLocal := func(f *form) bool { return f.sel("kind") != "local" }
	f.choice("web", i18n.T("Web-Service"), []string{"—", "http", "https"}, orDefault(t.WebScheme, "—"), i18n.T("One-Click: zeigt die lokale URL und öffnet den Browser")).hidden = onlyLocal
	f.text("path", i18n.T("Pfad"), t.WebPath, i18n.T("z. B. /admin")).hidden = func(f *form) bool { return f.sel("kind") != "local" || f.sel("web") == "—" }
	f.toggle("autoopen", i18n.T("Browser öffnen"), t.AutoOpen, "").hidden = func(f *form) bool { return f.sel("web") == "—" || f.sel("kind") != "local" }
	f.toggle("autostart", "Autostart", t.AutoStart, i18n.T("mit der Verbindung starten"))
	a.form, a.formID, a.scr = f, t.ID, scrTunnelForm
	a.formTunnel = t
	return f.setFocus(0)
}

func (a *App) openSettings() tea.Cmd {
	c := a.st.LLM()
	f := &form{title: i18n.T("KI-Copilot: Spark/vLLM, Ollama, OpenAI, Claude oder jeder OpenAI-kompatible Server")}
	f.choice("prov", i18n.T("Anbieter"), providerLabels(false), c.Provider.Label(), i18n.T("←/→ wählt; Adresse und Modell werden vorbelegt"))
	f.text("base", i18n.T("Base-URL"), c.BaseURL, i18n.T("z. B. http://dgx-spark:8000/v1 · Ollama: http://localhost:11434/v1"))
	f.secret("key", i18n.T("API-Key"), c.APIKey.Reveal(), i18n.T("verschlüsselt gespeichert; leer für lokale Server"))
	f.text("model", i18n.T("Modell"), c.Model, i18n.T("Strg+L: Liste vom Endpunkt · Strg+T: Verbindung testen"))
	f.text("fast", i18n.T("Schnelles Modell"), c.FastModel, i18n.T("optional, selber Endpunkt: übernimmt Ghost-Text, Folgebefehle und Begleiter"))
	f.number("tokens", i18n.T("Max-Tokens"), c.MaxTokens, "")
	f.text("temp", i18n.T("Temperatur"), fmt.Sprintf("%.2f", c.Temperature), i18n.T("0.0–1.0; für Befehle niedrig (0.1–0.3)"))
	f.number("timeout", i18n.T("Timeout (s)"), c.TimeoutSec, "")
	f.toggle("suggest", i18n.T("Ghost-Text"), c.SuggestEnabled, i18n.T("Vorschläge während des Tippens"))
	f.number("delay", i18n.T("Verzögerung ms"), c.SuggestDelayMS, i18n.T("Pause nach dem letzten Tastendruck"))
	f.toggle("screen", i18n.T("Bildschirm senden"), c.SendScreen, i18n.T("letzte Zeilen als Kontext (Geheimnisse werden entfernt)"))
	f.number("lines", i18n.T("Kontext-Zeilen"), c.ContextLines, "")
	f.number("steps", i18n.T("Ziel: Schritte/min"), c.GoalStepsMin, i18n.T("Ratenlimit im Ziel-Modus"))
	f.toggle("insecure", i18n.T("http mit API-Key"), c.AllowInsecure, i18n.T("nur wenn du weißt, was du tust"))
	f.text("rules", i18n.T("Hausregeln"), c.Rules, i18n.T("gelten für jede KI-Aufgabe, z. B. „Debian 12, nie reboot vorschlagen“; Ordner: e im Manager"))
	f.pick("fallbacks", i18n.T("Ersatz-Anbieter"), "", func(string) string { return a.fallbackSummary(a.st.LLM()) },
		func() tea.Cmd { return a.settingsToFallbacks() }, i18n.T("⏎ öffnet: springen ein, wenn der Endpunkt nicht antwortet"))
	a.form, a.scr = f, scrSettings
	a.llm = llmUI{}
	a.llm.rememberProviders(f)
	return f.setFocus(0)
}

// formChanged zieht abhängige Felder nach, wenn sich eine Auswahl geändert
// hat (Verbindungsart → Port, KI-Anbieter → Adresse/Modell). Läuft nach
// jeder Taste und nach jedem Mausklick, damit Klick und Pfeiltaste
// dasselbe bewirken.
func (a *App) formChanged() {
	if a.form == nil {
		return
	}
	switch a.scr {
	case scrSettings:
		a.applyProviderDefaults()
	case scrConnForm:
		syncProtoPort(a.form)
	}
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
	if k, ok := msg.(tea.KeyPressMsg); ok && a.scr == scrConnForm && k.String() == "ctrl+t" {
		return a.startConnTest() // conntest.go
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && a.scr == scrTunnelForm && k.String() == "ctrl+d" && a.formTunnel.ID != "" {
		a.confirmDeleteTunnel(a.formTunnel)
		return nil
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" && a.scr == scrCmdSetForm {
		a.form, a.scr = nil, scrCmdSets
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
	a.formChanged()
	if !submit {
		return cmd
	}
	var err error
	var next tea.Cmd
	wasKey := a.scr == scrKeyForm || a.scr == scrSSHKeyForm || a.scr == scrRelayForm || a.scr == scrBackupForm
	switch a.scr {
	case scrKeyForm:
		next, err = a.saveKeyFix()
	case scrSSHKeyForm:
		next, err = a.startSSHKey()
	case scrRelayForm:
		next, err = a.startRelayPair()
	case scrMasterForm:
		err = a.saveMasterForm()
	case scrBackupForm:
		next, err = a.saveBackupForm()
	case scrConnForm:
		err = a.saveConn()
	case scrTunnelForm:
		err = a.saveTunnel()
	case scrSettings:
		err = a.saveSettings()
	case scrQuickForm:
		err = a.saveQuick()
	case scrMonitorForm:
		err = a.saveMonitor()
	case scrFolderForm:
		err = a.saveFolderForm()
	case scrHighlightForm:
		err = a.saveHighlight()
	case scrScheduleForm:
		err = a.saveScheduleForm()
	case scrRulesForm:
		err = a.saveConfigRules()
	case scrCmdSetForm:
		err = a.saveCmdSetForm()
	}
	if err != nil {
		a.form.err = err.Error()
		return nil
	}
	if a.scr == scrCmdSetForm {
		a.form, a.scr = nil, scrCmdSets
		return next
	}
	a.form = nil
	a.scr = scrManager
	if a.returnToTerminal {
		a.scr, a.returnToTerminal = scrTerminal, false
	}
	if !wasKey {
		a.flash(i18n.T("Gespeichert (verschlüsselt)"))
	}
	return next
}

func (a *App) saveConn() error {
	c, err := a.connFromForm()
	if err != nil {
		return err
	}
	return a.storeConn(c)
}

// connFromForm: Verbindung aus dem Formular, ohne zu speichern (auch für
// „Testen“, conntest.go).
func (a *App) connFromForm() (domain.Connection, error) {
	f := a.form
	c := a.formBase
	c.Alias, c.Host, c.Port, c.Username = f.str("alias"), f.str("host"), f.num("port"), f.str("user")
	c.Protocol = protoFromLabel(f.sel("proto"))
	c.SerialPort, c.Baud, c.Frame = "", 0, ""
	c.LocalShell, c.LocalDir = "", ""
	if c.Protocol == domain.ProtoLocal {
		c.LocalShell, c.LocalDir = f.str("lshell"), f.str("ldir")
		c.Host, c.Port, c.Username = "", 0, ""
		if c.Alias == "" {
			c.Alias = i18n.T("Lokal")
		}
	}
	if c.Protocol == domain.ProtoSerial {
		if p := f.sel("sport"); p != serialAuto {
			c.SerialPort = p
		}
		c.Baud, _ = strconv.Atoi(f.sel("baud"))
		c.Frame = f.sel("frame")
		c.Host, c.Port = "", 0
		if c.Alias == "" {
			c.Alias = i18n.T("Konsole")
		}
	}
	if c.Protocol == domain.ProtoTelnet && c.Port == 22 && a.formBase.Protocol != domain.ProtoTelnet {
		c.Port = 23 // Vorbelegung für SSH stehen gelassen
	}
	c.WinRMHTTPS, c.WinRMInsecure, c.WinRMAuth = false, false, ""
	if c.Protocol == domain.ProtoWinRM {
		c.WinRMHTTPS = f.flag("winrm_https")
		c.WinRMInsecure = c.WinRMHTTPS && f.flag("winrm_insecure")
		if v := f.sel("winrm_auth"); v == "basic" || v == "kerberos" {
			c.WinRMAuth = v
		}
		switch {
		case c.Port == 22 || c.Port == 23:
			c.Port = 0 // Vorbelegung einer anderen Verbindungsart
		case c.WinRMHTTPS && c.Port == 5985:
			c.Port = 5986
		case !c.WinRMHTTPS && c.Port == 5986:
			c.Port = 5985
		}
	}
	if c.Port == 0 && !c.Protocol.NoHost() {
		c.Port = c.DefaultPort()
	}
	c.Auth = domain.AuthMethod(f.sel("auth"))
	c.Password, c.KeyPath, c.KeyPassphrase, c.CertPath = "", "", "", ""
	if c.Protocol.Raw() {
		c.Auth = domain.AuthPassword
	}
	switch c.Auth {
	case domain.AuthPassword:
		if c.Protocol != domain.ProtoLocal {
			c.Password = domain.Secret(f.raw("password"))
		}
	case domain.AuthKeyFile:
		c.KeyPath, c.KeyPassphrase = f.str("keypath"), domain.Secret(f.raw("keypass"))
		c.CertPath = f.str("certpath")
		c.KeyPEM = a.formBase.KeyPEM
	}
	c.PassSource, c.OTPSource = "", ""
	if c.Protocol == domain.ProtoWinRM {
		c.PassSource = f.str("pwsrc")
		if err := pwsource.Valid(c.PassSource); err != nil {
			return c, err
		}
	}
	if !c.Protocol.Raw() {
		if c.Auth == domain.AuthPassword {
			c.PassSource = f.str("pwsrc")
		}
		c.OTPSource = f.str("otpsrc")
		for _, k := range []string{c.PassSource, c.OTPSource} {
			if err := pwsource.Valid(k); err != nil {
				return c, err
			}
		}
	}
	c.Group = f.val("group")
	c.Tags = nil
	for _, t := range strings.Split(f.str("tags"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			c.Tags = append(c.Tags, t)
		}
	}
	c.Color = accentFromLabel(f.sel("color"))
	c.GuardOff = !f.flag("guard")
	c.Device = deviceFromLabel(f.sel("device"))
	applyPersistFields(f, &c)
	c.ForwardAgent = f.flag("fwdagent") && !c.Protocol.Raw() && c.Device != domain.DeviceNetwork
	c.ForwardX11 = f.flag("fwdx11") && !c.Protocol.Raw() && c.Device != domain.DeviceNetwork
	c.Profile = profileFromForm(f, a.formBase.Profile)
	c.JumpHostID = f.val("jump")
	if c.Protocol.Raw() {
		c.JumpHostID, c.KeyPEM = "", ""
	}
	a.applyRelayField(f, &c)
	if c.JumpHostID != "" {
		if _, ok := a.st.Connection(c.JumpHostID); !ok {
			return c, errors.New(i18n.T("Jump-Host gibt es nicht mehr"))
		}
		if c.ID != "" && domain.JumpCreatesCycle(c.ID, c.JumpHostID, a.st.Connection) {
			return c, errors.New(i18n.T("Jump-Host würde einen Kreis bilden"))
		}
	}
	return c, nil
}

func (a *App) storeConn(c domain.Connection) error {
	secret := a.connFormKeychain(&c)
	saved, err := a.st.UpsertConnection(c)
	if err == nil {
		err = a.saveConnKeychain(saved, secret)
	}
	if err == nil {
		what := "Verbindung geändert"
		if a.formBase.ID == "" {
			what = "Verbindung angelegt"
		}
		a.auditAdd(audit.EvSettings, auditConn(saved), what)
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

// confirmDeleteTunnel: Tunnel nach Rückfrage löschen (aus dem Formular,
// Strg+D). Läuft er gerade in einer Sitzung, wird er vorher gestoppt.
func (a *App) confirmDeleteTunnel(t domain.Tunnel) {
	a.modal = confirmModal(i18n.T("Tunnel löschen"), "„"+t.Name+i18n.T("“ löschen?"), func() tea.Cmd {
		for _, s := range a.sessions {
			if s.tunnels != nil {
				s.tunnels.Stop(t.ID)
			}
		}
		if err := a.st.DeleteTunnel(t.ID); err != nil {
			a.flashErr(i18n.T("Tunnel: ") + err.Error())
			return nil
		}
		a.form = nil
		a.scr = scrManager
		if a.returnToTerminal {
			a.scr, a.returnToTerminal = scrTerminal, false
		}
		a.flash(i18n.T("Tunnel gelöscht: ") + t.Name)
		return nil
	})
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
		return errors.New(i18n.T("Ziel-Host und Ziel-Port fehlen"))
	}
	if t.Kind == domain.TunnelRemote && t.BindPort == 0 {
		return errors.New(i18n.T("Remote-Port fehlt"))
	}
	_, err := a.st.UpsertTunnel(t)
	return err
}

func (a *App) saveSettings() error {
	if a.llm.fallback {
		return a.saveFallbacks()
	}
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
	for _, d := range domain.Dialects() {
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
		p.Learned = append([]domain.Correction(nil), base.Learned...)
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
		p.Failed, p.Learned = nil, nil
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
