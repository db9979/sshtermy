package syncx

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

// Eigene Farben gehen mit dem Sync mit; ein Stand ohne Farben (ältere
// Version) löscht sie nicht, solange dort noch „eigene“ eingestellt ist.
func TestSyncCustomColors(t *testing.T) {
	src := &domain.Data{Settings: domain.Settings{Theme: "eigene", Colors: &domain.CustomColors{Base: "matrix", Accent: "#ff8800"}, Icons: "nerd"}}
	p := FromData(src, nil)
	dst := &domain.Data{}
	p.ApplyTo(dst)
	if dst.Settings.Colors == nil || dst.Settings.Colors.Accent != "#ff8800" || dst.Settings.Theme != "eigene" {
		t.Fatalf("Farben nicht übertragen: %+v", dst.Settings)
	}
	if dst.Settings.Icons != "" {
		t.Fatal("Symbolstil gehört zum Rechner (Schrift) und wird nicht übertragen")
	}
	src.Settings.Colors.Accent = "#000000"
	if dst.Settings.Colors.Accent != "#ff8800" {
		t.Fatal("Farben müssen kopiert werden")
	}
	old := FromData(&domain.Data{Settings: domain.Settings{Theme: "eigene"}}, nil)
	old.ApplyTo(dst)
	if dst.Settings.Colors == nil {
		t.Fatal("älterer Stand ohne Farben hat die eigenen Farben gelöscht")
	}
	FromData(&domain.Data{Settings: domain.Settings{Theme: "nacht"}}, nil).ApplyTo(dst)
	if dst.Settings.Colors != nil {
		t.Fatal("anderes Theme: eigene Farben übernehmen den Stand (nil)")
	}
}
