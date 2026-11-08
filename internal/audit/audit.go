// Package audit schreibt ein fälschungssicheres Audit-Protokoll: wer hat
// wann was getan (Verbindungen, Befehle, Dateiübertragungen, Änderungen an
// Konfigurationen und Einstellungen).
//
// Jede Zeile ist ein JSON-Objekt mit laufender Nummer und einem HMAC-SHA256
// über den Inhalt und den HMAC der vorigen Zeile (Kette). Der Schlüssel wird
// aus dem Store abgeleitet (vault.DeriveKey), ohne Master-Passwort lässt sich
// also keine Zeile ändern, einfügen, löschen oder umsortieren, ohne dass Verify
// es meldet. Die Kopfdatei (head) hält Nummer und HMAC der letzten Zeile und
// deckt so auch abgeschnittene Enden auf.
//
// Grenzen: Wer das Master-Passwort hat, kann die Kette neu schreiben; wer die
// Dateien löscht, löscht sie. Für mehr Schutz das Protokoll regelmäßig
// exportieren oder den Ordner auf ein Ziel ohne Löschrecht spiegeln.
package audit

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry: eine Zeile.
type Entry struct {
	N      uint64    `json:"n"`
	Time   time.Time `json:"t"`
	User   string    `json:"user"`             // Konto am Rechner
	Host   string    `json:"pc"`               // Rechnername
	Event  string    `json:"ev"`               // z. B. connect, command
	Target string    `json:"target,omitempty"` // Verbindung (Alias, user@host)
	Detail string    `json:"detail,omitempty"`
	Prev   string    `json:"prev"`
	MAC    string    `json:"mac,omitempty"`
}

// Ereignisse.
const (
	EvStart      = "start"   // sshTermy entsperrt
	EvConnect    = "connect" // Verbindung aufgebaut
	EvConnFail   = "connect-fail"
	EvDisconnect = "disconnect"
	EvCommand    = "command"   // Befehl am Prompt (Nutzer)
	EvAgent      = "agent"     // Befehl vom Agent-/Ziel-Modus
	EvBroadcast  = "broadcast" // Befehl an mehrere Hosts
	EvUpload     = "upload"
	EvDownload   = "download"
	EvConfig     = "config"   // Konfiguration geändert/gesichert/eingespielt
	EvSettings   = "settings" // Store/Einstellungen: Verbindung angelegt, Passwort geändert …
	EvAI         = "ai"       // Anfrage an eine KI (nur Host/Modell)
	EvBackup     = "backup"
)

const maxDetail = 2000

// Log schreibt in dir/audit.
type Log struct {
	mu   sync.Mutex
	dir  string
	key  []byte
	n    uint64
	prev string
	user string
	pc   string
}

// Open liest den Kopf (oder rekonstruiert ihn aus der letzten Datei) und
// hängt ab dort an.
func Open(dir string, key []byte) (*Log, error) {
	d := filepath.Join(dir, "audit")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return nil, err
	}
	l := &Log{dir: d, key: append([]byte(nil), key...), user: currentUser(), pc: hostname()}
	if h, err := l.readHead(); err == nil {
		l.n, l.prev = h.N, h.MAC
	} else if last, ok := l.lastEntry(); ok {
		l.n, l.prev = last.N, last.MAC
	}
	return l, nil
}

// Dir: Ordner mit den Protokolldateien.
func (l *Log) Dir() string { return l.dir }

// Add schreibt ein Ereignis. Fehler sind still: das Protokoll darf die
// Arbeit nicht aufhalten (Verify meldet Lücken).
func (l *Log) Add(ev, target, detail string) {
	if l == nil {
		return
	}
	_ = l.add(time.Now(), ev, target, detail)
}

func (l *Log) add(t time.Time, ev, target, detail string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(detail) > maxDetail {
		detail = detail[:maxDetail] + "…"
	}
	e := Entry{N: l.n + 1, Time: t.UTC().Truncate(time.Millisecond), User: l.user, Host: l.pc, Event: ev, Target: target, Detail: detail, Prev: l.prev}
	e.MAC = l.mac(e)
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.file(t), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	l.n, l.prev = e.N, e.MAC
	return l.writeHead()
}

// Monatsdateien: audit-2026-11.log
func (l *Log) file(t time.Time) string {
	return filepath.Join(l.dir, "audit-"+t.UTC().Format("2006-01")+".log")
}

func (l *Log) mac(e Entry) string {
	e.MAC = ""
	b, _ := json.Marshal(e)
	m := hmac.New(sha256.New, l.key)
	m.Write(b)
	return hex.EncodeToString(m.Sum(nil))
}

type head struct {
	N   uint64 `json:"n"`
	MAC string `json:"mac"`
	Sig string `json:"sig"`
}

func (l *Log) headSig(n uint64, mac string) string {
	m := hmac.New(sha256.New, l.key)
	fmt.Fprintf(m, "head|%d|%s", n, mac)
	return hex.EncodeToString(m.Sum(nil))
}

func (l *Log) writeHead() error {
	b, _ := json.Marshal(head{N: l.n, MAC: l.prev, Sig: l.headSig(l.n, l.prev)})
	p := filepath.Join(l.dir, "head")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (l *Log) readHead() (head, error) {
	var h head
	b, err := os.ReadFile(filepath.Join(l.dir, "head"))
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal(b, &h); err != nil {
		return h, err
	}
	if !hmac.Equal([]byte(h.Sig), []byte(l.headSig(h.N, h.MAC))) {
		return h, errors.New("audit: Kopf ungültig")
	}
	return h, nil
}

func (l *Log) files() []string {
	m, _ := filepath.Glob(filepath.Join(l.dir, "audit-*.log"))
	sort.Strings(m)
	return m
}

func (l *Log) lastEntry() (Entry, bool) {
	fs := l.files()
	if len(fs) == 0 {
		return Entry{}, false
	}
	b, err := os.ReadFile(fs[len(fs)-1])
	if err != nil {
		return Entry{}, false
	}
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	var e Entry
	if json.Unmarshal(lines[len(lines)-1], &e) != nil {
		return Entry{}, false
	}
	return e, true
}

// Result: Ergebnis der Prüfung.
type Result struct {
	Entries  []Entry
	Problems []string // leer = Kette lückenlos und unverändert
}

// OK: nichts gefunden.
func (r Result) OK() bool { return len(r.Problems) == 0 }

// Verify liest alle Dateien und prüft Kette, Nummern und Kopf.
func (l *Log) Verify() Result {
	l.mu.Lock()
	defer l.mu.Unlock()
	var r Result
	prev, n := "", uint64(0)
	bad := func(f string, a ...any) { r.Problems = append(r.Problems, fmt.Sprintf(f, a...)) }
	for _, f := range l.files() {
		fh, err := os.Open(f)
		if err != nil {
			bad("%s: %v", filepath.Base(f), err)
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		line := 0
		for sc.Scan() {
			line++
			if len(bytes.TrimSpace(sc.Bytes())) == 0 {
				continue
			}
			var e Entry
			if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
				bad("%s:%d: unlesbar", filepath.Base(f), line)
				continue
			}
			if !hmac.Equal([]byte(e.MAC), []byte(l.mac(e))) {
				bad("%s:%d: Eintrag %d verändert", filepath.Base(f), line, e.N)
			} else if e.Prev != prev {
				bad("%s:%d: Eintrag %d passt nicht an den vorigen (gelöscht oder umsortiert)", filepath.Base(f), line, e.N)
			}
			if e.N != n+1 {
				bad("%s:%d: Nummer %d nach %d (Lücke)", filepath.Base(f), line, e.N, n)
			}
			prev, n = e.MAC, e.N
			r.Entries = append(r.Entries, e)
		}
		fh.Close()
	}
	h, err := l.readHead()
	switch {
	case errors.Is(err, os.ErrNotExist):
		if n > 0 {
			bad("Kopfdatei fehlt")
		}
	case err != nil:
		bad("Kopfdatei ungültig")
	case h.N != n || h.MAC != prev:
		bad("Ende fehlt: Kopf nennt Eintrag %d, vorhanden bis %d", h.N, n)
	}
	return r
}

// Text: lesbare Fassung (eine Zeile pro Eintrag) für den Export.
func Text(es []Entry) string {
	var b strings.Builder
	for _, e := range es {
		fmt.Fprintf(&b, "%6d  %s  %s@%s  %-12s %s", e.N, e.Time.Local().Format("2006-01-02 15:04:05"), e.User, e.Host, e.Event, e.Target)
		if e.Detail != "" {
			b.WriteString("  " + strings.ReplaceAll(e.Detail, "\n", " ⏎ "))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	for _, k := range []string{"USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "?"
}

func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "?"
	}
	return h
}
