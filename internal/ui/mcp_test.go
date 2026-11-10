package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/mcp"
)

func mcpCall(a *App, tool, args string) (string, error, tea.Cmd) {
	var text string
	var err error
	got := false
	cmd := a.onMCP(mcpCallMsg{tool: tool, args: json.RawMessage(args), reply: func(t string, e error) { text, err, got = t, e, true }})
	if !got && cmd == nil && a.modal == nil {
		return "", nil, nil
	}
	return text, err, cmd
}

func TestMCPAccess(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal, a.scr = nil, scrManager
	for _, c := range []domain.Connection{
		{Alias: "web1", Host: "10.0.0.5", Username: "root"},
		{Alias: "kunde", Host: "10.9.9.9", Username: "root", Tags: []string{"ki-aus"}},
	} {
		if _, err := a.st.UpsertConnection(c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err, _ := mcpCall(a, "list_connections", `{}`); err == nil {
		t.Fatal("MCP aus muss ablehnen")
	}
	a.st.UpdateSettings(func(s *domain.Settings) { s.MCP = mcpRead })
	out, err, _ := mcpCall(a, "list_connections", `{}`)
	if err != nil || !strings.Contains(out, "web1") || strings.Contains(out, "kunde") || strings.Contains(strings.ToLower(out), "password") {
		t.Fatalf("Liste: %v\n%s", err, out)
	}
	if _, err, _ := mcpCall(a, "run_command", `{"connection":"kunde","command":"uptime"}`); err == nil {
		t.Fatal("ki-aus muss ablehnen")
	}
	if _, err, _ := mcpCall(a, "run_command", `{"connection":"web1","command":"systemctl restart nginx"}`); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("nur lesen: %v", err)
	}
	a.st.UpdateSettings(func(s *domain.Settings) { s.MCP = mcpAsk })
	var declined error
	a.onMCP(mcpCallMsg{tool: "run_command", args: json.RawMessage(`{"connection":"web1","command":"rm -rf /var/www"}`),
		reply: func(_ string, e error) { declined = e }})
	if a.modal == nil || !strings.Contains(a.modal.body, "rm -rf /var/www") || !strings.Contains(a.modal.body, "▲") {
		t.Fatalf("Rückfrage fehlt oder ohne Gefahrenhinweis: %+v", a.modal)
	}
	// gerade aufgegangen: ein getipptes j (aus „journalctl“) bestätigt nichts
	a.modalKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if a.modal == nil || a.modal.yesArmed {
		t.Fatal("Taste direkt nach dem Aufgehen nicht verworfen")
	}
	a.modal.guardUntil = time.Time{}
	a.modalKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if a.modal == nil || !a.modal.yesArmed {
		t.Fatal("Gefahr: erstes j darf noch nicht ausführen")
	}
	a.modalKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if declined == nil || a.modal != nil {
		t.Fatal("Ablehnen nicht gemeldet")
	}
	if _, err, _ := mcpCall(a, "read_tab", `{"tab":99}`); err == nil {
		t.Fatal("unbekannter Tab")
	}
	a.lock = &lockState{}
	if _, err, _ := mcpCall(a, "list_tabs", `{}`); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("gesperrt: %v", err)
	}
}

// Ende zu Ende: Brücke (mcp.Remote) → Socket → App.
func TestMCPBridge(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal, a.scr = nil, scrManager
	a.st.UpsertConnection(domain.Connection{Alias: "nas", Host: "192.168.1.20", Username: "admin"})
	a.st.UpdateSettings(func(s *domain.Settings) { s.MCP = mcpRead })
	a.startMCP()
	if a.mcpHost == nil {
		t.Fatal("kein Socket")
	}
	defer a.stopMCP()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case m := <-a.events:
				if c, ok := m.(mcpCallMsg); ok {
					a.onMCP(c)
				}
			case <-done:
				return
			}
		}
	}()
	r := &mcp.Remote{DataDir: a.st.Dir}
	out, err := r.Call("list_connections", json.RawMessage(`{"query":"nas"}`))
	if err != nil || !strings.Contains(out, "192.168.1.20") {
		t.Fatalf("%v %s", err, out)
	}
}
