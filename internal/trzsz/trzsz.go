// Package trzsz ist die Client-Seite des trzsz-Protokolls
// (https://trzsz.github.io, MIT): Dateien direkt im Terminal übertragen,
// wo SFTP nicht hinkommt, z. B. nach `sudo su`, hinter mehreren Sprüngen
// oder auf Geräten ohne SFTP-Server. Auf dem Server startet man `trz`
// (hochladen) oder `tsz datei` (herunterladen), pip/apt/brew-Paket „trzsz“.
//
// Der Server schreibt einen Auslöser („::TRZSZ:TRANSFER:R:1.1.7:…“) ins
// Terminal und spricht dann zeilenweise: "#TYP:inhalt\n", der Inhalt
// base64(zlib(…)). sshTermy meldet „Protokoll 1, ohne Binärmodus, ohne
// Ordner“ und überträgt damit jede Datei in Blöcken mit Bestätigung und
// MD5-Prüfung am Ende. Das ist langsamer als SFTP, läuft aber durch jede
// Shell.
package trzsz

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version: so meldet sich sshTermy beim Server.
const Version = "1.1.7"

// Trigger: der Auslöser im Terminal.
type Trigger struct {
	Mode    byte   // 'R' hochladen (trz), 'D' Ordner hochladen (trz -d), 'S' herunterladen (tsz)
	Version string // Version des Servers
	ID      string // eindeutige Kennung (Zeitstempel), leer bei alten Servern
	Start   int    // Position des Auslösers im Puffer
}

var triggerRe = regexp.MustCompile(`::TRZSZ:TRANSFER:([SRD]):(\d+\.\d+\.\d+)(?::(\d+))?(?::\d+)?`)

// Detect sucht den Auslöser in buf.
func Detect(buf []byte) (Trigger, bool) {
	i := bytes.LastIndex(buf, []byte("::TRZSZ:TRANSFER:"))
	if i < 0 {
		return Trigger{}, false
	}
	m := triggerRe.FindSubmatch(buf[i:])
	if m == nil {
		return Trigger{}, false
	}
	return Trigger{Mode: m[1][0], Version: string(m[2]), ID: string(m[3]), Start: i}, true
}

// ErrCanceled: vom Nutzer abgebrochen.
var ErrCanceled = errors.New("canceled")

// RemoteError: Fehlermeldung des Servers.
type RemoteError struct{ Msg string }

func (e *RemoteError) Error() string { return e.Msg }

// Progress meldet den Stand: Dateiname, übertragen, Größe.
type Progress func(name string, done, total int64)

// Transfer: eine laufende Übertragung. Feed bekommt die Ausgabe des
// Servers, w schreibt zum Server.
type Transfer struct {
	w        io.Writer
	Progress Progress
	Timeout  time.Duration // je Antwort des Servers; 0 = 30 s

	mu     sync.Mutex
	buf    []byte
	notify chan struct{}
	stop   chan struct{}
	once   sync.Once

	maxBuf int64
}

// New: Übertragung zum Server w.
func New(w io.Writer) *Transfer {
	return &Transfer{w: w, notify: make(chan struct{}, 1), stop: make(chan struct{})}
}

// Feed: Ausgabe des Servers (aus dem SSH-Strom).
func (t *Transfer) Feed(p []byte) {
	t.mu.Lock()
	t.buf = append(t.buf, p...)
	t.mu.Unlock()
	select {
	case t.notify <- struct{}{}:
	default:
	}
}

// Rest: noch nicht gelesene Ausgabe (gehört nach dem Ende wieder ins Terminal).
func (t *Transfer) Rest() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.buf
	t.buf = nil
	return r
}

// Cancel bricht ab (Strg+C); der Server bekommt eine Fehlermeldung.
func (t *Transfer) Cancel() { t.once.Do(func() { close(t.stop) }) }

func (t *Transfer) timeout() time.Duration {
	if t.Timeout > 0 {
		return t.Timeout
	}
	return 30 * time.Second
}

// readLine: nächste Zeile ohne "\n" (und ohne "\r" davor).
func (t *Transfer) readLine() ([]byte, error) {
	timer := time.NewTimer(t.timeout())
	defer timer.Stop()
	for {
		t.mu.Lock()
		if i := bytes.IndexByte(t.buf, '\n'); i >= 0 {
			line := append([]byte(nil), t.buf[:i]...)
			t.buf = t.buf[i+1:]
			t.mu.Unlock()
			if bytes.IndexByte(line, 0x03) >= 0 {
				return nil, ErrCanceled
			}
			return bytes.TrimRight(line, "\r"), nil
		}
		t.mu.Unlock()
		select {
		case <-t.notify:
		case <-t.stop:
			return nil, ErrCanceled
		case <-timer.C:
			return nil, errors.New("timeout")
		}
	}
}

func (t *Transfer) send(typ, payload string) error {
	select {
	case <-t.stop:
		return ErrCanceled
	default:
	}
	_, err := io.WriteString(t.w, "#"+typ+":"+payload+"\n")
	return err
}

func (t *Transfer) sendString(typ, s string) error { return t.send(typ, encode([]byte(s))) }
func (t *Transfer) sendInt(typ string, n int64) error {
	return t.send(typ, strconv.FormatInt(n, 10))
}

// recv: Zeile vom Typ typ. Junk (Prompt-Reste, tmux) vor dem „#“ wird
// abgeschnitten. Fehlermeldungen des Servers werden zu RemoteError.
func (t *Transfer) recv(typ string) (string, error) {
	for {
		line, err := t.readLine()
		if err != nil {
			return "", err
		}
		s := string(line)
		if i := strings.LastIndex(s, "#"+typ+":"); i >= 0 {
			return s[i+len(typ)+2:], nil
		}
		for _, bad := range []string{"#FAIL:", "#fail:", "#EXIT:"} {
			if i := strings.LastIndex(s, bad); i >= 0 {
				msg, err := decode(s[i+len(bad):])
				if err != nil {
					msg = []byte(s[i+len(bad):])
				}
				return "", &RemoteError{strings.TrimSpace(string(msg))}
			}
		}
		if strings.TrimSpace(s) == "" || !strings.Contains(s, "#") {
			continue // leere Zeile oder Junk
		}
		return "", fmt.Errorf("unerwartete Antwort statt %s: %.60q", typ, s)
	}
}

func (t *Transfer) recvString(typ string) (string, error) {
	p, err := t.recv(typ)
	if err != nil {
		return "", err
	}
	b, err := decode(p)
	return string(b), err
}

func (t *Transfer) recvInt(typ string) (int64, error) {
	p, err := t.recv(typ)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(p), 10, 64)
}

func (t *Transfer) checkInt(want int64) error {
	n, err := t.recvInt("SUCC")
	if err != nil {
		return err
	}
	if n != want {
		return fmt.Errorf("Prüfung fehlgeschlagen: %d ≠ %d", n, want)
	}
	return nil
}

func encode(b []byte) string {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write(b)
	_ = w.Close()
	return base64.StdEncoding.EncodeToString(z.Bytes())
}

func decode(s string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	r, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 64<<20))
}

type action struct {
	Lang             string `json:"lang"`
	Version          string `json:"version"`
	Confirm          bool   `json:"confirm"`
	Newline          string `json:"newline"`
	Protocol         int    `json:"protocol"`
	SupportBinary    bool   `json:"binary"`
	SupportDirectory bool   `json:"support_dir"`
}

type config struct {
	Overwrite bool  `json:"overwrite"`
	Timeout   int   `json:"timeout"`
	Protocol  int   `json:"protocol"`
	Bufsize   int64 `json:"bufsize"`
	Directory bool  `json:"directory"`
	Binary    bool  `json:"binary"`
}

// handshake: Zusage (oder Absage) schicken, Einstellungen des Servers lesen.
func (t *Transfer) handshake(confirm bool) (config, error) {
	a, _ := json.Marshal(action{Lang: "go", Version: Version, Confirm: confirm, Newline: "\n"})
	if err := t.sendString("ACT", string(a)); err != nil {
		return config{}, err
	}
	if !confirm {
		return config{}, nil
	}
	s, err := t.recvString("CFG")
	if err != nil {
		return config{}, err
	}
	var c config
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return c, err
	}
	if c.Protocol > 1 || c.Binary || c.Directory {
		return c, errors.New("Server verlangt eine Protokollvariante, die sshTermy nicht kann (Binärmodus/Ordner)")
	}
	if c.Timeout > 0 {
		t.Timeout = time.Duration(c.Timeout) * time.Second
	}
	t.maxBuf = c.Bufsize
	if t.maxBuf <= 0 || t.maxBuf > 10<<20 {
		t.maxBuf = 1 << 20
	}
	return c, nil
}

// Decline: Übertragung ablehnen (z. B. Ordner, Abbruch in der Auswahl).
func (t *Transfer) Decline() error {
	_, err := t.handshake(false)
	return err
}

// fail: Fehler an den Server melden, damit er sauber aufhört.
func (t *Transfer) fail(err error) error {
	msg := err.Error()
	if errors.Is(err, ErrCanceled) {
		msg = "Stopped"
	}
	var re *RemoteError
	if !errors.As(err, &re) {
		_, _ = io.WriteString(t.w, "#fail:"+encode([]byte(msg))+"\n")
	}
	return err
}

// Upload schickt die Dateien paths an den Server (trz). Rückgabe: Namen,
// unter denen der Server sie abgelegt hat.
func (t *Transfer) Upload(paths []string) ([]string, error) {
	type src struct {
		path string
		size int64
	}
	var files []src
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			_ = t.Decline()
			return nil, err
		}
		if st.IsDir() {
			_ = t.Decline()
			return nil, fmt.Errorf("%s ist ein Ordner (nur Dateien, für Ordner F5)", filepath.Base(p))
		}
		files = append(files, src{p, st.Size()})
	}
	if _, err := t.handshake(true); err != nil {
		return nil, t.fail(err)
	}
	if err := t.sendInt("NUM", int64(len(files))); err != nil {
		return nil, err
	}
	if err := t.checkInt(int64(len(files))); err != nil {
		return nil, t.fail(err)
	}
	var names []string
	for _, f := range files {
		name, err := t.sendFile(f.path, f.size)
		if err != nil {
			return names, t.fail(err)
		}
		names = append(names, name)
	}
	_ = t.sendString("EXIT", "Saved "+strings.Join(names, ", "))
	return names, nil
}

func (t *Transfer) sendFile(path string, size int64) (string, error) {
	base := filepath.Base(path)
	if err := t.sendString("NAME", base); err != nil {
		return "", err
	}
	remote, err := t.recvString("SUCC")
	if err != nil {
		return "", err
	}
	if err := t.sendInt("SIZE", size); err != nil {
		return "", err
	}
	if err := t.checkInt(size); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	chunk := int64(1024)
	buf := make([]byte, t.maxBuf)
	var done int64
	t.progress(base, 0, size)
	for done < size {
		start := time.Now()
		n, err := io.ReadFull(f, buf[:min(chunk, size-done)])
		if err != nil {
			return "", err
		}
		data := buf[:n]
		if err := t.send("DATA", encode(data)); err != nil {
			return "", err
		}
		h.Write(data)
		if err := t.checkInt(int64(n)); err != nil {
			return "", err
		}
		done += int64(n)
		t.progress(base, done, size)
		// Blockgröße wie trzsz: wachsen, solange es schnell geht
		if el := time.Since(start); int64(n) == chunk && el < 500*time.Millisecond && chunk < t.maxBuf {
			chunk = min(chunk*2, t.maxBuf)
		} else if el >= 2*time.Second && chunk > 1024 {
			chunk = 1024
		}
	}
	sum := h.Sum(nil)
	if err := t.send("MD5", encode(sum)); err != nil {
		return "", err
	}
	got, err := t.recv("SUCC")
	if err != nil {
		return "", err
	}
	if b, err := decode(got); err != nil || !bytes.Equal(b, sum) {
		return "", errors.New("MD5-Prüfung fehlgeschlagen")
	}
	return remote, nil
}

// Download holt die Dateien des Servers (tsz) nach dir. Rückgabe: lokale Pfade.
func (t *Transfer) Download(dir string) ([]string, error) {
	cfg, err := t.handshake(true)
	if err != nil {
		return nil, t.fail(err)
	}
	num, err := t.recvInt("NUM")
	if err != nil {
		return nil, t.fail(err)
	}
	if num < 0 || num > 10000 {
		return nil, t.fail(fmt.Errorf("unplausible Dateianzahl %d", num))
	}
	if err := t.sendInt("SUCC", num); err != nil {
		return nil, err
	}
	var out []string
	for i := int64(0); i < num; i++ {
		// Überschreiben wünscht der Server; vorhandene Dateien bleiben trotzdem
		// (neuer Name). Sonst ersetzte ein Server still etwas in Downloads.
		_ = cfg.Overwrite
		p, err := t.recvFile(dir, false)
		if err != nil {
			return out, t.fail(err)
		}
		out = append(out, p)
	}
	names := make([]string, len(out))
	for i, p := range out {
		names[i] = filepath.Base(p)
	}
	_ = t.sendString("EXIT", "Saved "+strings.Join(names, ", ")+" to "+dir)
	return out, nil
}

func (t *Transfer) recvFile(dir string, overwrite bool) (string, error) {
	name, err := t.recvString("NAME")
	if err != nil {
		return "", err
	}
	name = SafeName(name)
	if name == "" {
		return "", errors.New("ungültiger Dateiname")
	}
	local := name
	if !overwrite {
		if local, err = freeName(dir, name); err != nil {
			return "", err
		}
	}
	path := filepath.Join(dir, local)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			_ = os.Remove(path) // halbe Datei nicht liegen lassen
		}
	}()
	if err := t.sendString("SUCC", local); err != nil {
		return "", err
	}
	size, err := t.recvInt("SIZE")
	if err != nil {
		return "", err
	}
	if err := t.sendInt("SUCC", size); err != nil {
		return "", err
	}
	h := md5.New()
	var done int64
	t.progress(local, 0, size)
	for done < size {
		p, err := t.recv("DATA")
		if err != nil {
			return "", err
		}
		data, err := decode(p)
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			return "", err
		}
		h.Write(data)
		done += int64(len(data))
		if err := t.sendInt("SUCC", int64(len(data))); err != nil {
			return "", err
		}
		t.progress(local, done, size)
	}
	sum := h.Sum(nil)
	p, err := t.recv("MD5")
	if err != nil {
		return "", err
	}
	want, err := decode(p)
	if err != nil || !bytes.Equal(want, sum) {
		return "", errors.New("MD5-Prüfung fehlgeschlagen")
	}
	if err := t.send("SUCC", encode(sum)); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}

func (t *Transfer) progress(name string, done, total int64) {
	if t.Progress != nil {
		t.Progress(name, done, total)
	}
}

// SafeName: nur der Dateiname, ohne Pfad und Steuerzeichen ("" = unbrauchbar).
func SafeName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = name[strings.LastIndex(name, "/")+1:]
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if name == "." || name == ".." || strings.Trim(name, ". ") == "" {
		return ""
	}
	return name
}

// freeName: name oder name.0, name.1 … (nichts überschreiben).
func freeName(dir, name string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name, nil
	}
	for i := 0; i < 1000; i++ {
		n := fmt.Sprintf("%s.%d", name, i)
		if _, err := os.Stat(filepath.Join(dir, n)); os.IsNotExist(err) {
			return n, nil
		}
	}
	return "", fmt.Errorf("kein freier Name für %s", name)
}
