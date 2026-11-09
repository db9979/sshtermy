package ui

import (
	"testing"
	"time"
)

func TestGhostMemo(t *testing.T) {
	s := &session{}
	now := time.Now()
	s.ghostRemember("systemctl st", "atus nginx", now)
	s.ghostRemember("lsx", "", now)
	cases := []struct {
		typed, rest string
		hit         bool
	}{
		{"systemctl st", "atus nginx", true},
		{"systemctl stat", "us nginx", true}, // weitergetippt wie vorgeschlagen
		{"systemctl sto", "", false},         // abgewichen: neu fragen
		{"lsx", "", true},                    // „nichts Passendes“ gemerkt
		{"lsx -l", "", false},
	}
	for _, c := range cases {
		rest, hit := s.ghostLookup(c.typed, now.Add(time.Minute))
		if rest != c.rest || hit != c.hit {
			t.Errorf("%q: %q %v, erwartet %q %v", c.typed, rest, hit, c.rest, c.hit)
		}
	}
	if _, hit := s.ghostLookup("systemctl st", now.Add(ghostMemoTTL)); hit {
		t.Error("abgelaufener Eintrag benutzt")
	}
	for i := range ghostMemoMax + 10 {
		s.ghostRemember(string(rune('a'+i%26))+time.Duration(i).String(), "x", now.Add(time.Duration(i)*time.Second))
	}
	if len(s.ghostMemo) > ghostMemoMax {
		t.Fatalf("Speicher wächst: %d", len(s.ghostMemo))
	}
}
