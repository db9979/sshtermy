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
		k, err := hex.DecodeString(strings.TrimSpace(os.Getenv("UPDATE_SIGNING_KEY")))
		if err != nil || len(k) != ed25519.PrivateKeySize {
			fail("UPDATE_SIGNING_KEY fehlt oder ist ungültig")
		}
		data, err := os.ReadFile(os.Args[2])
		check(err)
		sig := ed25519.Sign(ed25519.PrivateKey(k), data)
		check(os.WriteFile(os.Args[2]+".sig", []byte(hex.EncodeToString(sig)+"\n"), 0o644))
	default:
		usage()
	}
}

func usage() { fail("Aufruf: sshtermy-sign keygen | sign <datei>") }

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(s string) { fmt.Fprintln(os.Stderr, s); os.Exit(1) }
