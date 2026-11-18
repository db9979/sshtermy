package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/packx"
)

// Packen und Entpacken (F5): z packt die Auswahl (oder die markierten
// Einträge) in ein Archiv im selben Ordner, u entpackt ein Archiv in einen
// neuen Ordner daneben. Auf dem Server laufen tar/zip/unzip dort, ohne dass
// etwas heruntergeladen wird; hier auf dem Rechner macht es Go selbst.

const packLimit = 30 * time.Minute

// askPack: Namen fürs Archiv abfragen, dann packen.
func (a *App) askPack(s *session, side int, paths []string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || len(paths) == 0 {
		a.flash(i18n.T("Erst einen Eintrag auswählen oder markieren"))
		return nil
	}
	ext := ".tar.gz"
	if isLocal(p.fs) {
		ext = ".zip" // lässt sich auf jedem Rechner mit Bordmitteln öffnen
	}
	name := baseName(p.fs, paths[0])
	if len(paths) > 1 {
		name = baseName(p.fs, p.path)
		if name == "" || name == "/" || name == "." {
			name = "archiv"
		}
	}
	name = uniqueName(p, packx.Stem(name)+ext)
	body := fmt.Sprintf(i18n.T("%d Einträge packen nach (Endung wählt das Format: .zip, .tar.gz, .tar):"), len(paths))
	if len(paths) == 1 {
		body = fmt.Sprintf(i18n.T("„%s“ packen nach (Endung wählt das Format: .zip, .tar.gz, .tar):"), baseName(p.fs, paths[0]))
	}
	a.modal = inputModal(i18n.T("Packen"), body, name, func(v string) tea.Cmd {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		if strings.ContainsAny(v, `/\`) {
			a.flashErr(i18n.T("Der Name darf kein / oder \\ enthalten"))
			return nil
		}
		return a.pack(s, side, paths, v)
	})
	return nil
}

func uniqueName(p *pane, name string) string {
	if _, err := p.fs.Stat(p.fs.Join(p.path, name)); err != nil {
		return name
	}
	stem, ext := packx.Stem(name), name[len(packx.Stem(name)):]
	for i := 2; i < 1000; i++ {
		n := fmt.Sprintf("%s-%d%s", stem, i, ext)
		if _, err := p.fs.Stat(p.fs.Join(p.path, n)); err != nil {
			return n
		}
	}
	return name
}

func (a *App) pack(s *session, side int, paths []string, archive string) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	dir, fsys, c := p.path, p.fs, s.client
	dst := fsys.Join(dir, archive)
	if _, err := fsys.Stat(dst); err == nil {
		a.flashErr(fmt.Sprintf(i18n.T("„%s“ gibt es schon"), archive))
		return nil
	}
	var names []string
	for _, x := range paths {
		if fsys.Dir(x) != dir {
			a.flashErr(i18n.T("Packen geht nur mit Einträgen aus diesem Ordner"))
			return nil
		}
		names = append(names, baseName(fsys, x))
	}
	cmd := ""
	if !isLocal(fsys) {
		var err error
		if cmd, err = packx.PackCommand(dir, archive, names); err != nil {
			a.flashErr(err.Error())
			return nil
		}
	}
	return a.startFileJob(s, i18n.T("Packen: ")+archive, packLimit, func(ctx context.Context) (func() tea.Cmd, error) {
		var err error
		if cmd == "" {
			err = packx.Create(ctx, dst, paths)
		} else {
			_, err = remoteRun(ctx, c, cmd, false)
		}
		if err != nil {
			return nil, err
		}
		return func() tea.Cmd {
			p.load(f.hidden)
			p.selectName(archive)
			p.marked = nil
			size := ""
			if e, err := fsys.Stat(dst); err == nil {
				size = " (" + humanBytes(e.Size) + ")"
			}
			f.last = a.s.OK.Render(fmt.Sprintf(i18n.T("✓ gepackt: %s%s"), archive, size))
			return a.filesPeek(s)
		}, nil
	})
}

// unpack: Archiv an Stelle i in einen neuen Ordner daneben entpacken.
func (a *App) unpack(s *session, side, i int) tea.Cmd {
	f := &s.files
	p := f.panes[side]
	if p == nil || i <= 0 || i >= len(p.entries) || p.entries[i].IsDir || !packx.IsArchive(p.entries[i].Name) {
		a.flash(i18n.T("u entpackt ein Archiv (.zip, .tar.gz, .tgz, .tar …)"))
		return nil
	}
	name := p.entries[i].Name
	fsys, c := p.fs, s.client
	archive := fsys.Join(p.path, name)
	target := uniqueName(p, packx.Stem(name))
	dst := fsys.Join(p.path, target)
	cmd := ""
	if !isLocal(fsys) {
		var err error
		if cmd, err = packx.ExtractCommand(archive, dst); err != nil {
			a.flashErr(err.Error())
			return nil
		}
	}
	return a.startFileJob(s, i18n.T("Entpacken: ")+name, packLimit, func(ctx context.Context) (func() tea.Cmd, error) {
		var err error
		if cmd == "" {
			err = packx.Extract(ctx, archive, dst)
		} else {
			_, err = remoteRun(ctx, c, cmd, false)
		}
		if err != nil {
			return nil, err
		}
		return func() tea.Cmd {
			p.load(f.hidden)
			p.selectName(target)
			f.last = a.s.OK.Render(fmt.Sprintf(i18n.T("✓ entpackt nach %s/"), target))
			return nil
		}, nil
	})
}
