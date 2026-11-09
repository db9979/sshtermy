package ui

import (
	"fmt"
	"image/color"
	"os"
	"testing"
)

// Alle Themes: Namen eindeutig, Zusatzfarben und Kopf-/Fußzeile lesbar.
func TestThemesMore(t *testing.T) {
	seen := map[string]bool{}
	dump := os.Getenv("THEME_DUMP") != ""
	for _, th := range Themes {
		if seen[th.Name] || th.Name == CustomName {
			t.Errorf("Name %q doppelt", th.Name)
		}
		seen[th.Name] = true
		if ThemeByName(th.Name).Name != th.Name {
			t.Errorf("%s nicht auffindbar", th.Name)
		}
		checks := []struct {
			what string
			a, b any
			min  float64
		}{
			{"Orange", accentColor(th, "orange"), th.Bg, 3.5},
			{"Cyan", accentColor(th, "cyan"), th.Bg, 3.5},
			{"Gedämpft auf Leiste", th.Muted, th.Surface, 3},
			{"Akzent auf Taste", th.Accent, th.Surface2, 3},
			{"Text auf Auswahl", th.Text, th.Surface2, 4.5},
			{"OK auf Taste", th.OK, th.Surface2, 3},
		}
		for _, c := range checks {
			r := contrast(c.a.(colorT), c.b.(colorT))
			if dump {
				fmt.Printf("%-16s %-22s %.2f\n", th.Name, c.what, r)
			}
			if r < c.min {
				t.Errorf("%s: %s nur %.1f:1", th.Name, c.what, r)
			}
		}
		if dump {
			for _, c := range []struct {
				n string
				c colorT
			}{{"Text", th.Text}, {"Muted", th.Muted}, {"Accent", th.Accent}, {"AI", th.AI}, {"OK", th.OK}, {"Warn", th.Warn}, {"Danger", th.Danger}} {
				fmt.Printf("%-16s %-22s %.2f\n", th.Name, c.n, contrast(c.c, th.Bg))
			}
		}
		if p := contrastProblems(th); len(p) > 0 {
			t.Errorf("%s: %v", th.Name, p)
		}
	}
}

type colorT = color.Color
