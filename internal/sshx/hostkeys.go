package sshx

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyPrompt wird bei einem unbekannten Host gefragt (Trust On First Use).
// Rückgabe true = Schlüssel merken und verbinden.
type HostKeyPrompt func(host string, fingerprint string, keyType string) bool

// ErrHostKeyChanged: der Server zeigt einen anderen Schlüssel als gespeichert.
// Das kann ein Man-in-the-Middle sein. sshtermy bietet hier bewusst KEIN
// "trotzdem verbinden" an; der Eintrag muss bewusst gelöscht werden.
var ErrHostKeyChanged = errors.New("ACHTUNG: Host-Key hat sich geändert (möglicher Man-in-the-Middle). Verbindung abgebrochen")

// HostKeyStore verwaltet eine known_hosts-Datei im OpenSSH-Format.
type HostKeyStore struct {
	mu   sync.Mutex
	path string
}

func NewHostKeyStore(path string) (*HostKeyStore, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	return &HostKeyStore{path: path}, nil
}

func (h *HostKeyStore) Callback(prompt HostKeyPrompt) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		check, err := knownhosts.New(h.path)
		if err != nil {
			return err
		}
		err = check(hostname, remote, key)
		var ke *knownhosts.KeyError
		switch {
		case err == nil:
			return nil
		case errors.As(err, &ke) && len(ke.Want) > 0:
			return ErrHostKeyChanged
		case errors.As(err, &ke):
			fp := ssh.FingerprintSHA256(key)
			if prompt == nil || !prompt(hostname, fp, key.Type()) {
				return fmt.Errorf("Host-Key von %s abgelehnt", hostname)
			}
			return h.add(hostname, remote, key)
		default:
			return err
		}
	}
}

func (h *HostKeyStore) add(hostname string, remote net.Addr, key ssh.PublicKey) error {
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	addrs := []string{knownhosts.Normalize(hostname)}
	if remote != nil {
		if r := knownhosts.Normalize(remote.String()); r != addrs[0] {
			addrs = append(addrs, r)
		}
	}
	_, err = fmt.Fprintln(f, knownhosts.Line(addrs, key))
	return err
}

// Algorithms: bekannte Key-Typen eines Hosts (leer = Standardliste).
func (h *HostKeyStore) Algorithms(addr string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	check, err := knownhosts.New(h.path)
	if err != nil {
		return nil
	}
	// Trick aus der x/crypto-Doku: mit einem Dummy-Key prüfen, KeyError.Want
	// enthält dann die gespeicherten Schlüssel.
	var ke *knownhosts.KeyError
	if err := check(addr, &net.TCPAddr{IP: net.IPv4zero}, dummyKey{}); errors.As(err, &ke) {
		seen := map[string]bool{}
		var out []string
		for _, k := range ke.Want {
			t := k.Key.Type()
			for _, a := range algosFor(t) {
				if !seen[a] {
					seen[a] = true
					out = append(out, a)
				}
			}
		}
		return out
	}
	return nil
}

func algosFor(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{keyType}
}

type dummyKey struct{}

func (dummyKey) Type() string                        { return "sshtermy-dummy" }
func (dummyKey) Marshal() []byte                     { return []byte("sshtermy-dummy") }
func (dummyKey) Verify([]byte, *ssh.Signature) error { return errors.New("dummy") }

// Lines liefert die Einträge der known_hosts-Datei (für den Sync).
func (h *HostKeyStore) Lines() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, err := os.ReadFile(h.path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

// AddLines ergänzt fehlende Einträge (aus dem Sync). Ungültige Zeilen werden
// verworfen; vorhandene Einträge bleiben unverändert.
func (h *HostKeyStore) AddLines(lines []string) (int, error) {
	have := map[string]bool{}
	for _, l := range h.Lines() {
		have[l] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var add []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || have[l] {
			continue
		}
		if _, _, _, _, _, err := ssh.ParseKnownHosts([]byte(l)); err != nil {
			continue
		}
		have[l] = true
		add = append(add, l)
	}
	if len(add) == 0 {
		return 0, nil
	}
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	_, err = f.WriteString(strings.Join(add, "\n") + "\n")
	return len(add), err
}
