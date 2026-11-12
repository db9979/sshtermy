// Package sharewire: was Gastgeber und Zuschauer beim Teilen einer Sitzung
// gemeinsam sprechen (Code, PAKE, Verschlüsselung, Anmeldung des Zuschauers).
// Ohne Netzwerk-Code, damit es auch als WebAssembly im Browser läuft
// (Zusehen ohne sshTermy, cmd/zusehen-wasm). Der Vermittler und die
// Gastgeber-Seite stehen in internal/relay.
package sharewire

import (
	"crypto/rand"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Rahmenarten (erstes Byte des entschlüsselten Rahmens).
const (
	FrameData = 'd' // Ausgabe des Terminals
	FrameSize = 's' // „Spalten Zeilen“
	FrameEnd  = 'e' // Gastgeber hat beendet
)

var (
	fpRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// IDRE: Kennung einer Freigabe.
	IDRE = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// IsFP: SHA-256-Fingerabdruck (hex, klein)?
func IsFP(s string) bool { return fpRE.MatchString(s) }

// NormBase prüft und vereinheitlicht eine Basis-URL: nur https, ohne
// Abfrage und Fragment, ohne abschließenden Schrägstrich.
func NormBase(s string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return "", i18n.NewError("Adresse des Vermittlers ungültig")
	}
	if u.Scheme != "https" {
		return "", i18n.NewError("Vermittler nur über https")
	}
	if u.User != nil {
		return "", i18n.NewError("Adresse des Vermittlers ohne Benutzer und Passwort")
	}
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

// Seal verschlüsselt einen Rahmen (XChaCha20-Poly1305, id als Zusatzdaten).
func Seal(key []byte, id string, typ byte, payload []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+1+len(payload)+aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	plain := append([]byte{typ}, payload...)
	return aead.Seal(nonce, nonce, plain, []byte(id)), nil
}

// Open entschlüsselt einen Rahmen.
func Open(key []byte, id string, frame []byte) (byte, []byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return 0, nil, err
	}
	if len(frame) < aead.NonceSize()+aead.Overhead()+1 {
		return 0, nil, i18n.NewError("Rahmen zu kurz")
	}
	plain, err := aead.Open(nil, frame[:aead.NonceSize()], frame[aead.NonceSize():], []byte(id))
	if err != nil {
		return 0, nil, i18n.NewError("Rahmen lässt sich nicht entschlüsseln (falscher Link?)")
	}
	return plain[0], plain[1:], nil
}

// Clean: nur druckbare Zeichen, gekürzt (Texte der Gegenseite sind Daten).
func Clean(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= n {
			break
		}
	}
	return b.String()
}
