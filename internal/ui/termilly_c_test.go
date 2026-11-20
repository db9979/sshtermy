package ui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/push"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/termemu"
)

func TestOldOutputAndClosedTabs(t *testing.T) {
	a := dangerApp(t)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "web.lan", Username: "root"})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := typedSession(c, "root@web:~# ", "")
	a.sessions = []*session{s}
	a.startLog(s)
	s.log.Add(sessionlog.Entry{At: time.Now(), Command: "uptime", Output: "\x1b[31m 21:03 up 4 days\x1b[0m\n"})
	a.saveLog(s)
	id := s.log.ID

	a.closeTab(s)
	if len(a.sessions) != 0 || len(a.closed) != 1 || a.closed[0].LogID != id {
		t.Fatalf("nicht gemerkt: %+v", a.closed)
	}
	// nach einem Neustart aus der Datei
	a.closed, a.closedLoaded = nil, false
	a.loadClosed()
	if len(a.closed) != 1 {
		t.Fatal("Datei nicht gelesen")
	}
	_ = a.openClosed()
	if a.modal == nil || a.modal.kind != mMenu || a.modal.items[0].label != "web" {
		t.Fatalf("Menü: %+v", a.modal)
	}
	a.modal = nil
	if a.reopenClosed(0) == nil || len(a.sessions) != 1 || len(a.closed) != 0 {
		t.Fatal("nicht wieder geöffnet")
	}
	n := a.sessions[0]
	if !strings.Contains(n.oldOut, "uptime") || strings.Contains(n.oldOut, "[31m") || n.oldLog != id {
		t.Fatalf("alte Ausgabe: %q", n.oldOut)
	}
	n.term = termemu.New(80, 10, discard{})
	n.seedOld()
	txt := n.term.ScreenText()
	if !strings.Contains(txt, "21:03 up 4 days") || !strings.Contains(txt, "Ausgabe vom letzten Mal") {
		t.Fatalf("Bildschirm:\n%s", txt)
	}
	// ohne neuen Befehl zeigt die nächste Sicherung weiter aufs alte Protokoll
	if a.tabRef(n).LogID != id {
		t.Error("LogID verloren")
	}
}

func TestSchedAILine(t *testing.T) {
	cmds := []string{"echo eins", "false", "printf 'a\\nb'"}
	out, _ := exec.Command("sh", "-c", schedAILine(cmds)).CombinedOutput()
	res := schedAISplit(string(out), cmds)
	if res[0].Output != "eins" || res[0].Exit != 0 || res[1].Exit != 1 || res[2].Output != "a\nb" {
		t.Fatalf("%+v\n%s", res, out)
	}
	text, okN, badN := schedAIReport(schedAIMsg{cmds: cmds, sum: "web2 voll",
		outs:     []agent.HostOutput{{Host: "web1"}, {Host: "web2"}, {Host: "web3", Err: "Zeitüberschreitung"}},
		verdicts: []agent.HostVerdict{{Host: "web1", OK: true, Finding: "gut"}, {Host: "web2", Finding: "97 %"}}})
	if okN != 1 || badN != 2 || !strings.Contains(text, "× web2: 97 %") || !strings.Contains(text, "$ false") {
		t.Fatalf("%d %d\n%s", okN, badN, text)
	}
}

func TestScheduleFormAI(t *testing.T) {
	a := dangerApp(t)
	if _, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "web.lan", Username: "root", Group: "Prod"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.UpsertConnection(domain.Connection{Alias: "sw", Host: "sw.lan", Username: "admin", Group: "Prod", Device: domain.DeviceNetwork}); err != nil {
		t.Fatal(err)
	}
	sc := domain.Schedule{Kind: domain.ScheduleAI, Folder: "Prod"}
	if _, err := a.schedSteps(sc); err == nil {
		t.Error("ohne Prüfauftrag gespeichert")
	}
	sc.Prompt = "läuft nginx?"
	if c := a.schedConns(sc); len(c) != 1 || c[0].Alias != "web" {
		t.Errorf("Ziele: %+v", c)
	}
}

func TestPushWhenAway(t *testing.T) {
	a := dangerApp(t)
	got := make(chan push.Message, 4)
	old, oldD := pushSendFn, desktopSend
	pushSendFn = func(_ context.Context, c push.Config, m push.Message) error {
		if c.Kind != push.Ntfy {
			t.Errorf("Art %q", c.Kind)
		}
		got <- m
		return nil
	}
	desktopSend = func(string, string, func()) {}
	defer func() { pushSendFn, desktopSend = old, oldD }()
	s, _ := typedSession(domain.Connection{Alias: "web"}, "", "")
	a.sessions = []*session{s}

	a.sendDesktopFor(s, "sshTermy · web", "make fertig")
	if a.notifyAway() || len(got) != 0 {
		t.Fatal("ohne Einrichtung oder am Rechner nichts schicken")
	}
	_ = a.st.UpdateSettings(func(x *domain.Settings) {
		x.Push = domain.PushConfig{Kind: push.Ntfy, URL: "https://ntfy.example/thema"}
	})
	a.lastKey = time.Now().Add(-pushIdle - time.Second) // lange keine Taste
	a.noticeDone(s, "export PASSWORD=geheim; make", 2*notifyAfter, true, false)
	select {
	case m := <-got:
		if strings.Contains(m.Body, "geheim") || !m.Urgent || !strings.Contains(m.Body, "make") {
			t.Errorf("%+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("keine Handy-Meldung")
	}
}
