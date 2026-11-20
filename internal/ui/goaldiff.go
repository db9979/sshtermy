package ui

// Änderungsvorschau: Ziel- und Agent-Modus sichern jede Datei, bevor ein
// Schritt sie ändert (goalux.go). d zeigt für jede dieser Dateien, was sich
// seitdem geändert hat, als Diff (vorher → jetzt), mit +/− je Datei. Gelesen
// wird der jetzige Stand über SFTP; ist eine Datei nur mit sudo lesbar,
// versucht sshTermy `sudo -n cat` (ohne Passwort, nur lesend). Danach holt u
// wie gewohnt alles zurück.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// fileNow: jetziger Stand einer gesicherten Datei.
type fileNow struct {
	Path    string
	Exists  bool
	Content []byte
	Err     string
}

type goalDiffMsg struct {
	sid  int
	list []fileBackup
	now  []fileNow
	err  error
}

// fileDiffStat: +/− Zeilen zwischen Sicherung und jetzt.
func fileDiffStat(b fileBackup, n fileNow) (lines []hostinfo.DiffLine, add, del int) {
	split := func(x []byte) []string {
		t := strings.ReplaceAll(string(x), "\r\n", "\n")
		t = strings.TrimSuffix(t, "\n")
		if t == "" {
			return nil
		}
		return strings.Split(t, "\n")
	}
	var before, after []string
	if b.Existed {
		before = split(b.Content)
	}
	if n.Exists {
		after = split(n.Content)
	}
	return hostinfo.Diff(before, after, 3)
}

// showGoalDiff liest den jetzigen Stand und zeigt den Diff.
func (a *App) showGoalDiff(s *session) tea.Cmd {
	var list []fileBackup
	for _, b := range s.goal.backups {
		if b.restorable() {
			list = append(list, b)
		}
	}
	if len(list) == 0 {
		a.flash(i18n.T("Keine gesicherten Änderungen zum Vergleichen"))
		return nil
	}
	if s.state != stConnected || !a.agentFilesOK(s) {
		a.flashErr(i18n.T("Vergleichen braucht eine Verbindung mit SFTP"))
		return nil
	}
	a.flash(i18n.T("Lese den jetzigen Stand der Dateien …"))
	sid := s.id
	local, cl := s.agentFSOf()
	return func() tea.Msg {
		fsys, closeFn, err := agentFS(local, cl)
		if err != nil {
			return goalDiffMsg{sid: sid, list: list, err: err}
		}
		defer closeFn()
		now := make([]fileNow, len(list))
		for i, b := range list {
			now[i] = readNow(fsys, cl, b)
		}
		return goalDiffMsg{sid: sid, list: list, now: now}
	}
}

func readNow(fsys sshx.FS, cl *sshx.Client, b fileBackup) fileNow {
	n := fileNow{Path: b.Path}
	p := b.Path
	if strings.HasPrefix(p, "~/") {
		p = fsys.Join(fsys.Home(), p[2:])
	}
	e, err := fsys.Stat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return n // gelöscht
	case err == nil && e.IsDir:
		n.Err = i18n.T("ist jetzt ein Ordner")
		return n
	case err == nil && e.Size > maxBackupSize:
		n.Err = i18n.T("zu groß")
		return n
	}
	if err == nil {
		if r, err := fsys.Open(p); err == nil {
			data, err := io.ReadAll(io.LimitReader(r, maxBackupSize+1))
			_ = r.Close()
			if err == nil && len(data) <= maxBackupSize {
				n.Exists, n.Content = true, data
				return n
			}
		}
	}
	// nur mit sudo lesbar: ohne Passwort versuchen, sonst ehrlich sagen
	if cl != nil && b.Sudo {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := cl.Exec(ctx, "sudo -n cat -- "+shQuote(p))
		if err == nil && res.Exit == 0 && len(res.Output) <= maxBackupSize {
			n.Exists, n.Content = true, res.Output
			return n
		}
	}
	n.Err = i18n.T("nicht lesbar")
	return n
}

func (a *App) goalDiffDone(m goalDiffMsg) {
	s := a.byID(m.sid)
	if s == nil {
		return
	}
	if m.err != nil {
		a.flashErr(i18n.T("Vergleichen: ") + m.err.Error())
		return
	}
	title, lines := a.goalDiffText(s, m.list, m.now)
	a.modal = textModal(title, lines)
}

// goalDiffText: Kopf mit Summen, dann je Datei ein Abschnitt.
func (a *App) goalDiffText(s *session, list []fileBackup, now []fileNow) (string, []string) {
	st := a.s
	var out []string
	sumAdd, sumDel, changed := 0, 0, 0
	for i, b := range list {
		n := now[i]
		head := st.Accent.Render("▸ " + b.Path)
		if n.Err != "" {
			out = append(out, head+"  "+st.Warn.Render(n.Err), "")
			continue
		}
		lines, add, del := fileDiffStat(b, n)
		switch {
		case !b.Existed && n.Exists:
			head += "  " + st.OK.Render(i18n.T("neu angelegt"))
		case b.Existed && !n.Exists:
			head += "  " + st.Danger.Render(i18n.T("gelöscht"))
		}
		if add+del == 0 && b.Existed == n.Exists {
			out = append(out, head+"  "+st.Muted.Render(i18n.T("unverändert")), "")
			continue
		}
		changed++
		sumAdd += add
		sumDel += del
		out = append(out, head+"  "+st.OK.Render(fmt.Sprintf("+%d", add))+" "+st.Danger.Render(fmt.Sprintf("−%d", del)))
		for j, l := range lines {
			if j == 400 {
				out = append(out, st.Muted.Render(fmt.Sprintf(i18n.T("  … %d weitere Zeilen"), len(lines)-400)))
				break
			}
			switch l.Op {
			case hostinfo.DiffHunk:
				out = append(out, st.AI.Render(l.Text))
			case hostinfo.DiffAdd:
				out = append(out, st.OK.Render("+ "+l.Text))
			case hostinfo.DiffDel:
				out = append(out, st.Danger.Render("- "+l.Text))
			default:
				out = append(out, st.Muted.Render("  "+l.Text))
			}
		}
		out = append(out, "")
	}
	out = append(out, st.Muted.Render(i18n.T("u im Panel holt alle Dateien zurück (so wie vor dem Ziel).")))
	title := fmt.Sprintf(i18n.T("Änderungen auf %s · %s  "), s.conn.Alias, plural(changed, i18n.T("Datei"), i18n.T("Dateien"))) +
		st.OK.Render(fmt.Sprintf("+%d", sumAdd)) + " " + st.Danger.Render(fmt.Sprintf("−%d", sumDel))
	return title, out
}
