package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
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

// Platzfresser finden (F5)
//
//   - i: Ordnergrößen. Rechnet aus, wie viel jeder Ordner hier belegt (wie
//     du -sk), schreibt es in die Spalte Größe und sortiert die größten nach
//     oben, Ordner und Dateien gemischt. Unten steht die Summe.
//   - ⇧I: doppelte Dateien ab hier. Gleiche Größe, dann gleiche Prüfsumme
//     (sha256). Die Liste zeigt, wie viel Platz frei würde; ⏎ springt zur
//     Datei, Entf legt sie in den Papierkorb.

const (
	spaceLimit = 10 * time.Minute
	dupMax     = 200000 // so viele Dateien höchstens ansehen
	dupBatch   = 200    // Dateien pro Prüfsummen-Befehl
)

// sizeOf: Größe eines Eintrags; Ordner nur, wenn i sie ausgerechnet hat.
func (p *pane) sizeOf(e sshx.Entry) (int64, bool) {
	if !e.IsDir {
		return e.Size, true
	}
	if p.sizes != nil && p.sizesPath == p.path {
		n, ok := p.sizes[e.Name]
		return n, ok
	}
	return 0, false
}

// duCommand: belegter Platz der Einträge in dir (KiB) und die Summe.
func duCommand(dir string) string {
	return "cd " + packx.Q(dir) + " && du -xsk ./* ./.[!.]* ./..?* 2>/dev/null; du -xsk . 2>/dev/null"
}

// parseDu: „123\t./name“ → Bytes je Name; „N\t.“ ist die Summe.
func parseDu(out string) (map[string]int64, int64) {
	sizes := map[string]int64{}
	var total int64
	for _, l := range strings.Split(out, "\n") {
		kb, name, ok := strings.Cut(strings.TrimRight(l, "\r"), "\t")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(kb), 10, 64)
		if err != nil {
			continue
		}
		if name == "." {
			total = n * 1024
			continue
		}
		name = strings.TrimPrefix(name, "./")
		if name != "" && !strings.Contains(name, "/") {
			sizes[name] = n * 1024
		}
	}
	return sizes, total
}

// localDirSize: Summe der Dateigrößen unter p (Links nicht verfolgt).
func localDirSize(ctx context.Context, p string) int64 {
	var n int64
	_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// dirSizes: i im Dateien-Fenster.
func (a *App) dirSizes(s *session) tea.Cmd {
	f := &s.files
	p := f.panes[f.side]
	if p == nil {
		return nil
	}
	dir, fsys, c := p.path, p.fs, s.client
	entries := append([]sshx.Entry(nil), p.entries...)
	var sizes map[string]int64
	var total int64
	return a.startFileJob(s, i18n.T("Ordnergrößen: ")+dir, spaceLimit, func(ctx context.Context) (func() tea.Cmd, error) {
		if isLocal(fsys) {
			sizes = map[string]int64{}
			for _, e := range entries {
				if e.Name == ".." {
					continue
				}
				if e.IsDir && !e.Link {
					sizes[e.Name] = localDirSize(ctx, filepath.Join(dir, e.Name))
				} else {
					sizes[e.Name] = e.Size
				}
				total += sizes[e.Name]
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		} else {
			out, err := remoteRun(ctx, c, duCommand(dir), true)
			if err != nil {
				return nil, err
			}
			sizes, total = parseDu(out)
			if len(sizes) == 0 && total == 0 {
				return nil, fmt.Errorf(i18n.T("du lieferte nichts: %s"), tailText(out, 1))
			}
		}
		return func() tea.Cmd {
			if p.path != dir {
				return nil
			}
			p.sizes, p.sizesPath = sizes, dir
			p.sortBy, p.desc = sortSize, true
			p.sortEntries()
			p.sel = min(1, len(p.entries)-1)
			type big struct {
				name string
				n    int64
			}
			var top []big
			for n, v := range sizes {
				top = append(top, big{n, v})
			}
			sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
			var parts []string
			for _, b := range top[:min(3, len(top))] {
				parts = append(parts, fmt.Sprintf("%s %s", b.name, humanBytes(b.n)))
			}
			f.last = a.s.OK.Render(fmt.Sprintf(i18n.T("Belegt: %s in %s · größte: %s · ⇧I sucht doppelte Dateien"), humanBytes(total), dir, strings.Join(parts, ", ")))
			return a.filesPeek(s)
		}, nil
	})
}

// ---------------------------------------------------------------- doppelte Dateien

type dupFile struct {
	rel  string
	size int64
}

type dupGroup struct {
	size  int64
	files []string // relativ zum Startordner
}

func (g dupGroup) wasted() int64 { return g.size * int64(len(g.files)-1) }

// dupListCommand: Größe und Pfad aller Dateien ab dir.
func dupListCommand(dir string, hidden bool) string {
	prune := ""
	if !hidden {
		prune = `\( -name '.?*' -prune \) -o `
	}
	return "cd " + packx.Q(dir) + " && if find . -maxdepth 0 -printf '' >/dev/null 2>&1; then " +
		"find . -xdev " + prune + "-type f -size +0 -printf '%s %p\\n' 2>/dev/null; else " +
		"find . -xdev " + prune + "-type f -size +0 -exec stat -c '%s %n' {} + 2>/dev/null; fi | head -n " + strconv.Itoa(dupMax)
}

func parseSizeList(out string) []dupFile {
	var fs []dupFile
	for _, l := range strings.Split(out, "\n") {
		n, p, ok := strings.Cut(strings.TrimRight(l, "\r"), " ")
		if !ok {
			continue
		}
		size, err := strconv.ParseInt(n, 10, 64)
		if err != nil || size <= 0 {
			continue
		}
		fs = append(fs, dupFile{strings.TrimPrefix(p, "./"), size})
	}
	return fs
}

// sameSize: nur Dateien, deren Größe mehrfach vorkommt.
func sameSize(files []dupFile) map[int64][]string {
	by := map[int64][]string{}
	for _, f := range files {
		by[f.size] = append(by[f.size], f.rel)
	}
	for k, v := range by {
		if len(v) < 2 {
			delete(by, k)
		}
	}
	return by
}

// hashCommand: Prüfsummen für rels (im Ordner dir).
func hashCommand(dir string, rels []string) string {
	var b strings.Builder
	b.WriteString("cd " + packx.Q(dir) + " && { command -v sha256sum >/dev/null 2>&1 && H=sha256sum || H=md5sum; } && $H --")
	for _, r := range rels {
		b.WriteString(" " + packx.Q("./"+r))
	}
	b.WriteString(" 2>/dev/null")
	return b.String()
}

// parseHashes: „summe  ./pfad“ → Pfad → Summe.
func parseHashes(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimPrefix(strings.TrimRight(l, "\r"), `\`)
		sum, p, ok := strings.Cut(l, "  ")
		if !ok {
			sum, p, ok = strings.Cut(l, " *")
		}
		if ok {
			m[strings.TrimPrefix(p, "./")] = sum
		}
	}
	return m
}

// groupDups: gleiche Größe + gleiche Summe; größte Verschwendung zuerst.
func groupDups(bySize map[int64][]string, sums map[string]string) []dupGroup {
	var gs []dupGroup
	for size, rels := range bySize {
		bySum := map[string][]string{}
		for _, r := range rels {
			if s := sums[r]; s != "" {
				bySum[s] = append(bySum[s], r)
			}
		}
		for _, same := range bySum {
			if len(same) > 1 {
				sort.Strings(same)
				gs = append(gs, dupGroup{size, same})
			}
		}
	}
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].wasted() != gs[j].wasted() {
			return gs[i].wasted() > gs[j].wasted()
		}
		return gs[i].files[0] < gs[j].files[0]
	})
	return gs
}

func localSum(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// findDups: ⇧I im Dateien-Fenster.
func (a *App) findDups(s *session) tea.Cmd {
	f := &s.files
	p := f.panes[f.side]
	if p == nil {
		return nil
	}
	side, dir, fsys, hidden, c := f.side, p.path, p.fs, f.hidden, s.client
	var groups []dupGroup
	return a.startFileJob(s, i18n.T("Doppelte Dateien in ")+dir, spaceLimit, func(ctx context.Context) (func() tea.Cmd, error) {
		var files []dupFile
		if isLocal(fsys) {
			es, err := walkLocal(ctx, dir, hidden)
			if err != nil {
				return nil, err
			}
			for _, e := range es {
				if e.isDir {
					continue
				}
				if fi, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(e.rel))); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
					files = append(files, dupFile{e.rel, fi.Size()})
				}
			}
		} else {
			out, err := remoteRun(ctx, c, dupListCommand(dir, hidden), true)
			if err != nil {
				return nil, err
			}
			files = parseSizeList(out)
		}
		bySize := sameSize(files)
		var cand []string
		for _, rels := range bySize {
			cand = append(cand, rels...)
		}
		sort.Strings(cand)
		sums := map[string]string{}
		if isLocal(fsys) {
			for _, r := range cand {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				sums[r] = localSum(filepath.Join(dir, filepath.FromSlash(r)))
			}
		} else {
			for i := 0; i < len(cand); i += dupBatch {
				out, err := remoteRun(ctx, c, hashCommand(dir, cand[i:min(len(cand), i+dupBatch)]), true)
				if err != nil {
					return nil, err
				}
				for k, v := range parseHashes(out) {
					sums[k] = v
				}
			}
		}
		groups = groupDups(bySize, sums)
		return func() tea.Cmd { return a.showDups(s, side, dir, groups) }, nil
	})
}

func (a *App) showDups(s *session, side int, dir string, groups []dupGroup) tea.Cmd {
	f := &s.files
	if len(groups) == 0 {
		f.last = a.s.OK.Render(i18n.T("✓ keine doppelten Dateien in ") + dir)
		return nil
	}
	var total int64
	var items []pickItem
	for _, g := range groups {
		total += g.wasted()
		items = append(items, pickItem{label: fmt.Sprintf(i18n.T("%d× %s · %s frei möglich"), len(g.files), humanBytes(g.size), humanBytes(g.wasted())), disabled: i18n.T("Gruppe, darunter die Dateien")})
		for _, r := range g.files {
			items = append(items, pickItem{value: r, label: r, hint: humanBytes(g.size), indent: 1})
		}
	}
	f.last = a.s.Warn.Render(fmt.Sprintf(i18n.T("%d Gruppen doppelter Dateien · %s ließen sich freigeben"), len(groups), humanBytes(total)))
	pk := newPicker(fmt.Sprintf(i18n.T("Doppelte Dateien · %s frei möglich"), humanBytes(total)), i18n.T("filtern"), items, "\x00", nil)
	pk.help = i18n.T("⏎ springt hin · Entf legt die Datei in den Papierkorb")
	pk.onPick = func(it pickItem) tea.Cmd { return a.jumpToFound(s, side, dir, it.value) }
	pk.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		if key != "delete" || it == nil || it.value == "" {
			return nil, false
		}
		p := f.panes[side]
		if p == nil {
			return nil, true
		}
		full := p.fs.Join(append([]string{dir}, strings.Split(it.value, "/")...)...)
		done, err := moveToTrash(p.fs, []string{full})
		if err != nil {
			a.flashErr(errShort(err))
			return nil, true
		}
		fsys, rel := p.fs, it.value
		a.offerUndoFn(fmt.Sprintf(i18n.T("„%s“ im Papierkorb"), baseName(p.fs, full)), func() error { return restoreTrash(fsys, done) }, func() { p.load(f.hidden) })
		for i, x := range pk.items {
			if x.value == rel {
				pk.items = append(pk.items[:i:i], pk.items[i+1:]...)
				break
			}
		}
		p.load(f.hidden)
		return nil, true
	}
	return a.openPicker(pk)
}
