package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestSplitLayoutsCoverArea(t *testing.T) {
	a := splitApp(6)
	a.w, a.h = 190, 50
	ar := a.areaRect()
	for _, m := range layoutOrder[1:] {
		a.setSplit(m)
		rs := a.paneRects()
		if len(rs) != m.slots() || len(a.panes) != m.slots() {
			t.Fatalf("%v: %d Bereiche, %d Plätze", m, len(rs), len(a.panes))
		}
		c, _ := m.grid()
		area := 0
		for _, r := range rs {
			area += r.w * r.h
			if r.x < 0 || r.x+r.w > a.w || r.y < ar.y || r.y+r.h > ar.y+ar.h {
				t.Errorf("%v: %+v außerhalb", m, r)
			}
		}
		if want := (ar.w - (c - 1)) * ar.h; area != want {
			t.Errorf("%v: Fläche %d, erwartet %d", m, area, want)
		}
	}
}

func TestMoveBoundaryAndEqualize(t *testing.T) {
	a := splitApp(2)
	a.setSplit(splitCols)
	before := a.paneRects()[0].w
	a.resizeFocused("right")
	if w := a.paneRects()[0].w; w != before+2 {
		t.Fatalf("Breite %d, erwartet %d", w, before+2)
	}
	// nie kleiner als minPaneW
	for i := 0; i < 100; i++ {
		a.resizeFocused("left")
	}
	if w := a.paneRects()[0].w; w != minPaneW {
		t.Fatalf("Mindestbreite %d", w)
	}
	// Maus: Linie auf Spalte 80 ziehen
	a.moveBoundary(true, 0, 80-a.boundaryAt(true, 0))
	if x := a.boundaryAt(true, 0); x != 80 {
		t.Fatalf("Linie bei %d", x)
	}
	a.equalize()
	if a.fracX != nil || a.paneRects()[0].w != before {
		t.Fatal("gleich groß klappt nicht")
	}
	// untereinander: Zeilen
	a.setSplit(splitRows)
	h := a.paneRects()[0].h
	a.resizeFocused("down")
	if a.paneRects()[0].h != h+1 {
		t.Fatal("Zeile nicht größer")
	}
}

func TestZoomUsesWholeArea(t *testing.T) {
	a := splitApp(4)
	a.setSplit(splitGrid)
	a.toggleZoom()
	ar := a.areaRect()
	if r := a.termRect(a.current()); r.w != ar.w || r.h != ar.h-1 {
		t.Fatalf("Zoom-Fläche %+v", r)
	}
	if a.paneAt(ar.w-2, ar.y+ar.h-2) != nil {
		t.Fatal("verdeckter Bereich bekommt die Maus")
	}
	a.setSplit(splitCols)
	if a.zoom {
		t.Fatal("Zoom bleibt nach Wechsel der Aufteilung")
	}
}

func TestDragSeparatorAndSwapTitles(t *testing.T) {
	a := splitApp(2)
	a.scr = scrTerminal
	a.setSplit(splitCols)
	x := a.boundaryAt(true, 0)
	y := a.areaRect().y + 5
	a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseMotionMsg{X: x + 10, Y: y, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: x + 10, Y: y, Button: tea.MouseLeft})
	if got := a.boundaryAt(true, 0); got != x+10 {
		t.Fatalf("Linie bei %d, erwartet %d", got, x+10)
	}
	// Titel von Bereich 0 auf Bereich 1 ziehen: tauschen
	rs := a.paneRects()
	p0, p1 := a.panes[0], a.panes[1]
	a.mouse(tea.MouseClickMsg{X: rs[0].x + 3, Y: rs[0].y, Button: tea.MouseLeft})
	a.mouse(tea.MouseMotionMsg{X: rs[1].x + 5, Y: rs[1].y + 3, Button: tea.MouseLeft})
	if a.dropOn != 2 {
		t.Fatalf("Ziel %d", a.dropOn)
	}
	a.mouse(tea.MouseReleaseMsg{X: rs[1].x + 5, Y: rs[1].y + 3, Button: tea.MouseLeft})
	if a.panes[0] != p1 || a.panes[1] != p0 {
		t.Fatalf("nicht getauscht: %v", a.panes)
	}
}

func TestWorkspaceSaveOpenAndFolderGrid(t *testing.T) {
	a := dangerApp(t)
	a.w, a.h = 160, 46
	var conns []domain.Connection
	for _, n := range []string{"web-01", "web-02", "db-01"} {
		c, err := a.st.UpsertConnection(domain.Connection{Alias: n, Host: "127.0.0.1", Port: 1, Username: "u", Auth: domain.AuthAgent, Group: "Prod"})
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	a.openConns(conns, splitMain, []float64{0.6, 0.4}, nil)
	if a.split != splitMain || len(a.sessions) != 3 {
		t.Fatalf("Aufteilung %v, %d Tabs", a.split, len(a.sessions))
	}
	a.saveWorkspace("Web")
	ws := a.st.Settings().Workspaces
	if len(ws) != 1 || ws[0].Layout != "main" || len(ws[0].Conns) != 3 || ws[0].FracX[0] != 0.6 {
		t.Fatalf("gespeichert: %+v", ws)
	}
	// noch einmal öffnen: offene Tabs werden genommen
	a.setSplit(splitOff)
	a.openWorkspace(ws[0])
	if len(a.sessions) != 3 || a.split != splitMain {
		t.Fatalf("wieder geöffnet: %d Tabs, %v", len(a.sessions), a.split)
	}
	// beim Beenden „Zuletzt“
	a.rememberLayout()
	if l := a.st.Settings().LastLayout; l == nil || l.Layout != "main" {
		t.Fatalf("Zuletzt: %+v", l)
	}
	// Ordner als Raster
	a.setSplit(splitOff)
	a.openFolderGrid("Prod")
	if a.split != splitMain {
		t.Fatalf("Ordner mit 3 Verbindungen: %v", a.split)
	}
	if items := a.workspaceItems(); len(items) != 2 || items[0].value != "last" {
		t.Fatalf("Liste: %+v", items)
	}
}

func TestRestoreKeepsSplit(t *testing.T) {
	a := dangerApp(t)
	a.w, a.h = 160, 46
	var ss []*session
	for i, n := range []string{"a", "b", "c"} {
		c, _ := a.st.UpsertConnection(domain.Connection{Alias: n, Host: "127.0.0.1", Port: 1, Username: "u", Auth: domain.AuthAgent})
		ss = append(ss, &session{id: i + 1, conn: c, state: stConnected, chipArmed: -1})
	}
	a.sessions, a.nextSID = ss, 3
	a.setSplit(splitGrid)
	a.swapPanes(0, 2)
	a.fracX = []float64{0.7, 0.3}
	a.saveRestore()
	n, err := New(a.st)
	if err != nil {
		t.Fatal(err)
	}
	n.w, n.h = 160, 46
	n.restoreTabs()
	if n.split != splitGrid || len(n.fracX) != 2 || n.fracX[0] != 0.7 {
		t.Fatalf("Aufteilung %v %v", n.split, n.fracX)
	}
	if s := n.byID(n.panes[0]); s == nil || s.conn.Alias != "c" {
		t.Fatalf("Bereich 0: %+v", n.panes)
	}
	if n.panes[3] != 0 {
		t.Fatalf("leerer Bereich gefüllt: %v", n.panes)
	}
}
