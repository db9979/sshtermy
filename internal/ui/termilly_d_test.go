package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

func dockApp(t *testing.T) (*App, *session) {
	t.Helper()
	a := dangerApp(t)
	a.w, a.h = 160, 50
	s, _ := typedSession(domain.Connection{Alias: "web"}, "$ ", "")
	a.sessions, a.cur = []*session{s}, 0
	s.panel = panelTunnels
	return a, s
}

func TestPanelDock(t *testing.T) {
	a, s := dockApp(t)
	r := a.termRect(s)
	if b := a.termBox(s); b.x != r.x || b.w != r.w-a.panelWidth(s) {
		t.Fatalf("rechts: %+v in %+v", b, r)
	}
	_ = a.cycleDock(1) // links
	if a.panelDock(s) != dockLeft {
		t.Fatal(a.panelDock(s))
	}
	pw := a.panelWidth(s)
	if b := a.termBox(s); b.x != r.x+pw || !a.inPanel(s, r.x+1, r.y+2) || a.inTermArea(s, r.x+1, r.y+2) || !a.inTermArea(s, r.x+pw+1, r.y+2) {
		t.Fatalf("links: %+v", b)
	}
	body, _ := a.paneBody(s, true)
	if first := strings.Split(ansiStrip(body), "\n")[0]; !strings.HasPrefix(first, "╭") && !strings.HasPrefix(first, "┌") {
		t.Errorf("Panel nicht links: %q", first)
	}
	_ = a.cycleDock(1) // unten
	ph := a.panelHeight(s)
	if b := a.termBox(s); b.w != r.w || b.h != r.h-ph || a.panelBox(s).y != r.y+r.h-ph {
		t.Fatalf("unten: %+v", b)
	}
	if c, rows := a.paneSize(s); c != r.w || rows != r.h-ph {
		t.Errorf("Terminalgröße %d×%d", c, rows)
	}
	body, _ = a.paneBody(s, true)
	if n := len(strings.Split(body, "\n")); n != r.h {
		t.Errorf("%d Zeilen, erwartet %d", n, r.h)
	}
	// Höhe ziehen
	pb := a.panelBox(s)
	a.mouse(tea.MouseClickMsg{X: pb.x + 5, Y: pb.y, Button: tea.MouseLeft})
	if !a.panelDrag {
		t.Fatal("Rand nicht gegriffen")
	}
	a.mouse(tea.MouseMotionMsg{X: pb.x + 5, Y: pb.y - 4, Button: tea.MouseLeft})
	a.mouse(tea.MouseReleaseMsg{X: pb.x + 5, Y: pb.y - 4, Button: tea.MouseLeft})
	if got := a.st.Settings().PanelH; got != ph+4 {
		t.Errorf("Höhe %d, erwartet %d", got, ph+4)
	}
	// zu wenig Zeilen: wieder rechts
	a.h = 16
	if a.panelDock(s) != dockRight {
		t.Error("unten trotz kleinem Fenster")
	}
	_ = a.resetPanelSize()
	if a.st.Settings().PanelH != 0 {
		t.Error("nicht zurückgesetzt")
	}
}

func TestFilesAndLogBeside(t *testing.T) {
	a, s := dockApp(t)
	a.w = 200
	s.panel = panelNone
	r := a.termRect(s)
	s.files.open = true
	if a.filesBeside(s) {
		t.Fatal("ohne Schalter daneben")
	}
	_ = a.toggleFilesBeside()
	if !a.filesBeside(s) || a.termBox(s).w != r.w-a.filesWidth(s) {
		t.Fatalf("nicht daneben: %+v", a.termBox(s))
	}
	body, _ := a.paneBody(s, true)
	if !strings.Contains(ansiStrip(body), "keine Verbindung") {
		t.Error("Dateien fehlen")
	}
	// Klick ins Terminal: Tasten gehen ans Terminal
	a.mouse(tea.MouseClickMsg{X: r.x + 2, Y: r.y + 2, Button: tea.MouseLeft})
	if s.filesFocus() {
		t.Fatal("Fokus bleibt bei den Dateien")
	}
	_ = a.toggleFiles(s) // F5 holt sie nach vorn
	if !s.filesFocus() {
		t.Fatal("F5 holt die Dateien nicht nach vorn")
	}
	_ = a.toggleFiles(s) // und schließt
	if s.files.open || a.termBox(s).w != r.w {
		t.Fatal("nicht zu")
	}
	// Log-Blick daneben
	a.lscan = logScanState{sid: s.id, conn: s.conn, back: scrTerminal}
	a.scr = scrLogScan
	if !a.logBeside(s) || a.termBox(s).w != r.w-a.filesWidth(s) {
		t.Fatal("Log-Blick nicht daneben")
	}
	if v, _ := a.viewTerminal(); !strings.Contains(ansiStrip(v), "Log-Blick") {
		t.Errorf("Log-Blick fehlt:\n%s", v)
	}
}

func TestPaneStatus(t *testing.T) {
	a, s := dockApp(t)
	s.panel = panelNone
	_, _ = s.term.Write([]byte("\x1b]7;file://h/srv/www/app/current\x07"))
	exit := 2
	s.blocks = append(s.blocks, cmdBlock{cmd: "make", failed: true, exit: &exit, dur: 3 * time.Second})
	if got := a.paneStatus(s, time.Now()); got != "…/app/current · × 2 3 s" {
		t.Errorf("%q", got)
	}
	_ = a.togglePaneStatus()
	if a.paneStatus(s, time.Now()) != "" {
		t.Error("Schalter wirkt nicht")
	}
	if shortDir("~/src") != "~/src" || shortDir("/") != "/" {
		t.Error(shortDir("~/src"))
	}
}
