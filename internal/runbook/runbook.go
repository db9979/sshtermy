// Package runbook füllt Variablen in Runbook-Schritten aus.
//
// Ein Schritt darf Platzhalter enthalten: {{vlan}} fragt vor dem Start nach
// einem Wert, {{vlan=20}} schlägt 20 vor. Einige Namen füllt sshTermy pro Host
// selbst aus (siehe Builtins), z. B. „ping -c1 {{host}}“ oder
// „hostname {{alias}}“.
package runbook

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/db9979/sshtermy/internal/domain"
)

var varRx = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_-]*)\s*(?:=([^}]*))?\}\}`)

// Var: eine Variable, nach der vor dem Start gefragt wird.
type Var struct {
	Name    string
	Default string
}

// Builtins: Variablen, die pro Host automatisch gefüllt werden.
var Builtins = []string{"host", "alias", "user", "port"}

// Builtin meldet, ob name pro Host automatisch gefüllt wird.
func Builtin(name string) bool {
	for _, b := range Builtins {
		if strings.EqualFold(b, name) {
			return true
		}
	}
	return false
}

// Vars: alle Variablen zum Ausfüllen, in der Reihenfolge ihres ersten
// Auftretens. Der erste Vorschlag (={…}) gewinnt.
func Vars(steps []string) []Var {
	var out []Var
	seen := map[string]int{}
	for _, s := range steps {
		for _, m := range varRx.FindAllStringSubmatch(s, -1) {
			name := m[1]
			if Builtin(name) {
				continue
			}
			key := strings.ToLower(name)
			if i, ok := seen[key]; ok {
				if out[i].Default == "" {
					out[i].Default = strings.TrimSpace(m[2])
				}
				continue
			}
			seen[key] = len(out)
			out = append(out, Var{Name: name, Default: strings.TrimSpace(m[2])})
		}
	}
	return out
}

// HasBuiltins: enthält ein Schritt {{host}}, {{alias}} …?
func HasBuiltins(steps []string) bool {
	for _, s := range steps {
		for _, m := range varRx.FindAllStringSubmatch(s, -1) {
			if Builtin(m[1]) {
				return true
			}
		}
	}
	return false
}

// Fill ersetzt alle Platzhalter: eigene Variablen aus vals (Groß/klein egal,
// fehlt ein Wert, gilt der Vorschlag), eingebaute aus der Verbindung c.
func Fill(steps []string, vals map[string]string, c domain.Connection) []string {
	low := map[string]string{}
	for _, v := range Vars(steps) {
		low[strings.ToLower(v.Name)] = v.Default
	}
	for k, v := range vals {
		if v == "" {
			continue
		}
		low[strings.ToLower(k)] = v
	}
	port := c.Port
	if port == 0 {
		port = 22
	}
	builtin := map[string]string{"host": c.Host, "alias": c.Alias, "user": c.Username, "port": strconv.Itoa(port)}
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = varRx.ReplaceAllStringFunc(s, func(m string) string {
			sm := varRx.FindStringSubmatch(m)
			key := strings.ToLower(sm[1])
			if v, ok := builtin[key]; ok {
				return v
			}
			return low[key]
		})
	}
	return out
}

// Missing: Variablen ohne Wert und ohne Vorschlag.
func Missing(steps []string, vals map[string]string) []string {
	var out []string
	for _, v := range Vars(steps) {
		if strings.TrimSpace(vals[v.Name]) == "" && v.Default == "" {
			out = append(out, v.Name)
		}
	}
	return out
}
