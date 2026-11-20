package ui

// Ghost-Text aus dem eigenen Verlauf: Bevor die KI gefragt wird, sucht
// sshTermy im Verlauf dieses Tabs (neueste zuerst) und in den Befehlen, die
// auf dieser Verbindung schon ohne Fehler liefen (store/cmdstats.go, auch
// von anderen Rechnern per Sync). Passt etwas, steht der Vorschlag nach
// wenigen Millisekunden da, ohne KI, ohne Netz und ohne Tokens. Die KI kommt
// nur noch dran, wenn der Verlauf nichts weiß. Passwortartige Zeilen werden
// nie vorgeschlagen. Abschaltbar (Palette „Ghost-Text aus dem Verlauf“).

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// histGhostDelay: kurz warten, bis der Server das Getippte zurückgeschickt
// hat (die Eingabezeile wird vom Bildschirm gelesen).
const histGhostDelay = 60 * time.Millisecond

type histGhostTickMsg struct{ sid, seq int }

func (a *App) histGhostOn() bool {
	return a.st != nil && !a.st.Settings().NoHistGhost && a.st.LLM().SuggestEnabled
}

func (a *App) toggleHistGhost() tea.Cmd {
	on := !a.histGhostOn()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoHistGhost = !on }); err != nil {
		a.flash(i18n.T("Fehler: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Ghost-Text schlägt zuerst Befehle aus deinem Verlauf vor"))
	} else {
		a.flash(i18n.T("Ghost-Text nur noch von der KI"))
	}
	return nil
}

func (a *App) scheduleHistGhost(s *session) tea.Cmd {
	if !a.histGhostOn() {
		return nil
	}
	sid, seq := s.id, s.ghostSeq
	return tea.Tick(histGhostDelay, func(time.Time) tea.Msg { return histGhostTickMsg{sid, seq} })
}

// fireHistGhost setzt den Vorschlag aus dem Verlauf, falls einer passt.
func (a *App) fireHistGhost(s *session, seq int) {
	if seq != s.ghostSeq || s.term == nil || a.visibleGhost(s) != "" {
		return
	}
	typed, ok := s.term.TypedInput()
	if !ok || len(strings.TrimSpace(typed)) < 2 || agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
		return
	}
	if rest := a.histGhost(s, typed); rest != "" {
		s.ghost, s.ghostFor, s.ghostHist = rest, typed, true
	}
}

// histGhost: Rest des passenden Befehls aus dem Verlauf ("" = nichts).
func (a *App) histGhost(s *session, typed string) string {
	try := func(c string) string {
		c = strings.TrimSpace(c)
		if len(c) <= len(typed) || !strings.HasPrefix(c, typed) || strings.ContainsAny(c, "\n\r") {
			return ""
		}
		if secretish.MatchString(c) || agent.Redact(c) != c {
			return ""
		}
		return c[len(typed):]
	}
	for i := len(s.history) - 1; i >= 0; i-- {
		if r := try(s.history[i]); r != "" {
			return r
		}
	}
	if s.conn.ID == "" || a.st == nil {
		return ""
	}
	for _, u := range a.st.TopCommands(s.conn.ID, 0) {
		if u.Hidden {
			continue
		}
		if r := try(u.Cmd); r != "" {
			return r
		}
	}
	return ""
}
