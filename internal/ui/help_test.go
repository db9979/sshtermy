package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestSplitColumnsBalanced(t *testing.T) {
	cols := splitColumns([]int{5, 9, 4, 9, 10, 4, 6, 9, 5}, 3)
	if len(cols) != 3 {
		t.Fatalf("want 3 columns, got %d", len(cols))
	}
	n := 0
	for _, c := range cols {
		n += len(c)
	}
	if n != 9 {
		t.Fatalf("all sections must be placed, got %d", n)
	}
}

func TestHelpFitsWindow(t *testing.T) {
	s := NewStyles(Nacht)
	for _, sz := range [][2]int{{140, 40}, {120, 30}, {90, 30}, {70, 24}} {
		m := &modal{kind: mHelp, ctx: hcTerminal}
		out := m.view(s, sz[0], sz[1])
		if w, h := lipgloss.Width(out), lipgloss.Height(out); w > sz[0] || h > sz[1] {
			t.Errorf("%dx%d: help is %dx%d", sz[0], sz[1], w, h)
		}
	}
}
