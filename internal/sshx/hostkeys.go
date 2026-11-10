package sshx

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/db9979/sshtermy/internal/i18n"
)

// HostKeyPrompt wird bei einem unbekannten Host gefragt (Trust On First Use).
// Rückgabe true = Schlüssel merken und verbinden.
type HostKeyPrompt func(host string, fingerprint string, keyType string) bool

// ErrHostKeyChanged: der Server zeigt einen anderen Schlüssel als gespeichert.
// Das kann ein Man-in-the-Middle sein. sshtermy bietet hier bewusst KEIN
// "trotzdem verbinden" an; der neue Schlüssel muss bewusst übernommen werden
// (HostKeyChangedError + Replace, Oberfläche mit zweiter Bestätigung).
var ErrHostKeyChanged = i18n.NewError("ACHTUNG: Host-Key hat sich geändert (möglicher Man-in-the-Middle). Verbindung abgebrochen")

// HostKeyChangedError trägt, was die Oberfläche zum Vergleichen und
// Übernehmen braucht. errors.Is(err, ErrHostKeyChanged) bleibt wahr.
type HostKeyChangedError struct {
	Host   string        // Name, wie verbunden (host:port)
	Remote string        // Adresse der Gegenstelle
	Key    ssh.PublicKey // neuer Schlüssel
	Old    []string      // Fingerprints der gespeicherten Schlüssel
}

func (e *HostKeyChangedError) Error() string        { return ErrHostKeyChanged.Error() }
func (e *HostKeyChangedError) Is(target error) bool { return target == ErrHostKeyChanged }

// Fingerprint des neuen Schlüssels (SHA256:…).
func (e *HostKeyChangedError) Fingerprint() string { return ssh.FingerprintSHA256(e.Key) }

func changedErr(hostname string, remote net.Addr, key ssh.PublicKey, ke *knownhosts.KeyError) error {
	e := &HostKeyChangedError{Host: hostname, Key: key}
	if remote != nil {
		e.Remote = remote.String()
	}
	for _, k := range ke.Want {
		e.Old = append(e.Old, k.Key.Type()+" "+ssh.FingerprintSHA256(k.Key))
	}
	return e
}

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
		err := h.check(hostname, remote, key)
		h.mu.Unlock()
		var ke *knownhosts.KeyError
		switch {
		case err == nil:
			return nil
		case errors.As(err, &ke) && len(ke.Want) > 0:
			return changedErr(hostname, remote, key, ke)
		case errors.As(err, &ke):
			// Die Rückfrage läuft OHNE Sperre: sie wartet auf den Benutzer, und
			// die Oberfläche greift währenddessen selbst auf den Store zu
			// (z. B. Sync liest Lines()). Mit gehaltener Sperre fror die UI ein
			// und der Dialog reagierte auf keine Taste mehr.
			fp := ssh.FingerprintSHA256(key)
			if prompt == nil || !prompt(hostname, fp, key.Type()) {
				return fmt.Errorf(i18n.T("Host-Key von %s abgelehnt"), hostname)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			// Inzwischen evtl. von einer parallelen Verbindung oder dem Sync ergänzt.
			switch err := h.check(hostname, remote, key); {
			case err == nil:
				return nil
			case errors.As(err, &ke) && len(ke.Want) > 0:
				return changedErr(hostname, remote, key, ke)
			}
			return h.add(hostname, remote, key)
		default:
			return err
		}
	}
}

// check prüft gegen die known_hosts-Datei (Aufrufer hält h.mu).
func (h *HostKeyStore) check(hostname string, remote net.Addr, key ssh.PublicKey) error {
	check, err := knownhosts.New(h.path)
	if err != nil {
		return err
	}
	return check(hostname, remote, key)
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

// Replace ersetzt die gespeicherten Schlüssel eines Hosts durch einen neuen
// (nach Neuinstallation oder Werksreset). Entfernt werden nur Zeilen, die zu
// host oder remote passen (auch gehashte); @-Markierungen und Kommentare
// bleiben. Danach wird der neue Schlüssel wie beim ersten Verbinden gemerkt.
func (h *HostKeyStore) Replace(host, remote string, key ssh.PublicKey) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, err := os.ReadFile(h.path)
	if err != nil {
		return 0, err
	}
	var ra net.Addr
	if remote != "" {
		if a, err := net.ResolveTCPAddr("tcp", remote); err == nil {
			ra = a
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(h.path), ".kh-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)
	matches := func(line string) bool {
		if os.WriteFile(tmpName, []byte(line+"\n"), 0o600) != nil {
			return false
		}
		check, err := knownhosts.New(tmpName)
		if err != nil {
			return false
		}
		var ke *knownhosts.KeyError
		for _, name := range []string{host, remote} {
			if name == "" {
				continue
			}
			addr := ra
			if addr == nil {
				addr = &net.TCPAddr{IP: net.IPv4zero}
			}
			if err := check(name, addr, dummyKey{}); errors.As(err, &ke) && len(ke.Want) > 0 {
				return true
			}
		}
		return false
	}
	var keep []string
	removed := 0
	for _, l := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(l)
		if t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "@") && matches(t) {
			removed++
			continue
		}
		keep = append(keep, l)
	}
	out := strings.TrimRight(strings.Join(keep, "\n"), "\n")
	if out != "" {
		out += "\n"
	}
	if err := os.WriteFile(h.path, []byte(out), 0o600); err != nil {
		return removed, err
	}
	return removed, h.add(host, ra, key)
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
