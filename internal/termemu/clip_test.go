package termemu

import (
	"io"
	"testing"
)

func TestOSC52(t *testing.T) {
	tm := New(80, 24, io.Discard)
	defer tm.Close()
	_, _ = tm.Write([]byte("\x1b]52;c;aGFsbG8gd2VsdA==\x07"))
	if got := tm.TakeClipboard(); got != "hallo welt" {
		t.Fatalf("Zwischenablage = %q", got)
	}
	if got := tm.TakeClipboard(); got != "" {
		t.Fatalf("zweites Abholen = %q", got)
	}
	// Abfrage wird ignoriert, ST als Abschluss
	_, _ = tm.Write([]byte("\x1b]52;c;?\x1b\\"))
	if got := tm.TakeClipboard(); got != "" {
		t.Fatalf("Abfrage gesetzt: %q", got)
	}
}

func TestParseClipboard(t *testing.T) {
	for in, want := range map[string]string{"52;c;YQ==": "a", "p;YWI": "ab", "c;": "", "c;!!": "", "nothing": ""} {
		got, _ := parseClipboard(in)
		if got != want {
			t.Errorf("%q = %q, will %q", in, got, want)
		}
	}
}
