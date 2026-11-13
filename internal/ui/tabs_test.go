package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

func tabsApp(n int) *App {
	a := &App{w: 140, h: 40, s: NewStyles(Nacht), keys: newKeymap(nil)}
	for i := 1; i <= n; i++ {
		a.sessions = append(a.sessions, &session{id: i, conn: domain.Connection{Alias: "host-" + string(rune('a'+i-1))}, state: stClosed, chipArmed: -1})
	}
	a.nextSID = n
	return a
}

func aliases(a *App) string {
	var out []string
	for _, s := range a.sessions {
		out = append(out, s.tabName())
	}
	return strings.Join(out, ",")
}

func TestMoveTabKeepsActive(t *testing.T) {
	a := tabsApp(4)
	a.cur = 1 // host-b
	a.moveTab(a.sessions[1], 1)
	if got := aliases(a); got != "host-a,host-c,host-b,host-d" {
		t.Fatalf("order %s", got)
	}
	if a.current().conn.Alias != "host-b" {
		t.Fatalf("active changed to %s", a.current().conn.Alias)
	}
	a.moveTab(a.sessions[0], -1) // ganz links: bleibt
	a.moveTab(a.sessions[3], 5)  // ganz rechts: bleibt
	if got := aliases(a); got != "host-a,host-c,host-b,host-d" {
		t.Fatalf("edges moved: %s", got)
	}
	a.moveTabTo(3, 0)
	if got := aliases(a); got != "host-d,host-a,host-c,host-b" || a.current().conn.Alias != "host-b" {
		t.Fatalf("moveTabTo: %s cur %s", got, a.current().conn.Alias)
	}
}

func TestCloseTabAskOnlyWhenConnected(t *testing.T) {
	a := tabsApp(3)
	a.closeTabAsk(a.sessions[0], nil) // getrennt: sofort zu
	if len(a.sessions) != 2 || a.modal != nil {
		t.Fatalf("closed tab should close directly: %d tabs, modal %v", len(a.sessions), a.modal)
	}
	a.sessions[0].state = stConnected
	a.closeTabAsk(a.sessions[0], nil)
	if a.modal == nil || a.modal.kind != mConfirm || len(a.sessions) != 2 {
		t.Fatal("connected tab must ask first")
	}
	a.modal.onYes()
	if len(a.sessions) != 1 || a.sessions[0].conn.Alias != "host-c" {
		t.Fatalf("after yes: %s", aliases(a))
	}
}

func TestTabNameAndList(t *testing.T) {
	a := tabsApp(2)
	a.sessions[1].title = "Logs"
	items := a.tabListItems()
	if len(items) != 2 || !strings.Contains(items[1].label, "Logs (host-b)") {
		t.Fatalf("items %+v", items)
	}
	a.openTabList()
	if a.pick == nil || a.pick.sel != 0 {
		t.Fatal("tab list not open on current tab")
	}
	// Ziffer springt (Suchfeld leer)
	a.pickerKey(tea.KeyPressMsg{Code: '2', Text: "2"})
	if a.pick != nil || a.cur != 1 {
		t.Fatalf("digit jump: pick %v cur %d", a.pick, a.cur)
	}
}

func TestTabStripPlusAndClose(t *testing.T) {
	a := tabsApp(3)
	for _, w := range []int{60, 80, 140} {
		a.w, a.cur = w, 1
		plain := ansi.Strip(zoneRe.ReplaceAllString(a.tabStrip(a.w-lipgloss.Width(a.headerLeft())-1), ""))
		if !strings.HasSuffix(plain, "+ ") && !strings.HasSuffix(plain, "+") && !strings.Contains(plain, " + ") {
			t.Errorf("w=%d: kein + am Ende: %q", w, plain)
		}
		if !strings.Contains(plain, "×") {
			t.Errorf("w=%d: kein × am aktiven Tab: %q", w, plain)
		}
		if h := a.header(a.tabStrip(a.w - lipgloss.Width(a.headerLeft()) - 1)); lipgloss.Width(h) > w {
			t.Errorf("w=%d: Kopfzeile %d breit", w, lipgloss.Width(h))
		}
	}
}

// „+“/Alt+N öffnen den Manager; ⏎ startet die Verbindung in einem weiteren
// Tab, Esc führt ohne neuen Tab zurück.
func TestNewTabViaManager(t *testing.T) {
	a := newPaletteTestApp(t)
	if _, err := a.st.AddConnections([]domain.Connection{{Alias: "ki-lab", Host: "10.0.0.9", Username: "root"}}); err != nil {
		t.Fatal(err)
	}
	a.nextSID++
	a.sessions = append(a.sessions, &session{id: a.nextSID, conn: domain.Connection{ID: "alt", Alias: "alt"}, state: stClosed, chipArmed: -1, quickArmed: -1})
	a.scr = scrTerminal
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}

	a.openNewTab()
	if a.scr != scrManager || !a.mgr.forTab {
		t.Fatalf("Manager nicht offen: scr %v forTab %v", a.scr, a.mgr.forTab)
	}
	a.updateManager(esc)
	if a.scr != scrTerminal || len(a.sessions) != 1 {
		t.Fatalf("Esc: scr %v, %d Tabs", a.scr, len(a.sessions))
	}

	a.openNewTab()
	for i, r := range a.mgrRows() {
		if !r.isFolder() && r.conn.Alias == "ki-lab" {
			a.mgr.sel = i
		}
	}
	a.updateManager(enter)
	if len(a.sessions) != 2 || a.current().conn.Alias != "ki-lab" || a.sessions[0].conn.Alias != "alt" {
		t.Fatalf("kein zusätzlicher Tab: %s", aliases(a))
	}
	if a.scr != scrTerminal || a.mgr.forTab {
		t.Fatalf("nach ⏎: scr %v forTab %v", a.scr, a.mgr.forTab)
	}
}
