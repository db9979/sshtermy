//go:build windows || darwin || linux

package gui

import (
	"image/png"
	"os"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// script steuert das Fenster für Bildschirmfotos (Webseite, Doku, Tests):
// SSHTERMY_GUI_SCRIPT="wait:120;key:ctrl+space;wait:30;shot:/tmp/a.png;quit"
// wait zählt Ticks (60 je Sekunde).
type script struct {
	steps []string
	wait  int
	shot  string
}

func newScript(s string) *script {
	if s == "" {
		return nil
	}
	return &script{steps: strings.Split(s, ";")}
}

func (s *script) step(w *window) {
	if s.wait > 0 {
		s.wait--
		return
	}
	for len(s.steps) > 0 && s.shot == "" {
		st := strings.TrimSpace(s.steps[0])
		s.steps = s.steps[1:]
		cmd, arg, _ := strings.Cut(st, ":")
		switch cmd {
		case "wait":
			s.wait, _ = strconv.Atoi(arg)
			return
		case "key":
			if k, ok := parseKey(arg); ok {
				w.sendOrdered(encodeKey(k))
			}
		case "text":
			w.sendOrdered([]byte(arg))
		case "shot":
			s.shot = arg
			w.forceDraw = true
			return
		case "quit":
			if w.onQuit != nil {
				w.onQuit()
			}
			return
		}
	}
}

func (s *script) afterDraw(screen *ebiten.Image) {
	if s.shot == "" {
		return
	}
	path := s.shot
	s.shot = ""
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, screen)
}
