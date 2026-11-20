package ui

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"

	"github.com/db9979/sshtermy/internal/ui/clickzone"
)

// Orte und Verlauf im Dateien-Fenster (F5)
//
//   - g (oder ♦ in der Pfadleiste): Orte der aktiven Seite: gemerkte Ordner,
//     Home, /, übliche Server-Ordner (/var/log, /etc …), eingehängte
//     Laufwerke, Papierkorb und zuletzt besuchte Ordner. Tippen filtert.
//   - m merkt den aktuellen Ordner (noch einmal m: vergessen). Server-Ordner
//     gehören zur Verbindung (gehen mit dem Sync), lokale zu diesem Rechner.
//   - Alt+← / Alt+→: voriger / nächster Ordner wie im Browser.
//   - Ist das Fenster breit genug, stehen die Orte links als Leiste.

const (
	histMax      = 50
	sidebarMinW  = 150 // ab dieser Breite steht die Orte-Leiste links
	sidebarWidth = 24
)

// pushHist: nach einem Ordnerwechsel von old (Verlauf für Alt+←).
func (p *pane) pushHist(old string) {
	if p.histMove || old == "" || old == p.path {
		return
	}
	p.back = append(p.back, old)
	if len(p.back) > histMax {
		p.back = p.back[len(p.back)-histMax:]
	}
	p.fwd = nil
}

// histGo: Alt+← (d = -1) / Alt+→ (d = 1).
func (p *pane) histGo(d int, hidden bool) bool {
	from := &p.back
	to := &p.fwd
	if d > 0 {
		from, to = &p.fwd, &p.back
	}
	for len(*from) > 0 {
		target := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
		cur := p.path
		p.histMove = true
		p.goTo(target, hidden)
		p.histMove = false
		if p.path == target {
			*to = append(*to, cur)
			return true
		}
	}
	return false
}

type place struct {
	label, path, group string
}

// commonServerDirs: übliche Ordner auf Linux-Servern (nur, wenn es sie gibt).
var commonServerDirs = []string{"/var/log", "/etc", "/srv", "/opt", "/var/www", "/var/lib/docker/volumes", "/tmp", "/home"}

// skipFS: Dateisysteme, die keine echten Laufwerke sind.
var skipFS = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true, "cgroup": true, "cgroup2": true,
	"securityfs": true, "pstore": true, "debugfs": true, "tracefs": true, "configfs": true, "fusectl": true,
	"mqueue": true, "hugetlbfs": true, "autofs": true, "binfmt_misc": true, "bpf": true, "nsfs": true,
	"squashfs": true, "overlay": true, "rpc_pipefs": true, "efivarfs": true, "ramfs": true, "selinuxfs": true,
	"fuse.gvfsd-fuse": true, "fuse.portal": true, "fuse.lxcfs": true, "nfsd": true,
}

// parseMounts: /proc/mounts → Einhängepunkte echter Laufwerke (ohne /).
func parseMounts(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 || skipFS[f[2]] {
			continue
		}
		mp := unescapeMount(f[1])
		switch {
		case mp == "/", strings.HasPrefix(mp, "/proc"), strings.HasPrefix(mp, "/sys"), strings.HasPrefix(mp, "/dev"),
			strings.HasPrefix(mp, "/snap/"), strings.HasPrefix(mp, "/var/lib/docker/"), strings.HasPrefix(mp, "/var/lib/kubelet/"),
			strings.HasPrefix(mp, "/run") && !strings.HasPrefix(mp, "/run/media/"), mp == "/boot/efi":
			continue
		}
		if !slices.Contains(out, mp) {
			out = append(out, mp)
		}
	}
	return out
}

// unescapeMount: Leerzeichen & Co. stehen in /proc/mounts als \040.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func readMounts(fsys sshx.FS) []string {
	if isLocal(fsys) {
		switch runtime.GOOS {
		case "windows":
			var out []string
			for c := 'C'; c <= 'Z'; c++ {
				d := string(c) + `:\`
				if _, err := os.Stat(d); err == nil {
					out = append(out, d)
				}
			}
			return out
		case "darwin":
			var out []string
			es, _ := os.ReadDir("/Volumes")
			for _, e := range es {
				out = append(out, filepath.Join("/Volumes", e.Name()))
			}
			return out
		}
	}
	r, err := fsys.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer r.Close()
	b, _ := io.ReadAll(io.LimitReader(r, 256<<10))
	return parseMounts(string(b))
}

// fileMarks: gemerkte Ordner der Seite.
func (a *App) fileMarks(s *session, side int) []string {
	if side == 1 {
		return s.conn.FileMarks
	}
	return a.st.Settings().FileMarks
}

// toggleMark: m im Dateien-Fenster.
func (a *App) toggleFileMark(s *session, side int) tea.Cmd {
	p := s.files.panes[side]
	if p == nil {
		return nil
	}
	marks := slices.Clone(a.fileMarks(s, side))
	dir := p.path
	removed := false
	if i := slices.Index(marks, dir); i >= 0 {
		marks = slices.Delete(marks, i, i+1)
		removed = true
	} else {
		marks = append(marks, dir)
	}
	if err := a.saveFileMarks(s, side, marks); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	if removed {
		a.flash(i18n.T("Ordner vergessen: ") + dir)
	} else {
		a.flashOK(i18n.T("Ordner gemerkt ♦ ") + dir + i18n.T(" · g zeigt die Orte"))
	}
	return nil
}

func (a *App) saveFileMarks(s *session, side int, marks []string) error {
	if side == 0 {
		return a.st.UpdateSettings(func(st *domain.Settings) { st.FileMarks = marks })
	}
	c, ok := a.st.Connection(s.conn.ID)
	if !ok { // Schnellverbindung ohne Eintrag: nur für diese Sitzung
		s.conn.FileMarks = marks
		return nil
	}
	c.FileMarks = marks
	if _, err := a.st.UpsertConnection(c); err != nil {
		return err
	}
	for _, x := range a.sessions {
		if x.conn.ID == c.ID {
			x.conn.FileMarks = marks
		}
	}
	return nil
}

// placesFor: alle Orte der Seite. Laufwerke und übliche Ordner werden
// einmal je geöffnetem Dateien-Fenster gelesen.
func (a *App) placesFor(s *session, side int) []place {
	f := &s.files
	p := f.panes[side]
	if p == nil {
		return nil
	}
	var out []place
	for _, m := range a.fileMarks(s, side) {
		out = append(out, place{"♦ " + m, m, i18n.T("Gemerkt")})
	}
	home := p.fs.Home()
	out = append(out, place{"~ " + i18n.T("Home"), home, i18n.T("Orte")})
	root := "/"
	if isLocal(p.fs) && runtime.GOOS == "windows" {
		root = filepath.VolumeName(home) + `\`
	}
	out = append(out, place{"/ " + root, root, i18n.T("Orte")})
	if f.placeCache[side] == nil {
		var extra []place
		if !isLocal(p.fs) {
			for _, d := range commonServerDirs {
				if e, err := p.fs.Stat(d); err == nil && e.IsDir {
					extra = append(extra, place{"▸ " + d, d, i18n.T("Server")})
				}
			}
		}
		for _, m := range readMounts(p.fs) {
			extra = append(extra, place{"◉ " + m, m, i18n.T("Laufwerke")})
		}
		f.placeCache[side] = append(extra, place{"◌ " + i18n.T("Papierkorb"), trashLocation(p.fs).files, i18n.T("Orte")})
	}
	out = append(out, f.placeCache[side]...)
	seen := map[string]bool{p.path: true}
	n := 0
	for i := len(p.back) - 1; i >= 0 && n < 5; i-- {
		d := p.back[i]
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, place{"‹ " + d, d, i18n.T("Zuletzt")})
		n++
	}
	return out
}

// openPlaces: g im Dateien-Fenster.
func (a *App) openPlaces(s *session, side int) tea.Cmd {
	f := &s.files
	if f.panes[side] == nil {
		return nil
	}
	f.side = side
	var items []pickItem
	for _, pl := range a.placesFor(s, side) {
		items = append(items, pickItem{value: pl.path, label: pl.label, hint: pl.group})
	}
	pk := newPicker(i18n.T("Orte · ")+i18n.T(f.panes[side].fs.Label()), i18n.T("Ort suchen"), items, f.panes[side].path, nil)
	pk.help = i18n.T("⏎ springt hin · m im Dateien-Fenster merkt einen Ordner · Entf vergisst ♦")
	pk.onPick = func(it pickItem) tea.Cmd { return a.goPlace(s, side, it.value) }
	pk.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		if key != "delete" || it == nil || !strings.HasPrefix(it.label, "♦ ") {
			return nil, false
		}
		marks := slices.DeleteFunc(slices.Clone(a.fileMarks(s, side)), func(m string) bool { return m == it.value })
		if err := a.saveFileMarks(s, side, marks); err != nil {
			a.flashErr(err.Error())
			return nil, true
		}
		for i, x := range pk.items {
			if x.label == it.label {
				pk.items = slices.Delete(pk.items, i, i+1)
				break
			}
		}
		return nil, true
	}
	return a.openPicker(pk)
}

func (a *App) goPlace(s *session, side int, dir string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil {
		return nil
	}
	if inTrash(p.fs, dir) {
		return a.openTrash(s, side)
	}
	f.side = side
	p.goTo(dir, f.hidden)
	return nil
}

// viewPlacesBar: Orte-Leiste links (breite Fenster), für die aktive Seite.
func (a *App) viewPlacesBar(s *session, w, h int) string {
	st := a.s
	f := &s.files
	side := f.side
	p := f.panes[side]
	var b strings.Builder
	b.WriteString(st.TabActive.Foreground(st.T.Accent).Render(" "+i18n.T("Orte")+" ") + st.Muted.Render(" "+trunc(i18n.T(p.fs.Label()), w-8)) + "\n")
	rows := h - 1
	group := ""
	n := 0
	for _, pl := range a.placesFor(s, side) {
		if n >= rows {
			break
		}
		if pl.group != group {
			if n > 0 && n < rows-1 {
				b.WriteString("\n")
				n++
			}
			group = pl.group
			b.WriteString(st.Muted.Render(" "+trunc(pl.group, w-2)) + "\n")
			n++
			if n >= rows {
				break
			}
		}
		label := " " + trunc(pl.label, w-2)
		style := st.Text
		if pl.path == p.path {
			style = st.Selected.Width(w)
		}
		target := pl.path
		b.WriteString(a.z.Mark(style.Render(label), clickzone.Action{
			Click: func(tea.Mouse, bool) tea.Cmd { return a.goPlace(s, side, target) },
			Tip:   func() string { return target },
		}) + "\n")
		n++
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.TrimSuffix(b.String(), "\n"))
}
