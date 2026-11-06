package ui

// Bildschirm-Tests („Golden Files“): Jede Ansicht wird in fester Größe
// gezeichnet, ohne Farben in Text umgewandelt und mit der Datei unter
// testdata/golden/ verglichen. So fällt auf, wenn eine neue Funktion eine
// andere Ansicht verschiebt, abschneidet oder kaputtmacht.
//
// Absichtlich geänderte Ansichten neu schreiben:
//
//	make golden                     (oder)
//	go test ./internal/ui -run TestGolden -update
//
// Danach die Änderungen an testdata/golden/*.txt im Diff ansehen und mit
// committen. Bei einem Merge-Konflikt in diesen Dateien einfach nach dem
// Merge neu erzeugen, statt den Konflikt von Hand zu lösen.
//
// Jede Ansicht wird auf Deutsch und Englisch geprüft. Zusätzlich muss jedes
// Bild genau das Fenster füllen: keine Zeile breiter als das Fenster und
// genau so viele Zeilen wie das Fenster hoch ist.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/termemu"
)

var updateGolden = flag.Bool("update", os.Getenv("UPDATE_GOLDEN") != "", "Golden-Dateien in testdata/golden neu schreiben")

// gold ist eine App im Testaufbau: eigener Store-Ordner, feste Daten, keine
// Netzwerkzugriffe (Kommandos aus Update werden verworfen, nicht ausgeführt).
type gold struct {
	t *testing.T
	a *App
}

// goldenConns: feste Beispielverbindungen (Ordner, Tags, Netzwerkgerät,
// Jump-Host), dieselben in jedem Test.
var goldenConns = []domain.Connection{
	{Alias: "web-01", Host: "10.0.0.11", Username: "deploy", Auth: domain.AuthAgent, Group: "Produktion / Web", Tags: []string{"prod", "nginx"}},
	{Alias: "web-02", Host: "10.0.0.12", Username: "deploy", Auth: domain.AuthAgent, Group: "Produktion / Web", Tags: []string{"prod", "nginx"}},
	{Alias: "db-01", Host: "10.0.1.5", Port: 2222, Username: "postgres", Auth: domain.AuthPassword, Password: "geheim-golden", Group: "Produktion", Tags: []string{"prod", "db"}},
	{Alias: "switch-core", Host: "192.168.1.2", Username: "manager", Auth: domain.AuthPassword, Password: "switch-golden", Group: "Netzwerk", Device: domain.DeviceNetwork},
	{Alias: "nas", Host: "nas.local", Username: "admin", Auth: domain.AuthAgent, Tags: []string{"home"}},
	{Alias: "spark", Host: "spark.lan", Username: "dominik", Auth: domain.AuthAgent},
}

// goldenHome: Home-Ordner im Test (muss nicht existieren).
const goldenHome = "/home/golden"

func newGold(t *testing.T, lang string, w, h int, seed bool) *gold {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SSHTERMY_HOME", dir)
	t.Setenv("HOME", goldenHome) // fester Pfad, steht z. B. im Import-Bildschirm
	t.Setenv("SSHTERMY_DEBUG", "")
	prev := i18n.Lang()
	i18n.Set(lang)
	t.Cleanup(func() { i18n.Set(prev) })

	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if seed {
		if err := st.Init([]byte("golden-test"), false); err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateSettings(func(s *domain.Settings) {
			s.TourDone, s.Language, s.ExplainKeys = true, lang, false
		}); err != nil {
			t.Fatal(err)
		}
		llm := st.LLM()
		llm.BaseURL, llm.Model = "http://spark.lan:8000/v1", "qwen3-coder"
		if err := st.SetLLM(llm); err != nil {
			t.Fatal(err)
		}
		ids := map[string]string{}
		for _, c := range goldenConns {
			if c.Alias == "db-01" {
				c.JumpHostID = ids["web-01"]
			}
			c.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			got, err := st.UpsertConnection(c)
			if err != nil {
				t.Fatal(err)
			}
			ids[c.Alias] = got.ID
		}
		if _, err := st.UpsertTunnel(domain.Tunnel{ConnectionID: ids["web-01"], Name: "Grafana", Kind: domain.TunnelLocal, BindHost: "127.0.0.1", BindPort: 3000, TargetHost: "localhost", TargetPort: 3000, WebScheme: "http"}); err != nil {
			t.Fatal(err)
		}
	}
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	a.intro = nil
	g := &gold{t: t, a: a}
	g.send(tea.WindowSizeMsg{Width: w, Height: h})
	if seed {
		g.send(unlockedMsg{})
	}
	return g
}

// send gibt eine Nachricht an die App. Zurückgegebene Kommandos laufen nicht
// (sie würden SSH, KI oder GitHub ansprechen).
func (g *gold) send(m tea.Msg) *gold {
	g.t.Helper()
	g.a.Update(m)
	return g
}

// keys drückt Tasten nacheinander, z. B. "n", "tab", "ctrl+s", "f1".
func (g *gold) keys(ks ...string) *gold {
	g.t.Helper()
	for _, k := range ks {
		g.send(keyMsg(k))
	}
	return g
}

// typ tippt Text Zeichen für Zeichen.
func (g *gold) typ(s string) *gold {
	g.t.Helper()
	for _, r := range s {
		g.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return g
}

var namedKeys = map[string]rune{
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "space": tea.KeySpace,
	"backspace": tea.KeyBackspace, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft,
	"right": tea.KeyRight, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
}

func keyMsg(s string) tea.KeyPressMsg {
	var mod tea.KeyMod
	for {
		switch {
		case strings.HasPrefix(s, "ctrl+") && len(s) > 5:
			mod |= tea.ModCtrl
			s = s[5:]
			continue
		case strings.HasPrefix(s, "shift+") && len(s) > 6:
			mod |= tea.ModShift
			s = s[6:]
			continue
		case strings.HasPrefix(s, "alt+") && len(s) > 4:
			mod |= tea.ModAlt
			s = s[4:]
			continue
		}
		break
	}
	if r, ok := namedKeys[s]; ok {
		return tea.KeyPressMsg{Code: r, Mod: mod}
	}
	var n int
	if _, err := fmt.Sscanf(s, "f%d", &n); err == nil && n >= 1 && n <= 20 && s == fmt.Sprintf("f%d", n) {
		return tea.KeyPressMsg{Code: tea.KeyF1 + rune(n-1), Mod: mod}
	}
	r := []rune(s)[0]
	k := tea.KeyPressMsg{Code: r, Mod: mod}
	if mod == 0 {
		k.Text = s
	}
	return k
}

// session öffnet eine Sitzung ohne echte SSH-Verbindung; out landet im
// Terminal-Emulator, als käme es vom Server.
func (g *gold) session(alias, out string) *session {
	g.t.Helper()
	a := g.a
	var conn domain.Connection
	for _, c := range a.st.Connections() {
		if c.Alias == alias {
			conn = c
		}
	}
	a.nextSID++
	s := &session{id: a.nextSID, conn: conn, state: stConnected, chipArmed: -1, quickArmed: -1}
	s.term = termemu.New(80, 24, discard{})
	a.sessions = append(a.sessions, s)
	a.cur = len(a.sessions) - 1
	a.scr = scrTerminal
	a.resize(s)
	_, _ = s.term.Write([]byte(strings.ReplaceAll(out, "\n", "\r\n")))
	return s
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// screen zeichnet das aktuelle Bild als reinen Text (ohne Farben).
func (g *gold) screen() string {
	v := g.a.View()
	out := ansi.Strip(v.Content)
	out = strings.ReplaceAll(out, runtime.GOOS+"/"+runtime.GOARCH, "linux/amd64") // Über-Dialog
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

// check prüft die Fenstergröße und vergleicht mit der Golden-Datei.
func (g *gold) check(name string) {
	g.t.Helper()
	v := g.a.View()
	if w, h := lipgloss.Width(v.Content), lipgloss.Height(v.Content); w > g.a.w || h != g.a.h {
		g.t.Errorf("%s: Bild ist %dx%d, Fenster %dx%d", name, w, h, g.a.w, g.a.h)
	}
	got := g.screen()
	path := filepath.Join("testdata", "golden", name+".txt")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			g.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			g.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		g.t.Errorf("%s fehlt (neu erzeugen: make golden)\n%s", path, got)
		return
	}
	if string(want) != got {
		writeActual(g.t, name, got)
		g.t.Errorf("%s weicht ab (gewollt? dann: make golden)\n%s", path, lineDiff(string(want), got))
	}
}

// writeActual legt das abweichende Bild unter testdata/golden-actual ab
// (die CI lädt den Ordner als Artefakt hoch).
func writeActual(t *testing.T, name, got string) {
	dir := filepath.Join("testdata", "golden-actual")
	if os.MkdirAll(dir, 0o755) == nil {
		_ = os.WriteFile(filepath.Join(dir, name+".txt"), []byte(got), 0o644)
	}
}

// lineDiff: knapper Zeilenvergleich (− erwartet, + jetzt).
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	shown := 0
	for i := 0; i < max(len(w), len(g)) && shown < 20; i++ {
		var a, c string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			c = g[i]
		}
		if a != c {
			fmt.Fprintf(&b, "Zeile %d\n  − %s\n  + %s\n", i+1, a, c)
			shown++
		}
	}
	return b.String()
}

// goldenCase: eine Ansicht. setup bringt die App dorthin, wie ein Mensch es
// täte (Tasten), oder setzt den Zustand direkt, wo es kein Netz geben soll.
type goldenCase struct {
	name  string
	w, h  int
	empty bool // ohne Store (Ersteinrichtung)
	setup func(g *gold)
}

const termOut = "Linux web-01 6.8.0-45-generic #45-Ubuntu SMP x86_64\n\n" +
	"deploy@web-01:~$ systemctl status nginx --no-pager | head -3\n" +
	"● nginx.service - A high performance web server and a reverse proxy server\n" +
	"     Loaded: loaded (/lib/systemd/system/nginx.service; enabled)\n" +
	"     Active: active (running) since Mon 2026-11-02 08:15:00 UTC\n" +
	"deploy@web-01:~$ "

var goldenCases = []goldenCase{
	{name: "unlock-setup", w: 100, h: 30, empty: true, setup: func(g *gold) {}},
	{name: "unlock", w: 100, h: 30, setup: func(g *gold) {
		g.a.st.Lock()
		g.a.unlock = newUnlock(g.a.st)
		g.a.scr = scrUnlock
	}},
	{name: "manager", w: 120, h: 32, setup: func(g *gold) {}},
	{name: "manager-narrow", w: 80, h: 24, setup: func(g *gold) {}},
	{name: "manager-detail", w: 120, h: 32, setup: func(g *gold) { g.keys("down", "down", "down", "down") }},
	{name: "manager-search", w: 120, h: 32, setup: func(g *gold) { g.keys("/").typ("web") }},
	{name: "manager-empty", w: 100, h: 30, setup: func(g *gold) {
		for _, c := range g.a.st.Connections() {
			_ = g.a.st.DeleteConnection(c.ID)
		}
		g.a.mgr = newManager()
	}},
	{name: "conn-form-new", w: 100, h: 32, setup: func(g *gold) { g.keys("n") }},
	{name: "conn-form-edit", w: 100, h: 32, setup: func(g *gold) { g.keys("down", "down", "down", "down", "e") }},
	{name: "settings", w: 110, h: 36, setup: func(g *gold) { g.keys("s") }},
	{name: "tunnel-form", w: 100, h: 30, setup: func(g *gold) { g.keys("down", "down", "down", "down", "t") }},
	{name: "quick-form", w: 100, h: 30, setup: func(g *gold) { g.keys("down", "down", "down", "down", "q") }},
	{name: "import", w: 100, h: 30, setup: func(g *gold) { g.keys("i") }},
	{name: "keys", w: 110, h: 34, setup: func(g *gold) { g.keys("b") }},
	{name: "broadcast", w: 120, h: 32, setup: func(g *gold) { g.keys("m") }},
	{name: "scan", w: 100, h: 30, setup: func(g *gold) { g.keys("f") }},
	{name: "sync", w: 100, h: 30, setup: func(g *gold) { g.keys("y") }},
	{name: "logs", w: 100, h: 30, setup: func(g *gold) { g.keys("l") }},
	{name: "help-manager", w: 120, h: 36, setup: func(g *gold) { g.keys("f1") }},
	{name: "about", w: 100, h: 30, setup: func(g *gold) { g.keys("f12") }},
	{name: "palette", w: 100, h: 30, setup: func(g *gold) { g.keys("ctrl+space") }},
	{name: "palette-filter", w: 100, h: 30, setup: func(g *gold) { g.keys("ctrl+space").typ("tunnel") }},
	{name: "llm-status", w: 100, h: 30, setup: func(g *gold) { g.a.openLLMStatus() }},
	{name: "tour", w: 100, h: 30, setup: func(g *gold) { g.a.modal = g.a.tourModal() }},
	{name: "hostkey", w: 100, h: 30, setup: func(g *gold) {
		g.session("web-01", "")
		g.a.modal = hostKeyModal(hostKeyAskMsg{host: "web-01 (10.0.0.11:22)", typ: "ssh-ed25519", fp: "SHA256:Golden0123456789abcdefGHIJKLMNOPqrstuvwxyz0"})
	}},
	{name: "crash-report", w: 100, h: 30, setup: func(g *gold) {
		g.a.modal = crashModal(&crash.Report{Title: "Crash: runtime error: index out of range [3] with length 3",
			Path: goldenHome + "/.config/sshtermy/crash/sshtermy-crash-20261106-071500.txt"})
	}},
	{name: "terminal", w: 100, h: 30, setup: func(g *gold) { g.session("web-01", termOut) }},
	{name: "terminal-narrow", w: 80, h: 24, setup: func(g *gold) { g.session("web-01", termOut) }},
	{name: "terminal-network", w: 100, h: 30, setup: func(g *gold) { g.session("switch-core", "switch-core# ") }},
	{name: "terminal-help", w: 120, h: 36, setup: func(g *gold) { g.session("web-01", termOut); g.keys("f1") }},
	{name: "terminal-goal", w: 120, h: 32, setup: func(g *gold) { g.session("web-01", termOut); g.keys("f2") }},
	{name: "terminal-ask", w: 120, h: 32, setup: func(g *gold) { g.session("web-01", termOut); g.keys("f3") }},
	{name: "terminal-tunnels", w: 120, h: 32, setup: func(g *gold) { g.session("web-01", termOut); g.keys("f4") }},
	{name: "terminal-host", w: 120, h: 32, setup: func(g *gold) { g.session("web-01", termOut); g.keys("f6") }},
	{name: "terminal-quick", w: 100, h: 30, setup: func(g *gold) { g.session("web-01", termOut); g.keys("f9") }},
	{name: "terminal-search", w: 100, h: 30, setup: func(g *gold) { g.session("web-01", termOut); g.keys("ctrl+f").typ("nginx") }},
	{name: "terminal-explain-keys", w: 120, h: 30, setup: func(g *gold) { g.session("web-01", termOut); g.keys("shift+f9") }},
	{name: "terminal-split", w: 140, h: 36, setup: func(g *gold) {
		g.session("web-01", termOut)
		g.session("web-02", strings.ReplaceAll(termOut, "web-01", "web-02"))
		g.keys("ctrl+t")
	}},
	{name: "terminal-closed", w: 100, h: 30, setup: func(g *gold) {
		s := g.session("db-01", "")
		s.state, s.err = stClosed, "dial tcp 10.0.1.5:2222: connect: connection refused"
	}},
}

func TestGolden(t *testing.T) {
	if runtime.GOOS != "linux" {
		// Pfade, DPAPI-Hinweis und Tastennamen unterscheiden sich je System;
		// die Bilder gelten für Linux (so läuft auch die CI).
		t.Skip("Golden-Bilder werden unter Linux geprüft")
	}
	for _, lang := range []string{i18n.DE, i18n.EN} {
		for _, c := range goldenCases {
			t.Run(c.name+"."+lang, func(t *testing.T) {
				g := newGold(t, lang, c.w, c.h, !c.empty)
				c.setup(g)
				g.check(c.name + "." + lang)
			})
		}
	}
}
