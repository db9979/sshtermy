package sessionlog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func TestSaveLoadEncrypted(t *testing.T) {
	dir := t.TempDir()
	b, err := Open(dir, key(1))
	if err != nil {
		t.Fatal(err)
	}
	l := New("c1", "web01", "root@web01:22")
	l.Add(Entry{Command: "systemctl restart nginx", Output: "Job for nginx.service failed", Failed: true})
	l.Add(Entry{Command: "export TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123", Output: "password=hunter2"})
	if err := b.Save(l); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, l.ID+ext))
	for _, plain := range []string{"nginx", "web01", "hunter2"} {
		if bytes.Contains(raw, []byte(plain)) {
			t.Fatalf("Klartext %q in Datei", plain)
		}
	}
	got, err := b.Load(l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "web01" || len(got.Entries) != 2 || !got.Entries[0].Failed {
		t.Fatalf("got %+v", got)
	}
	if strings.Contains(got.Entries[1].Command, "ghp_") || strings.Contains(got.Entries[1].Output, "hunter2") {
		t.Fatalf("Geheimnis nicht entfernt: %+v", got.Entries[1])
	}

	// anderer Schlüssel: nicht lesbar, wird in List übersprungen
	b2, _ := Open(dir, key(2))
	if _, err := b2.Load(l.ID); err == nil {
		t.Fatal("fremder Schlüssel darf nicht entschlüsseln")
	}
	logs, bad, err := b2.List()
	if err != nil || len(logs) != 0 || bad != 1 {
		t.Fatalf("List mit fremdem Schlüssel: %d %d %v", len(logs), bad, err)
	}

	// Datei unter anderem Namen (AAD bindet die ID)
	other := New("c1", "x", "y")
	_ = os.WriteFile(filepath.Join(dir, other.ID+ext), raw, 0o600)
	if _, err := b.Load(other.ID); err == nil {
		t.Fatal("umbenannte Datei darf nicht akzeptiert werden")
	}

	b.Close()
	if err := b.Save(l); err == nil {
		t.Fatal("Save nach Close muss scheitern")
	}
}

func TestValidIDPreventsTraversal(t *testing.T) {
	b, _ := Open(t.TempDir(), key(1))
	if _, err := b.Load("../store"); err == nil {
		t.Fatal("Pfad außerhalb akzeptiert")
	}
	if err := b.Delete("../../x"); err == nil {
		t.Fatal("Delete außerhalb akzeptiert")
	}
}

func TestSearchAndMarkdown(t *testing.T) {
	l := New("c1", "switch-01", "admin@10.0.0.2:22")
	l.Add(Entry{Command: "show vlan", Output: "VLAN 10 Büro"})
	l.Add(Entry{Command: "conf t", Output: "% Invalid input", Failed: true})
	if !l.Matches("switch büro") || !l.Matches("INVALID") || l.Matches("nginx") {
		t.Fatal("Suche falsch")
	}
	if got := l.MatchingEntries("vlan"); len(got) != 1 || got[0] != 0 {
		t.Fatalf("MatchingEntries %v", got)
	}
	md := l.Markdown()
	for _, want := range []string{"# Sitzung switch-01", "### 2. `conf t` ✗ fehlgeschlagen", "```text\nVLAN 10 Büro\n```"} {
		if !strings.Contains(md, want) {
			t.Fatalf("Markdown ohne %q:\n%s", want, md)
		}
	}
}

func TestClip(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		b.WriteString("zeile\n")
	}
	out := Clip(b.String())
	if n := len(strings.Split(out, "\n")); n > MaxOutputLines+1 {
		t.Fatalf("%d Zeilen", n)
	}
	if !strings.Contains(out, "ausgelassen") {
		t.Fatal("kein Hinweis auf Kürzung")
	}
}
