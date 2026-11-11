package relay

import (
	"sync"
	"testing"
)

// testLogf: wie t.Logf, schweigt aber nach Testende. Handler und Anker
// laufen in eigenen Goroutinen und loggen sonst noch nach dem Test (Race).
func testLogf(t *testing.T) func(string, ...any) {
	var mu sync.Mutex
	done := false
	t.Cleanup(func() {
		mu.Lock()
		done = true
		mu.Unlock()
	})
	return func(f string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		if !done {
			t.Logf(f, a...)
		}
	}
}
