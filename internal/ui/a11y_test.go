package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/debuglog"
	"github.com/db9979/sshtermy/internal/domain"
)

// Barrierearm: Einstellung, keine Animation, × statt rotem Punkt.
func TestAccessibleMode(t *testing.T) {
	a := newPaletteTestApp(t)
	defer func() { accessible = false }()
	c := domain.Connection{ID: "x", Alias: "web", Host: "h"}
	a.reach.res = map[string]reachResult{"x": {ok: false}}
	if dot, _ := a.reachDot(c); dot != "●" {
		t.Fatalf("ohne Modus: %q", dot)
	}
	a.toggleAccessible()
	if !a.st.Settings().Accessible || a.st.Settings().Intro != "aus" || animEnabled() {
		t.Fatalf("Einstellung/Animation: %+v anim=%t", a.st.Settings(), animEnabled())
	}
	if dot, _ := a.reachDot(c); dot != "×" {
		t.Fatalf("mit Modus: %q", dot)
	}
	a.toggleAccessible()
	if accessible || a.st.Settings().Accessible {
		t.Fatal("ausschalten")
	}
}

// Debug-Protokoll: Schalter, Ereignisse, bereinigte Kopie ohne Host-Namen.
func TestDebugLogExport(t *testing.T) {
	a := newPaletteTestApp(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	defer debuglog.Set(false)
	if _, err := a.st.AddConnections([]domain.Connection{{Alias: "geheim-web", Host: "geheim.example.org", Username: "root"}}); err != nil {
		t.Fatal(err)
	}
	a.toggleDebug()
	if !a.debugOn() || !a.st.Settings().Debug {
		t.Fatal("nicht an")
	}
	a.debugMsg(connectErrMsg{sid: 7, err: os.ErrDeadlineExceeded})
	debuglog.Printf("ssh", "geheim-web: verbinde zu geheim.example.org als root")
	raw, _ := debuglog.Read()
	if !strings.Contains(raw, "#7 Fehler") || !strings.Contains(raw, "[start]") {
		t.Fatalf("Protokoll:\n%s", raw)
	}
	a.exportDebug()
	m, _ := filepath.Glob(filepath.Join(home, "sshtermy-debug-*.txt"))
	if len(m) != 1 {
		t.Fatalf("Export fehlt: %v", m)
	}
	b, _ := os.ReadFile(m[0])
	if strings.Contains(string(b), "geheim") || !strings.Contains(string(b), "verbinde") {
		t.Fatalf("nicht bereinigt:\n%s", b)
	}
	a.toggleDebug()
	if a.debugOn() {
		t.Fatal("nicht aus")
	}
}
