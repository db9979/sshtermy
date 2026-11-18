package ui

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/packx"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Datei finden (F5, /): liest einmal den Ordnerbaum ab dem aktuellen Ordner
// (höchstens findMax Einträge, findDepth Ebenen tief, ohne andere
// Laufwerke), dann sucht das Tippen unscharf darin: „ngcf“ findet
// nginx.conf. ⏎ springt zum Treffer. Versteckte Ordner nur, wenn sie
// gerade angezeigt werden (.).

const (
	findMax   = 50000
	findDepth = 12
	findLimit = 60 * time.Second
)

type foundEntry struct {
	rel   string // relativ zum Startordner, mit /
	isDir bool
}

// findCommand: Befehl für den Server (POSIX find, auch busybox).
func findCommand(dir string, hidden bool) string {
	prune := ""
	if !hidden {
		prune = `\( -name '.?*' -prune \) -o `
	}
	base := "cd " + packx.Q(dir) + " && find . -xdev -maxdepth " + strconv.Itoa(findDepth) + " " + prune
	return base + "-type d -print 2>/dev/null | head -n 20000 | sed 's|$|/|'; " +
		base + "! -type d -print 2>/dev/null | head -n " + strconv.Itoa(findMax)
}

// parseFind: Ausgabe von findCommand → Einträge (Ordner enden auf /).
func parseFind(out string) []foundEntry {
	var es []foundEntry
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimRight(l, "\r")
		isDir := strings.HasSuffix(l, "/")
		l = strings.TrimSuffix(l, "/")
		l = strings.TrimPrefix(l, "./")
		if l == "" || l == "." {
			continue
		}
		es = append(es, foundEntry{rel: l, isDir: isDir})
	}
	return es
}

// walkLocal: dasselbe auf diesem Rechner.
func walkLocal(ctx context.Context, dir string, hidden bool) ([]foundEntry, error) {
	var es []foundEntry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if !hidden && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && strings.Count(rel, "/") >= findDepth-1 {
			es = append(es, foundEntry{rel, true})
			return fs.SkipDir
		}
		es = append(es, foundEntry{rel, d.IsDir()})
		if len(es) >= findMax {
			return fs.SkipAll
		}
		return nil
	})
	return es, err
}

// openFind: / im Dateien-Fenster.
func (a *App) openFind(s *session) tea.Cmd {
	f := &s.files
	p := f.panes[f.side]
	if p == nil {
		return nil
	}
	side, dir, fsys, hidden, c := f.side, p.path, p.fs, f.hidden, s.client
	pk := newPicker(i18n.T("Datei finden in ")+trunc(dir, 50), i18n.T("Teil des Namens tippen, z. B. ngcf für nginx.conf"),
		[]pickItem{{label: i18n.T("⠿ lese den Ordnerbaum …"), disabled: i18n.T("lädt noch")}}, "\x00", nil)
	pk.fuzzy = true
	pk.help = i18n.T("unscharf: Buchstaben der Reihe nach · ⏎ springt hin")
	pk.onPick = func(it pickItem) tea.Cmd { return a.jumpToFound(s, side, dir, it.value) }
	cmd := a.openPicker(pk)
	load := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), findLimit)
		defer cancel()
		var es []foundEntry
		var err error
		if isLocal(fsys) {
			es, err = walkLocal(ctx, dir, hidden)
		} else {
			var out string
			out, err = remoteRun(ctx, c, findCommand(dir, hidden), true)
			es = parseFind(out)
		}
		return findLoadedMsg{pk: pk, es: es, err: err, fsys: fsys}
	}
	return tea.Batch(cmd, load)
}

type findLoadedMsg struct {
	pk   *picker
	es   []foundEntry
	err  error
	fsys sshx.FS
}

func (a *App) findLoaded(m findLoadedMsg) {
	if a.pick != m.pk {
		return
	}
	sort.SliceStable(m.es, func(i, j int) bool { return m.es[i].rel < m.es[j].rel })
	items := make([]pickItem, 0, len(m.es))
	for _, e := range m.es {
		label, hint := e.rel, ""
		if e.isDir {
			label += "/"
			hint = i18n.T("Ordner")
		}
		items = append(items, pickItem{value: e.rel, label: label, hint: hint})
	}
	if len(items) == 0 {
		msg := i18n.T("nichts gefunden")
		if m.err != nil {
			msg = errShort(m.err)
		}
		items = []pickItem{{label: msg, disabled: msg}}
	}
	m.pk.items = items
	m.pk.title = strings.Replace(m.pk.title, i18n.T("Datei finden in "), i18n.F("Datei finden (%d) in ", len(m.es)), 1)
	if len(m.es) >= findMax {
		m.pk.help = i18n.F("nur die ersten %d Einträge · tiefer im Baum anfangen findet mehr", findMax)
	}
}

// jumpToFound: zum Ordner des Treffers und ihn auswählen.
func (a *App) jumpToFound(s *session, side int, dir, rel string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || rel == "" {
		return nil
	}
	parts := strings.Split(rel, "/")
	full := p.fs.Join(append([]string{dir}, parts...)...)
	f.side = side
	p.goTo(p.fs.Dir(full), f.hidden)
	p.selectName(parts[len(parts)-1])
	return a.filesPeek(s)
}
