// Package sessionlog schreibt SSH-Sitzungen als Protokoll mit: Befehle, ihre
// Ausgabe (Klartext aus dem Terminal), Zeitpunkt und ob sie fehlgeschlagen
// sind. Die Protokolle liegen verschlüsselt auf dem Rechner, eine Datei je
// Sitzung. Mit dem Sync-Archiv (syncx/archive.go, zuschaltbar) kommen sie
// zusätzlich verschlüsselt auf die anderen Rechner.
//
// # Sicherheit
//
//   - Schlüssel: aus dem Store-Schlüssel abgeleitet (vault.DeriveKey), AES-256-GCM,
//     neue Nonce bei jedem Schreiben, AAD bindet Format und ID der Datei. Ohne
//     entsperrten Store ist kein Protokoll lesbar.
//   - Passwort-Eingaben landen nie im Protokoll: Eingaben hinter einer
//     Passwortabfrage sind keine Befehle (siehe ui), die Shell zeigt sie nicht an,
//     und Befehle mit Leerzeichen am Anfang werden (wie bei HISTCONTROL=ignorespace)
//     bewusst nicht protokolliert.
//   - Vor dem Speichern entfernt Redact offensichtliche Geheimnisse (Tokens,
//     password=…, private Schlüssel) aus Befehl und Ausgabe.
//   - Export (Markdown, Text, asciinema-Aufnahme) schreibt Klartext, aber nur
//     auf ausdrücklichen Wunsch.
package sessionlog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/recording"
	"github.com/db9979/sshtermy/internal/vault"
)

const (
	formatName = "sshtermy-sessionlog"
	formatVer  = 1
	ext        = ".slog"

	MaxEntries     = 2000      // Befehle je Sitzung
	MaxOutputLines = 200       // Zeilen Ausgabe je Befehl (Anfang + Ende)
	MaxOutputBytes = 16 * 1024 // Bytes Ausgabe je Befehl
)

// Entry ist ein ausgeführter Befehl.
type Entry struct {
	At       time.Time `json:"at"`
	Command  string    `json:"cmd"`
	Output   string    `json:"out,omitempty"`
	Duration float64   `json:"dur_s,omitempty"`
	Failed   bool      `json:"failed,omitempty"`
	Exit     *int      `json:"exit,omitempty"` // nur wenn die Shell ihn meldet (OSC 133)
	Reason   string    `json:"reason,omitempty"`
	Agent    bool      `json:"agent,omitempty"` // vom Agent-Modus ausgeführt
}

// Log ist eine Sitzung.
type Log struct {
	ID      string    `json:"id"`
	ConnID  string    `json:"conn_id"`
	Host    string    `json:"host"` // Alias
	Addr    string    `json:"addr"` // user@host:port
	System  string    `json:"system,omitempty"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitempty"`
	Entries []Entry   `json:"entries"`
	Summary string    `json:"summary,omitempty"` // KI-Zusammenfassung
	Device  string    `json:"device,omitempty"`  // Rechner, von dem das Protokoll per Sync kam
}

// Failures zählt fehlgeschlagene Befehle.
func (l *Log) Failures() int {
	n := 0
	for _, e := range l.Entries {
		if e.Failed {
			n++
		}
	}
	return n
}

// Add hängt einen Befehl an (bereinigt und gekürzt).
func (l *Log) Add(e Entry) {
	e.Command = agent.Redact(strings.TrimSpace(e.Command))
	e.Output = agent.Redact(Clip(e.Output))
	if len(l.Entries) >= MaxEntries {
		l.Entries = l.Entries[1:]
	}
	l.Entries = append(l.Entries, e)
}

// Clip kürzt Ausgabe auf Anfang und Ende (das Ende enthält meist den Fehler).
func Clip(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n "), "\n")
	if len(lines) > MaxOutputLines {
		h := MaxOutputLines / 4
		t := MaxOutputLines - h
		skipped := len(lines) - h - t
		lines = append(append(lines[:h:h], fmt.Sprintf(i18n.T("… (%d Zeilen ausgelassen) …"), skipped)), lines[len(lines)-t:]...)
	}
	out := strings.Join(lines, "\n")
	if len(out) > MaxOutputBytes {
		cut := len(out) - MaxOutputBytes
		out = i18n.T("… (gekürzt) …\n") + strings.ToValidUTF8(out[cut:], "")
	}
	return out
}

// Book verwaltet die Protokolldateien in einem Ordner.
type Book struct {
	dir string
	mu  sync.Mutex
	key []byte
}

// Open bereitet den Ordner vor. key: 32 Byte (siehe store.SessionLogKey).
func Open(dir string, key []byte) (*Book, error) {
	if len(key) != 32 {
		return nil, errors.New(i18n.T("sessionlog: Schlüssel muss 32 Byte haben"))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Book{dir: dir, key: append([]byte(nil), key...)}, nil
}

// Close nullt den Schlüssel.
func (b *Book) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	vault.Wipe(b.key)
	b.key = nil
}

// New legt ein neues (noch ungespeichertes) Protokoll an.
func New(connID, host, addr string) *Log {
	r := make([]byte, 6)
	_, _ = rand.Read(r)
	now := time.Now()
	return &Log{ID: now.Format("20060102-150405") + "-" + hex.EncodeToString(r), ConnID: connID, Host: host, Addr: addr, Started: now}
}

type fileFmt struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	ID         string `json:"id"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func aad(id string) []byte { return []byte(fmt.Sprintf("%s/v%d/%s", formatName, formatVer, id)) }

func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c == '-') {
			return false
		}
	}
	return true
}

func (b *Book) path(id string) string { return filepath.Join(b.dir, id+ext) }

// Save verschlüsselt das Protokoll und schreibt es atomar (Rechte 0600).
func (b *Book) Save(l *Log) error {
	if !validID(l.ID) {
		return errors.New(i18n.T("sessionlog: ungültige ID"))
	}
	plain, err := json.Marshal(l)
	if err != nil {
		return err
	}
	defer vault.Wipe(plain)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.key == nil {
		return vault.ErrLocked
	}
	nonce, ct, err := vault.SealGCM(b.key, plain, aad(l.ID))
	if err != nil {
		return err
	}
	raw, err := json.Marshal(fileFmt{Format: formatName, Version: formatVer, ID: l.ID, Nonce: nonce, Ciphertext: ct})
	if err != nil {
		return err
	}
	tmp := b.path(l.ID) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, b.path(l.ID))
}

// Load liest ein Protokoll.
func (b *Book) Load(id string) (*Log, error) {
	if !validID(id) {
		return nil, errors.New(i18n.T("sessionlog: ungültige ID"))
	}
	raw, err := os.ReadFile(b.path(id))
	if err != nil {
		return nil, err
	}
	var f fileFmt
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if f.Format != formatName || f.ID != id {
		return nil, errors.New(i18n.T("sessionlog: unbekanntes Format"))
	}
	b.mu.Lock()
	key := b.key
	if key == nil {
		b.mu.Unlock()
		return nil, vault.ErrLocked
	}
	plain, err := vault.OpenGCM(key, f.Nonce, f.Ciphertext, aad(id))
	b.mu.Unlock()
	if err != nil {
		return nil, errors.New(i18n.T("sessionlog: Datei verändert oder anderer Schlüssel"))
	}
	defer vault.Wipe(plain)
	var l Log
	if err := json.Unmarshal(plain, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// List lädt alle lesbaren Protokolle, neueste zuerst. Unlesbare Dateien
// (anderer Schlüssel, beschädigt) werden übersprungen und gezählt.
func (b *Book) List() (logs []*Log, unreadable int, err error) {
	ents, err := os.ReadDir(b.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ext) {
			continue
		}
		l, err := b.Load(strings.TrimSuffix(name, ext))
		if err != nil {
			unreadable++
			continue
		}
		logs = append(logs, l)
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].Started.After(logs[j].Started) })
	return logs, unreadable, nil
}

// Has: gibt es das Protokoll auf diesem Rechner schon?
func (b *Book) Has(id string) bool {
	if !validID(id) {
		return false
	}
	_, err := os.Stat(b.path(id))
	return err == nil
}

// Delete entfernt ein Protokoll.
func (b *Book) Delete(id string) error {
	if !validID(id) {
		return errors.New(i18n.T("sessionlog: ungültige ID"))
	}
	return os.Remove(b.path(id))
}

// ---------------------------------------------------------------- Suche

// Matches: passt das Protokoll zur Suche? Jedes Wort muss irgendwo vorkommen
// (Host, Adresse, Befehle, Ausgabe, Zusammenfassung, Datum).
func (l *Log) Matches(q string) bool {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return true
	}
	hay := strings.ToLower(l.Host + " " + l.Addr + " " + l.System + " " + l.Summary + " " + l.Started.Format("2006-01-02 02.01.2006"))
	for _, w := range words {
		if strings.Contains(hay, w) {
			continue
		}
		found := false
		for _, e := range l.Entries {
			if strings.Contains(strings.ToLower(e.Command), w) || strings.Contains(strings.ToLower(e.Output), w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// MatchingEntries: Indizes der Befehle, in denen eines der Suchwörter vorkommt.
func (l *Log) MatchingEntries(q string) []int {
	words := strings.Fields(strings.ToLower(q))
	var out []int
	for i, e := range l.Entries {
		c, o := strings.ToLower(e.Command), strings.ToLower(e.Output)
		for _, w := range words {
			if strings.Contains(c, w) || strings.Contains(o, w) {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Markdown

// Markdown: das Protokoll als lesbares Dokument.
func (l *Log) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, i18n.T("# Sitzung %s – %s\n\n"), l.Host, l.Started.Format("02.01.2006 15:04"))
	fmt.Fprintf(&b, i18n.T("- **Verbindung:** %s\n"), l.Addr)
	if l.System != "" {
		fmt.Fprintf(&b, i18n.T("- **System:** %s\n"), l.System)
	}
	end := i18n.T("– (läuft noch oder abgebrochen)")
	if !l.Ended.IsZero() {
		end = l.Ended.Format("02.01.2006 15:04") + " (" + l.Ended.Sub(l.Started).Round(time.Second).String() + ")"
	}
	fmt.Fprintf(&b, i18n.T("- **Beginn:** %s\n- **Ende:** %s\n"), l.Started.Format("02.01.2006 15:04:05"), end)
	fmt.Fprintf(&b, i18n.T("- **Befehle:** %d, davon fehlgeschlagen: %d\n\n"), len(l.Entries), l.Failures())
	if s := strings.TrimSpace(l.Summary); s != "" {
		b.WriteString(i18n.T("## Zusammenfassung (KI)\n\n") + s + "\n\n")
	}
	b.WriteString(i18n.T("## Befehle\n\n"))
	for i, e := range l.Entries {
		mark := ""
		if e.Failed {
			mark = i18n.T(" × fehlgeschlagen")
			if e.Exit != nil {
				mark += fmt.Sprintf(i18n.T(" (Exit %d)"), *e.Exit)
			}
		}
		if e.Agent {
			mark = i18n.T(" · Agent") + mark
		}
		fmt.Fprintf(&b, "### %d. `%s`%s\n\n", i+1, strings.ReplaceAll(e.Command, "`", "'"), mark)
		fmt.Fprintf(&b, "_%s", e.At.Format("15:04:05"))
		if e.Duration >= 1 {
			fmt.Fprintf(&b, " · %.0f s", e.Duration)
		}
		b.WriteString("_\n\n")
		if out := strings.TrimSpace(e.Output); out != "" {
			b.WriteString("```text\n" + strings.ReplaceAll(out, "```", "'''") + "\n```\n\n")
		}
	}
	b.WriteString(i18n.T("---\nErstellt mit sshTermy\n"))
	return b.String()
}

// Text: das Protokoll als schlichter Text, so wie es im Terminal aussah
// (Prompt, Befehl, Ausgabe), für Tickets, Mails und Dokus.
func (l *Log) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, i18n.T("Sitzung %s (%s) vom %s\n"), l.Host, l.Addr, l.Started.Format("02.01.2006 15:04"))
	if l.System != "" {
		fmt.Fprintf(&b, i18n.T("System: %s\n"), l.System)
	}
	if s := strings.TrimSpace(l.Summary); s != "" {
		b.WriteString("\n" + s + "\n")
	}
	pr := l.prompt()
	for _, e := range l.Entries {
		mark := ""
		if e.Failed {
			mark = i18n.T("   # fehlgeschlagen")
			if e.Exit != nil {
				mark += fmt.Sprintf(" (Exit %d)", *e.Exit)
			}
		}
		fmt.Fprintf(&b, "\n[%s] %s%s%s\n", e.At.Format("15:04:05"), pr, e.Command, mark)
		if out := strings.TrimRight(e.Output, "\n "); out != "" {
			b.WriteString(out + "\n")
		}
	}
	return b.String()
}

// prompt: nachgebauter Prompt „user@host$ “ für Text und Aufnahme.
func (l *Log) prompt() string {
	who := l.Addr
	if i := strings.LastIndex(who, ":"); i > 0 && !strings.Contains(who[i:], "]") {
		who = who[:i] // Port weglassen
	}
	if who == "" {
		who = l.Host
	}
	return who + "$ "
}

// Cast baut aus dem Protokoll eine abspielbare Aufnahme (asciicast v2):
// Befehle erscheinen wie getippt, danach ihre Ausgabe. Pausen zwischen den
// Befehlen werden auf höchstens idle gekürzt (wie „asciinema rec -i“).
// Farben und Vollbildprogramme kennt das Protokoll nicht; dafür gibt es die
// Live-Aufnahme im Terminal.
func (l *Log) Cast(idle time.Duration) []byte {
	if idle <= 0 {
		idle = 2 * time.Second
	}
	width := 80
	for _, e := range l.Entries {
		for _, line := range strings.Split(e.Output, "\n") {
			width = max(width, min(200, utf8.RuneCountInString(line)))
		}
	}
	b := recording.NewBuilder(width, 30, l.Host+" · "+l.Started.Format("02.01.2006 15:04"), l.Started)
	pr := "\x1b[32m" + l.prompt() + "\x1b[0m"
	last := l.Started
	b.Out(pr)
	for i, e := range l.Entries {
		gap := e.At.Sub(last)
		if i == 0 {
			gap = min(gap, time.Second)
		}
		b.Wait(min(max(gap, 300*time.Millisecond), idle))
		b.Marker(e.Command)
		b.Type(e.Command, 35*time.Millisecond)
		b.Wait(250 * time.Millisecond)
		b.Out("\r\n")
		if out := strings.TrimRight(e.Output, "\n "); out != "" {
			b.Wait(200 * time.Millisecond)
			b.Out(strings.ReplaceAll(strings.ReplaceAll(out, "\r\n", "\n"), "\n", "\r\n") + "\r\n")
		}
		if e.Failed {
			b.Out("\x1b[31m" + i18n.T("× fehlgeschlagen") + "\x1b[0m\r\n")
		}
		dur := time.Duration(e.Duration * float64(time.Second))
		last = e.At.Add(dur)
		b.Wait(min(dur, idle))
		b.Out(pr)
	}
	b.Wait(time.Second)
	return b.Cast()
}

// Transcript: kompakte Fassung fürs Sprachmodell (Ausgabe je Befehl gekürzt).
func (l *Log) Transcript(maxBytes int) string {
	var b strings.Builder
	fmt.Fprintf(&b, i18n.T("Host: %s (%s), System: %s, Beginn: %s\n"), l.Host, l.Addr, l.System, l.Started.Format("02.01.2006 15:04"))
	for i, e := range l.Entries {
		st := ""
		if e.Failed {
			st = i18n.T(" [FEHLGESCHLAGEN]")
		}
		fmt.Fprintf(&b, "\n%d. %s $ %s%s\n", i+1, e.At.Format("15:04"), e.Command, st)
		out := strings.Split(strings.TrimSpace(e.Output), "\n")
		if len(out) > 12 {
			out = append(append(out[:4:4], "…"), out[len(out)-6:]...)
		}
		if o := strings.TrimSpace(strings.Join(out, "\n")); o != "" {
			b.WriteString(o + "\n")
		}
	}
	s := b.String()
	if len(s) > maxBytes {
		s = i18n.T("… (Anfang gekürzt) …\n") + strings.ToValidUTF8(s[len(s)-maxBytes:], "")
	}
	return s
}
