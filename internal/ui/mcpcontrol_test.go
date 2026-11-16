package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

func mcpCtlApp(t *testing.T, level string) (*App, *session, *syncBuf) {
	t.Helper()
	a := dangerApp(t)
	a.st.UpdateSettings(func(s *domain.Settings) { s.MCP = level })
	c, _ := a.st.UpsertConnection(domain.Connection{Alias: "web1", Host: "10.0.0.5", Username: "root"})
	s, out := typedSession(c, "root@web1:~# ", "")
	a.sessions, a.cur = []*session{s}, 0
	return a, s, out
}

func TestMCPSendInputReadOnlyDirect(t *testing.T) {
	a, s, out := mcpCtlApp(t, mcpAsk)
	text, err, _ := mcpCall(a, "send_input", `{"tab":1,"text":"df -h"}`)
	if err != nil || a.modal != nil {
		t.Fatalf("lesender Befehl nicht direkt: %v %v", err, a.modal)
	}
	if !strings.Contains(text, "typed into tab 1") || !out.has("df -h\r") {
		t.Fatalf("nicht getippt: %q %q", text, out.String())
	}
	if s.run == nil || s.run.cmd != "df -h" {
		t.Fatal("Befehl nicht als Befehl erfasst")
	}
}

func TestMCPSendInputAsks(t *testing.T) {
	a, _, out := mcpCtlApp(t, mcpAsk)
	var got string
	var gotErr error
	a.onMCP(mcpCallMsg{tool: "send_input", args: []byte(`{"tab":1,"text":"rm -rf /var/www"}`), reply: func(t string, e error) { got, gotErr = t, e }})
	if a.modal == nil || !a.modal.twice {
		t.Fatalf("Gefahr ohne doppelte Rückfrage: %+v", a.modal)
	}
	if out.String() != "" {
		t.Fatal("vor der Freigabe getippt")
	}
	a.modal.onCancel()
	if gotErr == nil || got != "" {
		t.Fatal("Ablehnung nicht gemeldet")
	}

	// nur lesen: Tippen abgelehnt
	b, _, _ := mcpCtlApp(t, mcpRead)
	if _, err, _ := mcpCall(b, "send_input", `{"tab":1,"text":"uptime"}`); err == nil {
		t.Fatal("„nur lesen“ darf nicht tippen")
	}
}

func TestMCPSendInputNoPasswords(t *testing.T) {
	a, s, _ := mcpCtlApp(t, mcpAsk)
	s.term.Write([]byte("\r\n[sudo] password for root: "))
	if _, err, _ := mcpCall(a, "send_input", `{"tab":1,"text":"hunter2"}`); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("Passwort getippt: %v", err)
	}
}

func TestMCPWaitTab(t *testing.T) {
	a, s, _ := mcpCtlApp(t, mcpAsk)
	var got string
	done := false
	s.lastOutput = time.Now()
	a.onMCP(mcpCallMsg{tool: "wait_tab", args: []byte(`{"tab":1,"until":"text","text":"fertig"}`), reply: func(t string, _ error) { got, done = t, true }})
	if done || len(a.mcpWaits) != 1 {
		t.Fatal("hat nicht gewartet")
	}
	s.term.Write([]byte("backup fertig\r\nroot@web1:~# "))
	a.mcpWaitTick(time.Now())
	if !done || !strings.Contains(got, "backup fertig") || len(a.mcpWaits) != 0 {
		t.Fatalf("Warten nicht beendet: %q", got)
	}

	// Zeit um: Antwort mit timeout
	done = false
	a.onMCP(mcpCallMsg{tool: "wait_tab", args: []byte(`{"tab":1,"until":"text","text":"nie","timeout_seconds":1}`), reply: func(t string, _ error) { got, done = t, true }})
	a.mcpWaitTick(time.Now().Add(2 * time.Second))
	if !done || !strings.HasPrefix(got, "timeout") {
		t.Fatalf("kein timeout: %q", got)
	}

	// am Prompt
	done = false
	s.lastOutput = time.Now().Add(-3 * time.Second)
	a.onMCP(mcpCallMsg{tool: "wait_tab", args: []byte(`{"tab":1}`), reply: func(t string, _ error) { got, done = t, true }})
	a.mcpWaitTick(time.Now().Add(time.Second))
	if !done || !strings.Contains(got, "status: at prompt") {
		t.Fatalf("Prompt nicht erkannt: %q", got)
	}
}

func TestMCPListTabsStatus(t *testing.T) {
	a, _, _ := mcpCtlApp(t, mcpRead)
	text, err, _ := mcpCall(a, "list_tabs", `{}`)
	if err != nil || !strings.Contains(text, `"status": "at prompt"`) {
		t.Fatalf("%v %s", err, text)
	}
}
