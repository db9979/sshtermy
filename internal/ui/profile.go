package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sysprof"
)

// Systemprofil pro Verbindung: beim Verbinden aus SSH-Kennung und (nur bei
// Servern) unsichtbarer Abfrage, danach passiv aus dem Terminal. Lehnt das
// System einen Befehl ab („% Invalid input“), merkt sich das Profil das.

// profileConnected legt das frisch erkannte Profil über das gespeicherte.
func (a *App) profileConnected(s *session) {
	var old domain.SystemProfile
	if c, ok := a.st.Connection(s.conn.ID); ok && c.Profile != nil {
		old = *c.Profile
	}
	p, changed := sysprof.Merge(old, s.client.Profile)
	s.prof = p
	if changed {
		a.saveProfile(s)
	}
	// Login-Banner und erster Prompt abwarten, dann einmal mitlesen.
	s.profDue, s.profCmd = time.Now().Add(2500*time.Millisecond), ""
}

func (a *App) saveProfile(s *session) {
	if s.conn.ID == "" {
		return
	}
	_ = a.st.SetProfile(s.conn.ID, s.prof)
}

// profileCommand: Enter wurde gedrückt; nach der Ausgabe nachsehen.
func (a *App) profileCommand(s *session, cmd string) {
	s.profCmd = cmd
	s.profDue = time.Now().Add(900 * time.Millisecond)
}

// profileTick liest nach einer Ruhepause im Terminal mit.
func (a *App) profileTick(s *session) {
	if s.profDue.IsZero() || time.Now().Before(s.profDue) || s.term == nil || s.client == nil {
		return
	}
	if time.Since(s.lastOutput) < 700*time.Millisecond {
		return
	}
	cmd := s.profCmd
	s.profDue, s.profCmd = time.Time{}, ""
	screen := s.term.PlainLines(60)
	if strings.TrimSpace(screen) == "" {
		return
	}
	changed := false
	// Aus dem Bildschirm nur lernen, solange das Profil noch allgemein ist
	// (oder es ein Netzwerkgerät ist): Wer auf einem Linux-Server per ssh auf
	// einen Switch springt, soll das Profil des Servers nicht verbiegen.
	if !s.prof.Manual && (s.client.NetDevice || s.prof.Dialect == "" || s.prof.Dialect == "network") {
		fresh := sysprof.FromScreen(screen)
		var ch bool
		before := s.prof.Dialect
		s.prof, ch = sysprof.Merge(s.prof, fresh)
		if ch {
			changed = true
			if fresh.Dialect != "" && fresh.Dialect != before {
				if d, ok := domain.DialectByID(fresh.Dialect); ok {
					a.flash(s.conn.Alias + ": " + d.Name + " erkannt, die KI schlägt passende Befehle vor")
				}
			}
		}
	}
	if cmd != "" {
		if msg := sysprof.Rejection(sysprof.OutputAfter(screen, cmd)); msg != "" && rejectionFits(s, msg) {
			if s.prof.AddFailed(sysprof.FailedEntry(cmd, msg)) {
				changed = true
				a.flash("„" + trunc(cmd, 40) + "“ abgelehnt – gemerkt, die KI schlägt das hier nicht mehr vor")
			}
		}
	}
	if changed {
		a.saveProfile(s)
	}
}

// rejectionFits: Ablehnungen im Stil einer Geräte-CLI („% Invalid input“)
// zählen nur bei Netzwerkgeräten; auf Servern nur „command not found“.
func rejectionFits(s *session, msg string) bool {
	if s.client.NetDevice || s.prof.Family == domain.FamilyNetwork {
		return true
	}
	l := strings.ToLower(msg)
	return strings.Contains(l, "not found") || strings.Contains(l, "not recognized")
}

// ---------------------------------------------------------------- Befehl anpassen

type adaptMsg struct {
	sid  int
	from string
	sug  agent.Suggestion
	err  error
}

// startAdapt lässt die KI einen Befehl für das System dieser Sitzung umschreiben.
func (a *App) startAdapt(s *session, cmd string) tea.Cmd {
	ag, err := a.agentClient()
	if err != nil {
		a.flash("KI: " + err.Error())
		return nil
	}
	c := a.agentContext(s, true)
	sid := s.id
	a.flash("Schreibe „" + trunc(cmd, 40) + "“ für " + s.conn.Alias + " um …")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		sug, err := ag.Adapt(ctx, cmd, c)
		return adaptMsg{sid, cmd, sug, err}
	}
}

// onAdapt: Ergebnis kommt als Vorschlag (Alt+1), ersetzt aber nichts von
// allein; ausgeführt wird ohnehin erst nach Enter.
func (a *App) onAdapt(m adaptMsg) {
	s := a.byID(m.sid)
	if s == nil {
		return
	}
	switch {
	case m.err != nil:
		a.flash("Umschreiben: " + m.err.Error())
	case m.sug.Command == "":
		a.flash("Auf " + s.conn.Alias + " gibt es dafür keine Entsprechung" + reasonSuffix(m.sug.Reason))
	case m.sug.Command == strings.TrimSpace(m.from):
		a.flash("„" + trunc(m.from, 40) + "“ passt schon für " + s.conn.Alias)
	default:
		if m.sug.Reason == "" {
			m.sug.Reason = "für " + s.conn.Alias + " umgeschrieben"
		}
		s.chips, s.chipArmed = []agent.Suggestion{m.sug}, -1
		a.flash("Umgeschrieben: " + trunc(m.sug.Command, 60) + "  (" + a.keys.label(actChip1) + " fügt ein)")
	}
}

func reasonSuffix(r string) string {
	if r == "" {
		return ""
	}
	return ": " + r
}

// profileSummary für Anzeigen; fällt auf die alte Systemzeile zurück.
func (s *session) profileSummary() string {
	if !s.prof.Empty() {
		if sum := s.prof.Summary(); sum != "unbekannt" {
			return sum
		}
	}
	if s.client != nil {
		return s.client.Remote.String()
	}
	return "unbekannt"
}
