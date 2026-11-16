package sshx

import (
	"slices"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Algos: ausgehandelte Verfahren der Verbindung (leer, wenn unbekannt).
func algosOf(sc ssh.Conn) ssh.NegotiatedAlgorithms {
	if m, ok := sc.(ssh.AlgorithmsConnMetadata); ok {
		return m.Algorithms()
	}
	return ssh.NegotiatedAlgorithms{}
}

// weakAlgos: ausgehandelte Verfahren, die x/crypto als unsicher führt
// (SHA-1-Schlüsseltausch, CBC, hmac-sha1, ssh-rsa …). Sie kommen nur über den
// Rückfall für alte Geräte zustande (auth.go, o.legacy).
func weakAlgos(n ssh.NegotiatedAlgorithms) []string {
	old := ssh.InsecureAlgorithms()
	var out []string
	add := func(name string, list []string) {
		if name != "" && slices.Contains(list, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	add(n.KeyExchange, old.KeyExchanges)
	add(n.HostKey, old.HostKeys)
	for _, d := range []ssh.DirectionAlgorithms{n.Read, n.Write} {
		add(d.Cipher, old.Ciphers)
		add(d.MAC, old.MACs)
	}
	return out
}

// WeakNote: Hinweis, wenn die Verbindung alte Verfahren benutzt ("" sonst).
func (c *Client) WeakNote() string {
	if len(c.Weak) == 0 {
		return ""
	}
	return i18n.F("Vorsicht: %s spricht nur alte SSH-Verfahren (%s). Verschlüsselt, aber nicht mehr Stand der Technik – Firmware aktualisieren, wenn möglich.",
		c.Conn.Alias, strings.Join(c.Weak, ", "))
}

// PostQuantum: Schlüsseltausch mit ML-KEM (schützt auch gegen späteres
// Entschlüsseln mitgeschnittener Sitzungen).
func (c *Client) PostQuantum() bool {
	return strings.Contains(c.Algos.KeyExchange, "mlkem") || strings.Contains(c.Algos.KeyExchange, "sntrup")
}
