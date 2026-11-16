package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/localsock"
)

type fake struct{}

func (fake) Call(tool string, args json.RawMessage) (string, error) {
	if tool == "run_command" {
		return "", errors.New("vom Nutzer abgelehnt")
	}
	return tool + " " + string(args), nil
}

func TestServe(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_tabs","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"run_command","arguments":{"connection":"a","command":"rm -rf /"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"foo"}`,
		`kaputt`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := Serve(strings.NewReader(in), &out, fake{}); err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]any{}
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(l)
		}
		id, _ := json.Marshal(m["id"])
		byID[string(id)] = m
	}
	if len(byID) != 7 {
		t.Fatalf("Antworten: %d\n%s", len(byID), out.String())
	}
	init := byID["1"]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" {
		t.Errorf("Version: %v", init["protocolVersion"])
	}
	if n := len(byID["2"]["result"].(map[string]any)["tools"].([]any)); n != len(Tools) {
		t.Errorf("tools/list: %d", n)
	}
	if !strings.Contains(out.String(), `"text":"list_tabs {}"`) {
		t.Error("tools/call ohne Ergebnis")
	}
	if r := byID["4"]["result"].(map[string]any); r["isError"] != true {
		t.Errorf("Fehler nicht als isError: %v", r)
	}
	for _, id := range []string{"5", "6", "null"} {
		if byID[id]["error"] == nil {
			t.Errorf("id %s: kein Fehler", id)
		}
	}
}

func TestHostRemote(t *testing.T) {
	dir := t.TempDir()
	r := &Remote{DataDir: dir}
	if _, err := r.Call("list_tabs", json.RawMessage(`{}`)); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("ohne App: %v", err)
	}
	h, err := Listen(dir, func(tool string, args json.RawMessage) (string, error) {
		if tool == "fail" {
			return "", errors.New("nein")
		}
		return "ok:" + tool, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dir, infoFile)); err != nil || (st.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("mcp.json: %v", err)
	}
	got, err := r.Call("list_tabs", json.RawMessage(`{}`))
	if err != nil || got != "ok:list_tabs" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := r.Call("fail", nil); err == nil || err.Error() != "nein" {
		t.Fatalf("Fehler: %v", err)
	}
	// falsches Token: keine Antwort, Verbindung zu
	b, _ := os.ReadFile(filepath.Join(dir, infoFile))
	var in info
	json.Unmarshal(b, &in)
	nc, err := localsock.Dial(in.Addr)
	if err != nil {
		t.Fatal(err)
	}
	nc.Write([]byte("falsch\n{\"id\":1,\"tool\":\"x\"}\n"))
	buf := make([]byte, 10)
	if n, _ := nc.Read(buf); n != 0 {
		t.Fatalf("Antwort trotz falschem Token: %q", buf[:n])
	}
	nc.Close()
	h.Close()
	if _, err := os.Stat(filepath.Join(dir, infoFile)); err == nil {
		t.Fatal("mcp.json bleibt nach Close")
	}
}

// Fassung 2026-07-28: kein initialize, Version je Anfrage in _meta,
// server/discover, resultType und Cache-Felder; unbekannte Version → -32022.
func TestServeStateless(t *testing.T) {
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{` + meta + `}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{` + meta + `}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_tabs","arguments":{},` + meta + `}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := Serve(strings.NewReader(in), &out, fake{}); err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]any{}
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(l)
		}
		id, _ := json.Marshal(m["id"])
		byID[string(id)] = m
	}
	disc := byID["1"]["result"].(map[string]any)
	if vs := disc["supportedVersions"].([]any); len(vs) == 0 || vs[0] != ProtocolVersion || disc["resultType"] != "complete" || disc["cacheScope"] != "private" {
		t.Errorf("server/discover: %v", disc)
	}
	list := byID["2"]["result"].(map[string]any)
	if list["resultType"] != "complete" || list["ttlMs"] == nil || list["_meta"] == nil {
		t.Errorf("tools/list: %v", list)
	}
	if byID["3"]["result"].(map[string]any)["resultType"] != "complete" {
		t.Errorf("tools/call: %v", byID["3"])
	}
	e := byID["4"]["error"].(map[string]any)
	if e["code"].(float64) != -32022 || e["data"].(map[string]any)["requested"] != "1900-01-01" {
		t.Errorf("unbekannte Version: %v", e)
	}
}
