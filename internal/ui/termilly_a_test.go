package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/sshx"
)

func TestHistGhost(t *testing.T) {
	a := dangerApp(t)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "web.lan", Username: "root"})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := a.st.NoteCommand(c.ID, "systemctl status nginx"); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := typedSession(c, "root@web:~# ", "sys")
	s.history = []string{"systemctl restart php-fpm", "mysql -pGeheim"}
	if g := a.histGhost(s, "sys"); g != "temctl restart php-fpm" {
		t.Errorf("Tab-Verlauf zuerst: %q", g)
	}
	if g := a.histGhost(s, "systemctl st"); g != "atus nginx" {
		t.Errorf("Verbindungsverlauf: %q", g)
	}
	if g := a.histGhost(s, "mysql"); g != "" {
		t.Errorf("Passwort vorgeschlagen: %q", g)
	}
	// Ende-zu-Ende: Tick setzt den Vorschlag, die KI wird nicht gefragt
	s.ghostSeq = 7
	a.fireHistGhost(s, 7)
	if s.ghost != "temctl restart php-fpm" || !s.ghostHist || a.visibleGhost(s) == "" {
		t.Errorf("Ghost nicht gesetzt: %q %v", s.ghost, s.ghostHist)
	}
	if cmd := a.fireGhost(s, s.ghostSeq); cmd != nil {
		t.Error("KI trotz Vorschlag aus dem Verlauf gefragt")
	}
	_ = a.toggleHistGhost()
	if a.histGhostOn() {
		t.Error("Schalter wirkt nicht")
	}
}

func TestParsePrices(t *testing.T) {
	for in, want := range map[string][3]float64{
		"3 / 15": {3, 15, 1}, "0,8;4": {0.8, 4, 1}, "2": {2, 2, 1}, "": {0, 0, 1}, "x": {0, 0, 0}, "1/2/3": {0, 0, 0},
	} {
		i, o, ok := parsePrices(in)
		if i != want[0] || o != want[1] || ok != (want[2] == 1) {
			t.Errorf("%q: %v %v %v", in, i, o, ok)
		}
	}
	if fmtPrices(3, 15) != "3 / 15" || fmtPrices(0, 0) != "" {
		t.Error(fmtPrices(3, 15))
	}
}

func TestAIUsageView(t *testing.T) {
	a := dangerApp(t)
	if a.aiUsageView(time.Now()) != "" {
		t.Fatal("ohne Anfragen kein Abschnitt")
	}
	llm.OnUsage(llm.Usage{Task: "agent", Model: "m", In: 12000, Out: 800, PriceIn: 3, PriceOut: 15})
	llm.OnUsage(llm.Usage{Task: "ghost", Model: "m", In: 300, Out: 10, Estimate: true})
	v := ansiStrip(a.aiUsageView(time.Now()))
	for _, want := range []string{"heute", "2 Anfragen", "0,05 $", "30 Tage"} {
		if !strings.Contains(v, want) {
			t.Errorf("%q fehlt:\n%s", want, v)
		}
	}
	if err := a.st.SaveAIUsage(time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(a.st.AIUsageSince(time.Now())) != 2 {
		t.Error("Zeilen je Aufgabe erwartet")
	}
}

func TestClipHist(t *testing.T) {
	clipHist.list = nil
	clipRemember("journalctl -u nginx", "")
	clipRemember("export TOKEN=abc123", "")
	clipRemember("-----BEGIN OPENSSH PRIVATE KEY-----\nxx", "")
	clipRemember("df -h", "web")
	clipRemember("journalctl -u nginx", "")
	l := clipEntries()
	if len(l) != 2 || l[0].text != "journalctl -u nginx" || l[1].from != "web" {
		t.Fatalf("Verlauf: %+v", l)
	}
	for i := range clipHistMax + 3 {
		clipRemember(strings.Repeat("x", i+1), "")
	}
	if len(clipEntries()) != clipHistMax {
		t.Error("Verlauf wächst")
	}
	a := dangerApp(t)
	_ = a.openClipHist()
	if a.modal == nil || a.modal.kind != mMenu || len(a.modal.items) != clipHistMax {
		t.Fatal("Menü fehlt")
	}
	clipHist.list = nil
}

func TestWebQuery(t *testing.T) {
	q := webQuery("  nginx: [emerg] bind()\n  failed  password=geheim ")
	if strings.Contains(q, "geheim") || strings.Contains(q, "\n") || !strings.HasPrefix(q, "nginx: [emerg] bind()") {
		t.Errorf("%q", q)
	}
	a := dangerApp(t)
	_ = a.cycleSearchEngine(1)
	if _, e := a.searchEngine(); e.label != "Google" {
		t.Error(e.label)
	}
	m := a.selMenu(nil, "Permission denied", 3, 3)
	if len(m.items) < 2 || !strings.Contains(m.items[1].label, "Web") {
		t.Errorf("%+v", m.items)
	}
}

func TestCwdFromPrompt(t *testing.T) {
	for line, want := range map[string]string{
		"root@web:/var/log# ":    "/var/log",
		"dominik@nas:~/backup$ ": "~/backup",
		"(venv) user@host:~$ ls": "~",
		"user@mac ~/src % ":      "~/src",
		"[root@rhel log]# ":      "",
		"Switch# ":               "",
		"root@web:/srv/a b$ ":    "", // Leerzeichen im Pfad: lieber Home
	} {
		if got := cwdFromPrompt(line); got != want {
			t.Errorf("%q: %q, erwartet %q", line, got, want)
		}
	}
}

func TestStartPathFollowsTerm(t *testing.T) {
	dir := t.TempDir()
	s, _ := typedSession(domain.Connection{}, "", "")
	_, _ = s.term.Write([]byte("\x1b]7;file://host" + dir + "\x07$ "))
	p := startPath(s, sshx.LocalFS{}, false)
	if p.path != dir {
		t.Errorf("Ordner %q, erwartet %q", p.path, dir)
	}
	_, _ = s.term.Write([]byte("\x1b]7;file://host/gibt/es/nicht\x07"))
	if p := startPath(s, sshx.LocalFS{}, false); p.path != (sshx.LocalFS{}).Home() {
		t.Errorf("Rückfall auf Home: %q", p.path)
	}
}
