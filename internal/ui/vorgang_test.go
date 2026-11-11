package ui

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
	"github.com/db9979/sshtermy/internal/termemu"
)

func TestPromptHost(t *testing.T) {
	for line, want := range map[string]string{
		"dominik@web01:~$ ":            "web01",
		"postgres@db02.example.de:~$ ": "db02",
		"core-sw1# ":                   "core-sw1",
		"core-sw1(config-if)# ":        "core-sw1",
		"[admin@MikroTik] > ":          "mikrotik",
		"<HUAWEI>":                     "huawei",
		"PS C:\\Users\\dominik> ":      "",
		"$ ":                           "",
	} {
		if got := promptHost(line); got != want {
			t.Errorf("%q: %q, erwartet %q", line, got, want)
		}
	}
}

func TestFitsSystem(t *testing.T) {
	for _, c := range []struct {
		cmd, kind string
		ok        bool
	}{
		{"show vlan port 1/1/12", safety.Network, true},
		{"ip -br a", safety.Network, false},
		{"sudo systemctl reload postgresql", safety.Network, false},
		{"systemctl status nginx", "unix", true},
		{"show interfaces", "unix", false},
		{"configure terminal", "unix", false},
		{"end", "unix", true}, // „end“ allein ist zu unsicher
		{"Get-Service sshd", "windows", true},
		{"sudo apt update", "windows", false},
		{"Get-Service sshd", "unix", false},
	} {
		if ok, _ := fitsSystem(c.cmd, c.kind); ok != c.ok {
			t.Errorf("%q auf %s: %v, erwartet %v", c.cmd, c.kind, ok, c.ok)
		}
	}
}

// capture: was sshTermy in ein Terminal schreibt (vorgetippte Befehle).
type capture struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.Write(p)
}

func (c *capture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}

// await: das Terminal schreibt über eine Pipe (eigene Goroutine), also kurz
// auf den erwarteten Text warten.
func (c *capture) await(want string) string {
	for i := 0; i < 200 && c.String() != want; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	return c.String()
}

// settle: für „nichts geschrieben“-Prüfungen der Pipe kurz Zeit geben.
func (c *capture) settle() string {
	time.Sleep(30 * time.Millisecond)
	return c.String()
}

func (c *capture) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.b.Reset()
}

// vgSession: Sitzung mit Prompt, die Tastendrücke mitschreibt.
func vgSession(g *gold, alias, out string) (*session, *capture) {
	s := g.session(alias, "")
	c := &capture{}
	s.term = termemu.New(80, 24, c)
	g.a.resize(s)
	_, _ = s.term.Write([]byte(strings.ReplaceAll(out, "\n", "\r\n")))
	return s, c
}

const vgAnswerJSON = `{"antwort": "web02 hängt im falschen VLAN, die Datenbank kennt es noch nicht.",
 "befund": [{"fenster": 1, "text": "Port 1/1/12 ist in VLAN 1"}, {"fenster": 2, "text": "nur 169.254.x"}, {"fenster": 9, "text": "gibt es nicht"}],
 "schritte": [
  {"fenster": 1, "befehl": "show vlan port 1/1/12", "grund": "VLAN prüfen"},
  {"fenster": 2, "befehl": "ip -br a show ens18", "grund": "IP lesen", "merke": {"name": "web-ip", "muster": "(\\d+\\.\\d+\\.\\d+\\.\\d+)/"}},
  {"fenster": 3, "befehl": "grep -c {{web-ip}} /etc/postgresql/16/main/pg_hba.conf", "grund": "Eintrag da?"},
  {"fenster": 0, "system": "lb01", "befehl": "grep web02 /etc/haproxy/haproxy.cfg", "grund": "Load-Balancer"}
 ],
 "fehlt": [{"system": "lb01", "grund": "HAProxy"}]}`

func vgSetup3(t *testing.T) (*gold, [3]*session, [3]*capture) {
	g := newGold(t, i18n.DE, 140, 40, true)
	var ss [3]*session
	var cs [3]*capture
	ss[0], cs[0] = vgSession(g, "switch-core", "switch-core# ")
	ss[1], cs[1] = vgSession(g, "web-01", "deploy@web-01:~$ ")
	ss[2], cs[2] = vgSession(g, "db-01", "postgres@db-01:~$ ")
	g.keys("alt+v") // einrichten: aktueller Tab (3) ist vorausgewählt
	if g.a.vg == nil || g.a.vg.mode != vgSetup {
		t.Fatal("Alt+V öffnet das Einrichten nicht")
	}
	g.typ("web02 in Betrieb nehmen")
	g.keys("tab", "space", "down", "space", "enter")
	if g.a.vg == nil || !g.a.vg.active || len(g.a.vg.members) != 3 {
		t.Fatalf("Vorgang nicht mit 3 Fenstern gestartet: %+v", g.a.vg)
	}
	return g, ss, cs
}

func TestVorgangAskAndInsert(t *testing.T) {
	g, ss, cs := vgSetup3(t)
	a := g.a
	if a.vg.name != "web02 in Betrieb nehmen" {
		t.Fatalf("Ziel: %q", a.vg.name)
	}
	for i, s := range ss {
		if a.winNo(s) != i+1 {
			t.Fatalf("Fensternummer %d: %d", i+1, a.winNo(s))
		}
	}
	wins, _ := a.vgWindows()
	if len(wins) != 3 || wins[0].Kind() != safety.Network {
		t.Fatalf("Material: %d Fenster, erstes %q", len(wins), wins[0].Kind())
	}
	ans, err := agent.ParseMulti(vgAnswerJSON, wins)
	if err != nil {
		t.Fatal(err)
	}
	g.send(vgAnswerMsg{seq: a.vg.seq, ans: ans})
	view := ansi.Strip(g.screen())
	for _, want := range []string{"Vorgang", "web02 in Betrieb nehmen", "show vlan port 1/1/12", "{{web-ip}}", "lb01"} {
		if !strings.Contains(view, want) {
			t.Errorf("Ansicht ohne %q", want)
		}
	}
	if len(a.vg.ans.Findings) != 2 {
		t.Errorf("Befund aus nicht vorhandenem Fenster nicht verworfen: %+v", a.vg.ans.Findings)
	}
	// 2 tippt in Fenster 2 vor, ohne Enter, und holt es nach vorn
	g.keys("2")
	if got := cs[1].await("ip -br a show ens18"); got != "ip -br a show ens18" {
		t.Fatalf("Fenster 2 bekam %q", got)
	}
	if a.current() != ss[1] || a.vg.focus {
		t.Fatal("Zielfenster ist nicht vorn")
	}
	if cs[0].settle() != "" || cs[2].settle() != "" {
		t.Fatal("andere Fenster bekamen etwas")
	}
}

func TestVorgangFahrplanAndGuard(t *testing.T) {
	g, ss, cs := vgSetup3(t)
	a := g.a
	wins, _ := a.vgWindows()
	ans, _ := agent.ParseMulti(vgAnswerJSON, wins)
	g.send(vgAnswerMsg{seq: a.vg.seq, ans: ans})
	g.keys("z")
	if a.vg.mode != vgRun || cs[0].await("show vlan port 1/1/12") != "show vlan port 1/1/12" {
		t.Fatalf("Fahrplan Schritt 1: Modus %v, Fenster 1 %q", a.vg.mode, cs[0].String())
	}
	// Schritt 1 fertig → Schritt 2 im nächsten Takt in Fenster 2
	a.vgCommandDone(ss[0], &cmdRun{cmd: "show vlan port 1/1/12", at: time.Now()}, "1 DEFAULT access", false, agent.Failure{})
	a.vgTick()
	if cs[1].await("ip -br a show ens18") != "ip -br a show ens18" || a.current() != ss[1] {
		t.Fatalf("Schritt 2: %q", cs[1].String())
	}
	// fremder Befehl im selben Fenster zählt nicht als Schritt
	a.vgCommandDone(ss[1], &cmdRun{cmd: "uptime", at: time.Now()}, "up 3 days", false, agent.Failure{})
	if a.vg.steps[1].state != vsTyped {
		t.Fatal("fremder Befehl hat den Schritt abgeschlossen")
	}
	a.vgCommandDone(ss[1], &cmdRun{cmd: "ip -br a show ens18", at: time.Now()}, "ens18 UP 10.20.0.15/24", false, agent.Failure{})
	if a.vg.vals["web-ip"] != "10.20.0.15" {
		t.Fatalf("Wert nicht gemerkt: %v", a.vg.vals)
	}
	// Fenster 3 zeigt inzwischen einen anderen Rechner: Halt, nichts eingefügt
	_, _ = ss[2].term.Write([]byte("ssh db-02\r\npostgres@db-02:~$ "))
	a.vgTick()
	if cs[2].settle() != "" {
		t.Fatalf("trotz anderem Rechner vorgetippt: %q", cs[2].String())
	}
	if a.modal == nil || !strings.Contains(a.modal.title, "Halt") {
		t.Fatalf("kein Halt-Dialog: %+v", a.modal)
	}
	body := ansi.Strip(strings.Join(a.modal.lines, "\n"))
	if !strings.Contains(body, "db-02") || !strings.Contains(body, "nichts eingefügt") {
		t.Fatalf("Halt-Dialog nennt den Grund nicht:\n%s", body)
	}
	// zurück auf db-01: ⏎ im Fahrplan versucht es erneut, Wert ist eingesetzt
	a.modal = nil
	_, _ = ss[2].term.Write([]byte("exit\r\npostgres@db-01:~$ "))
	a.vg.focus = true
	g.keys("enter")
	if got := cs[2].await("grep -c 10.20.0.15 /etc/postgresql/16/main/pg_hba.conf"); got != "grep -c 10.20.0.15 /etc/postgresql/16/main/pg_hba.conf" {
		t.Fatalf("Schritt 3: %q", got)
	}
	// Schritt 4 betrifft ein System, das nicht offen ist
	a.vgCommandDone(ss[2], &cmdRun{cmd: "grep -c 10.20.0.15 /etc/postgresql/16/main/pg_hba.conf", at: time.Now()}, "0", false, agent.Failure{})
	a.vgTick()
	if !strings.Contains(a.vg.why, "lb01") {
		t.Fatalf("fehlendes System nicht gemeldet: %q", a.vg.why)
	}
	if len(a.vg.events) != 4 {
		t.Fatalf("Zeitleiste: %d Einträge", len(a.vg.events))
	}
	md := a.vgRecapText()
	for _, want := range []string{"web02 in Betrieb nehmen", "`show vlan port 1/1/12`", "Fenster 2"} {
		if !strings.Contains(md, want) {
			t.Errorf("Change-Protokoll ohne %q:\n%s", want, md)
		}
	}
}

func TestVorgangDangerNeedsConfirm(t *testing.T) {
	g, _, cs := vgSetup3(t)
	a := g.a
	wins, _ := a.vgWindows()
	ans, err := agent.ParseMulti(`{"antwort": "x", "schritte": [{"fenster": 2, "befehl": "rm -rf /var/www/shop", "grund": "weg"}]}`, wins)
	if err != nil {
		t.Fatal(err)
	}
	if len(ans.Steps[0].Danger) == 0 {
		t.Fatal("Gefahr nicht erkannt")
	}
	g.send(vgAnswerMsg{seq: a.vg.seq, ans: ans})
	g.keys("1")
	if cs[1].settle() != "" {
		t.Fatal("Gefahr beim ersten Druck vorgetippt")
	}
	g.keys("1")
	if cs[1].await("rm -rf /var/www/shop") != "rm -rf /var/www/shop" {
		if a.modal != nil {
			t.Log(ansi.Strip(strings.Join(a.modal.lines, "\n")))
		}
		t.Fatalf("zweiter Druck: %q", cs[1].String())
	}
	if strings.Contains(cs[1].String(), "\r") {
		t.Fatal("Gefahr ohne ⏎ des Menschen ausgeführt")
	}
	// Fahrplan: Gefahr wartet auf ⏎ in der Leiste
	cs[1].Reset()
	_, _ = a.byWin(2).term.Write([]byte("\r\ndeploy@web-01:~$ "))
	a.vg.focus = true
	g.keys("z")
	if a.vg.steps[0].state != vsArmed || cs[1].settle() != "" {
		t.Fatalf("Fahrplan tippte Gefahr ohne Bestätigung vor: %v %q", a.vg.steps[0].state, cs[1].String())
	}
}

func TestVorgangAgentReadsOnlyAlone(t *testing.T) {
	g, _, cs := vgSetup3(t)
	a := g.a
	wins, _ := a.vgWindows()
	ans, _ := agent.ParseMulti(`{"antwort": "x", "schritte": [
		{"fenster": 2, "befehl": "systemctl status nginx --no-pager", "grund": "lesen"},
		{"fenster": 2, "befehl": "sudo systemctl restart nginx", "grund": "ändert"}]}`, wins)
	g.send(vgAnswerMsg{seq: a.vg.seq, ans: ans, plan: true})
	if a.vg.mode != vgLanes {
		t.Fatal("Plan öffnet nicht die Bahnen")
	}
	g.keys("tab", "enter") // Agent an, freigeben
	if got := cs[1].await("systemctl status nginx --no-pager\r"); got != "systemctl status nginx --no-pager\r" {
		t.Fatalf("Lesendes nicht allein ausgeführt: %q", got)
	}
	cs[1].Reset()
	a.vgCommandDone(a.byWin(2), &cmdRun{cmd: "systemctl status nginx --no-pager", at: time.Now()}, "active (running)", false, agent.Failure{})
	_, _ = a.byWin(2).term.Write([]byte("\r\ndeploy@web-01:~$ "))
	a.vgTick()
	if got := cs[1].await("sudo systemctl restart nginx"); got != "sudo systemctl restart nginx" {
		t.Fatalf("Änderung nicht nur vorgetippt: %q", got)
	}
}

func TestVorgangCloseTabForgets(t *testing.T) {
	g, ss, _ := vgSetup3(t)
	a := g.a
	a.closeTab(ss[1])
	if a.vg.member(ss[1].id) != nil || len(a.vg.members) != 2 {
		t.Fatal("geschlossener Tab bleibt im Vorgang")
	}
	// Nummern bleiben: Fenster 3 heißt weiter 3
	if a.winNo(ss[2]) != 3 {
		t.Fatalf("Nummer hat sich verschoben: %d", a.winNo(ss[2]))
	}
}
