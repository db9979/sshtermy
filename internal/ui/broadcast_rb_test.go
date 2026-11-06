package ui

import (
	"testing"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
)

func TestBroadcastRunbookSteps(t *testing.T) {
	sw := domain.Connection{ID: "a", Alias: "sw1", Host: "10.0.0.2", Device: domain.DeviceNetwork}
	srv := domain.Connection{ID: "b", Alias: "web1", Host: "10.0.0.9"}
	b := &bcState{src: 1, books: []domain.Runbook{{Name: "VLAN", Steps: []string{"show vlan {{vlan=10}}", "ping {{host}}"}}},
		vals: map[string]string{"vlan": "20"}}
	per := b.perHost([]domain.Connection{sw, srv})
	if per[0][0] != "show vlan 20" || per[0][1] != "ping 10.0.0.2" || per[1][1] != "ping 10.0.0.9" {
		t.Fatalf("Schritte %q", per)
	}
	// KI-Vorschlag für den Switch, nur wenn eingeschaltet
	b.review = map[string]*hostReview{"a": {steps: per[0], r: agent.Review{Steps: []agent.StepReview{
		{OK: false, Adapted: agent.Suggestion{Command: "show vlans 20"}}, {OK: true}}}}}
	if got := b.hostSteps(sw); got[0] != "show vlan 20" {
		t.Errorf("ohne a: %q", got)
	}
	b.useAdapted = true
	if got := b.hostSteps(sw); got[0] != "show vlans 20" || b.hostSteps(srv)[0] != "show vlan 20" {
		t.Errorf("mit a: %q", got)
	}
	// geänderte Werte: alte Prüfung gilt nicht mehr
	b.vals["vlan"] = "30"
	if got := b.hostSteps(sw); got[0] != "show vlan 30" {
		t.Errorf("veraltete Prüfung angewendet: %q", got)
	}
	// Gefahrenregeln gelten für die tatsächlichen Schritte
	if d := allDangers([][]string{{"show vlans"}, {"reload"}, {"reload"}}); len(d) != 1 {
		t.Errorf("Gefahren %v", d)
	}
}
