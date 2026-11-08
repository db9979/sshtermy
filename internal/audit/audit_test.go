package audit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var key = []byte("0123456789abcdef0123456789abcdef")

func fill(t *testing.T) (*Log, string) {
	t.Helper()
	dir := t.TempDir()
	l, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)
	for i, ev := range []string{EvStart, EvConnect, EvCommand, EvCommand, EvDisconnect} {
		if err := l.add(t0.Add(time.Duration(i)*30*time.Minute), ev, "sw1 (admin@10.0.0.1)", "show run "+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	return l, dir
}

func TestChainOK(t *testing.T) {
	l, dir := fill(t)
	r := l.Verify()
	if !r.OK() || len(r.Entries) != 5 {
		t.Fatalf("%v %d", r.Problems, len(r.Entries))
	}
	if len(l.files()) != 2 {
		t.Fatalf("Monatsdateien: %v", l.files())
	}
	// Wieder öffnen: Kette geht weiter.
	l2, _ := Open(dir, key)
	l2.Add(EvCommand, "x", "y")
	if r := l2.Verify(); !r.OK() || len(r.Entries) != 6 {
		t.Fatalf("nach erneutem Öffnen: %v", r.Problems)
	}
	if !strings.Contains(Text(r.Entries), "show run a") {
		t.Fatal(Text(r.Entries))
	}
}

func edit(t *testing.T, l *Log, i int, fn func([][]byte) [][]byte) {
	t.Helper()
	f := l.files()[i]
	b, _ := os.ReadFile(f)
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	lines = fn(lines)
	os.WriteFile(f, append(bytes.Join(lines, []byte("\n")), '\n'), 0o600)
}

func TestTamper(t *testing.T) {
	cases := map[string]func(*Log){
		"geändert": func(l *Log) {
			edit(t, l, 1, func(ls [][]byte) [][]byte {
				ls[0] = bytes.Replace(ls[0], []byte("show run"), []byte("show xxx"), 1)
				return ls
			})
		},
		"gelöscht": func(l *Log) {
			edit(t, l, 1, func(ls [][]byte) [][]byte { return append(ls[:1], ls[2:]...) })
		},
		"vertauscht": func(l *Log) {
			edit(t, l, 1, func(ls [][]byte) [][]byte { ls[0], ls[1] = ls[1], ls[0]; return ls })
		},
		"Ende abgeschnitten": func(l *Log) {
			edit(t, l, 1, func(ls [][]byte) [][]byte { return ls[:len(ls)-1] })
		},
		"erste Datei weg": func(l *Log) { os.Remove(l.files()[0]) },
		"Kopf gefälscht": func(l *Log) {
			os.WriteFile(filepath.Join(l.dir, "head"), []byte(`{"n":1,"mac":"x","sig":"y"}`), 0o600)
		},
	}
	for name, fn := range cases {
		l, _ := fill(t)
		fn(l)
		if r := l.Verify(); r.OK() {
			t.Errorf("%s: nicht erkannt", name)
		}
	}
	// anderer Schlüssel: alles ungültig
	l, dir := fill(t)
	_ = l
	o, _ := Open(dir, []byte("anderer schlüssel"))
	if o.Verify().OK() {
		t.Error("fremder Schlüssel prüft als gültig")
	}
}
