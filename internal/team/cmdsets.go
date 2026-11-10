package team

import (
	"sort"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/domain"
)

// CmdSet: eigener Befehlssatz im Paket (YAML wie in der Datei). Beim
// Empfänger landet er unter befehlssaetze/team und wirkt erst nach Freigabe.
type CmdSet struct {
	ID   string `json:"id"`
	YAML string `json:"yaml"`
}

// cmdSetsFor: eigene Sätze, die die geteilten Verbindungen benutzen (samt
// ihrer eigenen Grundlagen). Eingebaute hat der Empfänger selbst.
func cmdSetsFor(conns []domain.Connection) []CmdSet {
	need := map[string]bool{}
	for _, c := range conns {
		if c.Profile != nil && c.Profile.Dialect != "" {
			need[c.Profile.Dialect] = true
		}
	}
	ids := make([]string, 0, len(need))
	for id := range need {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []CmdSet
	seen := map[string]bool{}
	var add func(id string, depth int)
	add = func(id string, depth int) {
		s, ok := cmdset.Lookup(id)
		if !ok || seen[id] || depth > 4 || s.Source != cmdset.Own || len(s.Raw) == 0 {
			return
		}
		seen[id] = true
		out = append(out, CmdSet{ID: s.ID, YAML: string(s.Raw)})
		if s.Base != "" {
			add(s.Base, depth+1)
		}
	}
	for _, id := range ids {
		add(id, 0)
	}
	return out
}
