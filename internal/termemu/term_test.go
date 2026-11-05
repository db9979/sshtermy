package termemu

import (
	"io"
	"strings"
	"testing"
)

func TestTypedInputAndGhost(t *testing.T) {
	tm := New(60, 5, io.Discard)
	defer tm.Close()
	tm.Write([]byte("\x1b[1;32mdominik@spark\x1b[0m:~$ docker p"))
	typed, ok := tm.TypedInput()
	if !ok || typed != "docker p" {
		t.Fatalf("TypedInput = %q %v", typed, ok)
	}
	v := tm.View("s -a", func(s string) string { return "[" + s + "]" })
	if !strings.Contains(v, "docker p[s -a]") {
		t.Fatalf("Ghost fehlt: %q", strings.Split(v, "\n")[0])
	}
	tm.Write([]byte("\r\nzeile 2\r\n"))
	if !strings.Contains(tm.PlainLines(10), "zeile 2") {
		t.Fatal("PlainLines")
	}
}
