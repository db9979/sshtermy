package syncx

import (
	"testing"
	"time"
)

func TestMergeCmdSets(t *testing.T) {
	t0 := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	a := &Payload{CmdSets: []CmdSet{{ID: "x", YAML: "alt", UpdatedAt: t0}, {ID: "weg", YAML: "w", UpdatedAt: t0}}}
	b := &Payload{CmdSets: []CmdSet{{ID: "x", YAML: "neu", UpdatedAt: t0.Add(time.Minute)}},
		Deleted: map[string]time.Time{CmdSetTombstone("weg"): t0.Add(time.Minute)}}
	for _, m := range []*Payload{Merge(a, b, t0), Merge(b, a, t0)} {
		if len(m.CmdSets) != 1 || m.CmdSets[0].YAML != "neu" {
			t.Fatalf("Merge: %+v", m.CmdSets)
		}
	}
}
