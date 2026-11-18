package ui

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/store"
)

// Papierkorb im Dateien-Fenster (F5)
//
// d/Entf verschiebt die Auswahl ohne Rückfrage in den Papierkorb, Strg+Z
// (oder ein Klick auf den Hinweis) holt sie zurück. ⇧D/⇧Entf löscht wie
// bisher endgültig, nach Rückfrage. Auf dem Server und unter Linux ist das
// der übliche Papierkorb nach freedesktop.org (~/.local/share/Trash, mit
// .trashinfo, den auch Dateimanager und trash-cli kennen), auf dem Mac
// ~/.Trash, unter Windows ein eigener Ordner im sshTermy-Datenordner.
// Geht das Verschieben nicht (anderes Laufwerk, keine Rechte), fragt
// sshTermy, ob es endgültig löschen soll.

// trashOverride: Tests legen den lokalen Papierkorb in einen Testordner.
var trashOverride string

type trashLoc struct {
	files string // hierhin wandern die Einträge
	info  string // .trashinfo-Dateien ("" = keine, Mac)
}

func trashLocation(fsys sshx.FS) trashLoc {
	if !isLocal(fsys) {
		root := fsys.Join(fsys.Home(), ".local", "share", "Trash")
		return trashLoc{fsys.Join(root, "files"), fsys.Join(root, "info")}
	}
	if trashOverride != "" {
		return trashLoc{filepath.Join(trashOverride, "files"), filepath.Join(trashOverride, "info")}
	}
	home := fsys.Home()
	switch runtime.GOOS {
	case "darwin":
		return trashLoc{filepath.Join(home, ".Trash"), ""}
	case "windows":
		dir, err := store.Dir()
		if err != nil {
			dir = filepath.Join(home, ".sshtermy")
		}
		root := filepath.Join(dir, "Papierkorb")
		return trashLoc{filepath.Join(root, "files"), filepath.Join(root, "info")}
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	root := filepath.Join(data, "Trash")
	return trashLoc{filepath.Join(root, "files"), filepath.Join(root, "info")}
}

// inTrash: liegt p im Papierkorb (dann löscht d endgültig)?
func inTrash(fsys sshx.FS, p string) bool {
	t := trashLocation(fsys).files
	return p == t || strings.HasPrefix(p, t+"/") || strings.HasPrefix(p, t+string(filepath.Separator))
}

// trashed: ein Eintrag im Papierkorb, zum Zurückholen.
type trashed struct {
	from, to, info string
}

// moveToTrash verschiebt paths in den Papierkorb. Bei einem Fehler bleibt
// das bis dahin Verschobene im Papierkorb (done), der Rest unberührt.
func moveToTrash(fsys sshx.FS, paths []string) (done []trashed, err error) {
	loc := trashLocation(fsys)
	if err := fsys.Mkdir(loc.files); err != nil {
		return nil, err
	}
	if loc.info != "" {
		if err := fsys.Mkdir(loc.info); err != nil {
			return nil, err
		}
	}
	for _, p := range paths {
		name := baseName(fsys, p)
		target, infoPath := "", ""
		for n := 1; n < 1000; n++ {
			cand := name
			if n > 1 {
				cand = fmt.Sprintf("%s.%d", name, n)
			}
			_, e1 := fsys.Stat(fsys.Join(loc.files, cand))
			e2 := errors.New("")
			if loc.info != "" {
				_, e2 = fsys.Stat(fsys.Join(loc.info, cand+".trashinfo"))
			}
			if e1 != nil && e2 != nil {
				target = fsys.Join(loc.files, cand)
				if loc.info != "" {
					infoPath = fsys.Join(loc.info, cand+".trashinfo")
				}
				break
			}
		}
		if target == "" {
			return done, errors.New(i18n.T("Papierkorb: kein freier Name"))
		}
		if infoPath != "" { // zuerst die Info, wie es die Vorgabe will
			if err := writeSmall(fsys, infoPath, trashInfo(p, time.Now())); err != nil {
				return done, err
			}
		}
		if err := fsys.Rename(p, target); err != nil {
			if infoPath != "" {
				_ = fsys.RemoveAll(infoPath)
			}
			return done, err
		}
		done = append(done, trashed{from: p, to: target, info: infoPath})
	}
	return done, nil
}

// restoreTrash holt Einträge zurück (rückwärts, damit Ordner vor ihrem
// Inhalt wieder da sind, falls beides drin war).
func restoreTrash(fsys sshx.FS, items []trashed) error {
	var errs []string
	for i := len(items) - 1; i >= 0; i-- {
		t := items[i]
		if err := fsys.Rename(t.to, t.from); err != nil {
			errs = append(errs, baseName(fsys, t.from)+": "+err.Error())
			continue
		}
		if t.info != "" {
			_ = fsys.RemoveAll(t.info)
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func trashInfo(p string, at time.Time) string {
	u := (&url.URL{Path: filepath.ToSlash(p)}).EscapedPath()
	return "[Trash Info]\nPath=" + u + "\nDeletionDate=" + at.Format("2006-01-02T15:04:05") + "\n"
}

func writeSmall(fsys sshx.FS, p, content string) error {
	w, err := fsys.Create(p, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, content); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// trashPaths: d/Entf im Dateien-Fenster.
func (a *App) trashPaths(s *session, side int, paths []string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || len(paths) == 0 {
		return nil
	}
	if inTrash(p.fs, paths[0]) {
		return a.askDelete(s, side, paths) // schon im Papierkorb: endgültig
	}
	done, err := moveToTrash(p.fs, paths)
	p.marked = nil
	p.load(f.hidden)
	if len(done) > 0 {
		name := baseName(p.fs, done[0].from)
		what := fmt.Sprintf(i18n.T("„%s“ im Papierkorb"), name)
		if len(done) > 1 {
			what = fmt.Sprintf(i18n.T("%d Einträge im Papierkorb"), len(done))
		}
		fsys := p.fs
		a.offerUndoFn(what, func() error { return restoreTrash(fsys, done) }, func() {
			for _, x := range f.panes {
				if x != nil {
					x.load(f.hidden)
				}
			}
			p.selectName(name)
		})
	}
	if err != nil {
		rest := paths[len(done):]
		a.modal = confirmModal(i18n.T("Papierkorb geht hier nicht"),
			fmt.Sprintf(i18n.T("%s\n(%s)\n\nStattdessen endgültig löschen?"), sudoListNames(rest), errShort(err)),
			func() tea.Cmd { return a.askDelete(s, side, rest) })
	}
	return nil
}

// openTrash: Papierkorb der aktiven Seite anzeigen.
func (a *App) openTrash(s *session, side int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil {
		return nil
	}
	dir := trashLocation(p.fs).files
	if err := p.fs.Mkdir(dir); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	f.side = side
	p.goTo(dir, f.hidden)
	a.flash(i18n.T("Papierkorb · d löscht hier endgültig, „Papierkorb leeren“ im Rechtsklick-Menü"))
	return nil
}

// emptyTrash: alles im Papierkorb endgültig löschen (nach Rückfrage).
func (a *App) emptyTrash(s *session, side int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil {
		return nil
	}
	loc := trashLocation(p.fs)
	es, err := p.fs.ReadDir(loc.files)
	if err != nil || len(es) == 0 {
		a.flash(i18n.T("Der Papierkorb ist leer"))
		return nil
	}
	a.modal = confirmModal(i18n.T("Papierkorb leeren?"), fmt.Sprintf(i18n.T("%d Einträge in %s endgültig löschen?"), len(es), loc.files), func() tea.Cmd {
		for _, e := range es {
			if err := p.fs.RemoveAll(p.fs.Join(loc.files, e.Name)); err != nil {
				a.flashErr(err.Error())
				break
			}
			if loc.info != "" {
				_ = p.fs.RemoveAll(p.fs.Join(loc.info, e.Name+".trashinfo"))
			}
		}
		p.load(f.hidden)
		return nil
	})
	return nil
}
