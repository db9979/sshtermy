package detach

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Hintergrunddienst (Idee aus herdr): sshTermy läuft als eigener Prozess
// ohne Terminal weiter („sshtermy keep-serve“), das Terminalfenster dockt
// mit „sshtermy“ nur an (Attach) und reicht Tasten, Größe und Bild roh
// durch. Schließt man das Fenster oder koppelt ab, laufen alle Sitzungen
// weiter; der nächste Start hängt sich wieder an. Der Ordner liegt im
// Datenordner (nur für den eigenen Benutzer lesbar) und enthält Adresse
// und Zugangswort; das Zugangswort bleibt, solange der Dienst läuft.

// Hub nimmt nacheinander beliebig viele Fenster an (immer nur eines zur Zeit).
type Hub struct {
	dir   string // Adresse + Zugangswort
	sock  string // Ordner des Sockets (Unix)
	ln    net.Listener
	token []byte
}

// ListenHub öffnet den Kanal des Hintergrunddienstes in dir.
func ListenHub(dir string) (*Hub, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	_ = os.Chmod(dir, 0o700)
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	sock, err := os.MkdirTemp("", "sshtermy-keep-")
	if err != nil {
		return nil, err
	}
	ln, addr, err := listen(sock)
	if err != nil {
		os.RemoveAll(sock)
		return nil, err
	}
	h := &Hub{dir: dir, sock: sock, ln: ln, token: []byte(hex.EncodeToString(tok))}
	if err := writeFileAtomic(filepath.Join(dir, tokenFile), h.token); err != nil {
		h.Close()
		return nil, err
	}
	if err := writeFileAtomic(filepath.Join(dir, addrFile), []byte(addr)); err != nil {
		h.Close()
		return nil, err
	}
	return h, nil
}

func writeFileAtomic(p string, b []byte) error {
	if err := os.WriteFile(p+".tmp", b, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// Accept wartet auf das nächste Fenster (falsches Zugangswort: weiter warten).
func (h *Hub) Accept() (*Conn, int, int, error) {
	for {
		nc, err := h.ln.Accept()
		if err != nil {
			return nil, 0, 0, err
		}
		c, cols, rows, err := handshake(nc, h.token)
		if err != nil {
			nc.Close()
			continue
		}
		return c, cols, rows, nil
	}
}

// Close beendet den Dienst-Kanal und entfernt Adresse und Zugangswort.
func (h *Hub) Close() {
	h.ln.Close()
	os.RemoveAll(h.sock)
	os.Remove(filepath.Join(h.dir, addrFile))
	os.Remove(filepath.Join(h.dir, tokenFile))
}

// HubAlive: läuft in dir ein Hintergrunddienst, der antwortet?
func HubAlive(dir string) bool {
	network, addr, err := readAddr(dir)
	if err != nil {
		return false
	}
	nc, err := dialTimeout(network, addr, time.Second)
	if err != nil {
		return false
	}
	nc.Close()
	return true
}

// WaitHub wartet, bis der gerade gestartete Dienst antwortet.
func WaitHub(dir string, d time.Duration) error {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if HubAlive(dir) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("detach: der Hintergrunddienst meldet sich nicht")
}

func dialTimeout(network, addr string, d time.Duration) (net.Conn, error) {
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() { c, err := dial(network, addr); ch <- res{c, err} }()
	select {
	case r := <-ch:
		return r.c, r.err
	case <-time.After(d):
		go func() {
			if r := <-ch; r.c != nil {
				r.c.Close()
			}
		}()
		return nil, errors.New("detach: Zeitüberschreitung")
	}
}
