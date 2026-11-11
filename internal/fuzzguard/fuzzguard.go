// Package fuzzguard gibt Fuzz-Tests ein Zeitlimit je Eingabe. go test -fuzz
// kennt keins: eine Eingabe, die den Parser minutenlang beschäftigt, lässt
// den Lauf nur stillstehen und geht als „bestanden“ durch (so blieb die
// unbegrenzte Wiederholung im Terminal-Emulator lange unentdeckt). Mit Run
// schlägt der Test fehl, und go speichert die Eingabe unter testdata/fuzz.
package fuzzguard

import (
	"testing"
	"time"
)

// Limit: so lange darf eine einzelne Eingabe brauchen. Großzügig, damit
// volle CI-Rechner keinen Fehlalarm auslösen; echte Hänger dauern Minuten.
const Limit = 10 * time.Second

// Run führt f aus und bricht den Test ab, wenn es länger als Limit dauert.
func Run(t *testing.T, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(Limit):
		t.Fatalf("Eingabe braucht länger als %s (Hänger)", Limit)
	}
}
