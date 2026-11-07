package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/trzsz"
)

// Dateien im Terminal (trzsz)
//
// Startet man auf dem Server `tsz datei` (herunterladen) oder `trz`
// (hochladen), erkennt sshTermy den Auslöser im Datenstrom und überträgt
// die Datei durch dieselbe Shell, auch nach `sudo su`, über mehrere Sprünge
// oder auf Geräten ohne SFTP. Heruntergeladenes landet in Downloads, beim
// Hochladen fragt sshTermy nach dem Pfad (Ziehen aus dem Explorer geht
// auch). Strg+C bricht ab. ZMODEM (`sz`/`rz`) wird erkannt und mit einem
// Hinweis abgebrochen, statt dass die Shell hängen bleibt.

// trzRun: laufende Übertragung einer Sitzung.
type trzRun struct {
	t    *trzsz.Transfer
	mode byte

	mu          sync.Mutex
	name        string
	done, total int64
}

func (r *trzRun) progress(name string, done, total int64) {
	r.mu.Lock()
	r.name, r.done, r.total = name, done, total
	r.mu.Unlock()
}

// label für die Statuszeile: „↓ datei.txt 45 %“.
func (r *trzRun) label() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	arrow := "↑"
	if r.mode == 'S' {
		arrow = "↓"
	}
	if r.name == "" {
		return arrow + " trzsz …"
	}
	pct := 100
	if r.total > 0 {
		pct = int(r.done * 100 / r.total)
	}
	return fmt.Sprintf("%s %s %d %%", arrow, trunc(r.name, 24), pct)
}

type trzStartMsg struct {
	sid  int
	mode byte
}
type trzDoneMsg struct {
	sid   int
	mode  byte
	paths []string
	err   error
}
type zmodemMsg struct{ sid int }

// zmodemStart: so beginnen sz (ZRQINIT) und rz (ZRINIT).
var zmodemStart = [][]byte{[]byte("**\x18B00"), []byte("**\x18B01")}

// trzScan läuft im Ausgabestrom (outWriter): Auslöser gefunden? Dann geht
// alles danach an die Übertragung, und Write liefert, was das Terminal noch
// zeigen soll.
func (s *session) trzScan(p []byte, ev chan<- tea.Msg) []byte {
	if r := s.trz.Load(); r != nil {
		r.t.Feed(p)
		return nil
	}
	tailHint := len(s.trzTail) > 0 && bytes.Contains(s.trzTail[max(0, len(s.trzTail)-40):], []byte("::"))
	if !tailHint && !bytes.Contains(p, []byte("TRZSZ")) && !bytes.Contains(p, []byte("\x18B0")) {
		s.trzTail = tailOf(s.trzTail, p)
		return p
	}
	joined := append(append([]byte(nil), s.trzTail...), p...)
	s.trzTail = tailOf(s.trzTail, p)
	if tr, ok := trzsz.Detect(joined); ok && (tr.ID == "" || tr.ID != s.trzLastID) {
		s.trzLastID = tr.ID
		r := &trzRun{mode: tr.Mode, t: trzsz.New(shellWriter{s})}
		r.t.Progress = r.progress
		s.trz.Store(r)
		s.trzTail = nil
		// Terminal bekommt nur, was vor dem Auslöser stand
		start := tr.Start - (len(joined) - len(p))
		if start < 0 {
			start = 0
		}
		select {
		case ev <- trzStartMsg{s.id, tr.Mode}:
		default:
			go func() { ev <- trzStartMsg{s.id, tr.Mode} }()
		}
		return p[:start]
	}
	for _, z := range zmodemStart {
		if bytes.Contains(p, z) {
			go func() { ev <- zmodemMsg{s.id} }()
			break
		}
	}
	return p
}

func tailOf(old, p []byte) []byte {
	const n = 64
	if len(p) >= n {
		return append(old[:0], p[len(p)-n:]...)
	}
	old = append(old, p...)
	if len(old) > n {
		old = old[len(old)-n:]
	}
	return old
}

// trzMsg: Nachrichten der Übertragung.
func (a *App) trzMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case trzStartMsg:
		s := a.byID(m.sid)
		if s == nil {
			return a.listen(), true
		}
		r := s.trz.Load()
		if r == nil {
			return a.listen(), true
		}
		switch m.mode {
		case 'S':
			dir := exportDir()
			a.flash(i18n.T("Datei vom Server wird übertragen (tsz) nach ") + dir)
			return tea.Batch(a.listen(), a.trzRunCmd(s, r, func() ([]string, error) { return r.t.Download(dir) })), true
		case 'D':
			a.flashErr(i18n.T("Ordner per trz -d geht nicht; Dateien einzeln mit trz oder Ordner mit F5 (SFTP)"))
			return tea.Batch(a.listen(), a.trzRunCmd(s, r, func() ([]string, error) { return nil, r.t.Decline() })), true
		}
		// hochladen: Pfad abfragen
		md := inputModal(i18n.T("Datei hochladen (trz)"), i18n.T("Pfad der Datei auf diesem Rechner; mehrere mit ; trennen. Esc bricht ab."),
			exportDir()+string(filepath.Separator), func(v string) tea.Cmd {
				paths := splitPaths(v)
				if len(paths) == 0 {
					return a.trzRunCmd(s, r, func() ([]string, error) { return nil, r.t.Decline() })
				}
				return a.trzRunCmd(s, r, func() ([]string, error) { return r.t.Upload(paths) })
			})
		md.in.Placeholder = i18n.T("Pfad")
		md.onCancel = func() tea.Cmd {
			return a.trzRunCmd(s, r, func() ([]string, error) { return nil, r.t.Decline() })
		}
		a.modal = md
		return tea.Batch(a.listen(), md.in.Focus()), true
	case trzDoneMsg:
		s := a.byID(m.sid)
		if s == nil {
			return nil, true
		}
		if r := s.trz.Swap(nil); r != nil {
			if rest := r.t.Rest(); len(rest) > 0 && s.term != nil {
				_, _ = s.term.Write(rest)
			}
		}
		switch {
		case m.err != nil && m.err == trzsz.ErrCanceled:
			a.flash(i18n.T("Übertragung abgebrochen"))
		case m.err != nil:
			a.flashErr(i18n.T("Übertragung fehlgeschlagen: ") + m.err.Error())
		case m.mode == 'S' && len(m.paths) > 0:
			a.flashOK(i18n.T("Heruntergeladen: ") + strings.Join(m.paths, ", "))
		case m.mode == 'R' && len(m.paths) > 0:
			a.flashOK(i18n.T("Hochgeladen: ") + strings.Join(m.paths, ", "))
		}
		return nil, true
	case zmodemMsg:
		if s := a.byID(m.sid); s != nil && s.shell != nil {
			// Abbruch: fünfmal CAN, dann Backspaces (so bricht lrzsz ab)
			_, _ = s.shell.Write([]byte("\x18\x18\x18\x18\x18\x08\x08\x08\x08\x08"))
			a.flashErr(i18n.T("ZMODEM (sz/rz) kann sshTermy nicht; auf dem Server trzsz nutzen (tsz/trz) oder F5 (SFTP)"))
		}
		return a.listen(), true
	}
	return nil, false
}

func (a *App) trzRunCmd(s *session, r *trzRun, run func() ([]string, error)) tea.Cmd {
	sid, mode := s.id, r.mode
	return func() tea.Msg {
		paths, err := run()
		return trzDoneMsg{sid, mode, paths, err}
	}
}

// trzKey: Tasten während der Übertragung (nur Abbruch).
func (s *session) trzKey(key string) bool {
	r := s.trz.Load()
	if r == nil {
		return false
	}
	if key == "ctrl+c" || key == "esc" {
		r.t.Cancel()
	}
	return true
}

// splitPaths: „a.txt; ~/b.txt“ → einzelne Pfade (~ aufgelöst, Anführungszeichen weg).
func splitPaths(v string) []string {
	var out []string
	home, _ := os.UserHomeDir()
	for _, p := range strings.Split(v, ";") {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p == "" {
			continue
		}
		if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
			p = filepath.Join(home, p[1:])
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() && strings.HasSuffix(p, string(filepath.Separator)) {
			continue // nur der vorbelegte Ordner, keine Datei gewählt
		}
		out = append(out, p)
	}
	return out
}
