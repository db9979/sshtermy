package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

// Ordner, Tags und Reihenfolge im Verbindungs-Manager
//
//   - Die Liste ist ein Baum: Ordner (▾ auf, ▸ zu) mit Unterordnern und
//     Verbindungen. ←/→ bzw. Leertaste klappen, -/+ klappen alle.
//   - o legt einen Ordner an (im aktuellen Ordner, „A / B“ verschachtelt),
//     r benennt um, d löst einen Ordner auf (Inhalt rückt eine Ebene hoch).
//   - Shift+↑/↓ schiebt den Eintrag unter seinen Geschwistern, v verschiebt
//     in einen anderen Ordner. Mit der Maus: ziehen und loslassen.
//   - / sucht in Name, Host, Benutzer, Ordner und Tags; #tag im Suchtext
//     oder # (Auswahl) filtert nach Tags.

type mgrRow struct {
	node  *domain.TreeNode // Ordner; nil = Verbindung
	conn  domain.Connection
	depth int
}

func (r mgrRow) isFolder() bool { return r.node != nil }

func (r mgrRow) key() string {
	if r.node != nil {
		return "f:" + r.node.Path
	}
	return "c:" + r.conn.ID
}

// path: Ordner der Zeile (Ordner selbst bzw. Ordner der Verbindung).
func (r mgrRow) path() string {
	if r.node != nil {
		return r.node.Path
	}
	return r.conn.Group
}

func (r mgrRow) name() string {
	if r.node != nil {
		return r.node.Name
	}
	return r.conn.Alias
}

type mgrDrag struct {
	from, over int
	moved      bool
	dbl        bool // Doppelklick: beim Loslassen nicht noch einmal klappen
}

// mgrFilter: Suchwörter und Tags aus Suchfeld und Tag-Filter.
func (a *App) mgrFilter() (words, tags []string) {
	for _, w := range strings.Fields(strings.ToLower(a.mgr.search.Value())) {
		if strings.HasPrefix(w, "#") && len(w) > 1 {
			tags = append(tags, w[1:])
		} else if w != "#" {
			words = append(words, w)
		}
	}
	if a.mgr.tag != "" {
		tags = append(tags, strings.ToLower(a.mgr.tag))
	}
	return
}

func connMatches(c domain.Connection, words, tags []string) bool {
	hay := strings.ToLower(c.Alias + " " + c.Host + " " + c.Group + " " + c.Username + " " + strings.Join(c.Tags, " "))
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	for _, t := range tags {
		found := false
		for _, ct := range c.Tags {
			if strings.EqualFold(ct, t) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// buildRows macht aus dem Baum die sichtbaren Zeilen. Beim Filtern sind alle
// Ordner mit Treffern aufgeklappt, leere fallen weg.
func buildRows(tree *domain.TreeNode, words, tags []string) []mgrRow {
	filtering := len(words) > 0 || len(tags) > 0
	var walk func(n *domain.TreeNode, depth int) []mgrRow
	walk = func(n *domain.TreeNode, depth int) []mgrRow {
		var out []mgrRow
		for _, f := range n.Folders {
			inner := walk(f, depth+1)
			if filtering {
				if len(inner) == 0 && (len(tags) > 0 || !allIn(strings.ToLower(f.Path), words)) {
					continue
				}
				out = append(out, mgrRow{node: f, depth: depth})
				out = append(out, inner...)
				continue
			}
			out = append(out, mgrRow{node: f, depth: depth})
			if !f.Collapsed {
				out = append(out, inner...)
			}
		}
		for _, c := range n.Conns {
			if !filtering || connMatches(c, words, tags) {
				out = append(out, mgrRow{conn: c, depth: depth})
			}
		}
		return out
	}
	return walk(tree, 0)
}

func allIn(hay string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

func (a *App) mgrFiltering() bool {
	w, t := a.mgrFilter()
	return len(w) > 0 || len(t) > 0
}

// mgrRows: aktuelle Zeilen; hält die Auswahl auf dem gemerkten Eintrag.
func (a *App) mgrRows() []mgrRow {
	m := &a.mgr
	words, tags := a.mgrFilter()
	tree := a.st.Tree()
	m.tree = tree
	rows := buildRows(tree, words, tags)
	if m.want != "" {
		for i, r := range rows {
			if r.key() == m.want {
				m.sel = i
			}
		}
		m.want = ""
	}
	m.sel = clamp(m.sel, 0, max(0, len(rows)-1))
	m.rows = rows
	return rows
}

func (a *App) mgrCur() (mgrRow, bool) {
	rows := a.mgrRows()
	if a.mgr.sel < len(rows) {
		return rows[a.mgr.sel], true
	}
	return mgrRow{}, false
}

// selectedConn: ausgewählte Verbindung (nicht bei Ordnern).
func (a *App) selectedConn() (domain.Connection, bool) {
	r, ok := a.mgrCur()
	if !ok || r.isFolder() {
		return domain.Connection{}, false
	}
	return r.conn, true
}

// mgrContext: Ordner, in dem neue Einträge landen.
func (a *App) mgrContext() string {
	if r, ok := a.mgrCur(); ok {
		return r.path()
	}
	return ""
}

// ---------------------------------------------------------------- Aktionen

func (a *App) toggleFolder(path string, collapsed bool) {
	if err := a.st.SetCollapsed(collapsed, path); err != nil {
		a.flash(err.Error())
	}
}

func (a *App) newFolderDialog() tea.Cmd {
	parent := a.mgrContext()
	body := "Name des neuen Ordners. Mit „ / “ verschachteln, z. B. „Kunde A / Switches“."
	if parent != "" {
		body += "\nLiegt in: " + parent
	}
	a.modal = inputModal("Neuer Ordner", body, "", func(name string) tea.Cmd {
		if name == "" {
			return nil
		}
		p, err := a.st.CreateFolder(domain.PathJoin(parent, name))
		if err != nil {
			a.flash(err.Error())
			return nil
		}
		a.expandTo(p)
		a.mgr.want = "f:" + p
		a.flash("Ordner „" + p + "“ angelegt")
		return nil
	})
	return a.modal.in.Focus()
}

// expandTo klappt alle Ordner bis einschließlich path auf.
func (a *App) expandTo(path string) {
	var ps []string
	for p := path; p != ""; p = domain.PathParent(p) {
		ps = append(ps, p)
	}
	if len(ps) > 0 {
		_ = a.st.SetCollapsed(false, ps...)
	}
}

func (a *App) renameFolderDialog(path string) tea.Cmd {
	a.modal = inputModal("Ordner umbenennen", "Neuer Name. Der ganze Pfad lässt sich ändern, so wandert der Ordner samt Inhalt woanders hin.", path, func(neu string) tea.Cmd {
		neu = domain.NormPath(neu)
		if neu == "" || neu == path {
			return nil
		}
		if err := a.st.MoveFolder(path, neu); err != nil {
			a.flash(err.Error())
			return nil
		}
		a.mgr.want = "f:" + neu
		a.flash("Ordner heißt jetzt „" + neu + "“")
		return nil
	})
	return a.modal.in.Focus()
}

func (a *App) deleteFolderDialog(n *domain.TreeNode) {
	path := n.Path
	parent := domain.PathParent(path)
	where := "die oberste Ebene"
	if parent != "" {
		where = "„" + parent + "“"
	}
	body := "Ordner „" + path + "“ auflösen?"
	if n.Total > 0 || len(n.Folders) > 0 {
		body += fmt.Sprintf("\nDer Inhalt (%s) rückt in %s. Es wird keine Verbindung gelöscht.", plural(n.Total, "Verbindung", "Verbindungen"), where)
	}
	a.modal = confirmModal("Ordner auflösen", body, func() tea.Cmd {
		if err := a.st.DeleteFolder(path); err != nil {
			a.flash(err.Error())
		}
		return nil
	})
}

// folderItems: Ordner für die Auswahlliste; skip sperrt einen Teilbaum.
func folderItems(tree *domain.TreeNode, skip string) []pickItem {
	items := []pickItem{{value: "", label: "— oberste Ebene", hint: plural(len(tree.Conns), "Verbindung", "Verbindungen")}}
	var walk func(n *domain.TreeNode, depth int)
	walk = func(n *domain.TreeNode, depth int) {
		for _, f := range n.Folders {
			it := pickItem{value: f.Path, label: "▸ " + f.Name, indent: depth, hint: plural(f.Total, "Verbindung", "Verbindungen"), search: f.Path}
			if depth > 0 {
				it.detail = domain.PathParent(f.Path)
			}
			if skip != "" && domain.InFolder(f.Path, skip) {
				it.disabled = "liegt im verschobenen Ordner"
			}
			items = append(items, it)
			walk(f, depth+1)
		}
	}
	walk(tree, 0)
	return items
}

// newFolderExtra: „+ Neuer Ordner …“ aus dem Suchtext.
func newFolderExtra(tree *domain.TreeNode) func(q string) *pickItem {
	return func(q string) *pickItem {
		p := domain.NormPath(q)
		if p == "" || tree.Find(p) != nil {
			return nil
		}
		return &pickItem{value: p, label: "+ Neuer Ordner „" + p + "“", hint: "⏎ anlegen"}
	}
}

func (a *App) moveToDialog(r mgrRow) tea.Cmd {
	tree := a.st.Tree()
	skip := ""
	if r.isFolder() {
		skip = r.node.Path
	}
	p := newPicker("„"+r.name()+"“ verschieben nach …", "Ordner suchen oder neuen Namen tippen", folderItems(tree, skip), domain.PathParent(r.path()), func(it pickItem) tea.Cmd {
		a.moveRowTo(r, it.value)
		return nil
	})
	if !r.isFolder() {
		p.current = r.conn.Group
	}
	p.extra = newFolderExtra(tree)
	return a.openPicker(p)
}

// moveRowTo: Verbindung bzw. Ordner in den Ordner target legen.
func (a *App) moveRowTo(r mgrRow, target string) {
	var err error
	if r.isFolder() {
		neu := domain.PathJoin(target, r.node.Name)
		err = a.st.MoveFolder(r.node.Path, neu)
		a.mgr.want = "f:" + neu
	} else {
		err = a.st.MoveConns(target, r.conn.ID)
		a.mgr.want = r.key()
	}
	if err != nil {
		a.flash(err.Error())
		return
	}
	a.expandTo(target) // Ziel aufklappen, damit man es sieht
	where := "oberste Ebene"
	if target != "" {
		where = "„" + target + "“"
	}
	a.flash("„" + r.name() + "“ liegt jetzt in " + where)
}

// moveStep: Eintrag unter seinen Geschwistern um d verschieben.
func (a *App) moveStep(r mgrRow, d int) {
	tree := a.st.Tree()
	a.mgr.want = r.key()
	if r.isFolder() {
		parent := tree.Find(domain.PathParent(r.node.Path))
		if parent == nil {
			return
		}
		paths := make([]string, len(parent.Folders))
		idx := -1
		for i, f := range parent.Folders {
			paths[i] = f.Path
			if f.Path == r.node.Path {
				idx = i
			}
		}
		j := idx + d
		if idx < 0 || j < 0 || j >= len(paths) {
			return
		}
		paths[idx], paths[j] = paths[j], paths[idx]
		if err := a.st.OrderFolders(paths); err != nil {
			a.flash(err.Error())
		}
		return
	}
	parent := tree.Find(r.conn.Group)
	if parent == nil {
		return
	}
	ids := make([]string, len(parent.Conns))
	idx := -1
	for i, c := range parent.Conns {
		ids[i] = c.ID
		if c.ID == r.conn.ID {
			idx = i
		}
	}
	j := idx + d
	if idx < 0 || j < 0 || j >= len(ids) {
		return
	}
	ids[idx], ids[j] = ids[j], ids[idx]
	if err := a.st.PlaceConns(r.conn.Group, ids); err != nil {
		a.flash(err.Error())
	}
}

func (a *App) tagDialog() tea.Cmd {
	count := map[string]int{}
	name := map[string]string{}
	for _, c := range a.st.Connections() {
		for _, t := range c.Tags {
			k := strings.ToLower(t)
			count[k]++
			if name[k] == "" {
				name[k] = t
			}
		}
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	items := []pickItem{{value: "", label: "— alle Verbindungen (kein Filter)"}}
	for _, k := range keys {
		items = append(items, pickItem{value: name[k], label: "#" + name[k], hint: plural(count[k], "Verbindung", "Verbindungen")})
	}
	if len(keys) == 0 {
		a.flash("Noch keine Tags. Tags vergibst du beim Bearbeiten (e), durch Komma getrennt.")
		return nil
	}
	p := newPicker("Nach Tag filtern", "Tag suchen", items, a.mgr.tag, func(it pickItem) tea.Cmd {
		a.mgr.tag = it.value
		a.mgr.sel, a.mgr.top = 0, 0
		return nil
	})
	return a.openPicker(p)
}

// ---------------------------------------------------------------- Ziehen

// dropPlan: was beim Loslassen von Zeile from auf Zeile to passiert.
func (a *App) dropPlan(rows []mgrRow, from, to int) (desc string, do func()) {
	if from < 0 || to < 0 || from >= len(rows) || to >= len(rows) || from == to {
		return "", nil
	}
	src, dst := rows[from], rows[to]
	tree := a.mgr.tree
	if tree == nil {
		tree = a.st.Tree()
	}
	if src.isFolder() {
		target := dst.path()
		if dst.isFolder() && domain.PathParent(dst.node.Path) == domain.PathParent(src.node.Path) {
			// Geschwister: Reihenfolge ändern
			parent := tree.Find(domain.PathParent(src.node.Path))
			if parent == nil {
				return "", nil
			}
			var paths []string
			for _, f := range parent.Folders {
				paths = append(paths, f.Path)
			}
			paths, after := placeRel(paths, src.node.Path, dst.node.Path)
			rel := "vor"
			if after {
				rel = "nach"
			}
			return rel + " „" + dst.node.Name + "“", func() {
				a.mgr.want = src.key()
				if err := a.st.OrderFolders(paths); err != nil {
					a.flash(err.Error())
				}
			}
		}
		if !dst.isFolder() {
			target = dst.conn.Group
		}
		if domain.InFolder(target, src.node.Path) {
			return "geht nicht: Ziel liegt im Ordner selbst", nil
		}
		if target == domain.PathParent(src.node.Path) {
			return "", nil
		}
		return "in „" + orDefault(target, "oberste Ebene") + "“", func() { a.moveRowTo(src, target) }
	}
	if dst.isFolder() {
		if dst.node.Path == src.conn.Group {
			return "", nil
		}
		return "in „" + dst.node.Path + "“", func() { a.moveRowTo(src, dst.node.Path) }
	}
	// Verbindung auf Verbindung: davor bzw. danach einsortieren
	group := dst.conn.Group
	var ids []string
	if n := tree.Find(group); n != nil {
		for _, c := range n.Conns {
			ids = append(ids, c.ID)
		}
	}
	after := false
	if src.conn.Group == group {
		ids, after = placeRel(ids, src.conn.ID, dst.conn.ID)
	} else {
		ids = insertBefore(ids, src.conn.ID, dst.conn.ID)
	}
	rel := "vor"
	if after {
		rel = "nach"
	}
	where := ""
	if group != src.conn.Group {
		where = " in „" + orDefault(group, "oberste Ebene") + "“"
	}
	return rel + " „" + dst.conn.Alias + "“" + where, func() {
		a.mgr.want = src.key()
		if err := a.st.PlaceConns(group, ids); err != nil {
			a.flash(err.Error())
		}
	}
}

// placeRel verschiebt x in list an die Stelle von target: nach unten
// gezogen landet es hinter target, nach oben davor.
func placeRel(list []string, x, target string) ([]string, bool) {
	from, to := indexOf(list, x), indexOf(list, target)
	if from < 0 || to < 0 {
		return list, false
	}
	out := append([]string(nil), list[:from]...)
	out = append(out, list[from+1:]...)
	after := from < to
	t := indexOf(out, target)
	if after {
		t++
	}
	out = append(out[:t], append([]string{x}, out[t:]...)...)
	return out, after
}

func insertBefore(list []string, x, target string) []string {
	out := []string{}
	for _, v := range list {
		if v == x {
			continue
		}
		if v == target {
			out = append(out, x)
		}
		out = append(out, v)
	}
	if indexOf(out, x) < 0 {
		out = append(out, x)
	}
	return out
}

func indexOf(list []string, x string) int {
	for i, v := range list {
		if v == x {
			return i
		}
	}
	return -1
}

// mgrDragMouse: Bewegen und Loslassen nach einem Klick auf eine Zeile.
func (a *App) mgrDragMouse(msg tea.MouseMsg) tea.Cmd {
	d := a.mgr.drag
	m := msg.Mouse()
	switch msg.(type) {
	case tea.MouseMotionMsg:
		if r := a.z.rowAt(m.X, m.Y); r >= 0 {
			if r != d.from {
				d.moved = true
			}
			d.over = r
			// am Rand weiterblättern
			if r == a.mgr.top && a.mgr.top > 0 {
				a.mgr.top--
			} else if r == a.mgr.top+a.mgr.listH-1 && r < len(a.mgr.rows)-1 {
				a.mgr.top++
			}
		}
		return nil
	case tea.MouseReleaseMsg:
		a.mgr.drag = nil
		r := a.z.rowAt(m.X, m.Y)
		rows := a.mgr.rows
		if !d.moved || r == d.from {
			if d.from < len(rows) && rows[d.from].isFolder() && !d.dbl {
				n := rows[d.from].node
				a.toggleFolder(n.Path, !n.Collapsed)
			}
			return nil
		}
		if _, do := a.dropPlan(rows, d.from, r); do != nil {
			do()
		}
	}
	return nil
}

// viewFolderDetails: rechte Spalte für einen Ordner (w = Spaltenbreite).
func (a *App) viewFolderDetails(n *domain.TreeNode, w int) string {
	s := a.s
	var b strings.Builder
	b.WriteString(s.Accent.Render(" ▾ "+trunc(n.Name, max(8, w-4))) + "\n\n")
	label := detailLabel(s, w)
	textW := max(10, w-1-label.GetWidth())
	row := func(k, v string) {
		b.WriteString(" " + label.Render(k) + s.Text.Render(ansi.Truncate(v, textW, "…")) + "\n")
	}
	row("Pfad", n.Path)
	row("Inhalt", plural(n.Total, "Verbindung", "Verbindungen"))
	if len(n.Folders) > 0 {
		row("Unterordner", fmt.Sprintf("%d", len(n.Folders)))
	}
	if c := a.connectedIn(n.Path); c > 0 {
		row("Verbunden", plural(c, "Sitzung", "Sitzungen"))
	}
	tags := map[string]int{}
	var walk func(t *domain.TreeNode)
	walk = func(t *domain.TreeNode) {
		for _, c := range t.Conns {
			for _, tg := range c.Tags {
				tags[tg]++
			}
		}
		for _, f := range t.Folders {
			walk(f)
		}
	}
	walk(n)
	if len(tags) > 0 {
		var ts []string
		for t := range tags {
			ts = append(ts, "#"+t)
		}
		sort.Strings(ts)
		row("Tags", strings.Join(ts, " "))
	}
	km := a.keys
	b.WriteString("\n " + s.PanelTitle.Render("Ordner") + "\n")
	keys := [][2]string{
		{"⏎", "auf-/zuklappen"}, {"n", "neue Verbindung hier"}, {km.label(actFolderNew), "Unterordner anlegen"},
		{km.label(actRename), "umbenennen"}, {km.label(actMoveTo), "verschieben"}, {km.label(actMoveUp) + " " + keyLabel("shift+down"), "Reihenfolge"},
		{"d", "auflösen, Inhalt bleibt"}, {km.label(actFoldAll) + " " + km.label(actUnfoldAll), "alle zu / auf"},
	}
	for i := range keys {
		keys[i][0] = strings.ReplaceAll(keys[i][0], "Shift+", "⇧")
	}
	kw := 0
	for _, k := range keys {
		kw = max(kw, lipgloss.Width(k[0]))
	}
	for _, k := range keys {
		line := " " + s.Accent.Render(fmt.Sprintf(" %-*s", kw, k[0])) + "  " + s.Text.Render(trunc(k[1], max(6, w-kw-5)))
		if parts := strings.Fields(k[0]); len(parts) == 1 {
			line = a.keyZone(k[0], line)
		}
		b.WriteString(line + "\n")
	}
	hint := "Mit der Maus: eine Zeile ziehen und auf einem Ordner oder zwischen zwei Verbindungen loslassen."
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(s.T.Muted).PaddingLeft(2).Width(max(20, w-1)).Render(hint) + "\n")
	return b.String()
}

// detailLabel: Beschriftungsspalte der Details, schmaler bei engem Fenster.
func detailLabel(s Styles, w int) lipgloss.Style {
	if w < 70 {
		return s.Label.Width(13)
	}
	return s.Label
}

// jumpChainText: „bastion → jump-lan → ziel“ (bzw. Hinweis bei Fehlern).
func (a *App) jumpChainText(c domain.Connection) string {
	get := func(id string) (domain.Connection, bool) {
		if id == c.ID {
			return c, true
		}
		return a.st.Connection(id)
	}
	chain, cycle := domain.JumpChain(c.ID, get)
	if len(chain) > 0 && chain[0].JumpHostID != "" && !cycle {
		return "Jump-Host fehlt (gelöscht?)"
	}
	names := make([]string, len(chain))
	for i, x := range chain {
		names[i] = x.Alias
	}
	out := strings.Join(names, " → ")
	if cycle {
		out = "⚠ Kreis: " + out + " → …"
	}
	return out
}
