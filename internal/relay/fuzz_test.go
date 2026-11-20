package relay

import (
	"testing"

	"github.com/db9979/sshtermy/internal/fuzzguard"
)

// Kopplungszeilen und Teilen-Links tippt oder fügt man ein; sie können
// beliebig kaputt sein.
func FuzzParse(f *testing.F) {
	f.Add("sshtermy-pair https://relay.example:8443 abcd 0123")
	f.Add("sshtermy://zusehen/7-taube-kompass-ofen")
	f.Add("")
	f.Add("\x00%%%://")
	f.Fuzz(func(t *testing.T, s string) {
		fuzzguard.Run(t, func() {
			_, _ = ParseLine(s)
			_, _ = ParseShareLink(s)
		})
	})
}
