package ui

import (
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Fortsetzungs-Prompt „>“: Ein unvollständiger Befehl (offenes
// Anführungszeichen, Heredoc, „|“ am Ende) lässt die Shell auf den Rest warten.
// Was sshTermy danach schickt, verschluckt sie als Fortsetzung, statt es
// auszuführen. Deshalb:
//
//   - sendLine: Steht die Shell schon am „>“, kommt erst Strg+C, kurz danach
//     der Befehl.
//   - commandTick: Bleibt ein von sshTermy gesendeter Befehl am „>“ stehen,
//     bricht Strg+C ihn ab, und er zählt als Fehlschlag mit klarem Grund (geht
//     im Ziel-, Agent- und Plan-Modus ans Modell, Antwortkarten bieten den Fix).
//   - agent.Incomplete: KI-Vorschläge, die so hängen blieben, werden gar nicht
//     erst gesendet (Modell bekommt eine gezielte Nachfrage).
//
// Selbst getippte Befehle bleiben unangetastet: Wer von Hand eine Schleife
// über mehrere Zeilen schreibt, will das „>“.

// contBreakDelay: Pause zwischen Strg+C und dem Befehl. Kämen beide im selben
// Paket an, verwürfe die Shell beim Strg+C auch den Befehl.
const contBreakDelay = 300 * time.Millisecond

// contGuard: Fortsetzungs-Prompts auf dieser Sitzung abfangen? Nicht bei
// Netzwerkgeräten (eigene Prompts, Strg+C hat dort andere Bedeutungen), nicht
// bei WinRM (Strg+C startet die Sitzung neu) und nicht, wenn sich gezeigt hat,
// dass der normale Prompt dieses Systems selbst nur „>“ ist.
func (a *App) contGuard(s *session) bool {
	return s.term != nil && !s.contHome && s.conn.Protocol != domain.ProtoWinRM && !s.netDevice()
}

// atContinuation: wartet die Shell auf die Fortsetzung eines Befehls?
func (a *App) atContinuation(s *session) bool {
	return a.contGuard(s) && s.term.AtContinuation()
}

// sendLine schickt einen Befehl von sshTermy (Ziel-/Agent-Modus, Antwortkarte,
// Favorit …) samt Enter. Hängt die Shell noch am „>“, kommt erst Strg+C.
func (a *App) sendLine(s *session, cmd string) {
	if s.term == nil {
		return
	}
	if r := s.run; r != nil && r.cmd == strings.TrimSpace(cmd) {
		r.sent = true
	}
	if !a.atContinuation(s) {
		s.term.SendText(cmd + "\r")
		return
	}
	t := s.term
	t.SendText("\x03")
	if r := s.run; r != nil {
		r.at = time.Now().Add(contBreakDelay) // erst nach dem eigentlichen Senden zählen
	}
	a.flash(i18n.T("Shell wartete am „>“ auf eine Fortsetzung: mit Strg+C abgebrochen, dann den Befehl gesendet"))
	time.AfterFunc(contBreakDelay, func() { t.SendText(cmd + "\r") })
}

// contCheck (aus commandTick, Ausgabe ruht, Prompt steht): bleibt ein von
// sshTermy gesendeter Befehl am „>“ hängen? true = abgefangen, Befehl läuft
// noch bis zum echten Prompt.
func (a *App) contCheck(s *session, r *cmdRun) bool {
	if !r.sent || !a.contGuard(s) {
		return false
	}
	if r.contWhy != "" {
		if s.term.AtContinuation() {
			// Strg+C half nicht: dann ist „>“ hier der normale Prompt
			s.contHome = true
			r.contWhy = ""
		}
		return false
	}
	if !s.term.AtContinuation() {
		return false
	}
	why := agent.Incomplete(r.cmd)
	if why != "" {
		why = i18n.T(why)
	} else {
		why = i18n.T("Befehl unvollständig")
	}
	r.contWhy = why
	s.term.SendText("\x03")
	r.at = time.Now()
	a.flash(i18n.F("Befehl blieb am „>“ hängen (%s): mit Strg+C abgebrochen", why))
	return true
}

// contReason: Grund für den Fehlschlag eines am „>“ abgebrochenen Befehls.
func contReason(why string) string {
	return i18n.F("unvollständig, Shell wartete am „>“ auf Fortsetzung (%s), sshTermy hat mit Strg+C abgebrochen", why)
}

// aiIncomplete: Grund, falls ein KI-Befehl die Shell am „>“ hängen ließe
// (nur Unix-Shells; Switches und PowerShell haben andere Regeln).
func (a *App) aiIncomplete(s *session, cmd string) string {
	if !a.contGuard(s) || a.safetyKind(s) != "unix" {
		return ""
	}
	if why := agent.Incomplete(cmd); why != "" {
		return i18n.T(why)
	}
	return ""
}
