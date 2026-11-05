package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/sshx"
)

// Dateien (F5): links lokal, rechts der Server, per SFTP über dieselbe
// SSH-Verbindung. c kopiert den markierten Eintrag auf die andere Seite
// (also hoch- oder runterladen), Ordner rekursiv.

type pane struct {
	fs      sshx.FS
	path    string
	entries []sshx.Entry
	sel     int
	err     string
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
}

func (p *pane) cd(dir string, hidden bool) {
	old := p.path
	if dir == ".." {
		p.path = p.fs.Dir(old)
	} else {
		p.path = p.fs.Join(old, dir)
	}
	p.sel = 0
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
			f.last = a.s.Warn.Render("Abgebrochen: " + x.what)
		case m.err != nil:
			f.last = a.s.Danger.Render("✗ " + x.what + ": " + m.err.Error())
		default:
			f.last = a.s.OK.Render(fmt.Sprintf("✓ %s · %s in %s (%s/s)", x.what, humanBytes(x.pr.Done.Load()),
				shortDur(d), humanBytes(int64(float64(x.pr.Done.Load())/max(d.Seconds(), 0.001)))))
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

func (a *App) startCopy(s *session, overwrite bool) tea.Cmd {
	f := &s.files
	if f.xfer != nil {
		a.flash("Es läuft schon eine Übertragung (x bricht ab)")
		return nil
	}
	src, dst := f.panes[f.side], f.panes[1-f.side]
	if src.sel <= 0 || src.sel >= len(src.entries) {
		return nil
	}
	e := src.entries[src.sel]
	sp := src.fs.Join(src.path, e.Name)
	target := dst.fs.Join(dst.path, e.Name)
	if !overwrite {
		if _, err := dst.fs.Stat(target); err == nil {
			a.modal = confirmModal("Überschreiben?", "„"+e.Name+"“ gibt es in "+dst.path+" schon.",
				func() tea.Cmd { return a.startCopy(s, true) })
			return nil
		}
	}
	arrow := "↑"
	if f.side == 1 {
		arrow = "↓"
	}
	ctx, cancel := context.WithCancel(context.Background())
	x := &xfer{what: arrow + " " + e.Name + " → " + dst.path, pr: &sshx.Progress{}, cancel: cancel, start: time.Now()}
	f.xfer, f.last = x, ""
	sid := s.id
	srcFS, dstFS, dstDir := src.fs, dst.fs, dst.path
	run := func() tea.Msg {
		defer cancel()
		total, _, err := sshx.Size(ctx, srcFS, sp)
		if err != nil {
			return filesDoneMsg{sid, err}
		}
		x.pr.Total.Store(total)
		return filesDoneMsg{sid, sshx.Copy(ctx, srcFS, sp, dstFS, dstDir, x.pr)}
	}
	return tea.Batch(run, filesTick(sid))
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
	if key == "esc" || key == "f5" {
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
	case "n":
		in := newInput("", false)
		in.Placeholder = "Name des neuen Ordners"
		f.mkdir = &in
		return f.mkdir.Focus()
	case "d", "delete":
		if p.sel <= 0 || p.sel >= len(p.entries) {
			return nil
		}
		e := p.entries[p.sel]
		target := p.fs.Join(p.path, e.Name)
		what := "Die Datei"
		if e.IsDir {
			what = "Den Ordner mit allem darin"
		}
		a.modal = confirmModal("Löschen?", fmt.Sprintf("%s: %s\n%s endgültig löschen?", p.fs.Label(), target, what), func() tea.Cmd {
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
		return [][2]string{{"⏎", "anlegen"}, {"Esc", "abbrechen"}}
	}
	k := [][2]string{{"Tab", "Seite"}, {"⏎/←", "öffnen/zurück"}, {"c", "kopieren ⇄"}, {"n", "Ordner"}, {"d", "löschen"},
		{".", "versteckte"}, {"~", "Home"}, {"r", "neu laden"}, {"Esc/F5", "Terminal"}}
	if s.files.xfer != nil {
		k = append([][2]string{{"x", "abbrechen"}}, k...)
	}
	return k
}

func (a *App) viewFiles(s *session, w, h int) string {
	st := a.s
	f := &s.files
	switch {
	case f.opening:
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, st.AI.Render("⠿ öffne SFTP-Kanal …"))
	case f.err != "":
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center,
			st.Danger.Render("✗ "+f.err)+"\n\n"+st.Muted.Render("Esc zurück zum Terminal"))
	case f.panes[0] == nil:
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, st.Muted.Render("keine Verbindung"))
	}
	// unten: Status/Fortschritt (2 Zeilen)
	status := ""
	switch {
	case f.mkdir != nil:
		f.mkdir.SetWidth(w - 20)
		status = st.Accent.Render(" Neuer Ordner: ") + f.mkdir.View()
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
	case f.last != "":
		status = " " + f.last
	default:
		status = st.Muted.Render(" c kopiert den markierten Eintrag auf die andere Seite: links → rechts lädt hoch, rechts → links lädt herunter")
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
	title := " " + p.fs.Label() + " "
	ts := st.Tab
	if active {
		ts = st.TabActive.Foreground(st.T.Accent)
	}
	b.WriteString(ts.Render(title) + " " + st.Muted.Render(truncLeft(p.path, w-lipgloss.Width(title)-2)) + "\n")
	rows := h - 1
	if p.err != "" {
		b.WriteString(st.Danger.Render(trunc(" ✗ "+p.err, w)) + "\n")
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
			icon = "↪ "
		}
		line := fmt.Sprintf("%s%-*s %*s %*s", icon, nameW, trunc(name, nameW), sizeW, size, dateW, date)
		switch {
		case i == p.sel && active:
			line = st.Selected.Width(w).Render(line)
		case i == p.sel:
			line = lipgloss.NewStyle().Background(st.T.Surface).Width(w).Render(line)
		case e.IsDir:
			line = st.Accent.Render(line)
		default:
			line = st.Text.Render(line)
		}
		i, e := i, e
		line = a.z.click(line, func(_ tea.Mouse, double bool) tea.Cmd {
			f.side, p.sel = side, i
			if double && e.IsDir {
				p.cd(e.Name, f.hidden)
			}
			return nil
		})
		b.WriteString(line + "\n")
	}
	for ; n < rows; n++ {
		b.WriteString("\n")
	}
	box := lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.TrimSuffix(b.String(), "\n"))
	return a.z.mark(box, zoneAction{
		block: true,
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
