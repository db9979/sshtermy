package termemu

import (
	"io"
	"strings"
	"testing"
	"time"
)

// Zählparameter vom Server dürfen den Emulator nicht festhalten: der größte
// Wert muss so schnell fertig sein wie ein kleiner (Fund des Fuzz-Tests:
// "CSI 2147483647 b" wiederholte das letzte Zeichen zwei Milliarden Mal).
func TestHugeCountsReturnQuickly(t *testing.T) {
	for _, fin := range "bXIZLMPST@ABCDEFGade`" {
		seq := "x\x1b[2147483647" + string(fin)
		tm := New(80, 24, io.Discard)
		done := make(chan struct{})
		go func() {
			_, _ = tm.Write([]byte(seq))
			tm.PlainLines(10)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%q hält den Emulator fest", seq)
		}
		tm.Close()
	}
}

// REP füllt höchstens einen Bildschirm und schreibt, was es soll.
func TestRepeatStillWorks(t *testing.T) {
	tm := New(20, 5, io.Discard)
	defer tm.Close()
	_, _ = tm.Write([]byte("a\x1b[3b"))
	if got := strings.TrimSpace(tm.PlainLines(5)); got != "aaaa" {
		t.Fatalf("REP: %q", got)
	}
}
