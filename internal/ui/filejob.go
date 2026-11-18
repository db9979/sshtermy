package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Hintergrund-Arbeiten im Dateien-Fenster (F5): Packen, Entpacken, Datei
// finden, Ordnergrößen, doppelte Dateien. Auf diesem Rechner erledigt das
// Go selbst, auf dem Server ein Befehl über dieselbe Verbindung (Exec, für
// das Terminal unsichtbar). Unten steht solange „⠿ …“, x bricht ab.

type fileJob struct {
	what   string
	cancel context.CancelFunc
	start  time.Time
}

type fileJobMsg struct {
	sid   int
	job   *fileJob
	err   error
	after func() tea.Cmd // im Update-Takt, nach Erfolg
}

// startFileJob lässt run im Hintergrund laufen; after kommt danach im
// Update-Takt (auch nach Fehlern nicht). Neu laden der Seiten macht das
// Ende selbst.
func (a *App) startFileJob(s *session, what string, limit time.Duration, run func(ctx context.Context) (func() tea.Cmd, error)) tea.Cmd {
	f := &s.files
	if f.job != nil {
		a.flash(fmt.Sprintf(i18n.T("Es läuft noch: %s (x bricht ab)"), f.job.what))
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	j := &fileJob{what: what, cancel: cancel, start: time.Now()}
	f.job, f.last = j, ""
	sid := s.id
	return func() tea.Msg {
		defer cancel()
		after, err := run(ctx)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && err != nil {
			err = errors.New(i18n.T("dauerte zu lange, abgebrochen"))
		}
		return fileJobMsg{sid: sid, job: j, err: err, after: after}
	}
}

func (a *App) fileJobDone(m fileJobMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil {
		return nil
	}
	f := &s.files
	if f.job == m.job {
		f.job = nil
	}
	switch {
	case errors.Is(m.err, context.Canceled):
		f.last = a.s.Warn.Render(i18n.T("Abgebrochen: ") + m.job.what)
	case m.err != nil:
		f.last = a.s.Danger.Render("× " + m.job.what + ": " + m.err.Error())
		if !f.open {
			a.flashErr(m.job.what + ": " + m.err.Error())
		}
	}
	for _, p := range f.panes {
		if p != nil {
			sel := ""
			if p.sel >= 0 && p.sel < len(p.entries) {
				sel = p.entries[p.sel].Name
			}
			p.load(f.hidden)
			p.selectName(sel)
		}
	}
	if m.err == nil && m.after != nil {
		return m.after()
	}
	return nil
}

// isLocal: Seite auf diesem Rechner (Go erledigt die Arbeit selbst).
func isLocal(fsys sshx.FS) bool {
	_, ok := fsys.(sshx.LocalFS)
	return ok
}

// remoteRun führt cmd auf dem Server aus. Exit ≠ 0 ist ein Fehler mit den
// letzten Zeilen der Ausgabe, außer okExit erlaubt ihn (z. B. du/find mit
// einzelnen „Permission denied“).
func remoteRun(ctx context.Context, c *sshx.Client, cmd string, okExit bool) (string, error) {
	if c == nil {
		return "", errors.New(i18n.T("keine Verbindung"))
	}
	if c.NetDevice {
		return "", errors.New(i18n.T("keine Exec-Befehle auf Netzwerkgeräten"))
	}
	r, err := c.Exec(ctx, cmd)
	out := string(r.Output)
	if err != nil {
		return out, err
	}
	if r.Exit != 0 && !okExit {
		msg := tailText(out, 2)
		if msg == "" {
			msg = fmt.Sprintf(i18n.T("Befehl endete mit %d"), r.Exit)
		}
		return out, errors.New(msg)
	}
	return out, nil
}

// tailText: die letzten n nicht leeren Zeilen, mit „ · “ verbunden.
func tailText(s string, n int) string {
	var out []string
	ls := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(ls) - 1; i >= 0 && len(out) < n; i-- {
		if l := strings.TrimSpace(ls[i]); l != "" {
			out = append([]string{l}, out...)
		}
	}
	return strings.Join(out, " · ")
}
