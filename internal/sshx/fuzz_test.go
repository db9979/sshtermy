package sshx

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzPPK: PuTTY-Keys aus fremder Hand (Team-Ordner, Import). Kein Absturz,
// und Argon2 nie mit Werten aus der Datei, die Speicher oder Zeit sprengen.
func FuzzPPK(f *testing.F) {
	for _, n := range []string{"ppk2-ed25519.ppk", "ppk3-ecdsa-enc.ppk", "ppk2-rsa-enc.ppk"} {
		if b, err := os.ReadFile(filepath.Join("testdata", n)); err == nil {
			f.Add(b, "test")
		}
	}
	f.Add([]byte("PuTTY-User-Key-File-3: ssh-ed25519\nEncryption: aes256-cbc\nKey-Derivation: Argon2id\nArgon2-Memory: 4294967295\nArgon2-Passes: 4294967295\nArgon2-Parallelism: 1\nArgon2-Salt: 00\n"), "x")
	f.Fuzz(func(t *testing.T, b []byte, pass string) {
		IsPPK(b)
		pf, err := readPPK(b)
		if err != nil {
			return
		}
		// Teure Schlüsselableitung nur mit kleinen Werten, sonst wird der Fuzzer lahm.
		if pf.version == 3 && pf.enc != "none" && (pf.argMem > 1024 || pf.argPass > 4) && pf.argMem <= ppkMaxArgonMem && pf.argPass <= ppkMaxArgonPasses {
			return
		}
		_, _ = parsePPK(b, []byte(pass))
	})
}
