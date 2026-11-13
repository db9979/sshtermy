package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/recording"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

// Aufzeichnung exportieren
//
//   - Live-Aufnahme (Palette „Aufnahme starten“): alles, was der Server
//     schickt, mit Zeitstempeln als asciinema-Datei (.cast). Farben, vim, htop
//     bleiben erhalten. Tastendrücke werden nicht aufgenommen. Beim Stoppen
//     (oder Schließen des Tabs) landet die Datei im Download-Ordner.
//   - Sitzungsprotokoll (F10 → Protokolle, x): als Markdown, als schlichter
//     Text oder als nachgebaute Aufnahme (.cast, Befehle wie getippt).
//   - Terminal-Verlauf (Palette): der sichtbare Verlauf des Tabs als Text.

type recorder = recording.Recorder

// recording: läuft gerade eine Aufnahme?
func (s *session) recording() bool { return s.rec.Load() != nil }

func (a *App) toggleRecording(s *session) tea.Cmd {
	if s.recording() {
		a.stopRecording(s)
		return nil
	}
	if s.term == nil {
		return nil
	}
	w, h := s.term.Size()
	r := recording.New(w, h, s.conn.Alias+" · "+time.Now().Format("02.01.2006 15:04"))
	// aktuellen Bildschirm als Startbild, damit die Aufnahme nicht leer beginnt
	if scr := s.term.PlainLines(h); strings.TrimSpace(scr) != "" {
		r.Output([]byte("\x1b[2J\x1b[H" + strings.ReplaceAll(scr, "\n", "\r\n")))
	}
	s.rec.Store(r)
	s.recStart = time.Now()
	a.flash(i18n.T("● Aufnahme läuft. Stoppen über die Palette („Aufnahme stoppen“) oder Tab schließen."))
	return nil
}

// stopRecording speichert die Aufnahme als .cast im Download-Ordner.
func (a *App) stopRecording(s *session) {
	r := s.rec.Swap(nil)
	if r == nil {
		return
	}
	p := exportPath(s.conn.Alias, s.recStart, ".cast")
	if err := os.WriteFile(p, r.Cast(), 0o600); err != nil {
		a.flashErr(i18n.T("Aufnahme nicht gespeichert: ") + err.Error())
		return
	}
	msg := fmt.Sprintf(i18n.T("Aufnahme gespeichert (%s): %s · abspielen mit „asciinema play“"), bcDur(r.Duration().Round(time.Second)), p)
	if r.Truncated() {
		msg += i18n.T(" · gekürzt (Größengrenze)")
	}
	a.flash(msg)
}

// resizeRecording: neue Terminalgröße in die Aufnahme.
func (s *session) resizeRecording(w, h int) {
	if r := s.rec.Load(); r != nil {
		r.Resize(w, h)
	}
}

func recordLabel(on bool) string {
	if on {
		return i18n.T("Aufnahme stoppen und speichern (.cast)")
	}
	return i18n.T("Aufnahme starten (abspielbar mit asciinema)")
}

// exportPath: Dateiname im Download-Ordner, z. B. sshtermy-web1-20261106-0800.cast.
func exportPath(host string, at time.Time, ext string) string {
	name := fmt.Sprintf("sshtermy-%s-%s%s", strings.Trim(unsafeName.ReplaceAllString(host, "_"), "_"), at.Format("20060102-1504"), ext)
	return filepath.Join(exportDir(), name)
}

// exportScrollback: Verlauf des Tabs als Text. Läuft tmux, liegt der
// Verlauf dort: dann holt sshTermy ihn mit capture-pane vom Server.
func (a *App) exportScrollback(s *session) tea.Cmd {
	if s.term == nil {
		return nil
	}
	n := s.term.Lines()
	lines := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lines = append(lines, strings.TrimRight(s.term.PlainLine(i), " "))
	}
	text := strings.Join(lines, "\n")
	alias := s.conn.Alias
	if s.inTmux() {
		c := s.client
		t, _ := tmuxTarget(s.mux.name)
		a.flash(i18n.T("Hole den Verlauf aus tmux …"))
		return func() tea.Msg {
			out, err := c.Output("tmux capture-pane -p -J -S -"+t, 15*time.Second)
			return relayMsg{apply: func(a *App) tea.Cmd {
				if err == nil && strings.TrimSpace(out) != "" {
					text = out
				}
				a.writeScrollback(alias, text)
				return nil
			}}
		}
	}
	a.writeScrollback(alias, text)
	return nil
}

func (a *App) writeScrollback(alias, text string) {
	text = strings.TrimRight(text, "\n") + "\n"
	p := exportPath(alias, time.Now(), ".txt")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		a.flashErr(i18n.T("Export: ") + err.Error())
		return
	}
	a.flash(i18n.T("Verlauf exportiert: ") + p)
}

// exportMenu: Protokoll exportieren (Logs-Ansicht, x).
func (a *App) exportMenu(l *sessionlog.Log) *modal {
	write := func(ext string, data []byte) tea.Cmd {
		p := exportPath(l.Host, l.Started, ext)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			a.flashErr(i18n.T("Export: ") + err.Error())
		} else {
			a.flash(i18n.T("Exportiert: ") + p)
		}
		return nil
	}
	items := []menuItem{
		{label: i18n.T("Markdown für Doku oder Wiki"), hint: ".md", run: func() tea.Cmd { return write(".md", []byte(l.Markdown())) }},
		{label: i18n.T("Text, wie im Terminal"), hint: ".txt", run: func() tea.Cmd { return write(".txt", []byte(l.Text())) }},
		{label: i18n.T("Abspielbare Aufnahme (asciinema)"), hint: ".cast", run: func() tea.Cmd { return write(".cast", l.Cast(2*time.Second)) }},
	}
	return &modal{kind: mMenu, title: i18n.T("Exportieren nach ") + exportDir(), items: items}
}
