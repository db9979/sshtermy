package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/llm"
)

// Netplan-Fall vom 2026-11-12: cat ohne sudo scheitert, sudo cat klappt.
func netplanHist() []DoneStep {
	one := 1
	zero := 0
	return []DoneStep{
		{Command: "ip link show | grep -E 'enx|usb'", Output: "55: enx0c3796d031b5: <BROADCAST,MULTICAST>", Exit: &zero},
		{Command: "cat /etc/netplan/*.yaml", Output: "cat: /etc/netplan/00-installer-config.yaml: Permission denied", Exit: &one, ExitHint: "Exit-Code 1, keine Berechtigung"},
		{Command: "sudo cat /etc/netplan/*.yaml", Output: "network:\n  ethernets:\n    enP7s7:\n      dhcp4: true", Exit: &zero},
	}
}

func TestRepeatOf(t *testing.T) {
	h := netplanHist()
	for _, c := range []struct {
		cmd       string
		rep, perm bool
	}{
		{"cat /etc/netplan/*.yaml", true, true},
		{"cat  /etc/netplan/*.yaml", true, true},
		{"cat /etc/netplan/90-NM-83d6a569.yaml", true, true},
		{"cat /etc/netplan/*.yaml | grep -A5 enx", true, true},
		{"sudo cat /etc/netplan/90-NM-83d6a569.yaml", false, false},
		{"ls /etc/netplan", false, false},
		{"cat /etc/hosts", false, false},
		{"ip link show enx0c3796d031b5", false, false},
	} {
		_, perm, rep := repeatOf(c.cmd, h)
		if rep != c.rep || perm != c.perm {
			t.Errorf("%q: rep=%v perm=%v", c.cmd, rep, perm)
		}
	}
	if !sudoWorked(h) {
		t.Fatal("sudo hat geklappt")
	}
}

func fakeLLM(t *testing.T, prompts *[]string, replies ...string) *Agent {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []llm.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		*prompts = append(*prompts, req.Messages[len(req.Messages)-1].Content)
		txt := replies[min(n, len(replies)-1)]
		n++
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"role": "assistant", "content": txt}, "finish_reason": "stop",
		}}})
	}))
	t.Cleanup(srv.Close)
	c, err := llm.New(domain.LLMConfig{BaseURL: srv.URL + "/v1", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	return &Agent{C: c}
}

// Das Modell korrigiert sich nach dem Hinweis; der Prompt zeigt die Ausgaben.
func TestNextStepAvoidsRepeat(t *testing.T) {
	var prompts []string
	a := fakeLLM(t, &prompts,
		`{"befehl": "cat /etc/netplan/90-NM-83d6a569.yaml", "grund": "lesen"}`,
		`{"fertig": true, "grund": "enx ist nicht konfiguriert"}`)
	st, err := a.NextStep(context.Background(), "USB-NIC einrichten", nil, netplanHist(), nil, "", Context{OS: "Ubuntu"})
	if err != nil || !st.Done {
		t.Fatalf("%+v %v", st, err)
	}
	if len(prompts) != 2 || !strings.Contains(prompts[1], "sudo hat in diesem Lauf schon geklappt") {
		t.Fatalf("Nachfrage: %q", prompts)
	}
	if !strings.Contains(prompts[0], "enP7s7") || !strings.Contains(prompts[0], "Permission denied") {
		t.Fatalf("Ausgaben fehlen im Prompt: %s", prompts[0])
	}
}

// Bleibt das Modell stur, setzt der Ziel-Modus sudo davor.
func TestNextStepAddsSudo(t *testing.T) {
	var prompts []string
	a := fakeLLM(t, &prompts, `{"befehl": "cat /etc/netplan/90-NM-83d6a569.yaml", "grund": "lesen"}`)
	st, err := a.NextStep(context.Background(), "x", nil, netplanHist(), nil, "", Context{OS: "Ubuntu"})
	if err != nil || st.Command != "sudo cat /etc/netplan/90-NM-83d6a569.yaml" || !st.Elevated {
		t.Fatalf("%+v %v", st, err)
	}
	if !strings.Contains(st.Reason, "sudo") {
		t.Fatal(st.Reason)
	}
	// Agent-Modus: kein sudo von sich aus, nur Hinweis
	st, err = a.AutoStep(context.Background(), "x", nil, netplanHist(), nil, AutoInfo{}, Context{OS: "Ubuntu"})
	if err != nil || st.Command != "cat /etc/netplan/90-NM-83d6a569.yaml" || !strings.Contains(st.Reason, "fehlgeschlagen") {
		t.Fatalf("%+v %v", st, err)
	}
}

// Ohne Wiederholung keine zusätzliche Anfrage.
func TestNextStepNoRepeatOneCall(t *testing.T) {
	var prompts []string
	a := fakeLLM(t, &prompts, `{"befehl": "sudo cat /etc/netplan/90-NM-83d6a569.yaml", "grund": "lesen"}`)
	if _, err := a.NextStep(context.Background(), "x", nil, netplanHist(), nil, "", Context{OS: "Ubuntu"}); err != nil || len(prompts) != 1 {
		t.Fatal(err, len(prompts))
	}
}
