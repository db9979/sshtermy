// Package mcp macht sshTermy zum MCP-Server (Model Context Protocol): ein
// KI-Client wie Claude Desktop oder Claude Code startet „sshtermy mcp“ und
// spricht über stdin/stdout JSON-RPC 2.0. Dieser Prozess hält selbst keine
// Daten: er reicht jeden Werkzeugaufruf über einen lokalen Socket an das
// laufende, entsperrte sshTermy weiter (Host/Listen), das entscheidet, ob es
// den Aufruf erlaubt (Einstellung „MCP-Zugriff“, Rückfrage, KI-Datenschutz).
package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/db9979/sshtermy/internal/version"
)

// ProtocolVersion: neueste Fassung, die dieser Server spricht.
const ProtocolVersion = "2025-06-18"

// Tool: Beschreibung für tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

// Tools: was sshTermy anbietet.
var Tools = []Tool{
	{"list_connections", "List the SSH connections saved in sshTermy (alias, host, user, folder, tags, device type, OS). Never includes passwords or keys.",
		obj(map[string]any{"query": str("optional filter on alias, host, folder or tag")})},
	{"list_tabs", "List the terminal tabs currently open in sshTermy with their connection and state.", obj(map[string]any{})},
	{"read_tab", "Read the visible text and recent scrollback of an open tab (secrets are redacted).",
		obj(map[string]any{"tab": num("tab number from list_tabs"), "lines": num("how many lines from the end, default 100, max 1000")}, "tab")},
	{"run_command", "Run a shell command on a saved connection in a separate, invisible session and return its output. Read-only commands run directly; anything that could change the system needs the user's confirmation in sshTermy (or is refused, depending on the user's setting). Network devices (switches/routers) accept their CLI show commands.",
		obj(map[string]any{
			"connection":      str("alias or ID of the connection (see list_connections)"),
			"command":         str("the command line"),
			"timeout_seconds": num("default 60, max 600"),
		}, "connection", "command")},
}

// Backend führt Werkzeuge aus (im Brückenprozess: Weiterleitung an die App).
type Backend interface {
	Call(tool string, args json.RawMessage) (string, error)
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Serve beantwortet JSON-RPC-Nachrichten (eine pro Zeile) bis EOF.
// Werkzeugaufrufe laufen nebenläufig, Antworten werden einzeln geschrieben.
func Serve(in io.Reader, out io.Writer, b Backend) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	var mu sync.Mutex
	var wg sync.WaitGroup
	send := func(r rpcResp) {
		r.JSONRPC = "2.0"
		data, _ := json.Marshal(r)
		mu.Lock()
		defer mu.Unlock()
		out.Write(append(data, '\n'))
	}
	for sc.Scan() {
		var q rpcReq
		if err := json.Unmarshal(sc.Bytes(), &q); err != nil {
			send(rpcResp{ID: json.RawMessage("null"), Error: &rpcErr{-32700, "parse error"}})
			continue
		}
		if len(q.ID) == 0 {
			continue // Benachrichtigung (notifications/initialized, cancelled …)
		}
		switch q.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(q.Params, &p)
			pv := ProtocolVersion
			if p.ProtocolVersion != "" && p.ProtocolVersion < pv {
				pv = p.ProtocolVersion // ältere Fassung des Clients übernehmen
			}
			send(rpcResp{ID: q.ID, Result: map[string]any{
				"protocolVersion": pv,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "sshtermy", "version": version.Version},
				"instructions":    "sshTermy gives access to the user's saved SSH connections. Prefer read-only commands; changes need the user's approval in sshTermy.",
			}})
		case "ping":
			send(rpcResp{ID: q.ID, Result: map[string]any{}})
		case "tools/list":
			send(rpcResp{ID: q.ID, Result: map[string]any{"tools": Tools}})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(q.Params, &p); err != nil || !known(p.Name) {
				send(rpcResp{ID: q.ID, Error: &rpcErr{-32602, "unknown tool"}})
				continue
			}
			if len(p.Arguments) == 0 {
				p.Arguments = json.RawMessage("{}")
			}
			wg.Add(1)
			go func(id json.RawMessage) {
				defer wg.Done()
				text, err := b.Call(p.Name, p.Arguments)
				res := map[string]any{"content": []textContent{{"text", text}}}
				if err != nil {
					res = map[string]any{"content": []textContent{{"text", err.Error()}}, "isError": true}
				}
				send(rpcResp{ID: id, Result: res})
			}(q.ID)
		default:
			send(rpcResp{ID: q.ID, Error: &rpcErr{-32601, fmt.Sprintf("method not found: %s", q.Method)}})
		}
	}
	wg.Wait()
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func known(name string) bool {
	for _, t := range Tools {
		if t.Name == name {
			return true
		}
	}
	return false
}
