package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Wenn sshTermy selbst in eine Switch-Shell tippt (F6, Log-Blick, Syslog
// einrichten), nimmt der Tab keine Tasten und kein Einfügen an. Esc bricht
// die Folge ab. Sonst landete ein „no “ von dir vor „logging …“ im
// Konfigurationsmodus.

// shellBusyKey: Taste für einen Tab, in den sshTermy gerade tippt. true =
// verbraucht.
func (a *App) shellBusyKey(s *session, k tea.KeyPressMsg) bool {
	if s == nil || !s.cap.active() {
		return false
	}
	if k.String() == "esc" {
		s.cap.abort()
		a.flash(i18n.T("Abgebrochen. sshTermy tippt nichts mehr in diese Shell."))
		return true
	}
	a.flash(i18n.T("sshTermy tippt gerade in diese Shell, deine Eingabe wartet nicht · Esc bricht ab"))
	return true
}

// configPrompt: Prompt im Konfigurationsmodus (switch(config)#, [edit])?
// Abfragen und Einrichtung starten dort nicht: Befehle hätten eine andere
// Bedeutung oder änderten etwas.
func configPrompt(p string) bool {
	l := strings.ToLower(strings.TrimSpace(p))
	if strings.Contains(l, "(config") || strings.Contains(l, "[edit") {
		return true
	}
	// Comware/Huawei: <Name> normal, [Name] bzw. [~Name] in der system-view
	return strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") && !strings.ContainsAny(l, "@ ")
}

// cliError: meldet das Gerät einen Fehler in der Ausgabe?
func cliError(lines []string) string {
	for _, l := range lines {
		x := strings.ToLower(strings.TrimSpace(l))
		for _, m := range []string{"invalid input", "unknown command", "invalid command", "incomplete command", "ambiguous",
			"syntax error", "command fail", "error:", "bad command", "command parse error", "unrecognized command", "not found"} {
			if strings.Contains(x, m) {
				return strings.TrimSpace(l)
			}
		}
	}
	return ""
}
