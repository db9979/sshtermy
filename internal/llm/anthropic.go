package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Claude über die Anthropic Messages-API (POST /v1/messages). Unterschiede
// zum OpenAI-Format: System-Text steht getrennt, max_tokens ist Pflicht, die
// Rollen müssen sich abwechseln und mit „user“ beginnen.

type anthropicRequest struct {
	Model         string    `json:"model"`
	System        string    `json:"system,omitempty"`
	Messages      []Message `json:"messages"`
	MaxTokens     int       `json:"max_tokens"`
	Temperature   *float64  `json:"temperature,omitempty"`
	Stream        bool      `json:"stream,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type anthropicEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// anthropicMessages trennt System-Text ab und fasst aufeinanderfolgende
// Nachrichten gleicher Rolle zusammen.
func anthropicMessages(msgs []Message) (string, []Message) {
	var sys []string
	var out []Message
	for _, m := range msgs {
		if m.Role == "system" {
			sys = append(sys, m.Content)
			continue
		}
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content += "\n\n" + m.Content
			continue
		}
		out = append(out, Message{Role: role, Content: m.Content})
	}
	if len(out) == 0 || out[0].Role != "user" {
		out = append([]Message{{Role: "user", Content: "…"}}, out...)
	}
	return strings.Join(sys, "\n\n"), out
}

func (c *conn) anthropicBody(msgs []Message, o Opts) anthropicRequest {
	mt, temp := c.params(o)
	if mt <= 0 {
		mt = 1024
	}
	sys, m := anthropicMessages(msgs)
	return anthropicRequest{Model: c.cfg.Model, System: sys, Messages: m, MaxTokens: mt, Temperature: &temp}
}

func (c *conn) anthropicComplete(ctx context.Context, msgs []Message, o Opts) (Result, error) {
	body := c.anthropicBody(msgs, o)
	status, raw, err := c.post(ctx, body)
	if err == nil && rejects(status, raw, "temperature") {
		body.Temperature = nil
		status, raw, err = c.post(ctx, body)
	}
	if err != nil {
		return Result{}, err
	}
	var r anthropicResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf(i18n.T("LLM: HTTP %d, keine JSON-Antwort"), status)
	}
	if r.Error != nil {
		return Result{}, fmt.Errorf("LLM: HTTP %d: %s", status, r.Error.Message)
	}
	if status/100 != 2 {
		return Result{}, fmt.Errorf("LLM: HTTP %d", status)
	}
	if status == http.StatusOK && len(r.Content) == 0 && r.StopReason == "" {
		return Result{}, fmt.Errorf(i18n.T("LLM: HTTP %d, keine JSON-Antwort"), status)
	}
	var b strings.Builder
	for _, p := range r.Content {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	finish := r.StopReason
	switch finish {
	case "max_tokens":
		finish = "length"
	case "end_turn", "stop_sequence":
		finish = "stop"
	}
	return Result{Content: b.String(), FinishReason: finish}, nil
}
