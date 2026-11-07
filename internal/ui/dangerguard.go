package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// Gefahren-Check beim Tippen
//
// Auf Produktiv-Hosts (siehe isProduction: Ordner, Tag oder Alias mit „prod“,
// Tag „live“) prüft sshTermy jede Zeile, bevor Enter an den Server geht. Trifft
// eine der festen Gefahrenregeln (internal/safety, auch für Switches: reload,
// erase, write erase, delete …), kommt erst ein roter Dialog. Ausführen nur mit
// „j“, ⏎ allein reicht nicht (Gewohnheit). Esc lässt die Zeile stehen, damit
// man sie korrigieren kann.
//
// Über die Palette lässt sich der Check auf alle Hosts ausweiten
// (Settings.DangerAll). Auf Produktiv-Hosts ist er nicht abschaltbar.

type dangerAsk struct {
	sid     int
	cmd     string // getippte Zeile, wie sie am Prompt steht
	reasons []string
	key     tea.KeyPressMsg // das zurückgehaltene Enter
	prod    bool
	root    bool
}

// netDevice: Switch/Router (Verbindung, Systemprofil oder Erkennung beim Aufbau).
func (s *session) netDevice() bool {
	return s.netDev() || s.prof.Family == domain.FamilyNetwork || s.conn.Device == domain.DeviceNetwork
}

// safetyKind: Regelgruppe für die Sitzung (unix, windows, network).
func (a *App) safetyKind(s *session) string {
	return safety.KindFor(a.remoteText(s), s.netDevice())
}

func (a *App) dangerAll() bool { return a.st.Settings().DangerAll }

// dangerGuardOn: wird auf dieser Verbindung beim Tippen geprüft?
func (a *App) dangerGuardOn(c domain.Connection) bool { return isProduction(c) || a.dangerAll() }

func dangerLabel(all bool) string {
	if all {
		return i18n.T("alle Hosts")
	}
	return i18n.T("nur Produktiv")
}

func (a *App) toggleDangerAll() tea.Cmd {
	on := !a.dangerAll()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.DangerAll = on }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Gefahren-Check auf allen Hosts: riskante Befehle fragen vor dem Ausführen nach."))
	} else {
		a.flash(i18n.T("Gefahren-Check nur noch auf Produktiv-Hosts."))
	}
	return nil
}

// dangerCheck: Enter am Prompt. true = zurückgehalten, Dialog ist offen.
func (a *App) dangerCheck(s *session, k tea.KeyPressMsg) bool {
	if s.term == nil || s.term.AltScreen() || !a.dangerGuardOn(s.conn) {
		return false
	}
	typed, ok := s.term.TypedInput()
	cmd := strings.TrimSpace(typed)
	if !ok || cmd == "" || agent.LooksLikeSecretPrompt(s.term.CursorLine()) {
		return false
	}
	if s.dangerOK != "" && s.dangerOK == cmd {
		s.dangerOK = ""
		return false
	}
	s.dangerOK = ""
	f := safety.Check(cmd, a.safetyKind(s))
	if len(f) == 0 {
		return false
	}
	d := &dangerAsk{sid: s.id, cmd: cmd, key: k, prod: isProduction(s.conn), root: s.state == stConnected && s.rootNow()}
	for _, x := range f {
		d.reasons = append(d.reasons, x.Reason)
	}
	a.modal = &modal{kind: mDanger, title: i18n.T("▲ Riskanter Befehl"), danger: d}
	return true
}

func (a *App) dangerKey(k tea.KeyPressMsg) tea.Cmd {
	d := a.modal.danger
	yn := yesNo(k)
	switch {
	case yn == 1:
		a.modal = nil
		s := a.byID(d.sid)
		if s == nil || s.term == nil || s.state != stConnected {
			return nil
		}
		s.dangerOK = d.cmd
		return a.terminalKey(s, d.key)
	case yn == -1 || k.String() == "q":
		a.modal = nil
		a.flash(i18n.T("Nicht ausgeführt. Die Zeile steht noch am Prompt."))
	case k.String() == "enter":
		a.modal.body = "again" // Hinweis: j drücken
	}
	return nil
}

func (a *App) dangerView(m *modal) string {
	st := a.s
	d := m.danger
	w := dialog{maxW: 80}.innerMax(a.w)
	s := a.byID(d.sid)
	host := ""
	if s != nil {
		host = s.conn.Alias + " (" + s.conn.Username + "@" + s.conn.ShowAddr() + ")"
	}
	var b strings.Builder
	if d.prod {
		b.WriteString(st.ChipDanger.Bold(true).Render(i18n.T("PRODUKTIV")) + " ")
	}
	if d.root {
		b.WriteString(st.ChipDanger.Bold(true).Render(i18n.T("▲ root")) + " ")
	}
	b.WriteString(st.Text.Render(host) + "\n\n")
	b.WriteString(st.Muted.Render("$ ") + st.Danger.Render(trunc(d.cmd, w-2)) + "\n\n")
	for _, r := range d.reasons {
		b.WriteString(st.Danger.Render("▲ "+r) + "\n")
	}
	if m.body == "again" {
		b.WriteString("\n" + st.Warn.Render(fmt.Sprintf(i18n.T("⏎ reicht hier nicht: %s drücken zum Ausführen."), yesKey())) + "\n")
	}
	keys := [][2]string{{yesKey(), i18n.T("trotzdem ausführen")}, {"Esc", i18n.T("abbrechen (Zeile bleibt stehen)")}}
	return st.dialog(dialog{icon: "▲", title: strings.TrimPrefix(m.title, "▲ "), tone: st.T.Danger, body: b.String(), keys: keys, width: w, maxW: 80}, a.w)
}
