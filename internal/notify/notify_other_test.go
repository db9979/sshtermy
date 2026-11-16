//go:build !windows && !darwin

package notify

import (
	"errors"
	"strings"
	"testing"
)

func TestSendClickLinux(t *testing.T) {
	old := notifySend
	defer func() { notifySend = old }()
	var calls []string
	notifySend = func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return []byte("default\n"), nil
	}
	clicked := false
	if err := send("t", "b", "Öffnen", func() { clicked = true }); err != nil || !clicked {
		t.Fatalf("Klick nicht gemeldet: %v %v", err, calls)
	}
	if !strings.Contains(calls[0], "--action=default=Öffnen") || !strings.Contains(calls[0], "--wait") {
		t.Fatalf("Aufruf: %v", calls)
	}

	// altes notify-send ohne --action: einfache Meldung, kein Klick
	calls, clicked = nil, false
	notifySend = func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if strings.Contains(calls[len(calls)-1], "--action") {
			return nil, errors.New("Unknown option --action")
		}
		return nil, nil
	}
	if err := send("t", "b", "Open", func() { clicked = true }); err != nil || clicked || len(calls) != 2 {
		t.Fatalf("Rückfall: %v %v %v", err, clicked, calls)
	}
}
