package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthObserve(t *testing.T) {
	observe(Observation{Model: "m", BaseURL: "u", Latency: 300 * time.Millisecond, Quick: true})
	observe(Observation{Model: "m", BaseURL: "u", Latency: 9 * time.Second}) // lange Antwort, keine Reaktionszeit
	last, ok, n := LastHealth()
	if last.Err != nil || last.Latency != 9*time.Second || ok.Latency != 300*time.Millisecond {
		t.Fatalf("last=%+v ok=%+v", last, ok)
	}
	observe(Observation{Model: "m", BaseURL: "u", Err: context.Canceled}) // Abbruch zählt nicht
	if _, _, n2 := LastHealth(); n2 != n {
		t.Fatal("Abbruch wurde gezählt")
	}
	observe(Observation{Model: "m", BaseURL: "u", Err: errors.New("connection refused")})
	if last, ok, _ := LastHealth(); last.Err == nil || ok.Latency != 300*time.Millisecond {
		t.Fatalf("Fehler: last=%+v ok=%+v", last, ok)
	}
}
