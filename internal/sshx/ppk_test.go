package sshx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/domain"
)

// Testdateien mit echtem puttygen 0.81 erzeugt (Passphrase „geheim“).
func TestParsePPK(t *testing.T) {
	for _, typ := range []string{"ed25519", "rsa", "ecdsa"} {
		pubLine, err := os.ReadFile(filepath.Join("testdata", "ppk-"+typ+".pub"))
		if err != nil {
			t.Fatal(err)
		}
		want, _, _, _, err := ssh.ParseAuthorizedKey(pubLine)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range []string{"ppk2", "ppk3"} {
			for _, enc := range []bool{false, true} {
				name := v + "-" + typ
				if enc {
					name += "-enc"
				}
				b, err := os.ReadFile(filepath.Join("testdata", name+".ppk"))
				if err != nil {
					t.Fatal(err)
				}
				if !IsPPK(b) || CheckPPK(b) != nil {
					t.Fatalf("%s: nicht als PPK erkannt", name)
				}
				s, err := parseKey(b, nil)
				if enc {
					var missing *ssh.PassphraseMissingError
					if !errors.As(err, &missing) {
						t.Fatalf("%s: ohne Passphrase: %v", name, err)
					}
					if _, err := parseKey(b, []byte("falsch")); err == nil {
						t.Fatalf("%s: falsche Passphrase angenommen", name)
					}
					s, err = parseKey(b, []byte("geheim"))
				}
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if !bytes.Equal(s.PublicKey().Marshal(), want.Marshal()) {
					t.Fatalf("%s: falscher öffentlicher Schlüssel", name)
				}
				sig, err := s.Sign(nil, []byte("hallo"))
				if err != nil || want.Verify([]byte("hallo"), sig) != nil {
					t.Fatalf("%s: Signatur ungültig: %v", name, err)
				}
			}
		}
	}
}

func TestPPKTampered(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join("testdata", "ppk3-ed25519.ppk"))
	b = bytes.Replace(b, []byte("Comment: test-ed25519"), []byte("Comment: test-ed25518"), 1)
	if _, err := parseKey(b, nil); err == nil {
		t.Fatal("veränderte Datei angenommen")
	}
	if CheckKeyFile(filepath.Join("testdata", "ppk2-rsa-enc.ppk")) != nil {
		t.Fatal("CheckKeyFile lehnt PPK ab")
	}
}

// Weg wie beim Verbinden: Keyfile-Verbindung, Passphrase per Rückfrage.
func TestLoadSignerPPK(t *testing.T) {
	asked := 0
	c := domain.Connection{Auth: domain.AuthKeyFile, KeyPath: filepath.Join("testdata", "ppk3-rsa-enc.ppk")}
	s, err := loadSigner(c, Options{AskSecret: func(string) (string, error) { asked++; return "geheim", nil }})
	if err != nil || s == nil || asked != 1 {
		t.Fatalf("%v, %d Rückfragen", err, asked)
	}
	if s.PublicKey().Type() != "ssh-rsa" {
		t.Fatal(s.PublicKey().Type())
	}
}
