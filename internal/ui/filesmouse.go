package ui

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Dateien (F5) mit der Maus
//
//   - Pfadleiste: Klick auf einen Teil („etc“ in / › etc › nginx) springt
//     dorthin. Rechts daneben Knöpfe: ↑ eine Ebene, ~ Home, ◌ neu laden,
//     + neuer Ordner, . versteckte Dateien.
//   - Spaltenköpfe Name/Größe/Geändert sortieren, ein zweiter Klick dreht um
//     (Taste s schaltet weiter).
//   - Strg+Klick markiert einzeln, Umschalt+Klick (oder Alt+Klick) einen
//     Bereich ab der Auswahl.
//   - Rechtsklick: Menü zum Eintrag oder (auf leerem Platz) zum Ordner.
//   - Maustaste „Zurück“: eine Ebene hoch bzw. Betrachter zu.

const (
	sortName = iota
	sortSize
	sortTime
)

// sortEntries ordnet nach der gewählten Spalte; „..“ bleibt oben, Ordner
// stehen vor Dateien.
func (p *pane) sortEntries() {
	if len(p.entries) < 2 {
		return
	}
	es := p.entries[1:]
	if p.entries[0].Name != ".." {
		es = p.entries
	}
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		var less, eq bool
		switch p.sortBy {
		case sortSize:
			less, eq = a.Size < b.Size, a.Size == b.Size
		case sortTime:
			less, eq = a.ModTime.Before(b.ModTime), a.ModTime.Equal(b.ModTime)
		default:
			la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name)
			less, eq = la < lb, la == lb
		}
		if eq {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		if p.desc {
			return !less
		}
		return less
	})
}

// sortClick: Spaltenkopf geklickt. Größe und Datum fangen absteigend an
// (größte/neueste zuerst), der Name aufsteigend.
func (p *pane) sortClick(col int, hidden bool) {
	sel := ""
	if p.sel >= 0 && p.sel < len(p.entries) {
		sel = p.entries[p.sel].Name
	}
	if p.sortBy == col {
		p.desc = !p.desc
	} else {
		p.sortBy, p.desc = col, col != sortName
	}
	p.sortEntries()
	p.selectName(sel)
}

func (p *pane) selectName(n string) {
	for i, e := range p.entries {
		if e.Name == n {
			p.sel = i
			return
		}
	}
}

func sortLabel(col int) string {
	switch col {
	case sortSize:
		return i18n.T("Größe")
	case sortTime:
		return i18n.T("Geändert")
	}
	return i18n.T("Name")
}

// goTo: in einen Ordner springen (Pfadleiste).
func (p *pane) goTo(dir string, hidden bool) {
	old := p.path
	p.path, p.sel, p.marked, p.peek = dir, 0, nil, nil
	p.load(hidden)
	if p.err != "" {
		e := p.err
		p.path = old
		p.load(hidden)
		p.err = e
	}
}

// crumbs: Teile des Pfads (Text, Ziel), vom Wurzelordner an.
func crumbs(fsys sshx.FS, p string) [][2]string {
	var out [][2]string
	for i := 0; i < 64; i++ {
		parent := fsys.Dir(p)
		name := baseName(fsys, p)
		if parent == p || name == "" || name == "." || name == p {
			out = append(out, [2]string{p, p})
			break
		}
		out = append(out, [2]string{name, p})
		p = parent
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// filesPaneTitle: Seitenname, klickbare Pfadleiste und Knöpfe (eine Zeile).
func (a *App) filesPaneTitle(s *session, f *filesState, side, w int) string {
	st := a.s
	p, active := f.panes[side], f.side == side
	title := " " + i18n.T(p.fs.Label()) + " "
	if n := len(p.marked); n > 0 {
		title += fmt.Sprintf(i18n.T("· %d markiert "), n)
	}
	ts := st.Tab
	if active {
		ts = st.TabActive.Foreground(st.T.Accent)
	}
	tr := a.z.click(ts.Render(title), func(tea.Mouse, bool) tea.Cmd { f.side = side; return a.filesPeek(s) })

	// Knöpfe rechts (nur wenn Platz ist)
	btn := lipgloss.NewStyle().Background(st.T.Surface2).Foreground(st.T.Accent)
	type button struct {
		label, tip string
		run        func() tea.Cmd
	}
	hiddenTip := i18n.T("versteckte Dateien zeigen (.)")
	if f.hidden {
		hiddenTip = i18n.T("versteckte Dateien ausblenden (.)")
	}
	buttons := []button{
		{"↑", i18n.T("eine Ebene hoch (←)"), func() tea.Cmd { p.cd("..", f.hidden); return nil }},
		{"~", i18n.T("Home-Ordner (~)"), func() tea.Cmd { p.goTo(p.fs.Home(), f.hidden); return nil }},
		{"◌", i18n.T("neu laden (r)"), func() tea.Cmd { p.load(f.hidden); return nil }},
		{"+", i18n.T("neuer Ordner (n)"), func() tea.Cmd { return a.filesMkdir(s) }},
		{".", hiddenTip, func() tea.Cmd { a.filesToggleHidden(s); return nil }},
	}
	var bar string
	if w >= 44 {
		for _, b := range buttons {
			b := b
			bar += " " + a.z.mark(btn.Render(" "+b.label+" "), zoneAction{
				click: func(tea.Mouse, bool) tea.Cmd { f.side = side; return b.run() },
				tip:   func() string { return b.tip },
			})
		}
	}
	room := w - lipgloss.Width(tr) - lipgloss.Width(bar) - 1

	// Pfadleiste: hinten anfangen, vorne mit … kürzen
	parts := crumbs(p.fs, p.path)
	sep := st.Muted.Render(" › ")
	var shown []string
	used := 0
	cut := false
	for i := len(parts) - 1; i >= 0; i-- {
		label := parts[i][0]
		wid := lipgloss.Width(label) + 3
		if used+wid > room && len(shown) > 0 {
			cut = true
			break
		}
		if wid > room {
			label = truncLeft(label, max(4, room-3))
		}
		target := parts[i][1]
		style := st.Accent
		if i == len(parts)-1 {
			style = st.Text
		}
		shown = append(shown, a.z.mark(style.Render(label), zoneAction{
			click: func(tea.Mouse, bool) tea.Cmd { f.side = side; p.goTo(target, f.hidden); return nil },
			tip:   func() string { return target },
		}))
		used += wid
	}
	for i, j := 0, len(shown)-1; i < j; i, j = i+1, j-1 {
		shown[i], shown[j] = shown[j], shown[i]
	}
	path := strings.Join(shown, sep)
	if cut {
		path = st.Muted.Render("…") + sep + path
	}
	left := tr + " " + path
	left = ansiTruncate(left, max(0, w-lipgloss.Width(bar)))
	return left + strings.Repeat(" ", max(0, w-lipgloss.Width(left)-lipgloss.Width(bar))) + bar
}

// paneHeader: Spaltenköpfe, ein Klick sortiert.
func (a *App) paneHeader(p *pane, f *filesState, side, nameW, sizeW, dateW int) string {
	st := a.s
	col := func(c, wid int, right bool) string {
		label := sortLabel(c)
		style := st.Muted
		if p.sortBy == c {
			style = st.Accent.Bold(true)
			if p.desc {
				label += " ↓"
			} else {
				label += " ↑"
			}
		}
		txt := fmt.Sprintf("%-*s", wid, trunc(label, wid))
		if right {
			txt = fmt.Sprintf("%*s", wid, trunc(label, wid))
		}
		return a.z.mark(style.Render(txt), zoneAction{
			click: func(tea.Mouse, bool) tea.Cmd { f.side = side; p.sortClick(c, f.hidden); return nil },
			tip:   func() string { return i18n.T("sortieren (s)") },
		})
	}
	return "  " + col(sortName, nameW, false) + " " + col(sortSize, sizeW, true) + " " + col(sortTime, dateW, true)
}

// entryClick: Klick auf Eintrag i (mit Strg/Umschalt/Doppelklick).
func (a *App) entryClick(s *session, side, i int, m tea.Mouse, double bool) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	anchor := p.sel
	f.side = side
	switch {
	case double:
		f.drag = nil
		return a.openEntry(s, side, i)
	case m.Mod&tea.ModCtrl != 0:
		p.toggleMark(i)
		p.sel = i
	case m.Mod&(tea.ModShift|tea.ModAlt) != 0 && f.side == side:
		lo, hi := min(anchor, i), max(anchor, i)
		if p.marked == nil {
			p.marked = map[string]bool{}
		}
		for k := max(1, lo); k <= hi && k < len(p.entries); k++ {
			p.marked[p.entries[k].Name] = true
		}
		p.sel = i
	default:
		p.sel = i
	}
	f.drag = &fileDrag{side: side, idx: i, over: side, overRow: i}
	return a.filesPeek(s)
}

// ---------------------------------------------------------------- Rechtsklick

// fileMenu: Menü zu Eintrag i der Seite side (i < 1: zum Ordner selbst).
func (a *App) fileMenu(s *session, side, i, x, y int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil {
		return nil
	}
	f.side, f.drag = side, nil
	other := f.panes[1-side]
	toOther := i18n.T("Nach rechts kopieren (↑ Server)")
	sendTo := i18n.T("Hochladen nach …")
	if side == 1 {
		toOther, sendTo = i18n.T("Nach links kopieren (↓ lokal)"), i18n.T("Herunterladen nach …")
	}
	remote := side == 1
	var items []menuItem
	title := p.path
	if i >= 1 && i < len(p.entries) {
		p.sel = i
		e := p.entries[i]
		full := p.fs.Join(p.path, e.Name)
		title = e.Name
		paths := p.pickedFor(i)
		many := ""
		if len(paths) > 1 {
			many = fmt.Sprintf(i18n.T("%d markierte"), len(paths))
		}
		if e.IsDir {
			items = append(items, menuItem{label: i18n.T("Öffnen"), hint: i18n.T("Doppelklick"), run: func() tea.Cmd { return a.openEntry(s, side, i) }})
		} else {
			items = append(items, menuItem{label: i18n.T("Ansehen"), hint: i18n.T("Doppelklick"), run: func() tea.Cmd { return a.openViewer(s, side, full) }})
			items = append(items, menuItem{label: i18n.T("Im Editor bearbeiten"), hint: "e", run: func() tea.Cmd { return a.editPath(s, side, full) }})
			items = append(items, menuItem{label: i18n.T("Mit Standardprogramm öffnen"), hint: "o", run: func() tea.Cmd { return a.openWithSystem(s, side, i) }})
		}
		hint := "c"
		if many != "" {
			hint = many
		}
		items = append(items,
			menuItem{label: toOther, hint: hint, run: func() tea.Cmd {
				return a.startTransfer(s, p.fs, paths, other.fs, other.path, side == 0, false)
			}},
			menuItem{label: sendTo, run: func() tea.Cmd { return a.askSendTo(s, side, paths) }},
			menuItem{label: i18n.T("Umbenennen"), hint: "F2", run: func() tea.Cmd { return a.askRename(s, side, i) }},
			menuItem{label: i18n.T("Rechte ändern"), hint: modeString(e.Mode), run: func() tea.Cmd { return a.askChmod(s, side, i) }},
			menuItem{label: i18n.T("Pfad kopieren"), hint: "y", run: func() tea.Cmd { return a.copyPath(full) }},
		)
		if remote && s.state == stConnected {
			dir := p.path
			if e.IsDir {
				dir = full
			}
			items = append(items, menuItem{label: i18n.T("Im Terminal: cd hierhin"), hint: trunc(dir, 24), run: func() tea.Cmd { return a.cdInTerminal(s, dir) }})
		}
		del := i18n.T("Löschen …")
		if many != "" {
			del = fmt.Sprintf(i18n.T("%s löschen …"), many)
		}
		items = append(items, menuItem{label: del, hint: "d", run: func() tea.Cmd { return a.askDelete(s, side, paths) }})
	} else {
		hidden := i18n.T("Versteckte zeigen")
		if f.hidden {
			hidden = i18n.T("Versteckte ausblenden")
		}
		items = append(items,
			menuItem{label: i18n.T("Neuer Ordner"), hint: "n", run: func() tea.Cmd { return a.filesMkdir(s) }},
			menuItem{label: i18n.T("Neu laden"), hint: "r", run: func() tea.Cmd { p.load(f.hidden); return nil }},
			menuItem{label: hidden, hint: ".", run: func() tea.Cmd { a.filesToggleHidden(s); return nil }},
			menuItem{label: i18n.T("Sortieren nach …"), hint: sortLabel((p.sortBy + 1) % 3), run: func() tea.Cmd { p.sortClick((p.sortBy+1)%3, f.hidden); return nil }},
			menuItem{label: i18n.T("Pfad kopieren"), hint: "y", run: func() tea.Cmd { return a.copyPath(p.path) }},
		)
		if remote && s.state == stConnected {
			dir := p.path
			items = append(items, menuItem{label: i18n.T("Im Terminal: cd hierhin"), hint: trunc(dir, 24), run: func() tea.Cmd { return a.cdInTerminal(s, dir) }})
		}
		if len(p.marked) > 0 {
			paths := p.picked()
			items = append(items, menuItem{label: fmt.Sprintf(i18n.T("%d markierte kopieren"), len(paths)), hint: "c",
				run: func() tea.Cmd { return a.startTransfer(s, p.fs, paths, other.fs, other.path, side == 0, false) }})
		}
	}
	a.modal = &modal{kind: mMenu, title: trunc(title, 40), items: items, at: [2]int{x, y}, anchored: true}
	return nil
}

func modeString(m fs.FileMode) string {
	return fmt.Sprintf("%03o %s", m.Perm(), m.Perm().String()[1:])
}

func (a *App) cdInTerminal(s *session, dir string) tea.Cmd {
	s.files.open, s.files.view = false, nil
	return a.sendCommand(s, "cd "+shQuote(dir))
}

// askSendTo: auf die andere Seite in einen frei wählbaren Ordner kopieren.
func (a *App) askSendTo(s *session, side int, paths []string) tea.Cmd {
	f := &s.files
	src, dst := f.panes[side], f.panes[1-side]
	title := i18n.T("Hochladen nach")
	if side == 1 {
		title = i18n.T("Herunterladen nach")
	}
	a.modal = inputModal(title, i18n.T("Ordner auf der anderen Seite (wird angelegt, falls es ihn nicht gibt):"), dst.path, func(dir string) tea.Cmd {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return nil
		}
		if dir == "~" || strings.HasPrefix(dir, "~/") {
			dir = dst.fs.Join(dst.fs.Home(), strings.TrimPrefix(strings.TrimPrefix(dir, "~"), "/"))
		}
		if err := dst.fs.Mkdir(dir); err != nil {
			if side == 0 && a.offerSudo(s, dst.fs, i18n.T("Hochladen nach ")+dir, err, func() tea.Cmd {
				return a.startSudoTransfer(s, src.fs, paths, dst.fs, dir, true)
			}) {
				return nil
			}
			a.flashErr(err.Error())
			return nil
		}
		return a.startTransfer(s, src.fs, paths, dst.fs, dir, side == 0, false)
	})
	return nil
}

// askRename: Eintrag i umbenennen.
func (a *App) askRename(s *session, side, i int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || i <= 0 || i >= len(p.entries) {
		return nil
	}
	old := p.entries[i].Name
	a.modal = inputModal(i18n.T("Umbenennen"), p.fs.Join(p.path, old), old, func(name string) tea.Cmd {
		name = strings.TrimSpace(name)
		if name == "" || name == old {
			return nil
		}
		if strings.ContainsAny(name, `/\`) {
			a.flashErr(i18n.T("Der Name darf kein / oder \\ enthalten"))
			return nil
		}
		from, to := p.fs.Join(p.path, old), p.fs.Join(p.path, name)
		done := func(err error) tea.Cmd {
			if err != nil {
				return nil
			}
			p.load(f.hidden)
			p.selectName(name)
			p.peek = nil
			a.flashOK(fmt.Sprintf(i18n.T("„%s“ heißt jetzt „%s“"), old, name))
			return nil
		}
		if err := p.fs.Rename(from, to); err != nil {
			if !a.offerSudo(s, p.fs, i18n.T("Umbenennen: ")+from+" → "+name, err, func() tea.Cmd {
				return a.runSudo(s, "mv "+from+" "+to, func(ctx context.Context, sx *sshx.Sudo) error { return sx.Rename(ctx, from, to) }, done)
			}) {
				a.flashErr(i18n.T("Umbenennen ging nicht: ") + err.Error())
			}
			return nil
		}
		return done(nil)
	})
	return nil
}

// askChmod: Rechte von Eintrag i ändern (oktal).
func (a *App) askChmod(s *session, side, i int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || i <= 0 || i >= len(p.entries) {
		return nil
	}
	e := p.entries[i]
	full := p.fs.Join(p.path, e.Name)
	cur := fmt.Sprintf("%03o", e.Mode.Perm())
	a.modal = inputModal(i18n.T("Rechte ändern"), full+"\n"+i18n.T("jetzt: ")+modeString(e.Mode)+"\n"+i18n.T("Oktal, z. B. 644 (Datei) oder 755 (Ordner/Skript)"), cur, func(v string) tea.Cmd {
		n, err := strconv.ParseUint(strings.TrimSpace(v), 8, 32)
		if err != nil || n > 0o7777 {
			a.flashErr(i18n.T("Bitte oktal angeben, z. B. 644"))
			return nil
		}
		mode := fs.FileMode(n & 0o777)
		if n&0o4000 != 0 {
			mode |= fs.ModeSetuid
		}
		if n&0o2000 != 0 {
			mode |= fs.ModeSetgid
		}
		if n&0o1000 != 0 {
			mode |= fs.ModeSticky
		}
		done := func(err error) tea.Cmd {
			if err != nil {
				return nil
			}
			p.load(f.hidden)
			p.selectName(e.Name)
			a.flashOK(fmt.Sprintf(i18n.T("Rechte von „%s“: %o"), e.Name, n))
			return nil
		}
		if err := p.fs.Chmod(full, mode); err != nil {
			if !a.offerSudo(s, p.fs, fmt.Sprintf(i18n.T("Rechte ändern: %s → %o"), full, n), err, func() tea.Cmd {
				return a.runSudo(s, fmt.Sprintf("chmod %o %s", n, full), func(ctx context.Context, sx *sshx.Sudo) error { return sx.Chmod(ctx, full, mode) }, done)
			}) {
				a.flashErr(i18n.T("Rechte ändern ging nicht: ") + err.Error())
			}
			return nil
		}
		return done(nil)
	})
	return nil
}

// askDelete: Einträge nach Rückfrage löschen.
func (a *App) askDelete(s *session, side int, paths []string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || len(paths) == 0 {
		return nil
	}
	var body string
	if len(paths) == 1 {
		what := i18n.T("Die Datei")
		if e, err := p.fs.Stat(paths[0]); err == nil && e.IsDir {
			what = i18n.T("Den Ordner mit allem darin")
		}
		body = fmt.Sprintf(i18n.T("%s: %s\n%s endgültig löschen?"), i18n.T(p.fs.Label()), paths[0], what)
	} else {
		names := make([]string, 0, 4)
		for _, x := range paths[:min(4, len(paths))] {
			names = append(names, baseName(p.fs, x))
		}
		more := ""
		if len(paths) > 4 {
			more = " …"
		}
		body = fmt.Sprintf(i18n.T("%s: %d Einträge in %s\n(%s%s)\nendgültig löschen, Ordner mit allem darin?"), i18n.T(p.fs.Label()), len(paths), p.path, strings.Join(names, ", "), more)
	}
	a.modal = confirmModal(i18n.T("Löschen?"), body, func() tea.Cmd {
		done := func(error) tea.Cmd {
			p.marked = nil
			p.load(f.hidden)
			return nil
		}
		for i, x := range paths {
			if err := p.fs.RemoveAll(x); err != nil {
				rest := paths[i:]
				if a.offerSudo(s, p.fs, i18n.T("Löschen:\n")+sudoListNames(rest), err, func() tea.Cmd {
					return a.runSudo(s, "rm -rf "+strings.Join(rest, " "), func(ctx context.Context, sx *sshx.Sudo) error { return sx.Remove(ctx, rest...) }, done)
				}) {
					return done(nil)
				}
				p.err = err.Error()
				break
			}
		}
		return done(nil)
	})
	return nil
}

// filesBack: Maustaste „Zurück“.
func (a *App) filesBack(s *session, x, y int) tea.Cmd {
	f := &s.files
	if f.view != nil {
		f.view = nil
		return nil
	}
	if f.panes[0] == nil {
		return nil
	}
	side, _ := a.z.fileAt(x, y)
	if side < 0 {
		side = f.side
	}
	f.side = side
	f.panes[side].cd("..", f.hidden)
	return nil
}
