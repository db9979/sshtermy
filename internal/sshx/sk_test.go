package sshx

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"golang.org/x/crypto/ssh"
)

// TestMain: der Testlauf dient ssh-add selbst als SSH_ASKPASS (wie main).
func TestMain(m *testing.M) {
	if AskpassMain() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// skFile baut den Kopf einer OpenSSH-Schlüsseldatei mit sk-Schlüssel (so
// wie ssh-keygen -t ed25519-sk sie schreibt; der private Teil ist hier egal).
func skFile(t *testing.T) []byte {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	str := func(b []byte) []byte {
		out := make([]byte, 4, 4+len(b))
		binary.BigEndian.PutUint32(out, uint32(len(b)))
		return append(out, b...)
	}
	var blob []byte
	blob = append(blob, str([]byte(ssh.KeyAlgoSKED25519))...)
	blob = append(blob, str(pub)...)
	blob = append(blob, str([]byte("ssh:"))...)
	var raw []byte
	raw = append(raw, "openssh-key-v1\x00"...)
	raw = append(raw, str([]byte("none"))...)
	raw = append(raw, str([]byte("none"))...)
	raw = append(raw, str(nil)...)
	raw = append(raw, 0, 0, 0, 1)
	raw = append(raw, str(blob)...)
	raw = append(raw, str([]byte("privat"))...)
	b64 := base64.StdEncoding.EncodeToString(raw)
	var pem bytes.Buffer
	pem.WriteString("-----BEGIN OPENSSH PRIVATE KEY-----\n")
	for len(b64) > 70 {
		pem.WriteString(b64[:70] + "\n")
		b64 = b64[70:]
	}
	pem.WriteString(b64 + "\n-----END OPENSSH PRIVATE KEY-----\n")
	return pem.Bytes()
}

func TestSKKeyDetection(t *testing.T) {
	sk := skFile(t)
	if !IsSKKey(sk) || skKeyType(sk) != ssh.KeyAlgoSKED25519 {
		t.Fatalf("sk-Schlüssel nicht erkannt: %q", skKeyType(sk))
	}
	pk, err := skPublicKey(sk)
	if err != nil || pk.Type() != ssh.KeyAlgoSKED25519 {
		t.Fatalf("öffentlicher Schlüssel: %v", err)
	}
	k, err := GenerateKey(KeyEd25519, "t", []byte("geheim"))
	if err != nil {
		t.Fatal(err)
	}
	if IsSKKey(k.PrivatePEM) || skKeyType(k.PrivatePEM) != ssh.KeyAlgoED25519 {
		t.Fatalf("normaler Schlüssel als sk erkannt: %q", skKeyType(k.PrivatePEM))
	}
	if IsSKKey([]byte("-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----")) {
		t.Fatal("PEM-RSA als sk erkannt")
	}
}

// Der Weg über den eigenen ssh-agent (ssh-agent + ssh-add mit Askpass),
// hier mit einem normalen Schlüssel, weil es im Test keinen Token gibt.
func TestPrivateAgentWithAskpass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows nutzt den Agent-Dienst")
	}
	for _, b := range []string{"ssh-agent", "ssh-add"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skip("OpenSSH fehlt:", b)
		}
	}
	defer StopSKAgent()
	k, err := GenerateKey(KeyEd25519, "t", []byte("geheim"))
	if err != nil {
		t.Fatal(err)
	}
	touched := 0
	asked := 0
	ask := func(string) (string, error) { asked++; return "geheim", nil }
	s, err := skSigner(k.PrivatePEM, "", "", ask, "Passphrase", func() { touched++ })
	if err != nil {
		t.Fatalf("skSigner: %v", err)
	}
	if asked != 1 {
		t.Fatalf("Passphrase %d-mal gefragt", asked)
	}
	if !bytes.Equal(s.PublicKey().Marshal(), k.Signer.PublicKey().Marshal()) {
		t.Fatal("falscher Schlüssel aus dem Agent")
	}
	sig, err := s.Sign(rand.Reader, []byte("daten"))
	if err != nil || touched != 1 {
		t.Fatalf("Signatur: %v, berührt %d", err, touched)
	}
	if err := s.PublicKey().Verify([]byte("daten"), sig); err != nil {
		t.Fatal(err)
	}
}

// Falsche Passphrase: ssh-add darf nicht endlos nachfragen, sshTermy meldet es.
func TestPrivateAgentWrongPassphrase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows nutzt den Agent-Dienst")
	}
	for _, b := range []string{"ssh-agent", "ssh-add"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skip("OpenSSH fehlt:", b)
		}
	}
	defer StopSKAgent()
	k, err := GenerateKey(KeyEd25519, "t", []byte("geheim"))
	if err != nil {
		t.Fatal(err)
	}
	ask := func(string) (string, error) { return "falsch", nil }
	if _, err := skSigner(k.PrivatePEM, "", "auch falsch", ask, "Passphrase", nil); err == nil {
		t.Fatal("falsche Passphrase angenommen")
	}
}
