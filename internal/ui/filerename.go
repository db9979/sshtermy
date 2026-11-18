package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Viele Dateien im Editor umbenennen (F5, ⇧F2 oder R)
//
// Die Namen der markierten Einträge (ohne Markierung: alle im Ordner)
// kommen als Liste, eine Zeile je Name, in den eigenen Editor. Nach dem
// Speichern zeigt sshTermy, was es umbenennen würde, oder pro Zeile, was
// nicht geht (doppelt, / im Namen, gibt es schon). Erst nach ⏎ wird
// umbenannt. Tauschen (a↔b) und Ketten gehen, weil alles erst unter einem
// Zwischennamen geparkt wird. Geht unterwegs etwas schief, wird alles
// zurückgedreht.

const bulkRenLife = 30 * time.Minute

type bulkRename struct {
	sid, side int
	dir       string
	fsys      sshx.FS
	old       []string
	file      string
	mod       time.Time
	started   time.Time
	asking    bool
}

type bulkRenTick struct{}

type renPair struct{ from, to string }

func bulkRenAfter() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return bulkRenTick{} })
}

// startBulkRename: ⇧F2 im Dateien-Fenster.
func (a *App) startBulkRename(s *session, side int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil {
		return nil
	}
	var names []string
	for _, e := range p.entries {
		if e.Name == ".." {
			continue
		}
		if len(p.marked) == 0 || p.marked[e.Name] {
			names = append(names, e.Name)
		}
	}
	if len(names) == 0 {
		a.flash(i18n.T("Hier gibt es nichts umzubenennen"))
		return nil
	}
	for _, n := range names {
		if strings.ContainsAny(n, "\r\n") {
			a.flashErr(i18n.T("Ein Name enthält einen Zeilenumbruch, das geht nur einzeln mit F2"))
			return nil
		}
	}
	dir, err := os.MkdirTemp("", "sshtermy-umbenennen-")
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.openTmp = append(a.openTmp, dir)
	file := filepath.Join(dir, "namen.txt")
	if err := os.WriteFile(file, []byte(strings.Join(names, "\n")+"\n"), 0o600); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	st, _ := os.Stat(file)
	b := &bulkRename{sid: s.id, side: side, dir: p.path, fsys: p.fs, old: names, file: file, started: time.Now()}
	if st != nil {
		b.mod = st.ModTime()
	}
	a.bulkRen = b
	a.flash(fmt.Sprintf(i18n.T("%d Namen im Editor: ändern und speichern, dann fragt sshTermy nach"), len(names)))
	return tea.Batch(a.launchEditor(file), bulkRenAfter())
}

func (a *App) bulkRenameTick() tea.Cmd {
	b := a.bulkRen
	if b == nil {
		return nil
	}
	if time.Since(b.started) > bulkRenLife || a.byID(b.sid) == nil {
		a.bulkRen = nil
		return nil
	}
	if b.asking {
		if a.modal != nil {
			return bulkRenAfter()
		}
		b.asking = false // Rückfrage mit Nein geschlossen: wieder auf Speichern warten
	}
	st, err := os.Stat(b.file)
	if err != nil || !st.ModTime().After(b.mod) {
		return bulkRenAfter()
	}
	b.mod = st.ModTime()
	data, err := os.ReadFile(b.file)
	if err != nil {
		return bulkRenAfter()
	}
	pairs, errs := planRenames(b.old, strings.Split(string(data), "\n"), func(n string) bool {
		_, err := b.fsys.Stat(b.fsys.Join(b.dir, n))
		return err == nil
	})
	switch {
	case len(errs) > 0:
		a.modal = &modal{kind: mInfo, title: i18n.T("So geht das Umbenennen nicht"), body: strings.Join(errs[:min(12, len(errs))], "\n") + "\n\n" + i18n.T("Im Editor korrigieren und noch einmal speichern.")}
		return bulkRenAfter()
	case len(pairs) == 0:
		a.flash(i18n.T("Keine Namen geändert"))
		return bulkRenAfter()
	}
	var lines []string
	for _, r := range pairs[:min(12, len(pairs))] {
		lines = append(lines, trunc(r.from, 30)+" → "+trunc(r.to, 30))
	}
	if len(pairs) > 12 {
		lines = append(lines, fmt.Sprintf(i18n.T("… und %d weitere"), len(pairs)-12))
	}
	b.asking = true
	m := confirmModal(fmt.Sprintf(i18n.T("%d umbenennen?"), len(pairs)), b.dir+"\n\n"+strings.Join(lines, "\n"), func() tea.Cmd {
		a.bulkRen = nil
		s := a.byID(b.sid)
		if err := applyRenames(b.fsys, b.dir, pairs); err != nil {
			a.flashErr(i18n.T("Umbenennen ging nicht, nichts geändert: ") + err.Error())
		} else {
			a.flashOK(fmt.Sprintf(i18n.T("%d Einträge umbenannt"), len(pairs)))
		}
		if s != nil && s.files.panes[b.side] != nil {
			p := s.files.panes[b.side]
			p.marked = nil
			p.load(s.files.hidden)
		}
		return nil
	})
	a.modal = m
	return bulkRenAfter()
}

// planRenames vergleicht die alten Namen mit den Zeilen aus dem Editor.
// exists meldet, ob es einen Namen im Ordner schon gibt.
func planRenames(old, lines []string, exists func(string) bool) ([]renPair, []string) {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) != len(old) {
		return nil, []string{fmt.Sprintf(i18n.T("Es müssen %d Zeilen bleiben, jetzt sind es %d (Zeilen nicht löschen oder umsortieren)"), len(old), len(lines))}
	}
	isOld := map[string]bool{}
	for _, o := range old {
		isOld[o] = true
	}
	seen := map[string]int{}
	var pairs []renPair
	var errs []string
	for i, l := range lines {
		n := strings.TrimRight(l, "\r")
		switch {
		case strings.TrimSpace(n) == "":
			errs = append(errs, fmt.Sprintf(i18n.T("Zeile %d: leer"), i+1))
			continue
		case strings.ContainsAny(n, `/\`):
			errs = append(errs, fmt.Sprintf(i18n.T("Zeile %d: „%s“ enthält / oder \\"), i+1, n))
			continue
		case n == "." || n == "..":
			errs = append(errs, fmt.Sprintf(i18n.T("Zeile %d: „%s“ geht nicht"), i+1, n))
			continue
		}
		if j, ok := seen[n]; ok {
			errs = append(errs, fmt.Sprintf(i18n.T("Zeile %d: „%s“ steht schon in Zeile %d"), i+1, n, j+1))
			continue
		}
		seen[n] = i
		if n == old[i] {
			continue
		}
		if !isOld[n] && exists(n) {
			errs = append(errs, fmt.Sprintf(i18n.T("Zeile %d: „%s“ gibt es schon"), i+1, n))
			continue
		}
		pairs = append(pairs, renPair{old[i], n})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return pairs, nil
}

// applyRenames: erst alle unter Zwischennamen parken, dann auf die neuen
// Namen. Bei einem Fehler wird alles Bisherige zurückgedreht.
func applyRenames(fsys sshx.FS, dir string, pairs []renPair) error {
	type move struct{ from, to string }
	var done []move
	undo := func() error {
		var errs []string
		for i := len(done) - 1; i >= 0; i-- {
			if err := fsys.Rename(done[i].to, done[i].from); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return nil
	}
	do := func(from, to string) error {
		if err := fsys.Rename(fsys.Join(dir, from), fsys.Join(dir, to)); err != nil {
			if uerr := undo(); uerr != nil {
				return fmt.Errorf("%v (%s %v)", err, i18n.T("Zurückdrehen unvollständig:"), uerr)
			}
			return err
		}
		done = append(done, move{fsys.Join(dir, from), fsys.Join(dir, to)})
		return nil
	}
	stamp := time.Now().UnixNano()
	parked := make([]string, len(pairs))
	for i, r := range pairs {
		parked[i] = fmt.Sprintf(".sshtermy-ren-%d-%d", stamp, i)
		if err := do(r.from, parked[i]); err != nil {
			return err
		}
	}
	for i, r := range pairs {
		if err := do(parked[i], r.to); err != nil {
			return err
		}
	}
	return nil
}
