package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/changelog"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/update"
	"github.com/db9979/sshtermy/internal/version"
)

// „Was ist neu“: der eingebaute Changelog (F12 → w, Palette) und nach einem
// Update einmalig automatisch die Neuerungen seit der zuletzt gestarteten
// Version. Dazu: sauberer Neustart nach einem installierten Update.

// changelogModal zeigt die übergebenen Einträge; nil = alle.
func (a *App) changelogModal(entries []changelog.Entry) *modal {
	title := "Was ist neu in " + version.Name
	if entries == nil {
		entries = changelog.All()
	} else {
		title = "Neu in v" + version.Version
	}
	w := min(86, max(30, a.w-8)) // Textbreite im Modal (Rahmen + Abstand)
	s := a.s
	var lines []string
	for i, e := range entries {
		if i > 0 {
			lines = append(lines, "")
		}
		head := s.Accent.Render("v" + e.Version)
		if e.Version == version.Version {
			head += " " + s.Badge.Render("installiert")
		}
		if e.Date != "" {
			head += s.Muted.Render("  " + e.Date)
		}
		lines = append(lines, head)
		for _, it := range e.Items {
			wrapped := lipgloss.NewStyle().Width(w - 2).Render(it)
			for j, l := range strings.Split(wrapped, "\n") {
				pre := "• "
				if j > 0 {
					pre = "  "
				}
				lines = append(lines, s.Muted.Render(pre)+s.Text.Render(l))
			}
		}
	}
	if len(lines) == 0 {
		lines = []string{s.Muted.Render("Für diesen Build gibt es keinen Changelog.")}
	}
	// Nach einem Update (und im Changelog) dezent auf die Kaffee-Seite hinweisen.
	lines = append(lines, "", s.Muted.Render("☕ Gefällt dir sshTermy? b öffnet ")+s.Accent.Render(version.Coffee))
	return &modal{kind: mChangelog, title: title, lines: lines}
}

// changelogMaxBody: sichtbare Zeilen (Fensterrand 2, Rahmen+Abstand 4, Titel 2, Fuß 2).
func changelogMaxBody(h int) int { return max(3, h-2-4-2-2) }

func (m *modal) changelogView(s Styles, w, h int) string {
	maxBody := changelogMaxBody(h)
	lines := m.lines
	hint := ""
	if total := len(lines); total > maxBody {
		m.scroll = max(0, min(m.scroll, total-maxBody))
		lines = lines[m.scroll : m.scroll+maxBody]
		hint = "↑↓ blättern"
		if m.scroll+maxBody < total {
			hint = "▾ mehr · " + hint
		}
	} else {
		m.scroll = 0
	}
	out := s.Accent.Render("★ "+m.title) + "\n\n" + strings.Join(lines, "\n")
	if hint != "" {
		out += "\n\n" + s.Muted.Render(hint)
	}
	return s.Modal.Width(min(90, w-4)).Render(out)
}

func (m *modal) changelogKey(key string, h int) (closeIt bool) {
	page := max(1, changelogMaxBody(h)-1)
	switch key {
	case "up", "k":
		m.scroll = max(0, m.scroll-1)
	case "down", "j":
		m.scroll++
	case "pgup":
		m.scroll = max(0, m.scroll-page)
	case "pgdown", "space":
		m.scroll += page
	case "home", "g":
		m.scroll = 0
	case "esc", "q", "enter", "w":
		return true
	}
	return false
}

// openCoffee öffnet die Buy-Me-a-Coffee-Seite; in Tests austauschbar.
var openCoffee = func() { _ = sshx.OpenBrowser(version.Coffee) }

// showWhatsNew: nach dem Entsperren einmal je neuer Version die Neuerungen
// zeigen. Bei der Ersteinrichtung nicht (dann ist alles neu).
func (a *App) showWhatsNew(firstSetup bool) {
	cur := version.Version
	if cur == "dev" {
		return
	}
	last := a.st.Settings().LastVersion
	if last == cur {
		return
	}
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.LastVersion = cur })
	if firstSetup || (last != "" && !update.Newer(cur, last)) {
		return // frisch eingerichtet oder ältere Version gestartet
	}
	// Dominik 2026-11-05: nach jedem Update einmal die Kaffee-Seite im Browser öffnen.
	openCoffee()
	all := changelog.All()
	entries := changelog.Since(all, last, cur, update.Newer)
	if last == "" && len(entries) > 1 {
		entries = entries[:1] // Update von einer Version ohne „Was ist neu“
	}
	if len(entries) > 0 && a.modal == nil {
		a.modal = a.changelogModal(entries)
	}
}

// openSessions: verbundene oder gerade verbindende Sitzungen.
func (a *App) openSessions() int {
	n := 0
	for _, s := range a.sessions {
		if s.state != stClosed {
			n++
		}
	}
	return n
}

// askRestart: Update ist installiert. Ohne offene Sitzungen sofort neu
// starten, sonst erst nachfragen.
func (a *App) askRestart() tea.Cmd {
	v := a.restartPending
	if n := a.openSessions(); n > 0 {
		a.modal = &modal{kind: mConfirm, title: "Update installiert – jetzt neu starten?",
			body: "v" + v + " ist installiert und wird mit einem Neustart aktiv.\n\n" +
				"⚠ " + plural(n, "offene SSH-Sitzung wird", "offene SSH-Sitzungen werden") + " dabei getrennt\n" +
				"  (laufende Befehle, Tunnel und Dateiübertragungen brechen ab).\n\n" +
				"j = jetzt neu starten · n = später (F12 oder Palette)",
			onYes: func() tea.Cmd { return a.doRestart() }}
		return nil
	}
	return a.doRestart()
}

func (a *App) doRestart() tea.Cmd {
	a.restart = true
	return a.quit()
}

// Restart: soll main nach dem Beenden die neue Version starten?
func (a *App) Restart() bool { return a.restart }
