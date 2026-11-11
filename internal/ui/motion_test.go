package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
)

// holdRun: ⏎ zu den Zeitpunkten (ms nach dem Öffnen), Ergebnis: ausgeführt?
func holdRun(at []int) bool {
	t0 := time.Unix(1000, 0)
	h := holdState{last: t0}
	for _, ms := range at {
		if h.press(t0.Add(time.Duration(ms) * time.Millisecond)) {
			return true
		}
	}
	return false
}

// repeats: gehaltene Taste: Druck bei from, erste Wiederholung nach first,
// dann alle 33 ms bis to.
func repeats(from, first, to int) []int {
	out := []int{from}
	for ms := from + first; ms <= to; ms += 33 {
		out = append(out, ms)
	}
	return out
}

func TestHoldToConfirm(t *testing.T) {
	if !holdRun(repeats(800, 500, 2200)) {
		t.Error("1,4 s gehalten, aber nicht ausgeführt")
	}
	if holdRun(repeats(800, 500, 1700)) {
		t.Error("nur 0,9 s gehalten, trotzdem ausgeführt")
	}
	if holdRun([]int{800}) {
		t.Error("ein kurzes ⏎ hat ausgeführt")
	}
	// Taste war beim Öffnen schon unten (das ⏎, das den Dialog öffnete)
	if holdRun(repeats(30, 33, 3000)) {
		t.Error("gehaltenes ⏎ vom Öffnen des Dialogs hat ausgeführt")
	}
	// mehrmals schnell tippen (Gewohnheit) zählt nicht als Halten
	var taps []int
	for ms := 600; ms < 4000; ms += 350 {
		taps = append(taps, ms)
	}
	if holdRun(taps) {
		t.Error("Tippen im 350-ms-Takt hat ausgeführt")
	}
}

func TestHoldDrains(t *testing.T) {
	t0 := time.Unix(1000, 0)
	h := holdState{last: t0}
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	for _, ms := range repeats(500, 500, 1400) {
		h.press(at(ms))
	}
	h.update(at(1700)) // keine Wiederholung mehr: losgelassen
	if h.held {
		t.Fatal("Loslassen nicht erkannt")
	}
	if l := h.level(at(1700)); l <= 0 || l >= 1 {
		t.Fatalf("Füllstand nach dem Loslassen %v", l)
	}
	if l := h.level(at(3000)); l != 0 {
		t.Fatalf("Balken läuft nicht zurück: %v", l)
	}
	if h.active(at(3000)) {
		t.Fatal("Takt läuft nach dem Zurücklaufen weiter")
	}
}

func TestDangerHoldRuns(t *testing.T) {
	a := dangerApp(t)
	prod := domain.Connection{Alias: "web-prod", Host: "10.0.0.5", Username: "admin"}
	s, out := typedSession(prod, "admin@web-prod:~$ ", "reboot")
	a.sessions = []*session{s}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	a.terminalKey(s, enter)
	if a.modal == nil || a.modal.kind != mDanger {
		t.Fatal("kein Gefahren-Dialog")
	}
	// so tun, als wäre der Dialog vor einer Sekunde aufgegangen
	d := a.modal.danger
	d.hold.last = time.Now().Add(-time.Second)
	if !strings.Contains(ansi.Strip(a.dangerView(a.modal)), "halten") {
		t.Error("Dialog erklärt das Halten nicht")
	}
	a.dangerKey(enter)
	d.hold.start = time.Now().Add(-2 * time.Second) // 2 s gehalten
	d.hold.last = time.Now()                        // Tastenwiederholung gerade eben (unter Last kann der Test sonst eine Pause sehen)
	a.dangerKey(enter)
	if a.modal != nil {
		t.Fatal("Dialog nach dem Halten noch offen")
	}
	if !out.has("\r") {
		t.Fatal("Befehl ging nach dem Halten nicht an den Server")
	}
}

func TestMotionLevels(t *testing.T) {
	defer func(m string, acc bool) { motionSetting, accessible = m, acc }(motionSetting, accessible)
	t.Setenv("SSHTERMY_NO_ANIM", "")
	motionSetting, accessible = motionVoll, false
	if motionLevel() != 2 {
		t.Fatal("voll ≠ 2")
	}
	motionSetting = motionDezent
	if motionLevel() != 1 {
		t.Fatal("dezent ≠ 1")
	}
	motionSetting, accessible = motionVoll, true
	if motionLevel() != 0 || animEnabled() {
		t.Fatal("Barrierearm lässt Bewegung an")
	}
	accessible = false
	motionSetting = motionAus
	if animEnabled() {
		t.Fatal("Bewegung aus, aber Popups blenden noch")
	}
}

func TestTabSpinnerAndPulse(t *testing.T) {
	defer func(m string, acc bool) { motionSetting, accessible = m, acc }(motionSetting, accessible)
	t.Setenv("SSHTERMY_NO_ANIM", "")
	motionSetting, accessible = motionVoll, false
	a := dangerApp(t)
	x := &session{conn: domain.Connection{Alias: "db"}, state: stConnecting}
	now := time.Unix(1000, 0)
	g1 := ansi.Strip(a.tabDot(x, now))
	g2 := ansi.Strip(a.tabDot(x, now.Add(animFast)))
	if g1 == g2 || !strings.Contains(strings.Join(spinFrames, ""), g1) {
		t.Fatalf("kein Spinner: %q %q", g1, g2)
	}
	motionSetting = motionAus
	if g := ansi.Strip(a.tabDot(x, now)); g != "◌" {
		t.Fatalf("ohne Bewegung %q statt ◌", g)
	}
	x.state, x.unseen = stConnected, true
	accessible = true
	if g := ansi.Strip(a.tabDot(x, now)); g != "•" {
		t.Fatalf("Barrierearm: neue Ausgabe als %q statt •", g)
	}
}

func TestSparkAndBar(t *testing.T) {
	if s := sparkline([]float64{1, 2, 4, 8}); s != "▂▃▅█" {
		t.Fatalf("sparkline %q", s)
	}
	if w := ansi.StringWidth(smoothBar(0.37, 20, nil, nil)); w != 20 {
		t.Fatalf("Balken %d statt 20 Spalten breit", w)
	}
	if s := ansi.Strip(smoothBar(0.5, 4, nil, nil)); s != "██  " {
		t.Fatalf("halber Balken %q", s)
	}
}

func TestReachDownToast(t *testing.T) {
	a := dangerApp(t)
	saved, err := a.st.AddConnections([]domain.Connection{{Alias: "sw-07", Host: "10.0.0.17", Port: 22, Username: "admin"}})
	if err != nil || len(saved) != 1 {
		t.Fatal(err)
	}
	c := saved[0]
	a.onReach(reachMsg{map[string]reachResult{c.ID: {ok: true, rtt: 3 * time.Millisecond, at: time.Now()}}})
	a.onReach(reachMsg{map[string]reachResult{c.ID: {ok: false, err: "timeout", at: time.Now()}}})
	if len(a.toasts) == 0 || !strings.Contains(a.toasts[len(a.toasts)-1].text, "sw-07") {
		t.Fatalf("keine Meldung für den ausgefallenen Host: %+v", a.toasts)
	}
	if sp := ansi.Strip(a.reachSpark(c.ID)); sp != "█×" {
		t.Fatalf("Verlauf %q", sp)
	}
}

// TestMotionShots: Einzelbilder für Screenshots (SSHTERMY_MOTION_SHOTS=<ordner>).
func TestMotionShots(t *testing.T) {
	dir := os.Getenv("SSHTERMY_MOTION_SHOTS")
	if dir == "" {
		t.Skip()
	}
	a := dangerApp(t)
	a.w, a.h, a.intro = 110, 26, nil
	strip := regexp.MustCompile(`\x1b\[\d+z`)
	dump := func(name string) {
		v := a.View()
		_ = os.WriteFile(dir+"/"+name+".ansi", []byte(strip.ReplaceAllString(v.Content, "")), 0o644)
	}
	prod := domain.Connection{Alias: "web-prod", Host: "10.0.1.11", Username: "deploy", Group: "produktiv"}
	s1, _ := typedSession(prod, "deploy@web-prod:~$ ", "sudo reboot")
	s1.connectedAt = time.Now().Add(-2 * time.Hour)
	s2 := &session{id: 2, conn: domain.Connection{Alias: "db-01", Host: "10.0.1.20", Username: "postgres"}, state: stConnecting}
	s3, _ := typedSession(domain.Connection{Alias: "core-sw-01", Host: "10.0.0.2", Username: "admin"}, "core-sw-01#", "")
	s3.id, s3.unseen = 3, true
	s4, _ := typedSession(domain.Connection{Alias: "build-win", Host: "10.0.3.9", Username: "ci"}, "PS C:\\> ", "")
	s4.id = 4
	s4.flashTab(a.s.T.Danger)
	s4.flashAt = time.Now().Add(-200 * time.Millisecond)
	s4.notice = noticeFail
	a.sessions = []*session{s1, s2, s3, s4}
	dump("tabs")
	a.hov = hoverState{tip: func() string { return a.tabTip(s1) }, x: 40, y: 1, since: time.Now().Add(-time.Second)}
	dump("hover")
	a.hov = hoverState{}
	a.terminalKey(s1, tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.modal != nil && a.modal.danger != nil {
		d := a.modal.danger
		d.hold = holdState{held: true, rep: true, start: time.Now().Add(-700 * time.Millisecond), last: time.Now()}
		dump("gefahr")
	}
	a.modal = nil
	// Verbinden: Karte mit Schritten
	t0 := time.Now().Add(-900 * time.Millisecond)
	s2.connStep, s2.connStepAt = 2, [4]time.Time{t0, t0.Add(11 * time.Millisecond), t0.Add(140 * time.Millisecond)}
	a.cur = 1
	dump("verbinden")
	// KI fragen: denkt nach, dann halb getippte Antwort
	a.cur = 0
	s1.panel, s1.panelSeen = panelAsk, panelAsk
	s1.ask.input = newAIInput("")
	s1.ask.question, s1.ask.busy = "warum ist das so langsam?", true
	a.resize(s1)
	dump("ki-denkt")
	s1.ask.answer.WriteString("Auf orders fehlt ein Index für created_at, die Tabelle hat 48 Mio. Zeilen. Ein Index hilft:\n\n```sql\nCREATE INDEX CONCURRENTLY idx_orders_created ON orders(created_at);\n```\n")
	s1.ask.shown = 60
	dump("ki-tippt")
	s1.panel = panelNone
	a.resize(s1)
	// Palette: Treffer leuchten
	a.openPalette()
	a.pal.in.SetValue("web")
	a.View()
	a.pal.mo.queryAt = time.Now().Add(-100 * time.Millisecond)
	dump("palette")
	a.pal = nil
	// Manager: Verlauf, atmende Punkte, Lastbalken der verbundenen Verbindung
	saved, err := a.st.AddConnections([]domain.Connection{
		{Alias: "web-prod", Host: "10.0.1.11", Username: "deploy", Port: 22, Group: "produktiv"},
		{Alias: "db-01", Host: "10.0.1.20", Username: "postgres", Port: 22, Group: "produktiv"},
		{Alias: "access-sw-07", Host: "10.0.0.17", Username: "admin", Port: 22, Group: "netzwerk"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		res := map[string]reachResult{}
		for j, c := range saved {
			res[c.ID] = reachResult{ok: !(j == 2 && i == 11), rtt: time.Duration(2+(i*7+j*3)%9) * time.Millisecond, at: time.Now()}
		}
		a.onReach(reachMsg{res})
	}
	a.toasts = nil
	s1.conn = saved[0]
	s1.host.srv = &hostinfo.Server{CPUPct: 41, MemTotal: 100, MemFree: 53, DiskTotal: 100, DiskUsed: 73}
	a.scr = scrManager
	a.mgr.tree = nil
	a.mgr.sel = 1
	a.mgr.selSeen, a.mgr.selAt = 1, time.Now().Add(-time.Second)
	dump("manager")
	a.mgr.sel, a.mgr.selSeen = 4, 4
	dump("manager-web")
}

func TestFuzzyHits(t *testing.T) {
	if h := fuzzyHits("web", "Handbuch (Webseite)"); len(h) != 3 || h[0] != 10 {
		t.Fatalf("Treffer %v", h)
	}
	if h := fuzzyHits("xyz", "web-01"); h != nil {
		t.Fatalf("falscher Treffer %v", h)
	}
	got := ansi.Strip(hitLabel("web-01", fuzzyHits("w0", "web-01"), lipglossPlain(), lipglossPlain(), nil))
	if got != "web-01" {
		t.Fatalf("Beschriftung verändert: %q", got)
	}
}

func TestTypeOnAndReveal(t *testing.T) {
	defer func(m string, acc bool) { motionSetting, accessible = m, acc }(motionSetting, accessible)
	t.Setenv("SSHTERMY_NO_ANIM", "")
	motionSetting, accessible = motionVoll, false
	var q askState
	q.answer.WriteString(strings.Repeat("Wort ", 40))
	q.typeOn(time.Now())
	if q.shown == 0 || q.shown >= 200 || !q.typing() {
		t.Fatalf("Schreibmaschine: %d von 200", q.shown)
	}
	for i := 0; i < 100 && q.typing(); i++ {
		q.typeOn(time.Now())
	}
	if q.typing() || q.visibleAsk() != strings.TrimSpace(q.answer.String()) && !strings.HasPrefix(q.answer.String(), q.visibleAsk()) {
		t.Fatalf("Antwort nicht vollständig: %d", q.shown)
	}
	s := &session{revealAt: time.Now()}
	body := "a\nb\nc\nd\ne\nf\ng\nh"
	if got := revealLines(s, body, 8, time.Now()); strings.Count(got, "\n") != 7 || got == body {
		t.Fatalf("Einblenden: %q", got)
	}
	if got := revealLines(s, body, 8, time.Now().Add(time.Second)); got != body {
		t.Fatal("nach dem Einblenden fehlt etwas")
	}
	motionSetting = motionAus
	q2 := askState{}
	q2.answer.WriteString("fertig")
	q2.typeOn(time.Now())
	if q2.typing() || q2.visibleAsk() != "fertig" {
		t.Fatal("ohne Bewegung nicht sofort vollständig")
	}
}

func TestConnectCardSteps(t *testing.T) {
	a := dangerApp(t)
	t0 := time.Now().Add(-time.Second)
	s := &session{conn: domain.Connection{Alias: "db-01", Host: "10.0.1.20", Username: "pg"}, state: stConnecting,
		connStep: 2, connStepAt: [4]time.Time{t0, t0.Add(11 * time.Millisecond), t0.Add(140 * time.Millisecond)}}
	card := ansi.Strip(a.connectCard(s, 60))
	for _, want := range []string{"db-01", "✓ Verbindung aufbauen", "11 ms", "✓ Host-Schlüssel prüfen", "Anmelden", "○ Shell öffnen"} {
		if !strings.Contains(card, want) {
			t.Fatalf("Karte ohne %q:\n%s", want, card)
		}
	}
}

func lipglossPlain() lipgloss.Style { return lipgloss.NewStyle() }
