package ui

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestLLMErrHint(t *testing.T) {
	for in, want := range map[string]string{
		"dial tcp 127.0.0.1:11434: connect: connection refused": "lauscht nichts",
		"dial tcp: lookup dgx-spark: no such host":              "nicht auflösbar",
		"context deadline exceeded":                             "Timeout",
		"LLM: HTTP 401":                                         "API-Key",
		"LLM: The model `x` does not exist":                     "Strg+L",
		"LLM: HTTP 404, keine JSON-Antwort":                     "/v1",
	} {
		if got := llmErrHint(in); !strings.Contains(got, want) {
			t.Errorf("%q: %q, erwartet %q", in, got, want)
		}
	}
	e := &url.Error{Op: "Post", URL: "http://x/v1/chat/completions", Err: errors.New("connection refused")}
	if got := llmErrText(e); got != "connection refused" {
		t.Errorf("llmErrText: %q", got)
	}
}
