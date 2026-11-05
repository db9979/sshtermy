package ui

import (
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func splitApp(n int) *App {
	a := &App{w: 120, h: 40, s: NewStyles(Nacht)}
	for i := 1; i <= n; i++ {
		a.sessions = append(a.sessions, &session{id: i, conn: domain.Connection{Alias: "h"}, chipArmed: -1})
	}
	a.nextSID = n
	return a
}

func TestPaneRectsCoverArea(t *testing.T) {
	a := splitApp(4)
	for _, m := range []splitMode{splitCols, splitRows, splitGrid} {
		a.split = m
		ar := a.areaRect()
		area := 0
		for _, r := range a.paneRects() {
			area += r.w * r.h
			if r.x+r.w > a.w || r.y+r.h > ar.y+ar.h {
				t.Errorf("%v: rect %+v outside", m, r)
			}
		}
		c, _ := m.grid()
		if want := (ar.w - (c - 1)) * ar.h; area != want {
			t.Errorf("%v: area %d want %d", m, area, want)
		}
	}
}

func TestSplitFocusAndMove(t *testing.T) {
	a := splitApp(3)
	a.cur = 1
	a.setSplit(splitGrid)
	if a.panes[0] != 1 || a.panes[1] != 2 || a.panes[2] != 3 || a.panes[3] != 0 {
		t.Fatalf("panes = %v", a.panes)
	}
	if a.current().id != 2 {
		t.Fatalf("focus changed by split")
	}
	a.movePane("right") // 2 → 1 (Umbruch)
	if a.current().id != 1 {
		t.Fatalf("right wraps to slot 0, got %d", a.current().id)
	}
	a.movePane("down") // Slot 0 → Slot 2
	if a.current().id != 3 {
		t.Fatalf("down: got %d", a.current().id)
	}
	a.movePane("right") // Slot 3 leer → übersprungen, bleibt
	if a.current().id != 3 {
		t.Fatalf("empty pane must be skipped, got %d", a.current().id)
	}
	// neue Sitzung landet im leeren Bereich
	a.sessions = append(a.sessions, &session{id: 4, chipArmed: -1})
	a.focusSession(3)
	if a.panes[3] != 4 {
		t.Fatalf("new session should fill empty pane: %v", a.panes)
	}
	// fünfte ersetzt den aktiven Bereich
	a.sessions = append(a.sessions, &session{id: 5, chipArmed: -1})
	a.focusSession(4)
	if a.panes[3] != 5 {
		t.Fatalf("hidden session replaces focused pane: %v", a.panes)
	}
	// Tab schließen: Lücke wird mit verdeckter Sitzung gefüllt
	a.closeTab(a.current())
	if a.panes[3] != 4 || a.current() == nil || a.slotOf(a.current()) < 0 {
		t.Fatalf("after close: panes %v cur %v", a.panes, a.current())
	}
	a.setSplit(splitOff)
	if a.panes != nil || a.slotOf(a.current()) != -1 {
		t.Fatal("split off")
	}
}

func TestSplitTermSize(t *testing.T) {
	a := splitApp(2)
	a.setSplit(splitCols)
	c, r := a.termSize(a.sessions[0])
	if c != 60 || r != a.h-topRows-bottomRows-1 {
		t.Fatalf("pane term size %dx%d", c, r)
	}
	if x := a.termRect(a.sessions[1]).x; x != 61 {
		t.Fatalf("right pane x = %d", x)
	}
	a.sessions[0].panel = panelGoal
	if pw := a.panelWidth(a.sessions[0]); pw != 30 {
		t.Fatalf("panel width in pane = %d", pw)
	}
}

func TestSplitTooSmall(t *testing.T) {
	a := splitApp(4)
	a.w, a.h = 50, 20
	a.setSplit(splitGrid)
	if a.split != splitOff {
		t.Fatal("grid must be refused in tiny window")
	}
	a.cycleSplit()
	if a.split != splitRows {
		t.Fatalf("cycle should skip modes that do not fit, got %v", a.split)
	}
}
