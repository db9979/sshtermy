package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// Zustand der Sitzung in der Kopfzeile (rechts neben den Tabs): Warnungen,
// wer@wo und System, Verbindungsdauer, Latenz, Zeichensatz, Ziel/Tunnel/KI.
// Mit „prod“/„live“ in Gruppe, Tag oder Alias (domain.LooksProduction) steht
// vorn eine rote Marke, und als root gibt es immer eine deutliche Warnung.

// isProduction: Verbindung gehört zu einem Produktivsystem.
func isProduction(c domain.Connection) bool { return domain.IsProduction(c) }

// rootNow: arbeitet der Nutzer gerade als root? Anmeldename root, oder der
// Prompt endet auf „#“ (sudo -i, su -). Bei Netzwerkgeräten heißt „#“ nur
// Konfigurationsmodus, dort zählt nur der Anmeldename.
func (s *session) rootNow() bool {
	if s.conn.Username == "root" {
		return true
	}
	if s.term == nil || (s.netDev()) {
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

// prodChip: „▲ PRODUKTIV“ als rote Marke auf dunkelrotem Grund. Früher war
// die ganze Sitzungszeile rot; eine Marke warnt genauso, schreit aber nicht.
func (a *App) prodChip(short bool) string {
	st := a.s
	txt := i18n.T("▲ PRODUKTIV")
	if short {
		txt = i18n.T("▲ PROD")
	}
	return lipgloss.NewStyle().Foreground(st.T.Danger).Background(mixColor(st.T.Surface, st.T.Danger, 0.22)).
		Bold(true).Padding(0, 1).Render(txt)
}

// termHeader: eine Kopfzeile im Terminal. Links Logo und Tabs, rechts der
// Zustand der aktiven Sitzung (Warnungen, wer@wo, Leitung, Ziel, Tunnel, KI).
// Wird es eng, fallen rechts Teile nach Wichtigkeit weg, bevor die Tabs
// gekürzt werden.
func (a *App) termHeader(s *session) string {
	st := a.s
	logo := a.logoChip(">_ " + version.Name)
	lw := lipgloss.Width(logo)
	if s == nil {
		return a.headerRow(a.tabStrip(a.w - lw - 1))
	}
	prod := domain.LooksProduction(s.conn)

	// Warnungen: immer sichtbar
	warn := func(short bool) string {
		var b strings.Builder
		if prod {
			b.WriteString(a.prodChip(short))
		}
		if s.conn.GuardOff {
			b.WriteString(lipgloss.NewStyle().Background(st.T.Warn).Foreground(st.T.Bg).Padding(0, 1).Render(i18n.T("Schutz aus")))
		}
		if col := a.accentOf(s.conn); col != nil && !prod {
			b.WriteString(lipgloss.NewStyle().Background(col).Foreground(st.T.Bg).Bold(true).Padding(0, 1).Render(envLabel(envKind(s.conn))))
		}
		if s.state == stConnected && s.rootNow() {
			b.WriteString(st.ChipDanger.Bold(true).Render(i18n.T("▲ root")))
		}
		if s.recording() {
			b.WriteString(st.ChipDanger.Render(i18n.T("● REC")))
		}
		if r := s.trz.Load(); r != nil {
			b.WriteString(st.Badge.Foreground(st.T.AI).Bold(true).Render(r.label()))
		}
		if s.state == stConnected && s.client == nil && s.conn.Protocol.Raw() {
			b.WriteString(a.protoBadge(s)) // Konsole/Telnet: sagt, wo man tippt
		}
		return b.String()
	}
	info := s.conn.Target()
	if s.client != nil {
		info = s.conn.Username + "@" + s.conn.Host + "  ·  " + s.profileSummary()
	} else if s.state == stConnected && s.conn.Protocol.Raw() {
		info = s.conn.Target() + "  ·  " + s.profileSummary()
	}

	// Leitung
	var line []string
	if s.state == stConnected && s.client != nil {
		line = append(line, st.Badge.Foreground(st.T.Text).Render("◷ "+fmtDuration(time.Since(s.connectedAt))))
		if l := s.client.Latency(); l > 0 {
			c := st.T.OK
			switch {
			case l >= 250*time.Millisecond:
				c = st.T.Danger
			case l >= 80*time.Millisecond:
				c = st.T.Warn
			}
			ms := l.Milliseconds()
			txt := fmt.Sprintf("%d ms", ms)
			if ms == 0 {
				txt = "<1 ms"
			}
			if l >= 250*time.Millisecond {
				txt = i18n.T("▲ zäh · ") + txt
			}
			// Verlauf der letzten Messungen als Mini-Kurve (motion.go)
			sp := ""
			if motionLevel() > 0 && len(s.latHist) >= 3 {
				sp = a.latSpark(s.latHist) + lipgloss.NewStyle().Background(st.T.Surface2).Render(" ")
			}
			line = append(line, st.Badge.Foreground(c).PaddingRight(0).Render("∆ ")+sp+st.Badge.Foreground(c).PaddingLeft(0).Render(txt))
		}
		if enc := s.client.Remote.Encoding(); enc != "" {
			c := st.T.Muted
			if enc != "UTF-8" {
				c = st.T.Warn // Umlaute können falsch ankommen
			}
			line = append(line, st.Badge.Foreground(c).Render(enc))
		}
	}
	if s.state == stConnected && s.client == nil && s.conn.Protocol.Raw() {
		line = append(line, st.Badge.Foreground(st.T.Text).Render("◷ "+fmtDuration(time.Since(s.connectedAt))))
	}

	nT := 0
	if s.tunnels != nil {
		nT = len(s.tunnels.List())
	}
	cfg := a.st.LLM()
	// KI: Punkt = Zustand des Endpunkts (llmhealth.go), Klick = Erklärung
	h := a.llmHealth()
	// KI-Anzeige in drei Breiten: Modell + Antwortzeit, nur Antwortzeit, nur Punkt
	aiBadge := func(full, short bool) string {
		txt := "◆ " + trunc(h.model(), 22)
		if !full {
			txt = "◆"
		}
		if t := h.badgeText(); t != "" && (full || short) {
			txt += " · " + t
		}
		ai := st.BadgeAI.Render(txt)
		if !cfg.SuggestEnabled {
			ai = st.Badge.Foreground(st.T.Muted).Render(i18n.T("◆ Ghost aus"))
			if !full {
				ai = st.Badge.Foreground(st.T.Muted).Render("◆")
			}
		}
		ai = st.Badge.Foreground(a.llmColor(h.level)).PaddingRight(0).Render(h.dot(a.lh.probing)) + ai
		return a.z.click(ai, func(tea.Mouse, bool) tea.Cmd { return a.openLLMStatus() })
	}
	tun := func(short bool) string {
		txt := "↔ " + plural(nT, i18n.Pick("Tunnel", "tunnel"), i18n.Pick("Tunnel", "tunnels"))
		if short {
			txt = fmt.Sprintf("↔ %d", nT)
		}
		return a.keyZone(a.keys.label(actTunnels), st.Badge.Render(txt))
	}
	goal := ""
	if s.goal.active {
		goal = a.keyZone(a.keys.label(actGoal), st.Badge.Foreground(st.T.AI).Render(i18n.T("◎ Ziel aktiv")))
		if s.goal.planning || s.goal.fp != nil {
			goal = a.keyZone(a.keys.label(actGoal), st.Badge.Foreground(st.T.AI).Bold(true).Render(i18n.T("◇ PLAN")))
		} else if s.goal.auto {
			label, fg := i18n.T("◆ AGENT LÄUFT"), st.T.AI
			if s.goal.paused || (s.goal.cur != nil && s.goal.cur.Question != "") {
				label, fg = i18n.T("◆ AGENT WARTET"), st.T.Warn
			}
			goal = a.keyZone(a.keys.label(actGoal), st.Badge.Foreground(fg).Bold(true).Render(label))
		}
	}
	if s.scroll > 0 {
		goal = a.z.click(st.Badge.Foreground(st.T.Warn).Render(fmt.Sprintf(i18n.T("⇡ Verlauf −%d · Esc"), s.scroll)),
			func(tea.Mouse, bool) tea.Cmd { s.scroll, s.sel = 0, nil; return nil }) + " " + goal
	}
	hb := a.hostBadge(s)

	// rechte Hälfte in Stufen: erst wer@wo, dann Leitung, Host, Tunnel und
	// zuletzt die Breite der KI-Anzeige opfern.
	right := func(stage int) string {
		var parts []string
		if w := warn(stage >= 3); w != "" {
			parts = append(parts, w)
		}
		if stage < 1 {
			parts = append(parts, st.Muted.Render(trunc(info, 48)))
		}
		if stage < 2 && len(line) > 0 {
			parts = append(parts, strings.Join(line, ""))
		}
		if goal != "" {
			parts = append(parts, goal)
		}
		if hb != "" && stage < 4 {
			parts = append(parts, hb)
		}
		if stage < 5 || nT > 0 {
			parts = append(parts, tun(stage >= 2))
		}
		parts = append(parts, aiBadge(stage < 3, stage < 6))
		return strings.Join(parts, " ")
	}
	natural := a.tabsNatural()
	var r string
	for stage := 0; stage <= 6; stage++ {
		r = right(stage)
		if lw+1+natural+1+lipgloss.Width(r) <= a.w {
			break
		}
	}
	room := max(0, a.w-lw-1-lipgloss.Width(r)-1)
	tabs := a.tabStrip(room)
	return a.headerRow(tabs + strings.Repeat(" ", max(0, room-lipgloss.Width(tabs))) + " " + r)
}
