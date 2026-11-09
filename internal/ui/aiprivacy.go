package ui

// KI-Datenschutz: welche KI eine Verbindung bekommen darf, und was zuletzt
// an die KI ging.
//
//   - Einstellung (geht mit dem Sync): alle Anbieter, nur lokale (eigenes
//     Netz) oder KI aus.
//   - Tag an einer Verbindung, z. B. für Kundennetze: „ki-lokal“ oder
//     „ki-aus“ (englisch „local-ai“, „no-ai“). Ein Tag macht strenger, nie
//     lockerer als die Einstellung.
//   - Palette: „Letzte KI-Anfrage ansehen“ zeigt genau den Text, der
//     zuletzt hinausging (nach dem Entfernen von Geheimnissen).

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/sessionlog"
)

// tagPrivacy: Stufe aus den Tags einer Verbindung.
func tagPrivacy(c domain.Connection) string {
	level := llm.PrivacyAll
	for _, t := range c.Tags {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "ki-aus", "keine-ki", "no-ai", "ai-off":
			level = llm.Stricter(level, llm.PrivacyOff)
		case "ki-lokal", "local-ai", "ai-local":
			level = llm.Stricter(level, llm.PrivacyLocal)
		}
	}
	return level
}

// aiLevel: die strengste Stufe aus Einstellung und allen beteiligten Verbindungen.
func (a *App) aiLevel(conns ...domain.Connection) string {
	level := a.st.LLM().Privacy
	for _, c := range conns {
		level = llm.Stricter(level, tagPrivacy(c))
	}
	return level
}

// agentClientFor: KI-Zugang unter Beachtung des Datenschutzes.
func (a *App) agentClientFor(conns ...domain.Connection) (*agent.Agent, error) {
	c, err := llm.NewWithPrivacy(a.st.LLM(), a.aiLevel(conns...))
	if errors.Is(err, llm.ErrAIOff) {
		if a.st.LLM().Privacy == llm.PrivacyOff {
			return nil, errors.New(i18n.T("KI ist ausgeschaltet (Einstellungen › KI › KI-Datenschutz)"))
		}
		return nil, errors.New(i18n.T("KI ist für diese Verbindung aus (Tag ki-aus)"))
	}
	if err != nil {
		return nil, err
	}
	return &agent.Agent{C: c}, nil
}

// logConns: Verbindungen zu Protokollen (für die strengste Stufe).
func (a *App) logConns(list []*sessionlog.Log) []domain.Connection {
	var out []domain.Connection
	for _, l := range list {
		if c, ok := a.st.Connection(l.ConnID); ok {
			out = append(out, c)
		}
	}
	return out
}

func privacyLabel(level string) string {
	switch level {
	case llm.PrivacyLocal:
		return i18n.T("nur eigene KI")
	case llm.PrivacyOff:
		return i18n.T("KI aus")
	}
	return i18n.T("alle Anbieter")
}

func (a *App) cyclePrivacy(dir int) tea.Cmd {
	levels := []string{llm.PrivacyAll, llm.PrivacyLocal, llm.PrivacyOff}
	cur := -1
	for i, l := range levels {
		if l == a.st.LLM().Privacy {
			cur = i
		}
	}
	next := levels[cycleIndex(cur, len(levels), dir)]
	cfg := a.st.LLM()
	cfg.Privacy = next
	if err := a.st.SetLLM(cfg); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	a.flash(i18n.T("KI-Datenschutz: ") + privacyLabel(next))
	return nil
}

// lastRequestModal: was zuletzt an die KI ging.
func (a *App) lastRequestModal() *modal {
	r := llm.LastRequest()
	if r == nil {
		return textModal(i18n.T("Letzte KI-Anfrage"), []string{i18n.T("Seit dem Start ging noch nichts an eine KI.")})
	}
	where := i18n.T("außerhalb des eigenen Netzes")
	switch {
	case llm.IsLocalEndpoint("http://" + r.Host):
		where = i18n.T("im eigenen Netz")
	case llm.IsOwnEndpoint("http://"+r.Host, a.st.LLM().OwnHosts):
		where = i18n.T("eigener KI-Server")
	}
	lines := []string{
		fmt.Sprintf("%s · %s · %s (%s)", r.At.Format("15:04:05"), r.Model, r.Host, where),
		i18n.T("Passwörter, Keys und Tokens werden vor dem Senden entfernt."),
		"",
	}
	for _, m := range r.Messages {
		lines = append(lines, "── "+m.Role+" ──")
		lines = append(lines, strings.Split(strings.TrimRight(m.Content, "\n"), "\n")...)
		lines = append(lines, "")
	}
	return textModal(i18n.T("Letzte KI-Anfrage"), lines)
}
