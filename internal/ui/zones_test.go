package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestZonesSurviveLayout(t *testing.T) {
	var z zones
	hit := ""
	mk := func(name, s string) string {
		return z.click(s, func(tea.Mouse, bool) tea.Cmd { hit = name; return nil })
	}
	st := lipgloss.NewStyle().Background(lipgloss.Color("#333")).Padding(0, 1)
	left := lipgloss.NewStyle().Width(20).Render("Liste\n" + st.Render(mk("a", "Eintrag A")) + "\n" + mk("b", "Eintrag B"))
	right := lipgloss.NewStyle().Width(30).Render("Detail " + mk("c", lipgloss.NewStyle().Bold(true).Render("Knopf")))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "│", right)
	body = lipgloss.Place(60, 6, lipgloss.Center, lipgloss.Center, body)
	clean := z.scan(body, 0, 0)
	if strings.Contains(clean, "71000z") || strings.Contains(clean, "\x1b[710") {
		t.Fatalf("Marker nicht entfernt: %q", clean)
	}
	lines := strings.Split(ansi.Strip(clean), "\n")
	find := func(word string) (int, int) {
		for y, l := range lines {
			if i := strings.Index(l, word); i >= 0 {
				return ansi.StringWidth(l[:i]), y
			}
		}
		t.Fatalf("%q nicht gefunden", word)
		return 0, 0
	}
	for _, c := range []struct{ word, want string }{{"Eintrag A", "a"}, {"Eintrag B", "b"}, {"Knopf", "c"}} {
		x, y := find(c.word)
		hit = ""
		if act := z.at(x+2, y, false); act != nil {
			act.click(tea.Mouse{}, false)
		}
		if hit != c.want {
			t.Errorf("%s bei %d,%d: Treffer %q, erwartet %q", c.word, x+2, y, hit, c.want)
		}
	}
	if act := z.at(0, 0, false); act != nil {
		t.Errorf("Treffer außerhalb aller Zonen")
	}
}
