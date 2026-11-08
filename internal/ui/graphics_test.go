package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
)

func TestBrailleGraph(t *testing.T) {
	g := brailleGraph([]float64{0, 1, 0.5, 0.25}, 2, 2)
	if len(g) != 2 {
		t.Fatalf("Zeilen: %d", len(g))
	}
	for _, l := range g {
		if n := len([]rune(l)); n != 2 {
			t.Fatalf("Breite %d in %q", n, l)
		}
	}
	// Spalte 0: Werte 0 und 1 → rechte Punktspalte voll, linke leer
	if g[0] != "⢸⢀" && !strings.HasPrefix(g[0], string(rune(0x2800+0x08|0x10|0x20|0x80))) {
		t.Errorf("oben: %q", g[0])
	}
	if r := []rune(g[1])[0]; r != 0x2800+0x08+0x10+0x20+0x80 {
		t.Errorf("unten links: %U", r)
	}
	// Wenige Werte stehen rechtsbündig, der Rest bleibt leer
	g = brailleGraph([]float64{1}, 3, 1)
	if r := []rune(g[0]); r[0] != 0x2800 || r[1] != 0x2800 || r[2] == 0x2800 {
		t.Errorf("rechtsbündig: %q", g[0])
	}
	if s := scaleTo([]float64{1, 2, 4}, 1); s[2] != 1 || s[0] != 0.25 {
		t.Errorf("scaleTo: %v", s)
	}
}

func TestLiveStateRings(t *testing.T) {
	var l liveState
	for i := 0; i < liveFineMax+30; i++ {
		l.add(hostinfo.LivePoint{CPU: float64(i%10) / 10, At: time.Now()})
	}
	if len(l.fine) != liveFineMax {
		t.Fatalf("fein: %d", len(l.fine))
	}
	if want := (liveFineMax + 30) / liveCoarseN; len(l.coarse) != want {
		t.Fatalf("grob: %d, erwartet %d", len(l.coarse), want)
	}
	pts, step := l.points()
	if len(pts) != 150 || step != hostinfo.LiveEvery {
		t.Errorf("5 min: %d Punkte, %v", len(pts), step)
	}
	l.hour = true
	if pts, step := l.points(); len(pts) != len(l.coarse) || step != 24*time.Second {
		t.Errorf("1 h: %d Punkte, %v", len(pts), step)
	}
}

func liveSession(a *App) *session {
	s, _ := typedSession(domain.Connection{Alias: "web-01", Host: "10.0.1.11", Username: "deploy"}, "deploy@web-01:~$ ", "")
	srv := hostinfo.Server{CPUs: 16, CPUPct: 32, MemTotal: 64 << 30, MemFree: 20 << 30, DiskTotal: 500 << 30, DiskUsed: 210 << 30, Uptime: 41 * 24 * time.Hour, Updates: 0, UpdateTool: "apt", At: time.Now()}
	s.host.srv = &srv
	for i := 0; i < 160; i++ {
		f := float64(i)
		s.host.live.add(hostinfo.LivePoint{
			At: time.Now(), CPU: 0.3 + 0.2*sinApprox(f/7), Mem: 0.62 + 0.01*sinApprox(f/13),
			NetRx: 4e6 + 3e6*sinApprox(f/5), NetTx: 6e5, Net: 4.6e6 + 3e6*sinApprox(f/5), Disk: 2e6 * (1 + sinApprox(f/3)), HasDisk: true,
			TopProcs: []hostinfo.LiveProcPct{{Name: "postgres", Pct: 0.21}, {Name: "nginx", Pct: 0.06}, {Name: "node", Pct: 0.04}},
		})
	}
	return s
}

// sinApprox: Sinus ohne math-Import in der Testdatei.
func sinApprox(x float64) float64 {
	for x > 3.14159 {
		x -= 6.28318
	}
	return x - x*x*x/6 + x*x*x*x*x/120
}

func TestHostPanelLive(t *testing.T) {
	a := dangerApp(t)
	a.w, a.h, a.intro = 130, 40, nil
	s := liveSession(a)
	a.sessions = []*session{s}
	a.scr = scrTerminal
	s.panel = panelHost
	a.resize(s)
	v := ansi.Strip(a.View().Content)
	for _, want := range []string{"CPU", "RAM", "Platte", "jetzt", "↓"} {
		if !strings.Contains(v, want) {
			t.Errorf("schmal: %q fehlt", want)
		}
	}
	if !strings.ContainsAny(v, "⣀⣤⣶⣿") {
		t.Error("schmal: keine Braille-Graphen")
	}
	s.panelFocus = true
	a.update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !s.host.live.wide {
		t.Fatal("z schaltet nicht auf groß")
	}
	v = ansi.Strip(a.View().Content)
	if !strings.Contains(v, "Top-Prozesse") || !strings.Contains(v, "postgres") {
		t.Errorf("groß: Prozesse fehlen\n%s", v)
	}
	a.update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if !s.host.live.hour {
		t.Error("t schaltet den Zeitraum nicht um")
	}
	// Terminalgröße bleibt beim Umschalten auf groß gleich
	c1, _ := a.termSize(s)
	s.host.live.wide = false
	c2, _ := a.termSize(s)
	if c1 != c2 {
		t.Errorf("Terminal ändert die Größe: %d → %d", c1, c2)
	}
}

func TestLatencySpark(t *testing.T) {
	a := dangerApp(t)
	sp := ansi.Strip(a.latSpark([]time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 300 * time.Millisecond}))
	if r := []rune(sp); len(r) != 3 || r[2] != '█' || r[0] == '█' {
		t.Errorf("Kurve: %q", sp)
	}
}

func TestFramesStyle(t *testing.T) {
	defer setFrames("")
	setFrames(framesRund)
	if tc, bc := frameCorners(); tc[0] != '╭' || bc[1] != '╯' {
		t.Errorf("rund: %q %q", tc, bc)
	}
	if !shadowsOn() {
		t.Error("rund hat Schatten")
	}
	setFrames(framesFlach)
	if shadowsOn() {
		t.Error("flach hat keinen Schatten")
	}
	if tc, _ := frameCorners(); tc[0] != '┌' {
		t.Errorf("flach: %q", tc)
	}
	// Schatten lässt die Zeichen darunter stehen
	setFrames("")
	a := dangerApp(t)
	a.w, a.h = 40, 10
	base := strings.TrimSuffix(strings.Repeat(strings.Repeat("x", 40)+"\n", 10), "\n")
	out := ansi.Strip(a.overlay(base, "┌──┐\n│ab│\n└──┘"))
	if strings.Count(out, "x") != 400-12 {
		t.Errorf("Zeichen unter dem Schatten verloren: %d x", strings.Count(out, "x"))
	}
}

func TestSplitGlide(t *testing.T) {
	a := dangerApp(t)
	a.w, a.h, a.intro = 120, 30, nil
	s1, _ := typedSession(domain.Connection{Alias: "web-01", Host: "10.0.1.11", Username: "deploy"}, "$ ", "")
	s2, _ := typedSession(domain.Connection{Alias: "db-01", Host: "10.0.1.20", Username: "pg"}, "$ ", "")
	s2.id = 2
	a.sessions = []*session{s1, s2}
	a.scr = scrTerminal
	a.setSplit(splitCols)
	a.trackFocusGlide(a.current())
	if a.gliding(time.Now()) {
		t.Fatal("ohne Wechsel kein Rahmen")
	}
	a.focusSession(1)
	a.trackFocusGlide(a.current())
	if !a.gliding(time.Now()) {
		t.Fatal("Fokuswechsel startet keinen Rahmen")
	}
	if d := a.animNeed(time.Now()); d == 0 || d > animFast {
		t.Errorf("Takt beim Gleiten: %v", d)
	}
	a.sfx.at = time.Now().Add(-time.Second) // Rahmen fertig, Linie frei
	v := ansi.Strip(a.View().Content)
	if !strings.Contains(v, "┃") {
		t.Error("Trennlinie neben dem aktiven Bereich leuchtet nicht")
	}
	a.sfx.at = time.Now().Add(-time.Second)
	a.toggleZoom()
	if !a.gliding(time.Now()) {
		t.Error("Zoom ohne Rahmen")
	}
}

func TestGraphicsShots(t *testing.T) {
	dir := os.Getenv("SSHTERMY_MOTION_SHOTS")
	if dir == "" {
		t.Skip()
	}
	strip := regexp.MustCompile(`\x1b\[\d+z`)
	dump := func(a *App, name string) {
		_ = os.WriteFile(dir+"/"+name+".ansi", []byte(strip.ReplaceAllString(a.View().Content, "")), 0o644)
	}
	a := dangerApp(t)
	a.w, a.h, a.intro = 130, 36, nil
	s := liveSession(a)
	s.connectedAt = time.Now().Add(-90 * time.Minute)
	s.latHist = []time.Duration{18, 21, 19, 24, 22, 20, 35, 60, 120, 280, 300, 90, 30, 22, 20, 19}
	for i := range s.latHist {
		s.latHist[i] *= time.Millisecond
	}
	a.sessions = []*session{s}
	a.scr = scrTerminal
	s.panel, s.panelSeen, s.panelAt = panelHost, panelHost, time.Now().Add(-time.Second)
	a.resize(s)
	dump(a, "f6-schmal")
	s.host.live.wide = true
	dump(a, "f6-gross")
	s.host.live.wide, s.panel = false, panelNone
	a.resize(s)
	// Split mit leuchtender Linie und gleitendem Rahmen
	s2, _ := typedSession(domain.Connection{Alias: "db-01", Host: "10.0.1.20", Username: "postgres"}, "postgres@db-01:~$ ", "")
	s2.id = 2
	a.sessions = append(a.sessions, s2)
	a.setSplit(splitCols)
	a.focusSession(0)
	a.trackFocusGlide(a.current())
	a.focusSession(1)
	a.trackFocusGlide(a.current())
	a.sfx.at = time.Now().Add(-60 * time.Millisecond)
	dump(a, "split-gleiten")
	a.sfx.at = time.Time{}
	dump(a, "split")
	// Dialog mit Schatten, rund
	a.setSplit(splitOff)
	setFrames(framesRund)
	a.setStyles(a.s.T)
	defer setFrames("")
	a.terminalKey(s, tea.KeyPressMsg{Code: tea.KeyF1})
	a.openPalette()
	a.View()
	a.pal.mo.queryAt = time.Now().Add(-time.Second)
	a.fx = fadeFx{}
	dump(a, "palette-rund")
}
