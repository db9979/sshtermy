package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/doctor"
	"github.com/db9979/sshtermy/internal/domain"
)

func TestDoctorModal(t *testing.T) {
	a := newPaletteTestApp(t)
	a.modal = nil
	a.Update(doctorMsg{"✓ Datenordner  beschreibbar\n\nAlles in Ordnung.\n"})
	if a.modal == nil || a.modal.kind != mText || a.modal.title != "Selbstprüfung" {
		t.Fatalf("kein Ergebnisfenster: %+v", a.modal)
	}
	if !strings.Contains(a.modal.copy, "Alles in Ordnung.") {
		t.Fatal("c kopiert nicht das Ergebnis")
	}
}

func TestDoctorAICheck(t *testing.T) {
	r := aiCheck(domain.LLMConfig{})(context.Background())
	if r.Status != doctor.Info {
		t.Fatalf("ohne KI: %+v", r)
	}
	r = aiCheck(domain.LLMConfig{Provider: domain.ProviderOllama, BaseURL: "http://127.0.0.1:1", Model: "test", TimeoutSec: 2})(context.Background())
	if r.Status != doctor.Warn || !strings.HasPrefix(r.Detail, "test") {
		t.Fatalf("KI nicht erreichbar: %+v", r)
	}
}

func TestDoctorInPalette(t *testing.T) {
	a := newPaletteTestApp(t)
	found := false
	for _, it := range a.palMenuItems() {
		if it.menu == "help/debug" && strings.HasPrefix(it.label, "Selbstprüfung") {
			found = true
		}
	}
	if !found {
		t.Fatal("Selbstprüfung fehlt unter Hilfe › Fehlersuche")
	}
}
