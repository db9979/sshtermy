package ui

// Debug-Protokoll für Fehlerberichte (internal/debuglog): Schalter in den
// Einstellungen, --debug für einen Start, bereinigte Kopie zum Weitergeben.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/debuglog"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// ForceDebug: --debug. Schreibt schon ab dem Entsperren, ohne die Einstellung zu ändern.
func (a *App) ForceDebug() {
	a.debugForced = true
	if debuglog.Start(a.st.Dir, true) == nil {
		a.debugHeader()
	}
}

// startDebugLog nach dem Entsperren: Einstellung oder --debug.
func (a *App) startDebugLog() {
	want := a.debugForced || a.st.Settings().Debug
	was := debuglog.On()
	// Start auch bei „aus“: legt den Ordner fest, den der Schalter später braucht.
	if debuglog.Start(a.st.Dir, want) == nil && want && !was {
		a.debugHeader()
	}
}

func (a *App) debugOn() bool { return debuglog.On() }

// debugHeader: womit ein Fehler zu tun haben könnte (Version, System, Terminal).
func (a *App) debugHeader() {
	debuglog.Printf("start", "%s · %s/%s · Go %s", version.Long(), runtime.GOOS, runtime.GOARCH, runtime.Version())
	debuglog.Printf("start", "TERM=%s TERM_PROGRAM=%s COLORTERM=%s WT_SESSION=%t · Fenster %dx%d",
		os.Getenv("TERM"), os.Getenv("TERM_PROGRAM"), os.Getenv("COLORTERM"), os.Getenv("WT_SESSION") != "", a.w, a.h)
	if a.st.V != nil && a.st.Settings().Language != "" {
		set := a.st.Settings()
		debuglog.Printf("start", "Sprache=%s Theme=%s Symbole=%q Maus=%t Reconnect=%t", set.Language, set.Theme, set.Icons, !set.NoMouse, !set.NoAutoReconnect)
	}
}

func (a *App) toggleDebug() tea.Cmd {
	on := !a.debugOn()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Debug = on }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	if err := debuglog.Start(a.st.Dir, on || a.debugForced); err != nil {
		a.flashErr(i18n.T("Debug-Protokoll: ") + err.Error())
		return nil
	}
	if on {
		a.debugHeader()
		a.flash(i18n.T("Debug-Protokoll an: ") + debuglog.Path())
	} else {
		a.flash(i18n.T("Debug-Protokoll aus"))
	}
	return nil
}

// exportDebug: bereinigte Kopie (ohne Hosts, IPs, Benutzer, Geheimnisse) in
// den Download-Ordner, zum Anhängen an ein GitHub-Issue.
func (a *App) exportDebug() tea.Cmd {
	raw, err := debuglog.Read()
	if errors.Is(err, os.ErrNotExist) || (err == nil && raw == "") {
		a.flash(i18n.T("Noch kein Debug-Protokoll. Erst einschalten, dann den Fehler nachstellen."))
		return nil
	}
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	head := fmt.Sprintf("%s · %s/%s · %s\n\n", version.Long(), runtime.GOOS, runtime.GOARCH, time.Now().Format(time.RFC3339))
	p := filepath.Join(exportDir(), "sshtermy-debug-"+time.Now().Format("2006-01-02-1504")+".txt")
	if err := os.WriteFile(p, []byte(crash.Clean(head+raw, a.st.Sensitive())), 0o600); err != nil {
		a.flashErr(i18n.T("Speichern fehlgeschlagen: ") + err.Error())
		return nil
	}
	a.flashOK(i18n.T("Bereinigtes Debug-Protokoll gespeichert: ") + p)
	return nil
}

func (a *App) clearDebug() tea.Cmd {
	if err := debuglog.Clear(); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.flash(i18n.T("Debug-Protokoll geleert"))
	return nil
}

// debugMsg: Ereignisse aus der Oberfläche ins Protokoll (aus App.Update).
func (a *App) debugMsg(msg tea.Msg) {
	if !debuglog.On() {
		return
	}
	who := func(sid int) string {
		if s := a.byID(sid); s != nil {
			p := s.conn.Protocol
			if p == "" {
				p = "ssh"
			}
			return fmt.Sprintf("#%d %s (%s)", sid, s.conn.Alias, p)
		}
		return fmt.Sprintf("#%d", sid)
	}
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		debuglog.Printf("ui", "Fenster %dx%d", m.Width, m.Height)
	case connectedMsg:
		extra := ""
		if m.client != nil {
			extra = fmt.Sprintf(" · Netzgerät=%t · %s", m.client.NetDevice, m.client.Remote.OS)
		}
		debuglog.Printf("conn", "%s verbunden%s %s", who(m.sid), extra, m.note)
	case connectErrMsg:
		debuglog.Printf("conn", "%s Fehler: %v", who(m.sid), m.err)
	case connectNoticeMsg:
		debuglog.Printf("conn", "%s Hinweis: %s", who(m.sid), m.text)
	case shellClosedMsg:
		debuglog.Printf("conn", "%s Sitzung zu (abgerissen=%t): %v", who(m.sid), m.dropped, m.err)
	case updateMsg:
		switch {
		case m.err != nil:
			debuglog.Printf("update", "Prüfung fehlgeschlagen: %v", m.err)
		case m.rel != nil:
			debuglog.Printf("update", "neue Version v%s", m.rel.Version)
		default:
			debuglog.Printf("update", "aktuell")
		}
	case applyDoneMsg:
		debuglog.Printf("update", "installieren: %v", orOK(m.err))
	case syncDoneMsg:
		debuglog.Printf("sync", "fertig rev=%d: %v", m.rev, orOK(m.err))
	case askDoneMsg:
		debuglog.Err("ki", who(m.sid)+" Antwort", m.err)
	case llmTestMsg:
		debuglog.Printf("ki", "Endpunkt-Test: %+v", m.res)
	case filesDoneMsg:
		debuglog.Err("sftp", who(m.sid), m.err)
	case schedDoneMsg:
		debuglog.Printf("plan", "Zeitplan %s: %d Ergebnisse, %v", m.id, len(m.results), orOK(m.err))
	case lockDoneMsg:
		debuglog.Err("lock", "entsperren", m.err)
	}
}

func orOK(err error) any {
	if err == nil {
		return "ok"
	}
	return err
}
