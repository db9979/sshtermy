package gui

import (
	"io/fs"
	"strings"
)

// droppedPaths: die echten Pfade der abgelegten Dateien. Ebiten liefert ein
// fs.FS; jede abgelegte Datei bzw. jeder Ordner öffnet sich dort als
// *os.File mit dem echten Pfad als Name().
func droppedPaths(fsys fs.FS) []string {
	var out []string
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return nil
		}
		f, err := fsys.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		if n, ok := f.(interface{ Name() string }); ok && n.Name() != "" {
			out = append(out, n.Name())
			if d.IsDir() {
				return fs.SkipDir
			}
		}
		return nil
	})
	return out
}

// quotePaths: wie Windows Terminal, "a" "b" (ui/drop.go versteht das überall).
func quotePaths(paths []string) string {
	q := make([]string, len(paths))
	for i, p := range paths {
		q[i] = `"` + p + `"`
	}
	return strings.Join(q, " ")
}
