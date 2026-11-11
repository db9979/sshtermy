package theme

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

// Jedes Schema hat einen eindeutigen Namen, lässt sich darüber finden und
// ist auf seinem Hintergrund lesbar.
func TestAllThemes(t *testing.T) {
	seen := map[string]bool{}
	for _, th := range All {
		if th.Name == "" || th.Label == "" || seen[th.Name] {
			t.Errorf("Name fehlt oder doppelt: %q", th.Name)
		}
		seen[th.Name] = true
		if ByName(th.Name).Name != th.Name {
			t.Errorf("%s nicht auffindbar", th.Name)
		}
		if p := Problems(th); len(p) > 0 {
			t.Errorf("%s schlecht lesbar: %v", th.Name, p)
		}
		if g := Group(th); g != "Dunkel" && g != "Hell" && g != "Barrierefrei" {
			t.Errorf("%s: Gruppe %q", th.Name, g)
		}
	}
	if ByName("gibt-es-nicht").Name != Nacht.Name || ByName("").Name != Nacht.Name {
		t.Error("unbekannter Name muss Nacht ergeben")
	}
}

func TestCustomTheme(t *testing.T) {
	c := domain.CustomColors{Base: "nacht", Bg: "#ffffff", Text: "000000", Accent: "kaputt"}
	th := Custom(c)
	if th.Name != CustomName || Hex(th.Bg) != "#ffffff" || Hex(th.Text) != "#000000" {
		t.Fatalf("%+v", th)
	}
	if Hex(th.Accent) != Hex(Nacht.Accent) {
		t.Error("ungültiger Wert muss bei der Vorlage bleiben")
	}
	if th.Dark {
		t.Error("heller Hintergrund muss als hell gelten")
	}
	if got := Of(domain.Settings{Theme: CustomName, Colors: &c}); got.Name != CustomName {
		t.Errorf("Of: %q", got.Name)
	}
	if got := Of(domain.Settings{Theme: CustomName}); got.Name != Nacht.Name {
		t.Errorf("eigene Farben ohne Werte: %q", got.Name)
	}
}

func TestColorMath(t *testing.T) {
	w, _ := ParseHex("#ffffff")
	b, _ := ParseHex("000000")
	if c := Contrast(w, b); c < 20.9 || c > 21.1 {
		t.Errorf("Kontrast Weiß/Schwarz: %.2f", c)
	}
	if Hex(Mix(b, w, 0.5)) != "#808080" {
		t.Errorf("Mix: %s", Hex(Mix(b, w, 0.5)))
	}
	for _, bad := range []string{"", "#fff", "zzzzzz", "#1234567"} {
		if _, ok := ParseHex(bad); ok {
			t.Errorf("%q angenommen", bad)
		}
	}
}
