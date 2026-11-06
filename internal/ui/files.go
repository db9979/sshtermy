package ui

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Dateien (F5): links lokal, rechts der Server, per SFTP über dieselbe
// SSH-Verbindung. c kopiert den markierten Eintrag auf die andere Seite
// (also hoch- oder runterladen), Ordner rekursiv. Leertaste markiert mehrere.
//
// Mit der Maus: einen Eintrag auf die andere Seite ziehen kopiert ihn (auf
// einen Ordner gezogen: in diesen Ordner). Dateien aus dem Explorer/Finder
// ins Fenster ziehen lädt sie in den Server-Ordner hoch (drop.go), auch
// direkt im Terminal.

type pane struct {
	fs      sshx.FS
	path    string
	entries []sshx.Entry
	sel     int
	err     string
	marked  map[string]bool // mit Leertaste markierte Namen
}

// fileDrag: Ziehen eines Eintrags mit der Maus.
type fileDrag struct {
	side, idx int
	over      int // Seite unter der Maus (-1 = keine)
	overRow   int // Eintrag unter der Maus (-1 = keiner)
	moved     bool
}

type xfer struct {
	what   string // „↑ datei → /pfad“
	pr     *sshx.Progress
	cancel context.CancelFunc
	start  time.Time
}

type filesState struct {
	open    bool
	opening bool
	err     string
	remote  *sshx.RemoteFS
	panes   [2]*pane // 0 = lokal, 1 = Server
	side    int
	hidden  bool
	mkdir   *textinput.Model
	xfer    *xfer
	last    string // Ergebnis der letzten Übertragung
	drag    *fileDrag
	pending []string // abgelegte Dateien, die nach dem Öffnen hochgeladen werden
}

type filesOpenMsg struct {
	sid int
	rfs *sshx.RemoteFS
	err error
}
type filesDoneMsg struct {
	sid int
	err error
}
type filesTickMsg struct{ sid int }

func filesTick(sid int) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return filesTickMsg{sid} })
}

func (a *App) toggleFiles(s *session) tea.Cmd {
	f := &s.files
	if f.open {
		f.open = false
		return nil
	}
	if s.conn.Protocol.Raw() {
		a.flash(i18n.T("Dateien und Tunnel gibt es nur bei SSH-Verbindungen"))
		return nil
	}
	f.open = true
	if f.remote != nil || f.opening || s.client == nil {
		return nil
	}
	f.opening, f.err = true, ""
	sid, c := s.id, s.client
	return func() tea.Msg {
		r, err := c.OpenSFTP()
		return filesOpenMsg{sid, r, err}
	}
}

func (p *pane) load(hidden bool) {
	es, err := p.fs.ReadDir(p.path)
	if err != nil {
		p.err = err.Error()
		p.entries = nil
		return
	}
	p.err = ""
	out := []sshx.Entry{{Name: "..", IsDir: true}}
	for _, e := range es {
		if !hidden && strings.HasPrefix(e.Name, ".") {
			continue
		}
		out = append(out, e)
	}
	p.entries = out
	p.sel = min(p.sel, len(out)-1)
	for n := range p.marked {
		if !slicesContainsName(out, n) {
			delete(p.marked, n)
		}
	}
}

func slicesContainsName(es []sshx.Entry, name string) bool {
	for _, e := range es {
		if e.Name == name {
			return true
		}
	}
	return false
}

// picked: markierte Einträge (in Listenreihenfolge), sonst der ausgewählte.
func (p *pane) picked() []string {
	var out []string
	for _, e := range p.entries {
		if p.marked[e.Name] {
			out = append(out, p.fs.Join(p.path, e.Name))
		}
	}
	if len(out) == 0 && p.sel > 0 && p.sel < len(p.entries) {
		out = append(out, p.fs.Join(p.path, p.entries[p.sel].Name))
	}
	return out
}

// pickedFor: beim Ziehen von Eintrag i: alle markierten, wenn i dazugehört,
// sonst nur i.
func (p *pane) pickedFor(i int) []string {
	if i <= 0 || i >= len(p.entries) {
		return nil
	}
	if p.marked[p.entries[i].Name] {
		return p.picked()
	}
	return []string{p.fs.Join(p.path, p.entries[i].Name)}
}

func (p *pane) toggleMark(i int) {
	if i <= 0 || i >= len(p.entries) {
		return
	}
	if p.marked == nil {
		p.marked = map[string]bool{}
	}
	n := p.entries[i].Name
	if p.marked[n] {
		delete(p.marked, n)
	} else {
		p.marked[n] = true
	}
}

func (p *pane) cd(dir string, hidden bool) {
	old := p.path
	if dir == ".." {
		p.path = p.fs.Dir(old)
	} else {
		p.path = p.fs.Join(old, dir)
	}
	p.sel = 0
	p.marked = nil
	p.load(hidden)
	if p.err != "" {
		p.path = old
		e := p.err
		p.load(hidden)
		p.err = e
		return
	}
	if dir == ".." { // den Ordner markieren, aus dem man kam
		for i, e := range p.entries {
			if p.fs.Join(p.path, e.Name) == old {
				p.sel = i
			}
		}
	}
}

func (a *App) filesMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case filesOpenMsg:
		s := a.byID(m.sid)
		if s == nil {
			if m.rfs != nil {
				m.rfs.Close()
			}
			return nil, true
		}
		f := &s.files
		f.opening = false
		if m.err != nil {
			f.err = m.err.Error()
			return nil, true
		}
		f.remote = m.rfs
		l := &pane{fs: sshx.LocalFS{}}
		l.path = l.fs.Home()
		r := &pane{fs: m.rfs, path: m.rfs.Home()}
		l.load(f.hidden)
		r.load(f.hidden)
		f.panes, f.side = [2]*pane{l, r}, 1
		if len(f.pending) > 0 {
			paths := f.pending
			f.pending = nil
			return a.startTransfer(s, sshx.LocalFS{}, paths, m.rfs, r.path, true, false), true
		}
		return nil, true
	case filesTickMsg:
		if s := a.byID(m.sid); s != nil && s.files.xfer != nil {
			return filesTick(m.sid), true
		}
		return nil, true
	case filesDoneMsg:
		s := a.byID(m.sid)
		if s == nil {
			return nil, true
		}
		f := &s.files
		x := f.xfer
		f.xfer = nil
		if x == nil {
			return nil, true
		}
		d := time.Since(x.start)
		switch {
		case m.err == context.Canceled:
			f.last = a.s.Warn.Render(i18n.T("Abgebrochen: ") + x.what)
		case m.err != nil:
			f.last = a.s.Danger.Render("× " + x.what + ": " + m.err.Error())
		default:
			f.last = a.s.OK.Render(fmt.Sprintf("✓ %s · %s in %s (%s/s)", x.what, humanBytes(x.pr.Done.Load()),
				shortDur(d), humanBytes(int64(float64(x.pr.Done.Load())/max(d.Seconds(), 0.001)))))
			for _, p := range f.panes {
				if p != nil {
					p.marked = nil
				}
			}
		}
		for _, p := range f.panes {
			if p != nil {
				sel := p.sel
				p.load(f.hidden)
				p.sel = min(sel, len(p.entries)-1)
			}
		}
		if !f.open {
			a.flash(ansiPlainText(f.last))
		}
		return nil, true
	}
	return nil, false
}

// startCopy kopiert die markierten Einträge (oder den ausgewählten) der
// aktiven Seite auf die andere Seite.
func (a *App) startCopy(s *session, overwrite bool) tea.Cmd {
	f := &s.files
	src, dst := f.panes[f.side], f.panes[1-f.side]
	return a.startTransfer(s, src.fs, src.picked(), dst.fs, dst.path, f.side == 0, overwrite)
}

// baseName: letzter Teil eines Pfads auf dem jeweiligen Dateisystem.
func baseName(fsys sshx.FS, p string) string {
	if _, ok := fsys.(sshx.LocalFS); ok {
		return filepath.Base(p)
	}
	return path.Base(p)
}

// startTransfer kopiert paths von srcFS nach dstDir auf dstFS (up = Richtung
// Server, nur für den Pfeil). Gibt es Ziele schon, fragt es erst.
func (a *App) startTransfer(s *session, srcFS sshx.FS, paths []string, dstFS sshx.FS, dstDir string, up, overwrite bool) tea.Cmd {
	f := &s.files
	if len(paths) == 0 {
		return nil
	}
	if f.xfer != nil {
		a.flash(i18n.T("Es läuft schon eine Übertragung (x bricht ab)"))
		return nil
	}
	if !overwrite {
		var exist []string
		for _, p := range paths {
			if _, err := dstFS.Stat(dstFS.Join(dstDir, baseName(srcFS, p))); err == nil {
				exist = append(exist, baseName(srcFS, p))
			}
		}
		if len(exist) > 0 {
			what := "„" + exist[0] + "“"
			if len(exist) > 1 {
				what = fmt.Sprintf(i18n.T("%d Einträge (%s …)"), len(exist), exist[0])
			}
			a.modal = confirmModal(i18n.T("Überschreiben?"), what+i18n.T(" gibt es in ")+dstDir+i18n.T(" schon."),
				func() tea.Cmd { return a.startTransfer(s, srcFS, paths, dstFS, dstDir, up, true) })
			return nil
		}
	}
	arrow := "↓"
	if up {
		arrow = "↑"
	}
	name := baseName(srcFS, paths[0])
	if len(paths) > 1 {
		name = fmt.Sprintf(i18n.T("%d Einträge"), len(paths))
	}
	ctx, cancel := context.WithCancel(context.Background())
	x := &xfer{what: arrow + " " + name + " → " + dstDir, pr: &sshx.Progress{}, cancel: cancel, start: time.Now()}
	f.xfer, f.last = x, ""
	sid := s.id
	run := func() tea.Msg {
		defer cancel()
		var total int64
		for _, p := range paths {
			n, _, err := sshx.Size(ctx, srcFS, p)
			if err != nil {
				return filesDoneMsg{sid, err}
			}
			total += n
		}
		x.pr.Total.Store(total)
		for _, p := range paths {
			if err := sshx.Copy(ctx, srcFS, p, dstFS, dstDir, x.pr); err != nil {
				return filesDoneMsg{sid, err}
			}
		}
		return filesDoneMsg{sid, nil}
	}
	return tea.Batch(run, filesTick(sid))
}

// uploadDropped lädt im Fenster abgelegte Dateien in den Server-Ordner des
// Dateibrowsers hoch und öffnet ihn dafür, wenn nötig.
func (a *App) uploadDropped(s *session, paths []string) tea.Cmd {
	f := &s.files
	if f.remote != nil && f.panes[1] != nil {
		f.open = true
		return a.startTransfer(s, sshx.LocalFS{}, paths, f.remote, f.panes[1].path, true, false)
	}
	f.pending = paths
	if f.open {
		return nil // SFTP öffnet gerade
	}
	return a.toggleFiles(s)
}

// filesPaste: eingefügter Text im Dateibrowser. Abgelegte Dateien werden
// hochgeladen, anderer Text geht ins Eingabefeld (neuer Ordner).
func (a *App) filesPaste(s *session, m tea.PasteMsg) tea.Cmd {
	f := &s.files
	if f.mkdir != nil {
		var cmd tea.Cmd
		*f.mkdir, cmd = f.mkdir.Update(m)
		return cmd
	}
	if paths := droppedPaths(m.Content); paths != nil {
		return a.uploadDropped(s, paths)
	}
	a.flash(i18n.T("Hier lassen sich nur Dateien aus dem Explorer/Finder ablegen"))
	return nil
}

// dropMenu: Dateien ins Terminal gezogen. Hochladen oder als Text einfügen?
func (a *App) dropMenu(s *session, text string, paths []string) {
	dir := "~"
	if s.files.panes[1] != nil {
		dir = s.files.panes[1].path
	}
	name := baseName(sshx.LocalFS{}, paths[0])
	if len(paths) > 1 {
		name = fmt.Sprintf(i18n.T("%d Dateien"), len(paths))
	}
	a.modal = &modal{kind: mMenu, title: i18n.T("Abgelegt: ") + trunc(name, 40), items: []menuItem{
		{label: i18n.T("Auf den Server hochladen"), hint: trunc(dir, 30), run: func() tea.Cmd { return a.uploadDropped(s, paths) }},
		{label: i18n.T("Pfad als Text einfügen"), hint: i18n.T("wie bisher"), run: func() tea.Cmd { return a.guardedPaste(s, text) }},
	}}
}

// filesDragMouse: Bewegen und Loslassen nach einem Klick auf einen Eintrag.
func (a *App) filesDragMouse(s *session, msg tea.MouseMsg) tea.Cmd {
	f := &s.files
	d := f.drag
	m := msg.Mouse()
	side, row := a.z.fileAt(m.X, m.Y)
	switch msg.(type) {
	case tea.MouseMotionMsg:
		d.over, d.overRow = side, row
		if side != d.side || row != d.idx {
			d.moved = true
		}
	case tea.MouseReleaseMsg:
		f.drag = nil
		if !d.moved || side < 0 || side == d.side || f.panes[0] == nil {
			return nil
		}
		src, dst := f.panes[d.side], f.panes[side]
		dir := dst.path
		if row > 0 && row < len(dst.entries) && dst.entries[row].IsDir {
			dir = dst.fs.Join(dst.path, dst.entries[row].Name)
		}
		f.side = side
		return a.startTransfer(s, src.fs, src.pickedFor(d.idx), dst.fs, dir, d.side == 0, false)
	}
	return nil
}

// dragTarget: Ordner, in den das Loslassen gerade kopieren würde ("" = keiner).
func (f *filesState) dragTarget() (n int, dir string) {
	d := f.drag
	if d == nil || !d.moved || d.over < 0 || d.over == d.side || f.panes[0] == nil {
		return 0, ""
	}
	dst := f.panes[d.over]
	dir = dst.path
	if d.overRow > 0 && d.overRow < len(dst.entries) && dst.entries[d.overRow].IsDir {
		dir = dst.fs.Join(dst.path, dst.entries[d.overRow].Name)
	}
	return len(f.panes[d.side].pickedFor(d.idx)), dir
}

func (a *App) filesKey(s *session, k tea.KeyPressMsg) tea.Cmd {
	f := &s.files
	key := k.String()
	if f.mkdir != nil {
		switch key {
		case "esc":
			f.mkdir = nil
		case "enter":
			name := strings.TrimSpace(f.mkdir.Value())
			f.mkdir = nil
			p := f.panes[f.side]
			if name != "" && !strings.ContainsAny(name, `/\`) {
				if err := p.fs.Mkdir(p.fs.Join(p.path, name)); err != nil {
					p.err = err.Error()
				} else {
					p.load(f.hidden)
					for i, e := range p.entries {
						if e.Name == name {
							p.sel = i
						}
					}
				}
			}
		default:
			var cmd tea.Cmd
			*f.mkdir, cmd = f.mkdir.Update(k)
			return cmd
		}
		return nil
	}
	if key == "esc" || a.keys.is(actFiles, key) {
		f.open = false
		return nil
	}
	if key == "x" && f.xfer != nil {
		f.xfer.cancel()
		return nil
	}
	if f.panes[0] == nil {
		return nil
	}
	p := f.panes[f.side]
	page := max(1, a.h-10)
	switch key {
	case "tab", "shift+tab":
		f.side = 1 - f.side
	case "up", "k":
		p.sel = max(0, p.sel-1)
	case "down", "j":
		p.sel = min(len(p.entries)-1, p.sel+1)
	case "pgup":
		p.sel = max(0, p.sel-page)
	case "pgdown":
		p.sel = min(len(p.entries)-1, p.sel+page)
	case "home":
		p.sel = 0
	case "end":
		p.sel = len(p.entries) - 1
	case "enter", "right", "l":
		if p.sel < len(p.entries) && p.entries[p.sel].IsDir {
			p.cd(p.entries[p.sel].Name, f.hidden)
		}
	case "backspace", "left", "h":
		p.cd("..", f.hidden)
	case "~":
		p.path, p.sel = p.fs.Home(), 0
		p.load(f.hidden)
	case ".":
		f.hidden = !f.hidden
		for _, x := range f.panes {
			x.load(f.hidden)
		}
	case "r":
		p.load(f.hidden)
	case "c", "f6":
		return a.startCopy(s, false)
	case "space", "insert":
		p.toggleMark(p.sel)
		p.sel = min(len(p.entries)-1, p.sel+1)
	case "a":
		if len(p.marked) > 0 {
			p.marked = nil
		} else {
			for i := 1; i < len(p.entries); i++ {
				p.toggleMark(i)
			}
		}
	case "n":
		in := newInput("", false)
		in.Placeholder = i18n.T("Name des neuen Ordners")
		f.mkdir = &in
		return f.mkdir.Focus()
	case "d", "delete":
		if p.sel <= 0 || p.sel >= len(p.entries) {
			return nil
		}
		e := p.entries[p.sel]
		target := p.fs.Join(p.path, e.Name)
		what := i18n.T("Die Datei")
		if e.IsDir {
			what = i18n.T("Den Ordner mit allem darin")
		}
		a.modal = confirmModal(i18n.T("Löschen?"), fmt.Sprintf(i18n.T("%s: %s\n%s endgültig löschen?"), i18n.T(p.fs.Label()), target, what), func() tea.Cmd {
			if err := p.fs.RemoveAll(target); err != nil {
				p.err = err.Error()
			}
			p.load(f.hidden)
			return nil
		})
	}
	return nil
}

func (a *App) filesKeys(s *session) [][2]string {
	if s.files.mkdir != nil {
		return [][2]string{{"⏎", i18n.T("anlegen")}, {"Esc", i18n.T("abbrechen")}}
	}
	k := [][2]string{{"Tab", i18n.T("Seite")}, {"⏎/←", i18n.T("öffnen/zurück")}, {"c", i18n.T("kopieren ↔")}, {"Space", i18n.T("markieren")}, {"n", i18n.T("Ordner")}, {"d", i18n.T("löschen")},
		{".", i18n.T("versteckte")}, {"~", "Home"}, {"r", i18n.T("neu laden")}, {"Esc/F5", "Terminal"}}
	if s.files.xfer != nil {
		k = append([][2]string{{"x", i18n.T("abbrechen")}}, k...)
	}
	return k
}

func (a *App) viewFiles(s *session, w, h int) string {
	st := a.s
	f := &s.files
	switch {
	case f.opening:
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, st.AI.Render(i18n.T("⠿ öffne SFTP-Kanal …")))
	case f.err != "":
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center,
			st.Danger.Render("× "+f.err)+"\n\n"+a.hints(st.Muted, [2]string{"Esc", i18n.T("zurück zum Terminal")}))
	case f.panes[0] == nil:
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, st.Muted.Render(i18n.T("keine Verbindung")))
	}
	// unten: Status/Fortschritt (2 Zeilen)
	status := ""
	switch {
	case f.mkdir != nil:
		f.mkdir.SetWidth(w - 20)
		status = st.Accent.Render(i18n.T(" Neuer Ordner: ")) + f.mkdir.View()
	case f.xfer != nil:
		x := f.xfer
		done, total := x.pr.Done.Load(), x.pr.Total.Load()
		pct := 0.0
		if total > 0 {
			pct = float64(done) / float64(total)
		}
		barW := max(10, w/3)
		fill := int(pct * float64(barW))
		bar := st.AI.Render(strings.Repeat("━", fill)) + st.Muted.Render(strings.Repeat("━", barW-fill))
		rate := float64(done) / max(time.Since(x.start).Seconds(), 0.001)
		status = " " + bar + fmt.Sprintf(" %3.0f%%  %s / %s  %s/s  ", pct*100, humanBytes(done), humanBytes(total), humanBytes(int64(rate))) +
			st.Muted.Render(trunc(x.what, max(10, w-barW-40)))
	case f.drag != nil && f.drag.moved:
		if n, dir := f.dragTarget(); dir != "" {
			status = st.Accent.Render(fmt.Sprintf(i18n.T(" Loslassen kopiert %d → %s"), n, dir))
		} else {
			status = st.Muted.Render(i18n.T(" Auf die andere Seite ziehen zum Kopieren"))
		}
	case f.last != "":
		status = " " + f.last
	default:
		status = st.Muted.Render(i18n.T(" c oder Ziehen kopiert auf die andere Seite (Leertaste markiert mehrere). Dateien aus dem Explorer/Finder hier ablegen lädt hoch"))
	}
	ph := h - 2
	lw := (w - 1) / 2
	rw := w - 1 - lw
	sep := lipgloss.NewStyle().Foreground(st.T.Border).Render(strings.Repeat("│\n", ph-1) + "│")
	cols := lipgloss.JoinHorizontal(lipgloss.Top, a.viewPane(f, 0, lw, ph), sep, a.viewPane(f, 1, rw, ph))
	return cols + "\n" + lipgloss.NewStyle().Foreground(st.T.Border).Render(strings.Repeat("─", w)) + "\n" + ansiTruncate(status, w)
}

func (a *App) viewPane(f *filesState, side, w, h int) string {
	st := a.s
	p, active := f.panes[side], f.side == side
	var b strings.Builder
	title := " " + i18n.T(p.fs.Label()) + " "
	if n := len(p.marked); n > 0 {
		title += fmt.Sprintf(i18n.T("· %d markiert "), n)
	}
	ts := st.Tab
	if active {
		ts = st.TabActive.Foreground(st.T.Accent)
	}
	_, dropDir := f.dragTarget()
	dropping := dropDir != "" && f.drag.over == side
	tr := ts.Render(title)
	b.WriteString(tr + " " + st.Muted.Render(truncLeft(p.path, max(4, w-lipgloss.Width(tr)-1))) + "\n")
	rows := h - 1
	if p.err != "" {
		b.WriteString(st.Danger.Render(trunc(" × "+p.err, w)) + "\n")
		rows--
	}
	start := 0
	if p.sel >= rows {
		start = p.sel - rows + 1
	}
	sizeW, dateW := 9, 12
	nameW := max(8, w-sizeW-dateW-4)
	n := 0
	for i := start; i < len(p.entries) && n < rows; i, n = i+1, n+1 {
		e := p.entries[i]
		icon, name := "  ", e.Name
		size, date := "", ""
		if e.IsDir {
			icon = "▸ "
			name += "/"
			if e.Name != ".." {
				date = e.ModTime.Format("02.01. 15:04")
			}
		} else {
			size, date = humanBytes(e.Size), e.ModTime.Format("02.01. 15:04")
		}
		if e.Link {
			icon = "→ "
		}
		if p.marked[e.Name] {
			icon = "✓ "
		}
		line := fmt.Sprintf("%s%-*s %*s %*s", icon, nameW, trunc(name, nameW), sizeW, size, dateW, date)
		switch {
		case dropping && i > 0 && i == f.drag.overRow && e.IsDir:
			line = lipgloss.NewStyle().Foreground(st.T.Accent).Reverse(true).Width(w).Render(line)
		case i == p.sel && active:
			line = st.Selected.Width(w).Render(line)
		case i == p.sel:
			line = lipgloss.NewStyle().Background(st.T.Surface).Width(w).Render(line)
		case p.marked[e.Name]:
			line = st.Warn.Render(line)
		case e.IsDir:
			line = st.Accent.Render(line)
		default:
			line = st.Text.Render(line)
		}
		i, e := i, e
		line = a.z.mark(line, zoneAction{pane: side + 1, row: i + 1, click: func(_ tea.Mouse, double bool) tea.Cmd {
			f.side, p.sel = side, i
			if double && e.IsDir {
				f.drag = nil
				p.cd(e.Name, f.hidden)
				return nil
			}
			f.drag = &fileDrag{side: side, idx: i, over: side, overRow: i}
			return nil
		}})
		b.WriteString(line + "\n")
	}
	for ; n < rows; n++ {
		b.WriteString("\n")
	}
	box := lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.TrimSuffix(b.String(), "\n"))
	return a.z.mark(box, zoneAction{
		block: true,
		pane:  side + 1,
		click: func(tea.Mouse, bool) tea.Cmd { f.side = side; return nil },
		wheel: func(up bool) tea.Cmd {
			f.side = side
			if up {
				p.sel = max(0, p.sel-3)
			} else {
				p.sel = min(len(p.entries)-1, p.sel+3)
			}
			return nil
		},
	})
}

func humanBytes(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	v, i := float64(n), 0
	units := []string{"KB", "MB", "GB", "TB"}
	for v /= u; v >= u && i < len(units)-1; i++ {
		v /= u
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

func truncLeft(s string, n int) string {
	r := []rune(s)
	if n <= 1 || len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n+1:])
}

func ansiTruncate(s string, w int) string { return ansi.Truncate(s, w, "…") }
func ansiPlainText(s string) string       { return ansi.Strip(s) }

func shortDur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return d.Round(100 * time.Millisecond).String()
}
