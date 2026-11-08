package ui

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/localsh"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Server-Datei im eigenen Editor
//
// Im Dateien-Fenster (F5, Server-Seite) öffnet „e“ die Datei in einem Editor
// auf diesem Rechner (VS Code, Notepad++, TextEdit …), aus der Palette geht
// es auch mit einem Pfad. sshTermy holt die Datei per SFTP in einen privaten
// Ordner und schaut jede Sekunde, ob sie gespeichert wurde. Dann geht sie über
// dieselbe Verbindung zurück auf den Server. Schließt sich der Tab, endet das
// Beobachten; die lokale Kopie bleibt liegen, bis sshTermy beendet wird.

const editMaxSize = 8 << 20 // größere Dateien gehören nicht in einen Editor

type remoteEdit struct {
	sid    int
	host   string
	remote string
	local  string
	mod    time.Time
	sum    [32]byte
	busy   bool
}

type editOpenedMsg struct {
	sid    int
	fs     *sshx.RemoteFS // neu geöffnet (nil = vorhandenes benutzt)
	remote string
	local  string
	sum    [32]byte
	err    error
}

type editSavedMsg struct {
	sid   int
	fs    *sshx.RemoteFS // neu geöffnet (nach Neuverbinden)
	local string
	sum   [32]byte
	err   error
}

// editorCommand: Befehl, mit dem eine Datei geöffnet wird.
func editorCommand(setting, file string) (*exec.Cmd, error) {
	if s := strings.TrimSpace(setting); s != "" {
		args := localsh.SplitArgs(s)
		if len(args) == 0 {
			return nil, fmt.Errorf(i18n.T("Editor-Befehl „%s“ ist nicht lesbar"), s)
		}
		return exec.Command(args[0], append(args[1:], file)...), nil
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("notepad.exe", file), nil
	case "darwin":
		return exec.Command("open", "-t", file), nil
	}
	if e := os.Getenv("VISUAL"); e != "" && !terminalEditor(e) {
		return editorCommand(e, file)
	}
	return exec.Command("xdg-open", file), nil
}

// terminalEditor: vim, nano & Co. laufen im Terminal und würden sshTermy
// überdecken, darum nie automatisch.
func terminalEditor(cmd string) bool {
	base := filepath.Base(strings.Fields(cmd)[0])
	switch base {
	case "vi", "vim", "nvim", "nano", "emacs", "micro", "joe", "mcedit", "ne", "helix", "hx", "kak":
		return true
	}
	return false
}

func (a *App) editorSetting() string { return a.st.Settings().Editor }

// editRemote: Datei p des Servers im Editor öffnen.
func (a *App) editRemote(s *session, p string) tea.Cmd {
	if s == nil || s.client == nil {
		a.flash(i18n.T("Bearbeiten im eigenen Editor geht nur bei SSH-Verbindungen"))
		return nil
	}
	p = strings.TrimSpace(p)
	if p == "" {
		return nil
	}
	for _, e := range a.edits {
		if e.sid == s.id && e.remote == p {
			return a.launchEditor(e.local)
		}
	}
	sid, c := s.id, s.client
	fs := s.editFS
	if fs == nil && s.files.remote != nil {
		fs = s.files.remote
	}
	a.flash(i18n.T("Lade ") + path.Base(p) + i18n.T(" für den Editor …"))
	return func() tea.Msg {
		var opened *sshx.RemoteFS
		if fs == nil {
			r, err := c.OpenSFTP()
			if err != nil {
				return editOpenedMsg{sid: sid, err: err}
			}
			fs, opened = r, r
		}
		if strings.HasPrefix(p, "~/") {
			p = path.Join(fs.Home(), p[2:])
		} else if !strings.HasPrefix(p, "/") {
			p = path.Join(fs.Home(), p)
		}
		local, sum, err := downloadForEdit(fs, p)
		return editOpenedMsg{sid, opened, p, local, sum, err}
	}
}

func downloadForEdit(fs *sshx.RemoteFS, p string) (string, [32]byte, error) {
	var sum [32]byte
	st, err := fs.Stat(p)
	if err != nil {
		return "", sum, err
	}
	if st.IsDir {
		return "", sum, errors.New(i18n.T("Das ist ein Ordner"))
	}
	if st.Size > editMaxSize {
		return "", sum, fmt.Errorf(i18n.T("Datei ist größer als %s"), humanBytes(editMaxSize))
	}
	r, err := fs.Open(p)
	if err != nil {
		return "", sum, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, editMaxSize+1))
	if err != nil {
		return "", sum, err
	}
	dir, err := os.MkdirTemp("", "sshtermy-edit-")
	if err != nil {
		return "", sum, err
	}
	local := filepath.Join(dir, path.Base(p))
	if err := os.WriteFile(local, data, 0o600); err != nil {
		return "", sum, err
	}
	return local, sha256.Sum256(data), nil
}

func (a *App) launchEditor(local string) tea.Cmd {
	cmd, err := editorCommand(a.editorSetting(), local)
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		a.flashErr(i18n.T("Editor startet nicht: ") + err.Error() + i18n.T(" · Palette „Editor für Server-Dateien“"))
		return nil
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (a *App) editMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case editOpenedMsg:
		s := a.byID(m.sid)
		if s == nil {
			if m.fs != nil {
				m.fs.Close()
			}
			return nil, true
		}
		if m.fs != nil {
			s.editFS = m.fs
		}
		if m.err != nil {
			a.flashErr(i18n.T("Öffnen im Editor: ") + m.err.Error())
			return nil, true
		}
		st, _ := os.Stat(m.local)
		e := &remoteEdit{sid: s.id, host: s.tabName(), remote: m.remote, local: m.local, sum: m.sum}
		if st != nil {
			e.mod = st.ModTime()
		}
		a.edits = append(a.edits, e)
		a.flash(i18n.T("Im Editor: ") + path.Base(m.remote) + i18n.T(" · Speichern lädt die Datei zurück auf ") + s.tabName())
		return a.launchEditor(m.local), true
	case editSavedMsg:
		if s := a.byID(m.sid); s != nil && m.fs != nil {
			if s.editFS != nil {
				s.editFS.Close()
			}
			s.editFS = m.fs
		} else if m.fs != nil {
			m.fs.Close()
		}
		for _, e := range a.edits {
			if e.local != m.local {
				continue
			}
			e.busy = false
			if m.err != nil {
				a.flashErr(fmt.Sprintf(i18n.T("%s nicht zurückgeschrieben: %v · Kopie: %s"), path.Base(e.remote), m.err, e.local))
				return nil, true
			}
			e.sum = m.sum
			a.flashOK(i18n.T("Gespeichert auf ") + e.host + ": " + e.remote)
		}
		return nil, true
	}
	return nil, false
}

// editTick: gespeicherte Dateien zurück auf den Server bringen.
func (a *App) editTick() tea.Cmd {
	var cmds []tea.Cmd
	keep := a.edits[:0]
	for _, e := range a.edits {
		s := a.byID(e.sid)
		if s == nil || s.state == stClosed {
			a.flash(i18n.T("Tab geschlossen: Änderungen an ") + path.Base(e.remote) + i18n.T(" gehen nicht mehr zurück"))
			continue
		}
		keep = append(keep, e)
		if e.busy || s.state != stConnected || s.client == nil {
			continue // nach dem Neuverbinden geht es weiter
		}
		st, err := os.Stat(e.local)
		if err != nil || st.ModTime().Equal(e.mod) {
			continue
		}
		e.mod = st.ModTime()
		data, err := os.ReadFile(e.local)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		if sum == e.sum {
			continue // nur angefasst, nicht geändert
		}
		e.busy = true
		fs, c, sid, remote, local := s.editFSFor(), s.client, s.id, e.remote, e.local
		cmds = append(cmds, func() tea.Msg {
			var opened *sshx.RemoteFS
			if fs == nil {
				r, err := c.OpenSFTP()
				if err != nil {
					return editSavedMsg{sid: sid, local: local, sum: sum, err: err}
				}
				fs, opened = r, r
			}
			return editSavedMsg{sid, opened, local, sum, uploadEdited(fs, remote, data)}
		})
	}
	a.edits = keep
	return tea.Batch(cmds...)
}

// uploadEdited überschreibt die Datei an Ort und Stelle (wie vim mit
// backupcopy=yes): Besitzer, Gruppe und Rechte bleiben, auch bei Dateien,
// die einem anderen Benutzer gehören und nur für die Gruppe schreibbar sind.
func uploadEdited(fs *sshx.RemoteFS, remote string, data []byte) error {
	return fs.Rewrite(remote, data)
}

// editFSFor: SFTP-Verbindung für zurückgeschriebene Dateien.
func (s *session) editFSFor() *sshx.RemoteFS {
	if s.editFS != nil {
		return s.editFS
	}
	return s.files.remote
}

// cleanupEdits: lokale Kopien beim Beenden löschen.
func (a *App) cleanupEdits() {
	for _, e := range a.edits {
		_ = os.RemoveAll(filepath.Dir(e.local))
	}
	a.edits = nil
}

// askEditPath: Pfad abfragen (Palette).
func (a *App) askEditPath(s *session) tea.Cmd {
	m := inputModal(i18n.T("Server-Datei im eigenen Editor öffnen"),
		i18n.T("Pfad auf dem Server, z. B. /etc/nginx/nginx.conf oder ~/.bashrc. Speichern im Editor lädt die Datei zurück."), "",
		func(v string) tea.Cmd { return a.editRemote(s, v) })
	m.in.Placeholder = "/etc/hosts"
	a.modal = m
	return m.in.Focus()
}

// askEditor: Editor-Befehl einstellen.
func (a *App) askEditor() tea.Cmd {
	def := map[string]string{"windows": "Notepad", "darwin": "TextEdit (open -t)"}[runtime.GOOS]
	if def == "" {
		def = "xdg-open"
	}
	m := inputModal(i18n.T("Editor für Server-Dateien"),
		i18n.T("Befehl, dem sshTermy den Dateinamen anhängt, z. B. code --new-window, notepad++, subl, gedit. Leer = Standard: ")+def, a.editorSetting(),
		func(v string) tea.Cmd {
			v = strings.TrimSpace(v)
			if v != "" && terminalEditor(v) {
				a.flashErr(i18n.T("vim, nano & Co. laufen im Terminal: dafür die Datei einfach im Terminal öffnen"))
				return nil
			}
			_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Editor = v })
			a.flash(i18n.T("Editor: ") + orDefault(v, def))
			return nil
		})
	m.in.Placeholder = def
	a.modal = m
	return m.in.Focus()
}
