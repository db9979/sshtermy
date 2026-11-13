package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestTmuxScript(t *testing.T) {
	s, ok := tmuxScript("sshtermy", tmuxWheelUp, 3)
	if !ok || !strings.Contains(s, "copy-mode -e -t sshtermy \\; send-keys -X -N 3 -t sshtermy scroll-up") {
		t.Fatalf("Rad hoch: %q", s)
	}
	if !strings.Contains(s, "send-keys -t sshtermy Up Up Up") {
		t.Fatalf("less/vim in tmux brauchen weiter Pfeiltasten: %q", s)
	}
	if _, ok := tmuxScript("a b", tmuxPageUp, 1); ok {
		t.Fatal("unsichere Namen nie ins Skript")
	}
	if s, ok := tmuxScript("", tmuxBottom, 1); !ok || strings.Contains(s, "-t") {
		t.Fatalf("ohne Namen ohne -t: %q", s)
	}
}

// Ohne tmux-Fernsteuerung (kein SSH, kein tmux): nichts abfangen.
func TestTmuxOnlyWhenInTmux(t *testing.T) {
	a := dangerApp(t)
	s, _ := typedSession(domain.Connection{ID: "c1", Alias: "lab"}, "$ ", "")
	a.sessions = []*session{s}
	_, _ = s.term.Write([]byte("\x1b[?1049h"))
	s.mux = muxState{kind: "tmux", name: "sshtermy"}
	if s.inTmux() || a.tmuxWheel(s, true) {
		t.Fatal("ohne SSH-Client kein tmux-Blättern")
	}
}

// Echtes tmux (falls installiert): Rad, Seiten, Anfang, Ende.
func TestTmuxScriptReal(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("kein tmux")
	}
	name := fmt.Sprintf("sshtermy-test-%d", time.Now().UnixNano()%100000)
	if out, err := exec.Command("tmux", "new", "-d", "-s", name, "-x", "80", "-y", "20", "sh -c 'seq 1 300; sleep 30'").CombinedOutput(); err != nil {
		t.Skipf("tmux startet nicht: %v %s", err, out)
	}
	defer func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() }()
	time.Sleep(300 * time.Millisecond)
	state := func() string {
		out, _ := exec.Command("tmux", "display-message", "-p", "-t", name, "#{pane_in_mode} #{scroll_position}").Output()
		return strings.TrimSpace(string(out))
	}
	run := func(op tmuxOp, n int) {
		s, _ := tmuxScript(name, op, n)
		if out, err := exec.Command("sh", "-c", s).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	run(tmuxWheelUp, 3)
	if got := state(); got != "1 3" {
		t.Fatalf("Rad hoch: %q", got)
	}
	run(tmuxTop, 1)
	if got := state(); !strings.HasPrefix(got, "1 ") || got == "1 3" {
		t.Fatalf("Anfang: %q", got)
	}
	run(tmuxBottom, 1)
	if got := state(); got != "0" && !strings.HasPrefix(got, "0") {
		t.Fatalf("Ende: %q", got)
	}
}
