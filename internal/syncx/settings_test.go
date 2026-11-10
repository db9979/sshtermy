package syncx

import (
	"reflect"
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

// Jedes Feld von domain.Settings ist entweder gemeinsam (geht mit dem Sync)
// oder bewusst lokal. Ein neues Feld, das hier fehlt, lässt den Test
// scheitern: dann in SharedPrefs/SharedMore eintragen oder in localSettings
// und Settings.SyncedPart.
var localSettings = map[string]bool{
	"UseDPAPI": true, "NoMouse": true, "LastVersion": true, "NerdFont": true,
	"LastLayout": true, "Editor": true, "Schedules": true, "Debug": true, "Audit": true, "MCP": true, "SyslogPort": true,
}

func TestSettingsFieldsClassified(t *testing.T) {
	src := domain.NewData()
	fill(reflect.ValueOf(&src.Settings).Elem())
	src.Settings.Theme, src.Settings.Icons = "matrix", "uni" // Sonderfälle in applyShared umgehen
	src.SettingsUpdatedAt = time.Now()

	dst := domain.NewData()
	dst.Settings = domain.Settings{}
	FromData(src, nil).ApplyTo(dst)

	part := src.Settings.SyncedPart()
	st := reflect.TypeOf(domain.Settings{})
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		got := reflect.ValueOf(dst.Settings).Field(i)
		want := reflect.ValueOf(src.Settings).Field(i)
		inPart := !reflect.ValueOf(part).Field(i).IsZero()
		if localSettings[f.Name] {
			if !got.IsZero() {
				t.Errorf("%s ist lokal, kam aber per Sync an", f.Name)
			}
			if inPart {
				t.Errorf("%s ist lokal, fehlt aber in Settings.SyncedPart", f.Name)
			}
			continue
		}
		if !reflect.DeepEqual(got.Interface(), want.Interface()) {
			t.Errorf("%s geht nicht mit dem Sync (in SharedPrefs/SharedMore eintragen oder als lokal markieren)", f.Name)
		}
		if !inPart {
			t.Errorf("%s fehlt in Settings.SyncedPart", f.Name)
		}
	}
}

// fill setzt jedes Feld auf einen Wert ungleich null.
func fill(v reflect.Value) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.String:
		v.SetString("x")
	case reflect.Int, reflect.Int64:
		v.SetInt(7)
	case reflect.Float64:
		v.SetFloat(0.5)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fill(v.Field(i))
			}
		}
	case reflect.Ptr:
		p := reflect.New(v.Type().Elem())
		fill(p.Elem())
		v.Set(p)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0))
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k, e := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		fill(k)
		fill(e)
		m.SetMapIndex(k, e)
		v.Set(m)
	}
}

// Ein Stand von v1.0.310 (Prefs ohne More) lässt Begleiter und Config-Regeln stehen.
func TestSettingsSyncOldPeerKeepsMore(t *testing.T) {
	d := domain.NewData()
	d.Settings.Companion.Level, d.Settings.ConfigRules = 2, []string{"+ntp"}
	p := FromData(domain.NewData(), nil)
	p.Settings.Prefs.More = nil
	p.ApplyTo(d)
	if d.Settings.Companion.Level != 2 || len(d.Settings.ConfigRules) != 1 {
		t.Fatalf("%+v", d.Settings)
	}
}
