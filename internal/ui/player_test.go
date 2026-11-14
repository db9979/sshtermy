package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/recording"
)

func TestPlayer(t *testing.T) {
	a := vulnTestApp(t)
	a.w, a.h = 100, 30
	b := recording.NewBuilder(60, 10, "web01 · Test", time.Now())
	b.Out("$ uptime\r\n")
	b.Wait(2 * time.Second)
	b.Out("\x1b[32mload 0.10\x1b[0m\r\n")
	b.Marker("Schritt 2")
	b.Wait(60 * time.Second)
	b.Out("$ df -h\r\nFEHLER: Platte voll\r\n")
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	_ = os.MkdirAll(filepath.Join(dir, "Downloads"), 0o700)
	p := filepath.Join(dir, "Downloads", "web01.cast")
	if err := os.WriteFile(p, b.Cast(), 0o600); err != nil {
		t.Fatal(err)
	}
	a.openPlayerPicker()
	if a.pick == nil || len(a.pick.items) != 2 {
		t.Fatalf("picker: %+v", a.pick)
	}
	cmd := a.pickerChoose(pickItem{value: p})
	if a.scr != scrPlayer || cmd == nil || !a.play.playing {
		t.Fatal("spielt nicht")
	}
	key := func(s string) {
		var k tea.KeyPressMsg
		switch s {
		case "space":
			k = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		case "right":
			k = tea.KeyPressMsg{Code: tea.KeyRight}
		case "left":
			k = tea.KeyPressMsg{Code: tea.KeyLeft}
		default:
			k = tea.KeyPressMsg{Code: rune(s[0]), Text: s}
		}
		a.Update(k)
	}
	key("space") // Pause
	if a.play.playing {
		t.Fatal("Pause geht nicht")
	}
	key("right")
	view := a.viewPlayer()
	if !strings.Contains(view, "load 0.10") || strings.Contains(view, "Platte voll") || !strings.Contains(view, "Schritt 2") {
		t.Fatalf("nach 5 s:\n%s", view)
	}
	// Suche springt zum Treffer
	key("/")
	if a.modal == nil || a.modal.onText == nil {
		t.Fatal("keine Suche")
	}
	a.modal.onText("platte VOLL")
	a.modal = nil
	if v := a.viewPlayer(); !strings.Contains(v, "FEHLER: Platte voll") || !strings.Contains(v, "01:02") {
		t.Fatalf("Suche:\n%s", v)
	}
	// zurückspulen baut neu auf
	key("0")
	if v := a.viewPlayer(); strings.Contains(v, "load 0.10") || a.play.t != 0 {
		t.Fatalf("Anfang:\n%s", v)
	}
	// Pausen kürzen: Länge 4 s statt 62 s
	key("i")
	if d := a.play.cast.Duration(); d != 4 {
		t.Fatalf("gekürzt: %v", d)
	}
	key("+")
	if playSpeeds[a.play.speed] != 2 {
		t.Fatalf("Tempo %v", playSpeeds[a.play.speed])
	}
	// abspielen bis zum Ende
	a.play.playing, a.play.last = true, time.Now().Add(-10*time.Second)
	a.Update(playTickMsg{seq: a.play.seq})
	if a.play.playing || a.play.pos != len(a.play.cast.Events) {
		t.Fatal("nicht am Ende")
	}
	key("q")
	if a.scr == scrPlayer || a.play.term != nil {
		t.Fatal("nicht geschlossen")
	}
}
