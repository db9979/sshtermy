package sharewire

import "testing"

func TestWordsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, w := range shareWords {
		if seen[w] || w == "" {
			t.Fatalf("Wort doppelt/leer: %q", w)
		}
		seen[w] = true
	}
}
