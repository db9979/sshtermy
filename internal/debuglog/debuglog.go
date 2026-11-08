// Package debuglog schreibt ein Debug-Protokoll für Fehlerberichte:
// Verbindungsaufbau, Abbrüche, Updates, KI-Anfragen, Sync. Nur Ereignisse,
// nie Passwörter, Tastatureingaben oder Bildschirminhalte.
//
// Aus ist es ein einziger atomarer Vergleich pro Aufruf. An schreibt es nach
// <Datenordner>/debug.log; über 4 MiB wird die Datei zu debug.log.1 (die
// vorige fällt weg). Zum Weitergeben schreibt die Oberfläche eine bereinigte
// Kopie (ohne Hosts, IPs, Benutzer, Geheimnisse), siehe internal/ui.
package debuglog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// FileName im Datenordner.
	FileName = "debug.log"
	maxSize  = 4 << 20
)

var (
	on   atomic.Bool
	mu   sync.Mutex
	dir  string
	f    *os.File
	size int64
)

// Start legt den Ordner fest und schaltet das Protokoll ein oder aus.
func Start(d string, enabled bool) error {
	mu.Lock()
	dir = d
	mu.Unlock()
	return Set(enabled)
}

// Set schaltet ein oder aus. Beim Einschalten steht eine Kopfzeile in der Datei.
func Set(enabled bool) error {
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		on.Store(false)
		closeLocked()
		return nil
	}
	if dir == "" {
		return fmt.Errorf("debuglog: kein Ordner")
	}
	if err := openLocked(); err != nil {
		return err
	}
	on.Store(true)
	return nil
}

// On meldet, ob gerade protokolliert wird.
func On() bool { return on.Load() }

// Path: volle Adresse der Protokolldatei.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return filepath.Join(dir, FileName)
}

// Printf schreibt eine Zeile „Zeit [bereich] text“. Zeilenumbrüche im Text
// werden zu „ ⏎ “, damit jede Meldung eine Zeile bleibt.
func Printf(area, format string, args ...any) {
	if !on.Load() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	msg = strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\r", " ").Replace(msg)
	line := time.Now().Format("2006-01-02 15:04:05.000") + " [" + area + "] " + msg + "\n"
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		return
	}
	if size+int64(len(line)) > maxSize {
		closeLocked()
		p := filepath.Join(dir, FileName)
		_ = os.Remove(p + ".1")
		_ = os.Rename(p, p+".1")
		if openLocked() != nil {
			on.Store(false)
			return
		}
	}
	n, _ := f.WriteString(line)
	size += int64(n)
}

// Err: wie Printf, aber nur wenn err != nil (spart das if an jeder Stelle).
func Err(area, what string, err error) {
	if err != nil {
		Printf(area, "%s: %v", what, err)
	}
}

// Read liefert den Inhalt beider Dateien (ältere zuerst).
func Read() (string, error) {
	p := Path()
	old, _ := os.ReadFile(p + ".1")
	cur, err := os.ReadFile(p)
	if err != nil && len(old) == 0 {
		return "", err
	}
	return string(old) + string(cur), nil
}

// Clear löscht beide Dateien; läuft das Protokoll, beginnt es neu.
func Clear() error {
	mu.Lock()
	defer mu.Unlock()
	wasOn := f != nil
	closeLocked()
	p := filepath.Join(dir, FileName)
	_ = os.Remove(p + ".1")
	err := os.Remove(p)
	if os.IsNotExist(err) {
		err = nil
	}
	if wasOn {
		if e := openLocked(); e != nil {
			on.Store(false)
			return e
		}
	}
	return err
}

func openLocked() error {
	if f != nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fh, err := os.OpenFile(filepath.Join(dir, FileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, _ := fh.Stat()
	f, size = fh, 0
	if st != nil {
		size = st.Size()
	}
	return nil
}

func closeLocked() {
	if f != nil {
		_ = f.Close()
		f = nil
	}
}
