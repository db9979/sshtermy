package agent

import (
	"context"
	"testing"

	"github.com/db9979/sshtermy/internal/safety"
)

func TestParseReview(t *testing.T) {
	out := "<think>…</think>```json\n" + `{"schritte":[{"passt":true},{"passt":false,"befehl":"dnf install -y nginx","grund":"dnf statt apt"},{"passt":false,"befehl":"","grund":"gibt es nicht"}],"fazit":"zwei Schritte anders"}` + "\n```"
	r, err := parseReview(out, 3, "unix")
	if err != nil {
		t.Fatal(err)
	}
	if r.Fits() || r.Changed() != 2 || r.Summary != "zwei Schritte anders" {
		t.Errorf("%+v", r)
	}
	steps, ok := r.Apply([]string{"uptime", "apt-get install -y nginx", "snap install x"})
	if ok || len(steps) != 2 || steps[1] != "dnf install -y nginx" {
		t.Errorf("Apply %v %v", steps, ok)
	}
	if _, err := parseReview(`{"schritte":[{"passt":true}]}`, 2, "unix"); err != ErrReviewMismatch {
		t.Errorf("Anzahl: %v", err)
	}
	if _, err := parseReview(`{"schritte":[{"passt":false,"befehl":"ls\nrm -rf /"}]}`, 1, "unix"); err != ErrUnreadable {
		t.Errorf("mehrzeilig: %v", err)
	}
	// Gefahrenregeln gelten auch für Vorschläge der KI, auf Switches mit den Netzregeln
	r, _ = parseReview(`{"schritte":[{"passt":false,"befehl":"reload","grund":"x"}]}`, 1, safety.Network)
	if len(r.Steps[0].Adapted.Danger) == 0 {
		t.Error("reload nicht als Gefahr markiert")
	}
}

func TestReviewCall(t *testing.T) {
	a, _ := fakeQwen(t, `{"schritte":[{"passt":false,"befehl":"show vlans","grund":"ArubaOS-Switch"}],"fazit":"Cisco-Syntax"}`)
	r, err := a.Review(context.Background(), []string{"show vlan brief"}, Context{Host: "sw1", Net: true})
	if err != nil || r.Steps[0].Adapted.Command != "show vlans" {
		t.Fatalf("%+v %v", r, err)
	}
}
