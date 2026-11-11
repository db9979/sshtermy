package ui

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// Platzhalter in Schnellbefehlen: „systemctl restart {{dienst}}“ fragt beim
// Ausführen nach dem Wert. Der zuletzt eingegebene Wert je Name steht beim
// nächsten Mal schon da (nur bis zum Beenden, nichts wird gespeichert).
// „{{dienst:nginx}}“ gibt einen Vorschlag mit.

var rxQuickVar = regexp.MustCompile(`\{\{\s*([\p{L}\p{N}_ -]{1,40}?)\s*(?::([^{}]*))?\}\}`)

type quickVar struct {
	name, def string
}

// quickPlaceholders: die Platzhalter eines Befehls in Reihenfolge, jeder
// Name einmal.
func quickPlaceholders(cmd string) []quickVar {
	var out []quickVar
	seen := map[string]bool{}
	for _, m := range rxQuickVar.FindAllStringSubmatch(cmd, -1) {
		if name := strings.TrimSpace(m[1]); name != "" && !seen[name] {
			seen[name] = true
			out = append(out, quickVar{name, strings.TrimSpace(m[2])})
		}
	}
	return out
}

// fillQuick setzt die Werte ein.
func fillQuick(cmd string, vals map[string]string) string {
	return rxQuickVar.ReplaceAllStringFunc(cmd, func(m string) string {
		sub := rxQuickVar.FindStringSubmatch(m)
		if v, ok := vals[strings.TrimSpace(sub[1])]; ok {
			return v
		}
		return m
	})
}

// askQuickValues fragt die Platzhalter nacheinander ab und führt den Befehl
// danach aus. Esc bricht ab, ohne etwas zu tippen. Der fertige Befehl wird
// noch einmal auf Gefahren geprüft: der eingesetzte Wert kann ihn erst
// gefährlich machen.
func (a *App) askQuickValues(s *session, cmd string, vars []quickVar, got map[string]string) tea.Cmd {
	if got == nil {
		got = map[string]string{}
	}
	if len(got) == len(vars) {
		full := fillQuick(cmd, got)
		sid := s.id
		run := func() tea.Cmd {
			if s := a.byID(sid); s != nil && s.state == stConnected && s.term != nil {
				a.sendQuick(s, full)
			}
			return nil
		}
		if f := safety.Check(full, a.safetyKind(s)); len(f) > 0 && len(safety.Check(cmd, a.safetyKind(s))) == 0 {
			a.modal = confirmModal(i18n.T("Vorsicht"), f[0].Reason+"\n\n"+full+"\n\n"+i18n.T("Trotzdem ausführen?"), run)
			return nil
		}
		return run()
	}
	v := vars[len(got)]
	val := v.def
	if last, ok := a.quickVals[v.name]; ok {
		val = last
	}
	m := inputModal(i18n.T("Schnellbefehl")+" · "+v.name, trunc(cmd, 70), val, func(text string) tea.Cmd {
		if text == "" {
			a.flash(i18n.F("„%s“ ist leer: Schnellbefehl nicht ausgeführt.", v.name))
			return nil
		}
		if a.quickVals == nil {
			a.quickVals = map[string]string{}
		}
		a.quickVals[v.name] = text
		got[v.name] = text
		if s := a.byID(s.id); s != nil {
			return a.askQuickValues(s, cmd, vars, got)
		}
		return nil
	})
	m.in.Placeholder = v.name
	a.modal = m
	return nil
}
