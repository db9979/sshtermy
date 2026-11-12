package sshx

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/i18n"
)

// PuTTY-Schlüsseldateien (.ppk, Version 2 und 3) direkt lesen, damit Keys
// aus PuTTY ohne Pageant und ohne Umwandeln mit PuTTYgen funktionieren.
// Format: https://the.earth.li/~sgtatham/putty/0.80/htmldoc/AppendixC.html

const ppkMagic = "PuTTY-User-Key-File-"

// IsPPK: beginnt die Datei wie ein PuTTY-Key?
// Obergrenzen für Argon2 aus der Datei: 1 GiB Speicher, 1000 Durchläufe.
const (
	ppkMaxArgonMem    = 1 << 20 // KiB
	ppkMaxArgonPasses = 1000
)

func IsPPK(b []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(b, "\ufeff \t\r\n"), []byte(ppkMagic))
}

// parseKey liest OpenSSH/PEM- und PuTTY-Keys. Ist der Key verschlüsselt und
// pass leer, kommt *ssh.PassphraseMissingError (wie bei ssh.ParsePrivateKey).
func parseKey(b, pass []byte) (ssh.Signer, error) {
	if IsPPK(b) {
		k, err := parsePPK(b, pass)
		if err != nil {
			return nil, err
		}
		return ssh.NewSignerFromKey(k)
	}
	if len(pass) == 0 {
		return ssh.ParsePrivateKey(b)
	}
	return ssh.ParsePrivateKeyWithPassphrase(b, pass)
}

type ppkFile struct {
	version           int
	alg, enc, comment string
	pub, priv         []byte
	mac               []byte
	kdf               string
	argMem, argPass   uint32
	argPar            uint8
	argSalt           []byte
}

func readPPK(b []byte) (*ppkFile, error) {
	lines := strings.Split(strings.ReplaceAll(strings.TrimLeft(string(b), "\ufeff \t\r\n"), "\r", ""), "\n")
	f := &ppkFile{}
	bad := func(what string) error { return fmt.Errorf(i18n.T("PuTTY-Key unlesbar (%s)"), i18n.T(what)) }
	i := 0
	next := func() (string, string, bool) {
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i >= len(lines) {
			return "", "", false
		}
		k, v, ok := strings.Cut(lines[i], ":")
		i++
		return strings.TrimSpace(k), strings.TrimSpace(v), ok
	}
	block := func(n string) ([]byte, error) {
		cnt, err := strconv.Atoi(n)
		if err != nil || cnt < 0 || i+cnt > len(lines) {
			return nil, bad("Zeilenzahl")
		}
		s := strings.Join(lines[i:i+cnt], "")
		i += cnt
		out, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
		if err != nil {
			return nil, bad("Base64")
		}
		return out, nil
	}
	for {
		k, v, ok := next()
		if !ok {
			break
		}
		var err error
		switch {
		case strings.HasPrefix(k, ppkMagic):
			f.version, _ = strconv.Atoi(strings.TrimPrefix(k, ppkMagic))
			f.alg = v
		case k == "Encryption":
			f.enc = v
		case k == "Comment":
			f.comment = v
		case k == "Public-Lines":
			f.pub, err = block(v)
		case k == "Private-Lines":
			f.priv, err = block(v)
		case k == "Private-MAC":
			f.mac, err = hex.DecodeString(v)
		case k == "Key-Derivation":
			f.kdf = v
		case k == "Argon2-Memory":
			n, e := strconv.ParseUint(v, 10, 32)
			f.argMem, err = uint32(n), e
		case k == "Argon2-Passes":
			n, e := strconv.ParseUint(v, 10, 32)
			f.argPass, err = uint32(n), e
		case k == "Argon2-Parallelism":
			n, e := strconv.ParseUint(v, 10, 8)
			f.argPar, err = uint8(n), e
		case k == "Argon2-Salt":
			f.argSalt, err = hex.DecodeString(v)
		}
		if err != nil {
			return nil, bad(k)
		}
	}
	if f.version != 2 && f.version != 3 {
		return nil, bad("nur Version 2 und 3")
	}
	if f.pub == nil || f.priv == nil || f.mac == nil {
		return nil, bad("unvollständig")
	}
	if f.enc != "none" && f.enc != "aes256-cbc" {
		return nil, fmt.Errorf(i18n.T("PuTTY-Key: Verschlüsselung %s wird nicht unterstützt"), f.enc)
	}
	return f, nil
}

// CheckPPK: gültige PuTTY-Datei? (ohne Passphrase, nur Aufbau)
func CheckPPK(b []byte) error {
	_, err := readPPK(b)
	return err
}

func parsePPK(b, pass []byte) (any, error) {
	f, err := readPPK(b)
	if err != nil {
		return nil, err
	}
	encrypted := f.enc == "aes256-cbc"
	if encrypted && len(pass) == 0 {
		var pk ssh.PublicKey
		if p, err := ssh.ParsePublicKey(f.pub); err == nil {
			pk = p
		}
		return nil, &ssh.PassphraseMissingError{PublicKey: pk}
	}
	if !encrypted {
		pass = nil
	}

	var cipherKey, iv, macKey []byte
	var mac func() hash.Hash
	switch f.version {
	case 2:
		mac = sha1.New
		if encrypted {
			h0 := sha1.Sum(append([]byte{0, 0, 0, 0}, pass...))
			h1 := sha1.Sum(append([]byte{0, 0, 0, 1}, pass...))
			cipherKey = append(h0[:], h1[:]...)[:32]
			iv = make([]byte, aes.BlockSize)
		}
		m := sha1.Sum(append([]byte("putty-private-key-file-mac-key"), pass...))
		macKey = m[:]
	case 3:
		mac = sha256.New
		if encrypted {
			if f.argMem == 0 || f.argPass == 0 || f.argPar == 0 || len(f.argSalt) == 0 {
				return nil, errors.New(i18n.T("PuTTY-Key unlesbar (Argon2-Angaben fehlen)"))
			}
			// Eine präparierte Datei könnte sonst Terabytes Speicher oder
			// Milliarden Durchläufe verlangen (PuTTYgen: 8 MiB, ~10–40 Durchläufe).
			if f.argMem > ppkMaxArgonMem || f.argPass > ppkMaxArgonPasses {
				return nil, errors.New(i18n.T("PuTTY-Key unlesbar (Argon2-Angaben zu groß)"))
			}
			var out []byte
			switch f.kdf {
			case "Argon2id":
				out = argon2.IDKey(pass, f.argSalt, f.argPass, f.argMem, f.argPar, 80)
			case "Argon2i":
				out = argon2.Key(pass, f.argSalt, f.argPass, f.argMem, f.argPar, 80)
			default:
				return nil, fmt.Errorf(i18n.T("PuTTY-Key: Verfahren %s wird nicht unterstützt, bitte in PuTTYgen mit Argon2id speichern"), f.kdf)
			}
			cipherKey, iv, macKey = out[:32], out[32:48], out[48:80]
		} else {
			macKey = nil
		}
	}

	priv := append([]byte(nil), f.priv...)
	if encrypted {
		if len(priv)%aes.BlockSize != 0 {
			return nil, errors.New(i18n.T("PuTTY-Key unlesbar (Länge)"))
		}
		blk, _ := aes.NewCipher(cipherKey)
		cipher.NewCBCDecrypter(blk, iv).CryptBlocks(priv, priv)
	}

	h := hmac.New(mac, macKey)
	for _, s := range [][]byte{[]byte(f.alg), []byte(f.enc), []byte(f.comment), f.pub, priv} {
		_ = binary.Write(h, binary.BigEndian, uint32(len(s)))
		h.Write(s)
	}
	if subtle.ConstantTimeCompare(h.Sum(nil), f.mac) != 1 {
		if encrypted {
			return nil, x509.IncorrectPasswordError
		}
		return nil, errors.New(i18n.T("PuTTY-Key beschädigt (Prüfsumme falsch)"))
	}
	return ppkKey(f.alg, f.pub, priv)
}

type sshBuf []byte

func (b *sshBuf) str() ([]byte, bool) {
	if len(*b) < 4 {
		return nil, false
	}
	n := binary.BigEndian.Uint32(*b)
	if uint64(len(*b)-4) < uint64(n) {
		return nil, false
	}
	s := (*b)[4 : 4+n]
	*b = (*b)[4+n:]
	return s, true
}

func (b *sshBuf) mpint() (*big.Int, bool) {
	s, ok := b.str()
	if !ok {
		return nil, false
	}
	return new(big.Int).SetBytes(s), true
}

func ppkKey(alg string, pubBlob, privBlob []byte) (any, error) {
	bad := errors.New(i18n.T("PuTTY-Key unlesbar (Inhalt)"))
	pub, priv := sshBuf(pubBlob), sshBuf(privBlob)
	if t, ok := pub.str(); !ok || string(t) != alg {
		return nil, bad
	}
	switch {
	case alg == "ssh-rsa":
		e, ok1 := pub.mpint()
		n, ok2 := pub.mpint()
		d, ok3 := priv.mpint()
		p, ok4 := priv.mpint()
		q, ok5 := priv.mpint()
		if !(ok1 && ok2 && ok3 && ok4 && ok5) || !e.IsInt64() {
			return nil, bad
		}
		k := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: n, E: int(e.Int64())}, D: d, Primes: []*big.Int{p, q}}
		if err := k.Validate(); err != nil {
			return nil, bad
		}
		k.Precompute()
		return k, nil

	case alg == "ssh-ed25519":
		pk, ok1 := pub.str()
		seed, ok2 := priv.str()
		if !ok1 || !ok2 || len(pk) != ed25519.PublicKeySize || len(seed) != ed25519.SeedSize {
			return nil, bad
		}
		k := ed25519.NewKeyFromSeed(seed)
		if !bytes.Equal(k.Public().(ed25519.PublicKey), pk) {
			return nil, bad
		}
		return &k, nil

	case strings.HasPrefix(alg, "ecdsa-sha2-"):
		var curve elliptic.Curve
		switch alg {
		case "ecdsa-sha2-nistp256":
			curve = elliptic.P256()
		case "ecdsa-sha2-nistp384":
			curve = elliptic.P384()
		case "ecdsa-sha2-nistp521":
			curve = elliptic.P521()
		default:
			return nil, bad
		}
		_, ok1 := pub.str()
		pt, ok2 := pub.str()
		d, ok3 := priv.mpint()
		if !(ok1 && ok2 && ok3) {
			return nil, bad
		}
		// Skalar in feste Länge bringen und über die Go-1.25-API prüfen lassen
		// (liegt im gültigen Bereich); der öffentliche Punkt muss passen.
		size := (curve.Params().BitSize + 7) / 8
		if d.Sign() <= 0 || d.BitLen() > size*8 {
			return nil, bad
		}
		k, err := ecdsa.ParseRawPrivateKey(curve, d.FillBytes(make([]byte, size)))
		if err != nil {
			return nil, bad
		}
		if own, err := k.PublicKey.Bytes(); err != nil || !bytes.Equal(own, pt) {
			return nil, bad
		}
		return k, nil
	}
	return nil, fmt.Errorf(i18n.T("PuTTY-Key: Typ %s wird nicht unterstützt (bitte ed25519, RSA oder ECDSA)"), alg)
}
