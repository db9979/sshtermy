package syncx

import (
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// Alle Schalter wandern mit; rechnerspezifische Felder bleiben lokal.
func TestSettingsSyncAllPrefs(t *testing.T) {
	a := domain.NewData()
	a.Settings = domain.Settings{
		Theme: "matrix", Language: "en", LockAfterMin: 7,
		NoSessionLog: true, DangerAll: true, NoOSC52: true, NoDesktopNotify: true,
		Monitor: domain.MonitorConfig{CPU: 80, EveryMin: -1}, Icons: "uni", Intro: "aus",
		NoMouse: true, UseDPAPI: true, LastVersion: "1.0.1",
	}
	a.SettingsUpdatedAt = time.Now()

	b := domain.NewData()
	b.Settings = domain.Settings{LastVersion: "1.0.9"}
	FromData(a, nil).ApplyTo(b)

	s := b.Settings
	if s.Theme != "matrix" || s.Language != "en" || s.LockAfterMin != 7 || !s.NoSessionLog || !s.DangerAll ||
		!s.NoOSC52 || !s.NoDesktopNotify || s.Monitor.CPU != 80 || s.Monitor.EveryMin != -1 ||
		s.Icons != "uni" || s.Intro != "aus" {
		t.Fatalf("Einstellungen nicht übernommen: %+v", s)
	}
	if s.NoMouse || s.UseDPAPI || s.LastVersion != "1.0.9" {
		t.Fatalf("lokale Felder überschrieben: %+v", s)
	}
}

// Ein Stand von einer älteren Version (ohne Prefs) setzt nichts zurück.
func TestSettingsSyncOldPeerKeepsLocal(t *testing.T) {
	d := domain.NewData()
	d.Settings.DangerAll, d.Settings.Icons = true, "min"
	p := FromData(d, nil)
	p.Settings.Prefs = nil
	p.Settings.Theme = "hell"
	d2 := domain.NewData()
	d2.Settings.DangerAll, d2.Settings.Icons = true, "min"
	p.ApplyTo(d2)
	if !d2.Settings.DangerAll || d2.Settings.Icons != "min" || d2.Settings.Theme != "hell" {
		t.Fatalf("%+v", d2.Settings)
	}
}

// Nerd-Font-Symbole von einem anderen Rechner greifen erst nach der Probe.
func TestSettingsSyncNerdNeedsProbe(t *testing.T) {
	src := domain.NewData()
	src.Settings.Icons, src.Settings.NerdFont = "nerd", "ja"
	p := FromData(src, nil)

	fresh := domain.NewData()
	p.ApplyTo(fresh)
	if fresh.Settings.Icons != "nerd" || fresh.Settings.NerdFont != "nein" || fresh.Settings.IconsNow() != "" {
		t.Fatalf("ohne Probe: %+v", fresh.Settings)
	}

	had := domain.NewData()
	had.Settings.Icons = "nerd" // hier selbst gewählt (ältere Version, ohne NerdFont)
	p.ApplyTo(had)
	if had.Settings.IconsNow() != "nerd" {
		t.Fatalf("schon bestätigt: %+v", had.Settings)
	}

	ok := domain.NewData()
	ok.Settings.NerdFont = "ja"
	p.ApplyTo(ok)
	if ok.Settings.IconsNow() != "nerd" {
		t.Fatalf("Probe bestätigt: %+v", ok.Settings)
	}
}
