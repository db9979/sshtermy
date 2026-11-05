package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/version"
)

func TestIntroFrames(t *testing.T) {
	for _, th := range Themes {
		for f := 0; f <= int(introDur*introFPS); f++ {
			out := renderIntro(th, float64(f)/introFPS, 80, 24, false)
			lines := strings.Split(out, "\n")
			if len(lines) != 24 {
				t.Fatalf("%s Frame %d: %d Zeilen", th.Name, f, len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != 80 {
					t.Fatalf("%s Frame %d Zeile %d: Breite %d", th.Name, f, i, w)
				}
			}
		}
	}
	last := ansi.Strip(renderIntro(Nacht, introDur, 80, 24, true))
	for _, want := range []string{"█", "✓ entschlüsselt", version.Idea, "überspringt"} {
		if !strings.Contains(last, want) {
			t.Errorf("Endbild ohne %q", want)
		}
	}
	if strings.ContainsAny(last, `░▒▓{}#$%&`) {
		t.Error("Endbild enthält noch Zufallszeichen")
	}
	if strings.Contains(ansi.Strip(renderIntro(Nacht, introDur, 80, 24, false)), "entschlüsselt") {
		t.Error("ohne DPAPI-Entsperren darf nicht „entschlüsselt“ dastehen")
	}
}

func TestIntroSkipAndDisable(t *testing.T) {
	t.Setenv("SSHTERMY_NO_INTRO", "")
	dir := t.TempDir()
	if !IntroEnabled(dir) {
		t.Fatal("Startanimation sollte standardmäßig an sein")
	}
	a := &App{st: &store.Store{Dir: dir}, s: NewStyles(Nacht), keys: newKeymap(nil), intro: &introState{start: time.Now()}}
	a.w, a.h = 80, 24
	if !strings.Contains(ansi.Strip(a.View().Content), "überspringt") {
		t.Fatal("Animation wird nicht gezeigt")
	}
	// Taste überspringt und wird verschluckt
	if _, ok := a.introMsg(tea.KeyPressMsg{Code: 'q', Text: "q"}); !ok || a.intro != nil {
		t.Fatal("Taste hat die Animation nicht beendet")
	}
	// Zeit abgelaufen
	a.intro = &introState{start: time.Now().Add(-3 * time.Second)}
	if cmd, ok := a.introMsg(introFrameMsg(time.Now())); !ok || cmd != nil || a.intro != nil {
		t.Fatal("Animation endet nicht nach Ablauf")
	}
	// Abschalten über die Palette legt die Datei an
	a.toggleIntro()
	if IntroEnabled(dir) {
		t.Fatal("abschalten wirkt nicht")
	}
	if _, err := os.Stat(filepath.Join(dir, noIntroFile)); err != nil {
		t.Fatal(err)
	}
	a.toggleIntro()
	if !IntroEnabled(dir) {
		t.Fatal("einschalten wirkt nicht")
	}
	t.Setenv("SSHTERMY_NO_INTRO", "1")
	if IntroEnabled(dir) {
		t.Fatal("SSHTERMY_NO_INTRO wirkt nicht")
	}
	if introFits(40, 12) {
		t.Fatal("kleines Fenster sollte die Animation auslassen")
	}
}
