package safety

import "github.com/db9979/sshtermy/internal/cmdset"

// Befehlssätze (internal/cmdset) prüfen ihre Befehle mit ReadOnly und
// bringen zusätzliche Gefahrenregeln mit. Die Regeln aller aktiven Sätze
// gelten immer, egal welcher Satz zum Gerät gehört: Sie können die festen
// Regeln hier nur ergänzen, nie abschwächen.

func init() { cmdset.CheckRead = ReadOnly }

// withSetRules hängt Treffer der Befehlssatz-Regeln an (ohne doppelte Gründe).
func withSetRules(out []Finding, cmd string) []Finding {
	for _, f := range cmdset.CheckDanger(cmd) {
		dup := false
		for _, o := range out {
			if o.Reason == f.Reason {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, Finding{f.Reason})
		}
	}
	return out
}
