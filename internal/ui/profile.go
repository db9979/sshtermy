package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/localsh"
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
	var fresh domain.SystemProfile
	if s.client != nil {
		fresh = s.client.Profile
	} else if s.isLocal() {
		sh := s.conn.LocalShell
		if st, ok := s.shell.(*localsh.Stream); ok {
			sh = st.Shell
		}
		// Eigener Rechner: Profil steht fest (Shell kann je Rechner anders sein)
		fresh = localProfile(sh)
		if !old.Manual {
			old.Family, old.OS, old.Dialect, old.Shell = "", "", "", ""
		}
	} else if s.netDev() {
		fresh.Family = domain.FamilyNetwork // Telnet/Konsole: Rest kommt aus dem Bildschirm
	}
	p, changed := sysprof.Merge(old, fresh)
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
	if s.profDue.IsZero() || time.Now().Before(s.profDue) || s.term == nil || (s.client == nil && !s.conn.Protocol.Raw()) {
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
	if !s.prof.Manual && (s.netDev() || s.prof.Dialect == "" || s.prof.Dialect == "network") {
		fresh := sysprof.FromScreen(screen)
		var ch bool
		before := s.prof.Dialect
		s.prof, ch = sysprof.Merge(s.prof, fresh)
		if ch {
			changed = true
			if fresh.Dialect != "" && fresh.Dialect != before {
				if d, ok := domain.DialectByID(fresh.Dialect); ok {
					a.flash(s.conn.Alias + ": " + d.Name + i18n.T(" erkannt, die KI schlägt passende Befehle vor"))
				}
			}
		}
	}
	if cmd != "" {
		if msg := sysprof.Rejection(sysprof.OutputAfter(screen, cmd)); msg != "" && rejectionFits(s, msg) {
			if s.prof.AddFailed(sysprof.FailedEntry(cmd, msg)) {
				changed = true
				a.flash("„" + trunc(cmd, 40) + i18n.T("“ abgelehnt – gemerkt, die KI schlägt das hier nicht mehr vor"))
			}
		}
	}
	if changed {
		a.saveProfile(s)
	}
}

// profileFromSwitch: Die Host-Übersicht hat „show version“ gelesen; was sie
// dabei über Hersteller, Modell und Firmware erfahren hat, kommt ins Profil.
func (a *App) profileFromSwitch(s *session, v hostinfo.Vendor, sw *hostinfo.Switch) {
	dia := map[hostinfo.Vendor]string{hostinfo.VendorProCurve: "procurve", hostinfo.VendorArubaCX: "arubaos-cx", hostinfo.VendorCisco: "cisco-ios"}[v]
	if dia == "" {
		return
	}
	fresh := domain.SystemProfile{Family: domain.FamilyNetwork, OS: v.Name(), Dialect: dia}
	if sw != nil {
		fresh.Model, fresh.Version = sw.Model, sw.Firmware
	}
	var ch bool
	if s.prof, ch = sysprof.Merge(s.prof, fresh); ch {
		a.saveProfile(s)
	}
}

// hostVendor: Familie für die Host-Übersicht aus dem Profil (spart dort
// „show version“ beim ersten Aufruf nicht, aber wählt passende Befehle).
func hostVendor(p domain.SystemProfile) hostinfo.Vendor {
	switch p.Dialect {
	case "procurve":
		return hostinfo.VendorProCurve
	case "arubaos-cx":
		return hostinfo.VendorArubaCX
	case "cisco-ios":
		return hostinfo.VendorCisco
	}
	return hostinfo.VendorUnknown
}

// rejectionFits: Ablehnungen im Stil einer Geräte-CLI („% Invalid input“)
// zählen nur bei Netzwerkgeräten; auf Servern nur „command not found“.
func rejectionFits(s *session, msg string) bool {
	if s.netDev() || s.prof.Family == domain.FamilyNetwork {
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
		a.flashErr("KI: " + err.Error())
		return nil
	}
	c := a.agentContext(s, true)
	sid := s.id
	a.flash(i18n.T("Schreibe „") + trunc(cmd, 40) + i18n.T("“ für ") + s.conn.Alias + i18n.T(" um …"))
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
		a.flashErr(i18n.T("Umschreiben: ") + m.err.Error())
	case m.sug.Command == "":
		a.flash(i18n.T("Auf ") + s.conn.Alias + i18n.T(" gibt es dafür keine Entsprechung") + reasonSuffix(m.sug.Reason))
	case m.sug.Command == strings.TrimSpace(m.from):
		a.flash("„" + trunc(m.from, 40) + i18n.T("“ passt schon für ") + s.conn.Alias)
	default:
		if m.sug.Reason == "" {
			m.sug.Reason = i18n.T("für ") + s.conn.Alias + i18n.T(" umgeschrieben")
		}
		s.chips, s.chipArmed = []agent.Suggestion{m.sug}, -1
		a.flash(i18n.T("Umgeschrieben: ") + trunc(m.sug.Command, 60) + "  (" + a.keys.label(actChip1) + i18n.T(" fügt ein)"))
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
