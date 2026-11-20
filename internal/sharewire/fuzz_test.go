package sharewire

import (
	"bytes"
	"testing"

	"github.com/db9979/sshtermy/internal/fuzzguard"
)

// Rahmen und Einladungen kommen von fremden Rechnern (Zuschauer, Vermittler):
// nichts davon darf sshTermy zum Absturz bringen.
func FuzzOpen(f *testing.F) {
	key := bytes.Repeat([]byte{7}, 32)
	good, _ := Seal(key, "sitzung", 3, []byte("hallo"))
	f.Add(good, "sitzung")
	f.Add([]byte{}, "")
	f.Add(make([]byte, 41), "x")
	f.Fuzz(func(t *testing.T, frame []byte, id string) {
		fuzzguard.Run(t, func() {
			typ, p, err := Open(key, id, frame)
			if err == nil {
				// nur echte Rahmen gehen durch: neu versiegelt wieder lesbar
				again, _ := Seal(key, id, typ, p)
				if t2, p2, err := Open(key, id, again); err != nil || t2 != typ || !bytes.Equal(p2, p) {
					t.Errorf("Rundreise kaputt: %v", err)
				}
			}
		})
	})
}

func FuzzInvite(f *testing.F) {
	f.Add("7-taube-kompass-ofen")
	f.Add("sshtermy://zusehen/7-taube-kompass-ofen")
	f.Add("https://example.org/zusehen/#7 taube")
	f.Add("\x00\xff 🐟")
	f.Fuzz(func(t *testing.T, s string) {
		fuzzguard.Run(t, func() {
			_, _ = ParseInvite(s)
			_, _ = NormBase(s)
			_ = IsFP(s)
			if c := Clean(s, 40); len(c) > 40+4 {
				t.Errorf("Clean zu lang: %d", len(c))
			}
		})
	})
}
