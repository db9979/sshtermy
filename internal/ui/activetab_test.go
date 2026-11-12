package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Schrift auf dem aktiven Tab ist in jedem Theme und jeder Tab-Farbe lesbar.
func TestActiveTabInkReadable(t *testing.T) {
	for _, th := range Themes {
		fills := []string{"accent"}
		for _, d := range accentDefs {
			if d.id != accentAuto && d.id != accentNone {
				fills = append(fills, d.id)
			}
		}
		for _, id := range fills {
			fill := th.Accent
			if id != "accent" {
				fill = accentColor(th, id)
			}
			if c := contrast(inkOn(th, fill), fill); c < 4.5 {
				t.Errorf("%s/%s: Kontrast %.1f < 4,5", th.Name, id, c)
			}
		}
	}
}

// Ohne Farbe: genau ein ▶ in der Tab-Leiste, und zwar am aktiven Tab.
func TestActiveTabMarkWithoutColor(t *testing.T) {
	g := newGold(t, "de", 140, 30, true)
	g.session("web-01", termOut)
	g.session("web-02", termOut)
	g.a.focusSession(0)
	strip := ansi.Strip(g.a.tabStrip(g.a.w - lipgloss.Width(g.a.headerLeft()) - 1))
	if strings.Count(strip, activeMark) != 1 || !strings.Contains(strip, activeMark+" ● 1 web-01") {
		t.Fatalf("aktiver Tab ohne ▶: %q", strip)
	}
}
