package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Statuszeile je Bereich der geteilten Ansicht: In der Titelzeile jedes
// Bereichs steht rechts, was man sonst erst nach einem Wechsel sieht: der
// Ordner (OSC 7 oder Prompt), der laufende oder letzte Befehl mit Ergebnis
// und Dauer, die Last, wenn F6 sie kennt. Abschaltbar (Settings.NoPaneStatus).

func (a *App) paneStatusOn() bool { return a.st == nil || !a.st.Settings().NoPaneStatus }

func (a *App) togglePaneStatus() tea.Cmd {
	on := !a.paneStatusOn()
	_ = a.st.UpdateSettings(func(x *domain.Settings) { x.NoPaneStatus = !on })
	return nil
}

// paneStatus: kurzer Text für die Titelzeile ("" = nichts zu sagen).
func (a *App) paneStatus(s *session, now time.Time) string {
	if s == nil || !a.paneStatusOn() || s.state != stConnected || s.term == nil {
		return ""
	}
	var parts []string
	if d := shortDir(termDirOf(s)); d != "" {
		parts = append(parts, d)
	}
	switch {
	case s.run != nil:
		parts = append(parts, "⠿ "+trunc(firstWord(s.run.cmd), 14)+" "+fmtDur(now.Sub(s.run.at)))
	case len(s.blocks) > 0:
		b := s.blocks[len(s.blocks)-1]
		mark := "✓"
		if b.failed {
			mark = "×"
			if b.exit != nil {
				mark = fmt.Sprintf("× %d", *b.exit)
			}
		}
		parts = append(parts, mark+" "+fmtDur(b.dur))
	}
	if h := s.host.srv; h != nil && h.HasLoad {
		parts = append(parts, fmt.Sprintf(i18n.T("Last %.2f"), h.Load[0]))
	}
	return strings.Join(parts, " · ")
}

// termDirOf: Ordner des Terminals, wie er gemeldet oder im Prompt steht.
func termDirOf(s *session) string {
	if p := s.term.Cwd(); p != "" {
		return p
	}
	if s.term.AtIdlePrompt() {
		return cwdFromPrompt(s.term.CursorLine())
	}
	return ""
}

// shortDir: höchstens die letzten zwei Teile, „…/“ davor.
func shortDir(p string) string {
	if p == "" {
		return ""
	}
	p = strings.TrimSuffix(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" {
		return "/"
	}
	parts := strings.Split(p, "/")
	if len(parts) > 3 {
		p = "…/" + strings.Join(parts[len(parts)-2:], "/")
	}
	return trunc(p, 28)
}

func firstWord(cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return ""
	}
	if (f[0] == "sudo" || f[0] == "time") && len(f) > 1 {
		return f[0] + " " + f[1]
	}
	return f[0]
}
