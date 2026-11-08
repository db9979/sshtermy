package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

func TestParseKeyTolerant(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	h := hex.EncodeToString(priv)
	for name, in := range map[string]string{
		"plain":    h,
		"newline":  h + "\n",
		"wrapped":  h[:70] + "\n" + h[70:],
		"quoted":   `"` + h + `"`,
		"heading":  "UPDATE_SIGNING_KEY (Repository-Secret, geheim halten!):\n" + h,
		"both":     "UPDATE_PUBKEY (Repository-Variable, öffentlich):\n" + hex.EncodeToString(pub) + "\n\nUPDATE_SIGNING_KEY (Repository-Secret, geheim halten!):\n" + h,
		"0xprefix": "0x" + h,
	} {
		k, err := parseKey(in)
		if err != nil || !k.Equal(priv) {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err := parseKey(hex.EncodeToString(pub))
	if err == nil || !strings.Contains(err.Error(), "öffentliche") {
		t.Errorf("Public Key im Secret: %v", err)
	}
	_, err = parseKey(h[:100])
	if err == nil || strings.Contains(err.Error(), h[:20]) || !strings.Contains(err.Error(), "100") {
		t.Errorf("zu kurz: %v", err)
	}
	if _, err := parseKey(""); err == nil {
		t.Error("leer")
	}
}
