package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Regeln für Switch-Configs (Palette „Regeln für Switch-Configs“)
//
// Jede gesicherte running-config (F6 b oder Zeitplan „Config-Sicherung“)
// wird gegen diese Regeln geprüft: „+muster“ muss vorkommen, „-muster“ darf
// nicht. Verstöße stehen im F6-Panel und im Ergebnis des Zeitplans.

const maxConfigRules = 12

func (a *App) configRules() []hostinfo.ConfigRule {
	return hostinfo.ParseConfigRules(a.st.Settings().ConfigRules)
}

func (a *App) configRulesHint() string {
	n := len(a.st.Settings().ConfigRules)
	if n == 0 {
		return i18n.T("keine")
	}
	return fmt.Sprintf(i18n.T("%d Regeln"), n)
}

func (a *App) openConfigRulesForm() tea.Cmd {
	rules := a.st.Settings().ConfigRules
	f := &form{title: i18n.T("Regeln für Switch-Configs")}
	for i := 0; i < maxConfigRules; i++ {
		v := ""
		if i < len(rules) {
			v = rules[i]
		}
		h := i18n.T("+muster muss vorkommen, -muster darf nicht · regulärer Ausdruck, zeilenweise")
		if i == 0 {
			h = i18n.T("z. B. „+^ntp server“, „+^logging“, „-telnet-server“, „-community \"public\"“ · geprüft bei jeder Sicherung (F6 b, Zeitplan)")
		}
		f.text(fmt.Sprintf("r%d", i), fmt.Sprintf(i18n.T("Regel %d"), i+1), v, h)
	}
	f.get("r0").in.Placeholder = "+^ntp server"
	f.get("r1").in.Placeholder = "-telnet-server"
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.scr = f, scrRulesForm
	return f.setFocus(0)
}

func (a *App) saveConfigRules() error {
	f := a.form
	var rules []string
	for i := 0; i < maxConfigRules; i++ {
		v := f.str(fmt.Sprintf("r%d", i))
		if v == "" {
			continue
		}
		r, ok, err := hostinfo.ParseConfigRule(v)
		if err != nil {
			return fmt.Errorf(i18n.T("Regel %d: %v"), i+1, err)
		}
		if ok {
			rules = append(rules, r.String())
		}
	}
	return a.st.UpdateSettings(func(s *domain.Settings) { s.ConfigRules = rules })
}

// violationText: Verstöße in einer Zeile.
func violationText(vs []hostinfo.Violation) string {
	var parts []string
	for _, v := range vs {
		parts = append(parts, v.Text())
	}
	return strings.Join(parts, " · ")
}
