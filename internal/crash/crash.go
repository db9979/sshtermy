// Package crash schreibt bei einem Absturz einen lokalen Bericht ohne
// Passwörter und bietet an, ihn als GitHub-Issue einzureichen.
//
// Zwei Arten von Absturz werden erfasst:
//
//  1. Panik in der Oberfläche (Update/View/Kommandos): bubbletea fängt sie,
//     stellt das Terminal wieder her und schreibt den Stack nach stderr.
//     Guard liest stderr mit; nach Program.Run entsteht sofort der Bericht.
//  2. Harter Absturz in einer anderen Goroutine (z. B. SSH-Lesen) oder ein
//     fatal error der Laufzeit: der Prozess endet sofort. Die Laufzeit
//     schreibt den Stack per debug.SetCrashOutput in crash/fatal.log; beim
//     nächsten Start wird daraus der Bericht (Pending).
//
// Vor dem Speichern wird alles entfernt, was nach Geheimnis aussieht: alle
// Passwörter, Keys und API-Keys aus dem Store (wörtlich), Host-Namen und
// IP-Adressen, der Home-Ordner und typische Muster (password=…, Bearer …,
// Private-Key-Blöcke). Der Bericht ist reiner Text; nichts wird ohne
// Zustimmung verschickt. Das Issue öffnet sich im Browser, dort sieht man
// vor dem Absenden genau, was übertragen wird.
package crash

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

const (
	subdir    = "crash"
	fatalName = "fatal.log"      // laufende Sitzung: Ziel von SetCrashOutput
	prevName  = "fatal-prev.log" // Absturz der letzten Sitzung, noch ohne Bericht
	keep      = 10               // so viele Berichte bleiben liegen
	ringSize  = 256 << 10        // so viel stderr wird höchstens mitgelesen
)

// Dir: Ordner der Berichte im sshTermy-Ordner.
func Dir(base string) string { return filepath.Join(base, subdir) }

// Guard läuft vom Programmstart bis zum Ende.
type Guard struct {
	dir   string
	fatal *os.File

	orig *os.File // echtes stderr
	w    *os.File
	done chan struct{}
	mu   sync.Mutex
	buf  []byte
	last time.Time // letzte Ausgabe auf stderr
}

// Start richtet beides ein: SetCrashOutput und das Mitlesen von stderr. Ein
// Absturz der letzten Sitzung wird vorher nach fatal-prev.log verschoben
// (siehe Pending). Fehler sind nicht schlimm: dann gibt es eben keinen Bericht.
func Start(base string) *Guard {
	g := &Guard{dir: Dir(base)}
	if err := os.MkdirAll(g.dir, 0o700); err != nil {
		return g
	}
	fp := filepath.Join(g.dir, fatalName)
	if fi, err := os.Stat(fp); err == nil && fi.Size() > 0 {
		_ = os.Rename(fp, filepath.Join(g.dir, prevName))
	}
	if f, err := os.OpenFile(fp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600); err == nil {
		if debug.SetCrashOutput(f, debug.CrashOptions{}) == nil {
			g.fatal = f
		} else {
			f.Close()
		}
	}
	if r, w, err := os.Pipe(); err == nil {
		g.orig, g.w, g.done = os.Stderr, w, make(chan struct{})
		os.Stderr = w
		go g.copy(r)
	}
	return g
}

func (g *Guard) copy(r *os.File) {
	defer close(g.done)
	b := make([]byte, 32<<10)
	for {
		n, err := r.Read(b)
		if n > 0 {
			_, _ = g.orig.Write(b[:n])
			g.mu.Lock()
			g.buf = append(g.buf, b[:n]...)
			if len(g.buf) > ringSize {
				g.buf = append([]byte(nil), g.buf[len(g.buf)-ringSize:]...)
			}
			g.last = time.Now()
			g.mu.Unlock()
		}
		if err != nil {
			r.Close()
			return
		}
	}
}

// Panic liefert den von bubbletea ausgegebenen Absturz (Meldung + Stack) und
// beendet das Mitlesen. bubbletea schreibt den Stack bei Paniken in
// Kommandos erst kurz nach dem Ende von Run, deshalb wartet Panic, bis
// stderr eine Weile ruhig ist (höchstens zwei Sekunden).
func (g *Guard) Panic() string {
	if g.w == nil {
		return ""
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		has := bytes.Contains(g.buf, []byte("Caught panic:")) && bytes.Contains(g.buf, []byte("goroutine "))
		quiet := time.Since(g.last) > 200*time.Millisecond
		g.mu.Unlock()
		if has && quiet {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	g.stopCapture()
	g.mu.Lock()
	defer g.mu.Unlock()
	s := strings.ReplaceAll(string(g.buf), "\r\n", "\n")
	if i := strings.LastIndex(s, "Caught panic:"); i >= 0 {
		s = s[i:]
	}
	return strings.TrimSpace(s)
}

func (g *Guard) stopCapture() {
	if g.w == nil {
		return
	}
	os.Stderr = g.orig
	g.w.Close()
	<-g.done
	g.w = nil
}

// Close: normales Programmende. Die leere fatal.log bleibt liegen.
func (g *Guard) Close() {
	g.stopCapture()
	if g.fatal != nil {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		g.fatal.Close()
		g.fatal = nil
	}
}

// Pending: Absturz der letzten Sitzung (Rohtext) oder "". Der Rohtext liegt
// nur bis zum Bericht in fatal-prev.log, siehe Resolve.
func Pending(base string) string {
	b, err := os.ReadFile(filepath.Join(Dir(base), prevName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Resolve löscht den Rohtext der letzten Sitzung (nachdem der Bericht steht).
func Resolve(base string) { _ = os.Remove(filepath.Join(Dir(base), prevName)) }

// Kind: wo der Absturz passiert ist.
type Kind int

const (
	KindUI    Kind = iota // Panik in der Oberfläche, Terminal wurde wiederhergestellt
	KindFatal             // Prozess sofort beendet, Bericht beim nächsten Start
)

// Report ist ein fertiger, bereinigter Bericht.
type Report struct {
	Path  string // Datei im crash-Ordner
	Title string // Titel fürs Issue
	Text  string // Inhalt der Datei
}

// Write bereinigt raw und legt den Bericht als Datei ab. sensitive sind
// wörtlich zu entfernende Werte (Passwörter, Keys, Hosts aus dem Store).
func Write(base string, kind Kind, raw string, sensitive []string) (*Report, error) {
	r := Build(kind, raw, sensitive, time.Now())
	dir := Dir(base)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	r.Path = filepath.Join(dir, "sshtermy-crash-"+time.Now().Format("20060102-150405")+".txt")
	if err := os.WriteFile(r.Path, []byte(r.Text), 0o600); err != nil {
		return nil, err
	}
	prune(dir)
	return r, nil
}

// Build erzeugt den Bericht (ohne Datei). Der Text ist Englisch/Deutsch je
// nach eingestellter Sprache; Stack und Meldung bleiben, wie sie sind.
func Build(kind Kind, raw string, sensitive []string, at time.Time) *Report {
	clean := Clean(raw, sensitive)
	what := i18n.T("Absturz in der Oberfläche (Terminal wurde wiederhergestellt)")
	if kind == KindFatal {
		what = i18n.T("Programm wurde sofort beendet (Bericht beim nächsten Start erstellt)")
	}
	msg := firstMeaningful(clean)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", i18n.T("sshTermy-Absturzbericht"))
	fmt.Fprintf(&b, "%-10s %s\n", i18n.T("Version:"), version.Version+" ("+version.Commit+", "+version.Date+")")
	fmt.Fprintf(&b, "%-10s %s/%s, %s\n", i18n.T("System:"), runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Fprintf(&b, "%-10s %s\n", i18n.T("Zeit:"), at.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "%-10s %s\n", i18n.T("Art:"), what)
	fmt.Fprintf(&b, "%-10s %s\n\n", i18n.T("Meldung:"), msg)
	b.WriteString(i18n.T("Passwörter, Keys, Hosts, IP-Adressen und der Home-Ordner wurden entfernt.") + "\n\n")
	b.WriteString(clean)
	b.WriteString("\n")
	return &Report{Title: "Crash: " + trunc(msg, 80), Text: b.String()}
}

// firstMeaningful: die eigentliche Meldung (panic: …, fatal error: …).
func firstMeaningful(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "", l == "Caught panic:", strings.HasPrefix(l, "Restoring terminal"):
			continue
		case strings.HasPrefix(l, "goroutine "):
			return "?"
		}
		return l
	}
	return "?"
}

var (
	ipv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	ipv6 = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){3,7}[0-9a-fA-F]{1,4}\b`)
	// Benutzer im Pfad, falls der Home-Ordner anders geschrieben ist
	userPath = regexp.MustCompile(`(?i)(/home/|/Users/|\\Users\\)[^/\\\s]+`)
)

// Clean entfernt Geheimnisse und persönliche Angaben aus Text.
func Clean(s string, sensitive []string) string {
	if h, err := os.UserHomeDir(); err == nil && len(h) > 1 {
		s = strings.ReplaceAll(s, h, "~")
	}
	// längste zuerst, damit kein Rest eines längeren Werts stehen bleibt
	vals := append([]string(nil), sensitive...)
	sort.Slice(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) })
	mark := i18n.T("[ENTFERNT]")
	for _, v := range vals {
		if len(strings.TrimSpace(v)) >= 3 {
			s = strings.ReplaceAll(s, v, mark)
		}
	}
	s = userPath.ReplaceAllString(s, "${1}…")
	s = strings.ReplaceAll(agent.Redact(s), "[ENTFERNT]", mark)
	s = ipv4.ReplaceAllString(s, "x.x.x.x")
	s = ipv6.ReplaceAllString(s, "x:x::x")
	return s
}

// maxURL: Browser und GitHub vertragen nur begrenzt lange Adressen.
const maxURL = 7000

// IssueURL: neues Issue mit vorausgefülltem Titel und Bericht. Ist der
// Bericht zu lang, wird der Stack gekürzt und auf die Datei verwiesen.
func IssueURL(r *Report) string {
	base := "https://github.com/" + version.Repo + "/issues/new?"
	intro := i18n.T("Was hast du gerade gemacht, als sshTermy abgestürzt ist?") + "\n\n\n"
	body := func(text string) string {
		return intro + "```\n" + text + "\n```\n"
	}
	q := func(text string) string {
		return base + url.Values{"title": {r.Title}, "body": {body(text)}, "labels": {"crash"}}.Encode()
	}
	text := strings.TrimRight(r.Text, "\n")
	if u := q(text); len(u) <= maxURL {
		return u
	}
	more := "\n…\n" + i18n.T("(gekürzt, ganzer Bericht: ") + filepath.Base(r.Path) + ")"
	lo, hi := 0, len(text)
	for lo < hi { // längster Anfang, der noch passt
		mid := (lo + hi + 1) / 2
		if len(q(text[:mid]+more)) <= maxURL {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	cut := text[:lo]
	if i := strings.LastIndex(cut, "\n"); i > 0 {
		cut = cut[:i]
	}
	return q(cut + more)
}

func prune(dir string) {
	m, _ := filepath.Glob(filepath.Join(dir, "sshtermy-crash-*.txt"))
	sort.Strings(m)
	for len(m) > keep {
		_ = os.Remove(m[0])
		m = m[1:]
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Fprint schreibt die Meldung nach einem Absturz ins (wiederhergestellte)
// Terminal.
func Fprint(w io.Writer, r *Report) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, i18n.T("sshTermy ist abgestürzt. Das tut mir leid."))
	fmt.Fprintln(w, i18n.T("Bericht ohne Passwörter: ")+r.Path)
}
