package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Ausschneiden und Einfügen (F5): x schneidet die Auswahl (oder die
// markierten Einträge) aus, p fügt sie im Ordner der aktiven Seite ein.
// Auf derselben Seite wird verschoben (umbenannt, auf dem Server ohne
// Kopieren), auf der anderen Seite erst kopiert und nach Erfolg das
// Original gelöscht. Esc vergisst das Ausgeschnittene.

type fileCut struct {
	side  int
	fs    sshx.FS
	paths []string
}

func (c *fileCut) has(fsys sshx.FS, full string) bool {
	if c == nil || c.fs != fsys {
		return false
	}
	for _, p := range c.paths {
		if p == full {
			return true
		}
	}
	return false
}

// cutPicked: x im Dateien-Fenster.
func (a *App) cutPicked(s *session, side int, paths []string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || len(paths) == 0 {
		return nil
	}
	f.cut = &fileCut{side: side, fs: p.fs, paths: paths}
	p.marked = nil
	what := baseName(p.fs, paths[0])
	if len(paths) > 1 {
		what = fmt.Sprintf(i18n.T("%d Einträge"), len(paths))
	}
	a.flash(fmt.Sprintf(i18n.T("» %s ausgeschnitten · in den Zielordner wechseln, p fügt ein, Esc vergisst"), what))
	return nil
}

// pasteCut: p im Dateien-Fenster, fügt in den Ordner der Seite side ein.
func (a *App) pasteCut(s *session, side int) tea.Cmd {
	f := &s.files
	c := f.cut
	dst := f.panes[side]
	if dst == nil {
		return nil
	}
	if c == nil {
		a.flash(i18n.T("Erst mit x etwas ausschneiden"))
		return nil
	}
	src := f.panes[c.side]
	if src == nil {
		f.cut = nil
		return nil
	}
	if c.side != side { // andere Seite: kopieren, danach Original löschen
		paths := c.paths
		cmd := a.startTransfer(s, c.fs, paths, dst.fs, dst.path, c.side == 0, false)
		if f.xfer != nil {
			f.cut = nil
			srcFS := c.fs
			f.xfer.then = func() {
				for _, p := range paths {
					if err := srcFS.RemoveAll(p); err != nil {
						a.flashErr(i18n.T("Kopiert, aber das Original ließ sich nicht löschen: ") + err.Error())
						break
					}
				}
			}
			f.xfer.what = strings.Replace(f.xfer.what, "↑", "»↑", 1)
			f.xfer.what = strings.Replace(f.xfer.what, "↓", "»↓", 1)
		}
		return cmd
	}
	var moved []string
	for i, from := range c.paths {
		name := baseName(c.fs, from)
		to := dst.fs.Join(dst.path, name)
		if from == to {
			continue
		}
		if dst.path == from || strings.HasPrefix(dst.path, from+"/") || strings.HasPrefix(dst.path, from+`\`) {
			a.flashErr(fmt.Sprintf(i18n.T("„%s“ lässt sich nicht in sich selbst verschieben"), name))
			break
		}
		if err := dst.fs.Rename(from, to); err != nil {
			rest := c.paths[i:]
			dir := dst.path
			if !a.offerSudo(s, dst.fs, i18n.T("Verschieben nach ")+dir+":\n"+sudoListNames(rest), err, func() tea.Cmd {
				return a.runSudo(s, "mv "+strings.Join(rest, " ")+" "+dir, func(ctx context.Context, sx *sshx.Sudo) error {
					for _, r := range rest {
						if err := sx.Rename(ctx, r, dst.fs.Join(dir, baseName(dst.fs, r))); err != nil {
							return err
						}
					}
					return nil
				}, func(error) tea.Cmd { f.cut = nil; src.load(f.hidden); dst.load(f.hidden); return nil })
			}) {
				a.flashErr(i18n.T("Verschieben ging nicht: ") + err.Error())
			}
			break
		}
		moved = append(moved, name)
	}
	if len(moved) == len(c.paths) || len(moved) > 0 {
		f.cut = nil
	}
	src.load(f.hidden)
	dst.load(f.hidden)
	if len(moved) > 0 {
		dst.selectName(moved[0])
		what := moved[0]
		if len(moved) > 1 {
			what = fmt.Sprintf(i18n.T("%d Einträge"), len(moved))
		}
		a.flashOK(fmt.Sprintf(i18n.T("%s verschoben nach %s"), what, dst.path))
	}
	return nil
}
