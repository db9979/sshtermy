package notify

import "testing"

func TestClean(t *testing.T) {
	if got := clean("a\nb\x1b[31m  c", 50); got != "a b [31m c" {
		t.Fatalf("clean = %q", got)
	}
	if got := clean("äöüäöü", 4); got != "äöü…" {
		t.Fatalf("kürzen = %q", got)
	}
}

func TestQuote(t *testing.T) {
	if got := quoteAS(`say "hi" \o/`); got != `"say \"hi\" \\o/"` {
		t.Fatalf("AppleScript = %s", got)
	}
	if got := quotePS("it's"); got != "'it''s'" {
		t.Fatalf("PowerShell = %s", got)
	}
	if got := xmlEscape("<a&b>"); got != "&lt;a&amp;b&gt;" {
		t.Fatalf("XML = %s", got)
	}
}
