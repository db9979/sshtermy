package sshx

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// Ein Gerät, das nur SHA-1-Schlüsseltausch und CBC kann: sshTermy verbindet
// über den Rückfall und meldet die alten Verfahren.
func TestOldAlgorithmsReported(t *testing.T) {
	fs := startFakeSwitchWith(t, "SSH-2.0-RomSShell_5.40", func(c *ssh.ServerConfig) {
		c.KeyExchanges = []string{ssh.InsecureKeyExchangeDH14SHA1}
		c.Ciphers = []string{ssh.InsecureCipherAES128CBC}
		c.MACs = []string{ssh.HMACSHA1}
	})
	cl := dialFake(t, fs, domain.DeviceNetwork)
	if len(cl.Weak) == 0 {
		t.Fatalf("alte Verfahren nicht erkannt: %+v", cl.Algos)
	}
	note := cl.WeakNote()
	for _, want := range []string{"diffie-hellman-group14-sha1", "aes128-cbc"} {
		if !strings.Contains(note, want) {
			t.Errorf("Hinweis nennt %s nicht: %q", want, note)
		}
	}
	if cl.PostQuantum() {
		t.Error("SHA-1-Schlüsseltausch als quantensicher gemeldet")
	}
}

// Moderner Server: kein Hinweis, Schlüsseltausch quantensicher (Vorgabe von x/crypto).
func TestModernAlgorithmsQuiet(t *testing.T) {
	fs := startFakeSwitch(t, "SSH-2.0-RomSShell_5.40")
	cl := dialFake(t, fs, domain.DeviceNetwork)
	if cl.WeakNote() != "" {
		t.Fatalf("Hinweis bei modernem Server: %q", cl.WeakNote())
	}
	if !cl.PostQuantum() {
		t.Errorf("Schlüsseltausch %q nicht quantensicher", cl.Algos.KeyExchange)
	}
}
