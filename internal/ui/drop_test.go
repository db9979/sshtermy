package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitDropped(t *testing.T) {
	cases := []struct {
		in   string
		win  bool
		want []string
	}{
		{`/Users/me/My\ File.txt /Users/me/b.txt`, false, []string{"/Users/me/My File.txt", "/Users/me/b.txt"}},
		{`'/home/me/My File.txt' '/home/me/b.txt' `, false, []string{"/home/me/My File.txt", "/home/me/b.txt"}},
		{"file:///home/me/My%20File.txt\r\nfile:///tmp/x\n", false, []string{"/home/me/My File.txt", "/tmp/x"}},
		{`"/a b/c"`, false, []string{"/a b/c"}},
		{`"C:\Users\me\My File.txt" "C:\b.txt"`, true, []string{`C:\Users\me\My File.txt`, `C:\b.txt`}},
		{`C:\b.txt \\nas\share\c.iso`, true, []string{`C:\b.txt`, `\\nas\share\c.iso`}},
		{`file:///C:/x/y.txt`, true, []string{`C:/x/y.txt`}},
		// kein Dateipfad: normaler Text
		{`ls -la /tmp`, false, nil},
		{`relative/path`, false, nil},
		{`'/unclosed`, false, nil},
		{``, false, nil},
		{`file://server/share/x`, false, nil},
		{`/tmp`, true, nil}, // unter Windows nicht absolut
	}
	for _, c := range cases {
		if got := splitDropped(c.in, c.win); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitDropped(%q, %v) = %q, want %q", c.in, c.win, got, c.want)
		}
	}
}

func TestDroppedPathsMustExist(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a file.txt")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if filepath.Separator != '/' {
		t.Skip("Pfade im Test sind Unix-Schreibweise")
	}
	if got := droppedPaths("'" + a + "' "); !reflect.DeepEqual(got, []string{a}) {
		t.Fatalf("got %q", got)
	}
	if got := droppedPaths("'" + a + "' " + filepath.Join(dir, "fehlt")); got != nil {
		t.Fatalf("fehlende Datei: got %q", got)
	}
}
