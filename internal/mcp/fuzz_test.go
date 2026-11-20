package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/fuzzguard"
)

type nopBackend struct{}

func (nopBackend) Call(tool string, args json.RawMessage) (string, error) { return "ok", nil }

// Anfragen kommen von einem fremden Programm (KI-Werkzeug): jede Zeile muss
// beantwortet oder verworfen werden, ohne Absturz und ohne Hänger.
func FuzzServe(f *testing.F) {
	f.Add(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	f.Add(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_connections"}}`)
	f.Add(`{"jsonrpc":"2.0","id":"x","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":1}}}`)
	f.Add(`{"id":null}`)
	f.Add("\x00{\n}")
	f.Fuzz(func(t *testing.T, in string) {
		fuzzguard.Run(t, func() {
			var out bytes.Buffer
			if err := Serve(strings.NewReader(in), &out, nopBackend{}); err != nil && err != io.EOF {
				return // z. B. Zeile zu lang: Fehler ist erlaubt, Absturz nicht
			}
			for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
				if line != "" && !json.Valid([]byte(line)) {
					t.Errorf("Antwort ist kein JSON: %q", line)
				}
			}
		})
	})
}
