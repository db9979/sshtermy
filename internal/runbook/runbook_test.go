package runbook

import (
	"reflect"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestVarsAndFill(t *testing.T) {
	steps := []string{"vlan {{vlan=20}}", "name {{ Name }}", "ping {{host}} # {{alias}} {{VLAN}}", "ls {{ unfertig"}
	vs := Vars(steps)
	if !reflect.DeepEqual(vs, []Var{{"vlan", "20"}, {"Name", ""}}) {
		t.Fatalf("Vars %v", vs)
	}
	if !HasBuiltins(steps) || HasBuiltins(steps[:2]) {
		t.Error("HasBuiltins")
	}
	if m := Missing(steps, map[string]string{"vlan": ""}); !reflect.DeepEqual(m, []string{"Name"}) {
		t.Errorf("Missing %v", m)
	}
	c := domain.Connection{Alias: "sw1", Host: "10.0.0.2", Username: "admin"}
	got := Fill(steps, map[string]string{"name": "Gäste"}, c)
	want := []string{"vlan 20", "name Gäste", "ping 10.0.0.2 # sw1 20", "ls {{ unfertig"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Fill %q", got)
	}
	if got := Fill([]string{"{{port}} {{user}}"}, nil, c); got[0] != "22 admin" {
		t.Errorf("Builtins %q", got)
	}
}
