package sshx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/db9979/sshtermy/internal/i18n"
)

// FIDO2-/Sicherheitsschlüssel (sk-ssh-ed25519, sk-ecdsa-sha2-nistp256)
//
// Den Private Key solcher Schlüssel hält der YubiKey/Token selbst; die
// Datei enthält nur einen Verweis darauf. Signieren muss der Token, und mit
// ihm reden kann ohne C-Bibliothek nur OpenSSH. sshTermy startet deshalb
// einen eigenen, kurzlebigen ssh-agent (unter Windows den Agent-Dienst von
// Windows-OpenSSH), lädt die Schlüsseldatei mit ssh-add hinein und signiert
// darüber. Der Token blinkt dann; sshTermy zeigt „Sicherheitsschlüssel
// berühren“. Mit der Datei allein kann niemand etwas anfangen.

// skKeyType: Typ des öffentlichen Schlüssels in einer OpenSSH-Schlüsseldatei
// ("" = keine/unklar). Der öffentliche Teil steht unverschlüsselt im Kopf.
func skKeyType(pem []byte) string {
	const begin = "-----BEGIN OPENSSH PRIVATE KEY-----"
	i := bytes.Index(pem, []byte(begin))
	if i < 0 {
		return ""
	}
	rest := pem[i+len(begin):]
	j := bytes.Index(rest, []byte("-----END"))
	if j < 0 {
		return ""
	}
	raw, err := decodeB64Lines(rest[:j])
	if err != nil {
		return ""
	}
	const magic = "openssh-key-v1\x00"
	if !bytes.HasPrefix(raw, []byte(magic)) {
		return ""
	}
	b := raw[len(magic):]
	next := func() []byte {
		if len(b) < 4 {
			return nil
		}
		n := binary.BigEndian.Uint32(b)
		if uint32(len(b)-4) < n {
			b = nil
			return nil
		}
		v := b[4 : 4+n]
		b = b[4+n:]
		return v
	}
	next() // Verschlüsselung
	next() // KDF
	next() // KDF-Optionen
	if len(b) < 4 || binary.BigEndian.Uint32(b) != 1 {
		return ""
	}
	b = b[4:]
	pub, err := ssh.ParsePublicKey(next())
	if err != nil {
		return ""
	}
	return pub.Type()
}

func decodeB64Lines(b []byte) ([]byte, error) {
	s := strings.Join(strings.Fields(string(b)), "")
	return base64Std.DecodeString(s)
}

// IsSKKey: gehört die Schlüsseldatei zu einem FIDO2-Token?
func IsSKKey(pem []byte) bool {
	return strings.HasPrefix(skKeyType(pem), "sk-")
}

// ---------------------------------------------------------------- Agent

var skAgent struct {
	mu   sync.Mutex
	sock string
	dir  string
	cmd  *exec.Cmd
}

// skAgentSocket: eigener ssh-agent (Unix), unter Windows "" = Agent-Dienst.
func skAgentSocket() (string, error) {
	if runtime.GOOS == "windows" {
		return "", nil
	}
	skAgent.mu.Lock()
	defer skAgent.mu.Unlock()
	if skAgent.cmd != nil && skAgent.cmd.ProcessState == nil {
		return skAgent.sock, nil
	}
	bin, err := exec.LookPath("ssh-agent")
	if err != nil {
		return "", errors.New(i18n.T("ssh-agent (OpenSSH) nicht gefunden, er wird für Sicherheitsschlüssel gebraucht"))
	}
	dir, err := os.MkdirTemp("", "sshtermy-sk-")
	if err != nil {
		return "", err
	}
	sock := filepath.Join(dir, "agent.sock")
	cmd := exec.Command(bin, "-D", "-a", sock)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	go func() { _ = cmd.Wait() }()
	for i := 0; i < 50; i++ {
		if c, err := net.Dial("unix", sock); err == nil {
			c.Close()
			skAgent.sock, skAgent.dir, skAgent.cmd = sock, dir, cmd
			return sock, nil
		}
		time.Sleep(40 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	os.RemoveAll(dir)
	return "", errors.New(i18n.T("ssh-agent startet nicht"))
}

// StopSKAgent beendet den eigenen ssh-agent (beim Beenden von sshTermy).
func StopSKAgent() {
	skAgent.mu.Lock()
	defer skAgent.mu.Unlock()
	if skAgent.cmd != nil && skAgent.cmd.Process != nil {
		_ = skAgent.cmd.Process.Kill()
	}
	if skAgent.dir != "" {
		os.RemoveAll(skAgent.dir)
	}
	skAgent.cmd, skAgent.sock, skAgent.dir = nil, "", ""
}

func dialSKAgent(sock string) (net.Conn, error) {
	if sock == "" {
		return dialAgent()
	}
	return net.Dial("unix", sock)
}

// Askpass: ssh-add fragt die Passphrase über SSH_ASKPASS ab. sshTermy ruft
// sich dafür selbst auf und gibt die schon bekannte Passphrase aus.
const (
	askpassEnv    = "SSHTERMY_ASKPASS"
	askpassSecret = "SSHTERMY_ASKPASS_SECRET"
)

// AskpassMain: als SSH_ASKPASS gestartet? Dann Passphrase ausgeben und
// true (main beendet sich danach).
func AskpassMain() bool {
	if os.Getenv(askpassEnv) != "1" {
		return false
	}
	p := os.Getenv(askpassSecret)
	if p == "" {
		os.Exit(1) // keine Passphrase bekannt: ssh-add bricht ab, sshTermy fragt
	}
	fmt.Println(p)
	return true
}

// errSKPassphrase: ssh-add braucht eine (andere) Passphrase.
var errSKPassphrase = errors.New("passphrase")

// sshAdd lädt die Datei in den Agent unter sock.
func sshAdd(sock, path, pass string) error {
	bin, err := exec.LookPath("ssh-add")
	if err != nil {
		return errors.New(i18n.T("ssh-add (OpenSSH) nicht gefunden, es wird für Sicherheitsschlüssel gebraucht"))
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, path)
	env := os.Environ()
	if sock != "" {
		env = append(env, "SSH_AUTH_SOCK="+sock)
	}
	env = append(env, "SSH_ASKPASS="+self, "SSH_ASKPASS_REQUIRE=force", "DISPLAY=:0", askpassEnv+"=1", askpassSecret+"="+pass)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "passphrase") {
			return errSKPassphrase
		}
		if msg == "" {
			msg = err.Error()
		}
		return errors.New("ssh-add: " + msg)
	}
	return nil
}

// skSigner lädt die sk-Schlüsseldatei in den Agent und gibt einen Signer
// dafür zurück. touch wird vor jeder Signatur gerufen (Hinweis „berühren“).
func skSigner(pem []byte, path, pass string, ask SecretPrompt, label string, touch func()) (ssh.Signer, error) {
	sock, err := skAgentSocket()
	if err != nil {
		return nil, err
	}
	if path == "" {
		// eingebetteter Schlüssel: kurz als Datei für ssh-add
		dir, err := os.MkdirTemp("", "sshtermy-key-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		path = filepath.Join(dir, "key")
		if err := os.WriteFile(path, pem, 0o600); err != nil {
			return nil, err
		}
	}
	err = sshAdd(sock, path, pass)
	if errors.Is(err, errSKPassphrase) && ask != nil {
		p, aerr := ask(label)
		if aerr != nil {
			return nil, aerr
		}
		err = sshAdd(sock, path, p)
	}
	if errors.Is(err, errSKPassphrase) {
		return nil, errors.New(i18n.T("Passphrase des Sicherheitsschlüssels falsch"))
	}
	if err != nil {
		return nil, err
	}
	want, err := skPublicKey(pem)
	if err != nil {
		return nil, err
	}
	conn, err := dialSKAgent(sock)
	if err != nil {
		return nil, err
	}
	signers, err := agent.NewClient(conn).Signers()
	if err != nil {
		conn.Close()
		return nil, err
	}
	for _, s := range signers {
		if bytes.Equal(s.PublicKey().Marshal(), want.Marshal()) {
			return touchSigner{Signer: s, touch: touch}, nil
		}
	}
	conn.Close()
	return nil, errors.New(i18n.T("Sicherheitsschlüssel nach ssh-add nicht im Agent"))
}

// skPublicKey: öffentlicher Schlüssel aus dem Kopf der Datei.
func skPublicKey(pem []byte) (ssh.PublicKey, error) {
	const begin = "-----BEGIN OPENSSH PRIVATE KEY-----"
	i := bytes.Index(pem, []byte(begin))
	j := bytes.Index(pem, []byte("-----END"))
	if i < 0 || j < i {
		return nil, errors.New("kein OpenSSH-Schlüssel")
	}
	raw, err := decodeB64Lines(pem[i+len(begin) : j])
	if err != nil {
		return nil, err
	}
	b := raw[len("openssh-key-v1\x00"):]
	for k := 0; k < 3; k++ { // Verschlüsselung, KDF, KDF-Optionen
		n := binary.BigEndian.Uint32(b)
		b = b[4+n:]
	}
	b = b[4:] // Anzahl
	n := binary.BigEndian.Uint32(b)
	return ssh.ParsePublicKey(b[4 : 4+n])
}

// touchSigner meldet vor dem Signieren, dass der Token berührt werden will.
type touchSigner struct {
	ssh.Signer
	touch func()
}

func (t touchSigner) Sign(r ioReader, data []byte) (*ssh.Signature, error) {
	if t.touch != nil {
		t.touch()
	}
	return t.Signer.Sign(r, data)
}

type ioReader = io.Reader

var base64Std = base64.StdEncoding
