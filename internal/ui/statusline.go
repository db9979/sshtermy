package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Statuszeile pro Sitzung (zweite Zeile im Terminal): wer@wo und System
// links, rechts Verbindungsdauer, Latenz, Zeichensatz, Ziel/Tunnel/KI.
// Auf Produktiv-Hosts (Gruppe, Tag oder Alias mit „prod“, Tag „live“) ist
// die Zeile rot, und als root gibt es immer eine deutliche Warnung.

// isProduction: Verbindung gehört zu einem Produktivsystem.
func isProduction(c domain.Connection) bool {
	has := func(s string) bool {
		l := strings.ToLower(s)
		return strings.Contains(l, "prod") || l == "live"
	}
	if has(c.Group) || strings.Contains(strings.ToLower(c.Alias), "prod") {
		return true
	}
	for _, t := range c.Tags {
		if has(t) {
			return true
		}
	}
	return false
}

// rootNow: arbeitet der Nutzer gerade als root? Anmeldename root, oder der
// Prompt endet auf „#“ (sudo -i, su -). Bei Netzwerkgeräten heißt „#“ nur
// Konfigurationsmodus, dort zählt nur der Anmeldename.
func (s *session) rootNow() bool {
	if s.conn.Username == "root" {
		return true
	}
	if s.term == nil || (s.client != nil && s.client.NetDevice) {
		return false
	}
	if s.term.AtIdlePrompt() {
		s.rootPrompt = strings.HasSuffix(strings.TrimRight(s.term.CursorLine(), " "), "#")
	}
	return s.rootPrompt // in vim/htop den letzten Stand behalten
}

func fmtDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	h, m, sec := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func (a *App) sessionBar(s *session) string {
	st := a.s
	prod := isProduction(s.conn)
	bg := st.Header
	if prod {
		bg = lipgloss.NewStyle().Background(st.T.Danger).Foreground(st.T.Bg).Bold(true)
	}

	// links: Warnungen + wer@wo
	var left strings.Builder
	if prod {
		left.WriteString(lipgloss.NewStyle().Background(st.T.Bg).Foreground(st.T.Danger).Bold(true).Padding(0, 1).Render(i18n.T("PRODUKTIV")))
	}
	if col := a.accentOf(s.conn); col != nil && !prod {
		// Akzentfarbe (accent.go): Umgebung als Schild, sonst ein Farbblock
		left.WriteString(lipgloss.NewStyle().Background(col).Foreground(st.T.Bg).Bold(true).Padding(0, 1).Render(envLabel(envKind(s.conn))))
	}
	if s.state == stConnected && s.rootNow() {
		left.WriteString(st.ChipDanger.Bold(true).Render(i18n.T("⚠ root")))
	}
	if s.recording() {
		left.WriteString(st.ChipDanger.Render(i18n.T("● REC")))
	}
	info := s.conn.Username + "@" + s.conn.Addr()
	if s.client != nil {
		info = s.conn.Username + "@" + s.conn.Host + "  ·  " + s.profileSummary()
	}

	// rechts: Leitung
	var line []string
	if s.state == stConnected && s.client != nil {
		line = append(line, st.Badge.Foreground(st.T.Text).Render("⏱ "+fmtDuration(time.Since(s.connectedAt))))
		if l := s.client.Latency(); l > 0 {
			c := st.T.OK
			switch {
			case l >= 250*time.Millisecond:
				c = st.T.Danger
			case l >= 80*time.Millisecond:
				c = st.T.Warn
			}
			ms := l.Milliseconds()
			txt := fmt.Sprintf("↯ %d ms", ms)
			if ms == 0 {
				txt = "↯ <1 ms"
			}
			line = append(line, st.Badge.Foreground(c).Render(txt))
		}
		if enc := s.client.Remote.Encoding(); enc != "" {
			c := st.T.Muted
			if enc != "UTF-8" {
				c = st.T.Warn // Umlaute können falsch ankommen
			}
			line = append(line, st.Badge.Foreground(c).Render(enc))
		}
	}

	nT := 0
	if s.tunnels != nil {
		nT = len(s.tunnels.List())
	}
	cfg := a.st.LLM()
	// KI: Punkt = Zustand des Endpunkts (llmhealth.go), Klick = Erklärung
	h := a.llmHealth()
	ai := st.BadgeAI.Render("✦ " + trunc(cfg.Model, 22))
	if t := h.badgeText(); t != "" {
		ai = st.BadgeAI.Render("✦ " + trunc(cfg.Model, 22) + " · " + t)
	}
	if !cfg.SuggestEnabled {
		ai = st.Badge.Foreground(st.T.Muted).Render(i18n.T("✦ Ghost aus"))
	}
	ai = st.Badge.Foreground(a.llmColor(h.level)).PaddingRight(0).Render(h.dot(a.lh.probing)) + ai
	ai = a.z.click(ai, func(tea.Mouse, bool) tea.Cmd { return a.openLLMStatus() })
	tun := a.keyZone(a.keys.label(actTunnels), st.Badge.Render("⇄ "+plural(nT, i18n.Pick("Tunnel", "tunnel"), i18n.Pick("Tunnel", "tunnels"))))
	goal := ""
	if s.goal.active {
		goal = a.keyZone(a.keys.label(actGoal), st.Badge.Foreground(st.T.AI).Render(i18n.T("◎ Ziel aktiv")))
	}
	if s.scroll > 0 {
		goal = a.z.click(st.Badge.Foreground(st.T.Warn).Render(fmt.Sprintf(i18n.T("⇡ Verlauf −%d · Esc"), s.scroll)),
			func(tea.Mouse, bool) tea.Cmd { s.scroll, s.sel = 0, nil; return nil }) + " " + goal
	}
	if hb := a.hostBadge(s); hb != "" {
		tun = hb + " " + tun
	}
	right := strings.Join(line, "") + " " + goal + " " + tun + " " + ai
	if lw := lipgloss.Width(left.String()); a.w-lw-lipgloss.Width(right) < 24 && len(line) > 0 {
		right = goal + " " + tun + " " + ai // schmales Fenster: Leitung weglassen
	}
	l := left.String()
	room := a.w - lipgloss.Width(l) - lipgloss.Width(right) - 3
	text := " " + trunc(info, max(0, room))
	gap := max(1, a.w-lipgloss.Width(l)-lipgloss.Width(text)-lipgloss.Width(right))
	bar := l + bg.Render(text+strings.Repeat(" ", gap)) + right
	return ansi.Truncate(bar, a.w, "")
}
