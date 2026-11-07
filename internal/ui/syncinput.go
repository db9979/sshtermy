package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// Eingabe an alle Bereiche (Alt+I in der geteilten Ansicht)
//
// Was man im aktiven Bereich tippt, geht gleichzeitig an alle anderen
// sichtbaren, verbundenen Bereiche, live und interaktiv (z. B. dieselbe
// Änderung auf vier Switches). Die Titelzeilen zeigen „◈ alle“.
//
// Sicherheit: Enter geht nur durch, wenn die Zeile auf keinem der Hosts
// den Gefahren-Check auslösen würde; sonst wird sie nirgends ausgeführt.
// Einfügen geht nur einzeilig und ohne Gefahrenfund an alle. Ghost-Text
// und Folgebefehle gelten in dieser Zeit nicht (sie unterscheiden sich je
// Host).

func (a *App) toggleSyncInput() {
	if a.split == splitOff && !a.syncIn {
		a.flash(i18n.T("Eingabe an alle gibt es nur in der geteilten Ansicht (") + a.keys.label(actSplit) + ")")
		return
	}
	a.syncIn = !a.syncIn
	if a.syncIn {
		a.flash(i18n.T("Eingabe an alle Bereiche: an. Was du tippst, geht an jeden sichtbaren Host."))
	} else {
		a.flash(i18n.T("Eingabe an alle Bereiche: aus"))
	}
}

// syncTargets: die anderen Bereiche, die mittippen (leer = Eingabe normal).
func (a *App) syncTargets(s *session) []*session {
	if !a.syncIn || a.split == splitOff {
		return nil
	}
	var out []*session
	for _, id := range a.panes {
		x := a.byID(id)
		if x == nil || x == s || x.term == nil || x.state != stConnected || x.detached() || x.panelFocus || x.files.open {
			continue
		}
		out = append(out, x)
	}
	return out
}

// syncDanger: Würde Enter die Zeile von x durch den Gefahren-Check schicken?
func (a *App) syncDanger(x *session) string {
	if x.term == nil || x.term.AltScreen() || !a.dangerGuardOn(x.conn) {
		return ""
	}
	typed, ok := x.term.TypedInput()
	cmd := strings.TrimSpace(typed)
	if !ok || cmd == "" || agent.LooksLikeSecretPrompt(x.term.CursorLine()) {
		return ""
	}
	if f := safety.Check(cmd, a.safetyKind(x)); len(f) > 0 {
		return f[0].Reason
	}
	return ""
}

// syncKey: Taste im aktiven Bereich, die auch an tg geht.
func (a *App) syncKey(s *session, tg []*session, k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if a.keys.which(key, actChip1, actChip2, actChip3, actChip4) != "" || key == "alt+0" {
		a.flash(i18n.T("Vorschläge gelten je Host, bei Eingabe an alle nicht (") + a.keys.label(actSyncInput) + i18n.T(" schaltet ab)"))
		return nil
	}
	all := append([]*session{s}, tg...)
	if key == "enter" {
		for _, x := range all {
			if why := a.syncDanger(x); why != "" {
				a.flashErr(x.conn.Alias + ": " + why + i18n.T(". Nirgends ausgeführt; Eingabe an alle abschalten und einzeln bestätigen."))
				return nil
			}
		}
	}
	var cmd tea.Cmd
	for i, x := range all {
		x.ghost = "" // Ghost-Text unterscheidet sich je Host: Tab geht als Taste
		c := a.terminalKeyOne(x, k)
		if i == 0 {
			cmd = c // Ghost-Text-Anfragen nur für den aktiven Bereich
		}
	}
	s.ghost = ""
	return cmd
}

// syncPaste: Einfügen an alle (nur einzeilig und ohne Gefahrenfund).
func (a *App) syncPaste(s *session, tg []*session, text string) tea.Cmd {
	all := append([]*session{s}, tg...)
	for _, x := range all {
		pc := checkPaste(text, a.safetyKind(x))
		if pc.multi || pc.dangerous > 0 {
			a.flashErr(i18n.T("Bei Eingabe an alle geht nur einzeiliges, harmloses Einfügen. Nichts eingefügt."))
			return nil
		}
	}
	for _, x := range all {
		x.sel, x.scroll = nil, 0
		x.term.Paste(text)
	}
	return nil
}
