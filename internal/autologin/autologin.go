// Package autologin tippt bei Telnet und seriellen Konsolen Benutzer und
// Passwort ein, sobald das Gerät danach fragt (SSH meldet sich im Protokoll
// an, Telnet und Konsolen fragen im Klartext auf dem Bildschirm).
package autologin

import (
	"bytes"
	"strings"
	"sync"
	"time"
)

// Window: so lange nach dem Verbinden wird auf Anmeldeaufforderungen
// geachtet. Danach tippt sshTermy nie mehr von sich aus etwas.
const Window = 90 * time.Second

// Helper beobachtet die Ausgabe und antwortet über send.
type Helper struct {
	User, Pass string
	send       func(string)

	mu       sync.Mutex
	tail     []byte
	until    time.Time
	userSent bool
	passSent bool
	keySent  bool
}

// New: send schickt Text an das Gerät. Ohne Benutzer und Passwort reagiert
// der Helper nur auf „Press any key to continue“.
func New(user, pass string, send func(string)) *Helper {
	return &Helper{User: user, Pass: pass, send: send, until: time.Now().Add(Window)}
}

// Write füttert den Helper mit Geräteausgabe (implementiert io.Writer).
func (h *Helper) Write(p []byte) (int, error) {
	if h == nil {
		return len(p), nil
	}
	h.mu.Lock()
	if time.Now().After(h.until) || (h.passSent && h.keySent) {
		h.mu.Unlock()
		return len(p), nil
	}
	h.tail = append(h.tail, p...)
	if len(h.tail) > 256 {
		h.tail = h.tail[len(h.tail)-256:]
	}
	reply := h.check()
	h.mu.Unlock()
	if reply != "" {
		h.send(reply)
	}
	return len(p), nil
}

// check sieht nach, ob die letzte Zeile eine Aufforderung ist. Aufrufer hält mu.
func (h *Helper) check() string {
	line := lastLine(h.tail)
	l := strings.ToLower(strings.TrimRight(line, " \t"))
	switch {
	case !h.keySent && strings.Contains(l, "press any key to continue"):
		h.keySent = true
		h.tail = h.tail[:0]
		return " "
	case !h.userSent && h.User != "" && isUserPrompt(l):
		h.userSent = true
		h.tail = h.tail[:0]
		return h.User + "\r"
	case !h.passSent && h.Pass != "" && strings.HasSuffix(l, "password:"):
		h.passSent = true
		h.tail = h.tail[:0]
		return h.Pass + "\r"
	}
	return ""
}

func isUserPrompt(l string) bool {
	for _, p := range []string{"login:", "username:", "user name:", "user:", "login as:"} {
		if strings.HasSuffix(l, p) {
			return true
		}
	}
	return false
}

// lastLine: Text nach dem letzten Zeilenumbruch, ohne Steuersequenzen.
func lastLine(b []byte) string {
	if i := bytes.LastIndexAny(b, "\r\n"); i >= 0 {
		b = b[i+1:]
	}
	var out []byte
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == 0x1b { // ESC [ … Buchstabe überspringen
			j := i + 1
			if j < len(b) && b[j] == '[' {
				j++
				for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
					j++
				}
			}
			i = j
			continue
		}
		if c >= 0x20 && c != 0x7f {
			out = append(out, c)
		}
	}
	return string(out)
}
