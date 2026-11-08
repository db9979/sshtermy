package trzsz

import (
	"strings"
	"testing"
)

// FuzzSafeName: Dateinamen vom Server dürfen nie aus dem Zielordner führen.
func FuzzSafeName(f *testing.F) {
	for _, s := range []string{"../../etc/passwd", `..\..\x`, "a/b/c.txt", "..", " . ", "C:\\x", "nul\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		Detect([]byte(name))
		n := SafeName(name)
		if n == "" {
			return
		}
		if strings.ContainsAny(n, `/\:`) || n == "." || n == ".." {
			t.Fatalf("SafeName(%q) = %q", name, n)
		}
		for _, r := range n {
			if r < 0x20 || r == 0x7f {
				t.Fatalf("Steuerzeichen in %q", n)
			}
		}
	})
}
