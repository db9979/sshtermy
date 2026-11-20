package detach

import (
	"bytes"
	"testing"
)

// Rahmen aus der lokalen Verbindung zum abgetrennten Fenster.
func FuzzFrame(f *testing.F) {
	f.Add([]byte{1, 0, 0, 0, 4, 0, 80, 0, 24})
	f.Add([]byte{0, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		r := bytes.NewReader(b)
		for {
			fr, err := readFrame(r)
			if err != nil {
				return
			}
			_, _, _ = parseSize(fr.p)
		}
	})
}
