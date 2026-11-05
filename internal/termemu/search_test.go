package termemu

import (
	"io"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

func TestSearchScrollbackAndScreen(t *testing.T) {
	tm := New(30, 3, io.Discard)
	_, _ = tm.Write([]byte("alpha ok\r\nbeta Error here\r\ngamma\r\ndelta error\r\nGrüße error\r\nlast"))
	if tm.ScrollbackLen() == 0 {
		t.Fatal("expected scrollback")
	}
	m := tm.Search("error", false)
	if len(m) != 3 {
		t.Fatalf("smartcase: want 3 matches, got %v", m)
	}
	// Umlaut vor dem Treffer: Spalte muss stimmen (Grüße = 5 Spalten)
	last := m[2]
	if last.Col != 6 || last.Len != 5 {
		t.Fatalf("column mapping wrong: %+v", last)
	}
	if got := tm.LineText(last.Line); got != "Grüße error" {
		t.Fatalf("LineText = %q", got)
	}
	if m := tm.Search("Error", false); len(m) != 1 {
		t.Fatalf("exact case: want 1, got %v", m)
	}
	if m := tm.Search("", false); m != nil {
		t.Fatalf("empty query without filter must not match")
	}
}

func TestErrorFilter(t *testing.T) {
	tm := New(40, 4, io.Discard)
	_, _ = tm.Write([]byte("build ok\r\n0 errors found\r\nE: Unable to locate package foo\r\nPermission denied\r\nall good\r\n"))
	m := tm.Search("", true)
	if len(m) != 2 {
		t.Fatalf("want 2 error lines, got %d: %v", len(m), m)
	}
	if !strings.HasPrefix(tm.LineText(m[0].Line), "E: Unable") {
		t.Fatalf("first error line = %q", tm.LineText(m[0].Line))
	}
	if m := tm.Search("denied", true); len(m) != 1 {
		t.Fatalf("query inside error lines: %v", m)
	}
	for _, c := range []struct {
		s    string
		want bool
	}{
		{"ssh: connect to host x port 22: Connection refused", true},
		{"bash: foo: command not found", true},
		{"Fehler beim Öffnen", true},
		{"compiled without errors", false},
		{"terror", false},
		{"everything fine", false},
	} {
		if got := IsErrorLine(c.s); got != c.want {
			t.Errorf("IsErrorLine(%q) = %v", c.s, got)
		}
	}
}

func TestRenderMarked(t *testing.T) {
	tm := New(20, 2, io.Discard)
	_, _ = tm.Write([]byte("find me"))
	m := tm.Search("me", false)
	if len(m) != 1 {
		t.Fatalf("matches: %v", m)
	}
	plain := tm.Render(0, nil)
	marked := tm.RenderMarked(0, nil, []Mark{{Line: m[0].Line, Col: m[0].Col, Len: m[0].Len, Current: true}},
		MarkStyle{Current: uv.Style{Attrs: uv.AttrReverse}})
	if plain == marked {
		t.Fatal("marked render should differ")
	}
}
