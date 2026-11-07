package sshx

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/db9979/sshtermy/internal/i18n"
)

// X11-Weiterleitung (wie ssh -X)
//
// Der Server bekommt ein zufälliges Schein-Cookie. Öffnet ein Programm dort
// ein Fenster, kommt ein „x11“-Kanal zurück; sshTermy prüft das Schein-Cookie
// im ersten Paket, ersetzt es durch das echte Cookie des lokalen X-Servers
// (xauth) und verbindet zum lokalen Display. So sieht der Server das echte
// Cookie nie, und fremde Prozesse dort können ohne das Schein-Cookie nichts
// öffnen (wie OpenSSH).
//
// Display: $DISPLAY; unter Windows ohne DISPLAY localhost:0.0 (VcXsrv, X410,
// Xming). „:0“ = Unix-Socket /tmp/.X11-unix/X0, „host:0“ = TCP 6000+n,
// macOS/XQuartz: Pfad zum Socket vor dem „:“.

const x11Proto = "MIT-MAGIC-COOKIE-1"

type x11Fwd struct {
	display string
	fake    []byte // ans Ziel geschickt
	real    []byte // lokales Cookie, nil = keins (X-Server ohne Zugriffsschutz)
}

// X11Note: Hinweis für die Statuszeile, wenn die Weiterleitung nicht klappte.
func (c *Client) X11Note() string {
	if c.x11Err == nil {
		return ""
	}
	return i18n.T("X11: ") + c.x11Err.Error()
}

func (c *Client) requestX11(sess *ssh.Session) {
	c.x11Once.Do(func() {
		disp := localDisplay()
		if disp == "" {
			c.x11Err = errors.New(i18n.T("kein lokaler X-Server (DISPLAY leer)"))
			return
		}
		fake := make([]byte, 16)
		_, _ = rand.Read(fake)
		f := &x11Fwd{display: disp, fake: fake, real: xauthCookie(disp)}
		ch := c.Client.HandleChannelOpen("x11")
		if ch == nil {
			c.x11Err = errors.New(i18n.T("Kanal schon belegt"))
			return
		}
		c.x11 = f
		go func() {
			for nc := range ch {
				go f.serve(nc)
			}
		}()
	})
	if c.x11 == nil {
		return
	}
	payload := struct {
		Single bool
		Proto  string
		Cookie string
		Screen uint32
	}{false, x11Proto, hex.EncodeToString(c.x11.fake), displayScreen(c.x11.display)}
	ok, err := sess.SendRequest("x11-req", true, ssh.Marshal(&payload))
	switch {
	case err != nil:
		c.x11Err = err
	case !ok:
		c.x11Err = errors.New(i18n.T("vom Server abgelehnt (X11Forwarding no oder xauth fehlt dort?)"))
	}
}

// localDisplay: Display, zu dem weitergeleitet wird.
func localDisplay() string {
	if d := os.Getenv("DISPLAY"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		return "localhost:0.0"
	}
	return ""
}

// parseDisplay zerlegt „host:n.s“ in Netzwerk und Adresse des X-Servers.
func parseDisplay(d string) (network, addr string, err error) {
	i := strings.LastIndex(d, ":")
	if i < 0 {
		return "", "", fmt.Errorf("DISPLAY %q", d)
	}
	host, rest := d[:i], d[i+1:]
	num := rest
	if j := strings.Index(rest, "."); j >= 0 {
		num = rest[:j]
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return "", "", fmt.Errorf("DISPLAY %q", d)
	}
	switch {
	case strings.HasPrefix(host, "/"):
		// XQuartz: Pfad des Sockets steht vor dem „:“
		return "unix", d, nil
	case host == "" || host == "unix":
		if runtime.GOOS == "windows" {
			return "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(6000+n)), nil
		}
		return "unix", "/tmp/.X11-unix/X" + strconv.Itoa(n), nil
	}
	return "tcp", net.JoinHostPort(strings.TrimPrefix(strings.TrimSuffix(host, "]"), "["), strconv.Itoa(6000+n)), nil
}

func displayScreen(d string) uint32 {
	if i := strings.LastIndex(d, "."); i > strings.LastIndex(d, ":") {
		if n, err := strconv.Atoi(d[i+1:]); err == nil && n >= 0 {
			return uint32(n)
		}
	}
	return 0
}

// xauthCookie: echtes Cookie des lokalen Displays über `xauth list`.
func xauthCookie(disp string) []byte {
	path, err := exec.LookPath("xauth")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "list", disp).Output()
	if err != nil {
		return nil
	}
	return parseXauthList(out)
}

func parseXauthList(out []byte) []byte {
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[1] == x11Proto {
			if b, err := hex.DecodeString(f[2]); err == nil {
				return b
			}
		}
	}
	return nil
}

func (f *x11Fwd) serve(nc ssh.NewChannel) {
	ch, reqs, err := nc.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	defer ch.Close()
	setup, err := rewriteX11Setup(ch, f.fake, f.real)
	if err != nil {
		return
	}
	network, addr, err := parseDisplay(f.display)
	if err != nil {
		return
	}
	local, err := net.DialTimeout(network, addr, 5*time.Second)
	if err != nil {
		return
	}
	defer local.Close()
	if _, err := local.Write(setup); err != nil {
		return
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(local, ch)
		if cw, ok := local.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	_, _ = io.Copy(ch, local)
	_ = ch.CloseWrite()
	wg.Wait()
}

var errX11Cookie = errors.New("x11: falsches Cookie")

// rewriteX11Setup liest das erste Paket des X-Protokolls (Connection Setup),
// prüft das Schein-Cookie und gibt das Paket mit dem echten Cookie zurück
// (ohne echtes Cookie: ohne Anmeldedaten).
func rewriteX11Setup(r io.Reader, fake, real []byte) ([]byte, error) {
	head := make([]byte, 12)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, err
	}
	var bo binary.ByteOrder
	switch head[0] {
	case 'l':
		bo = binary.LittleEndian
	case 'B':
		bo = binary.BigEndian
	default:
		return nil, errors.New("x11: unbekannte Byte-Reihenfolge")
	}
	nameLen, dataLen := int(bo.Uint16(head[6:])), int(bo.Uint16(head[8:]))
	pad := func(n int) int { return (n + 3) &^ 3 }
	rest := make([]byte, pad(nameLen)+pad(dataLen))
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, err
	}
	name, data := rest[:nameLen], rest[pad(nameLen):pad(nameLen)+dataLen]
	if string(name) != x11Proto || !bytes.Equal(data, fake) {
		return nil, errX11Cookie
	}
	newName, newData := []byte(x11Proto), real
	if real == nil {
		newName = nil
	}
	out := make([]byte, 12, 12+pad(len(newName))+pad(len(newData)))
	copy(out, head)
	bo.PutUint16(out[6:], uint16(len(newName)))
	bo.PutUint16(out[8:], uint16(len(newData)))
	out = append(out, newName...)
	out = append(out, make([]byte, pad(len(newName))-len(newName))...)
	out = append(out, newData...)
	out = append(out, make([]byte, pad(len(newData))-len(newData))...)
	return out, nil
}
