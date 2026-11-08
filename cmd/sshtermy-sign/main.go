// sshtermy-sign signiert checksums.txt eines Releases mit Ed25519.
//
//	sshtermy-sign keygen            → gibt Public Key (für die Variable UPDATE_PUBKEY)
//	                                  und Private Key (für das Secret UPDATE_SIGNING_KEY) aus
//	UPDATE_SIGNING_KEY=… sshtermy-sign sign dist/checksums.txt
//	                                → schreibt dist/checksums.txt.sig (hex)
//
// Die App prüft die Signatur vor jedem Auto-Update (internal/update).
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "keygen":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		check(err)
		fmt.Println("UPDATE_PUBKEY (Repository-Variable, öffentlich):")
		fmt.Println(hex.EncodeToString(pub))
		fmt.Println("\nUPDATE_SIGNING_KEY (Repository-Secret, geheim halten!):")
		fmt.Println(hex.EncodeToString(priv))
	case "sign":
		if len(os.Args) != 3 {
			usage()
		}
		k, err := parseKey(os.Getenv("UPDATE_SIGNING_KEY"))
		if err != nil {
			fail(err.Error())
		}
		if pub := strings.TrimSpace(os.Getenv("UPDATE_PUBKEY")); pub != "" {
			want := hex.EncodeToString(k.Public().(ed25519.PublicKey))
			if !strings.EqualFold(pub, want) {
				fail("UPDATE_SIGNING_KEY passt nicht zu UPDATE_PUBKEY – beide Werte aus demselben keygen-Lauf eintragen (docs/signatur.md)")
			}
		}
		data, err := os.ReadFile(os.Args[2])
		check(err)
		sig := ed25519.Sign(ed25519.PrivateKey(k), data)
		check(os.WriteFile(os.Args[2]+".sig", []byte(hex.EncodeToString(sig)+"\n"), 0o644))
	default:
		usage()
	}
}

var hexRun = regexp.MustCompile(`[0-9a-fA-F]+`)

// parseKey liest den privaten Schlüssel auch dann, wenn beim Einfügen
// Zeilenumbrüche, Leerzeichen, Anführungszeichen oder die Überschrift aus
// der keygen-Ausgabe mitkamen. Die Fehlermeldung nennt nur Längen, nie den
// Schlüssel selbst.
func parseKey(raw string) (ed25519.PrivateKey, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("UPDATE_SIGNING_KEY ist leer")
	}
	flat := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n', '"', '\'':
			return -1
		}
		return r
	}, raw)
	flat = strings.TrimPrefix(strings.TrimPrefix(flat, "0x"), "0X")
	var lens []string
	for _, run := range hexRun.FindAllString(flat, -1) {
		if len(run) == 2*ed25519.PrivateKeySize {
			k, err := hex.DecodeString(run)
			if err == nil {
				return ed25519.PrivateKey(k), nil
			}
		}
		if len(run) >= 32 {
			lens = append(lens, fmt.Sprint(len(run)))
		}
	}
	switch {
	case len(lens) == 1 && lens[0] == "64":
		return nil, fmt.Errorf("UPDATE_SIGNING_KEY enthält 64 Hex-Zeichen – das ist der öffentliche Schlüssel. Ins Secret gehört der private (128 Zeichen, zweite Zeile von keygen)")
	case len(lens) > 0:
		return nil, fmt.Errorf("UPDATE_SIGNING_KEY ungültig: Hex-Folge mit %s Zeichen gefunden, erwartet 128 (zweite Zeile von keygen)", strings.Join(lens, "/"))
	}
	return nil, fmt.Errorf("UPDATE_SIGNING_KEY ungültig: keine Hex-Folge mit 128 Zeichen gefunden (zweite Zeile von keygen)")
}

func usage() { fail("Aufruf: sshtermy-sign keygen | sign <datei>") }

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(s string) { fmt.Fprintln(os.Stderr, s); os.Exit(1) }
