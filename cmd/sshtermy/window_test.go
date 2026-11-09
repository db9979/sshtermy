package main

import (
	"testing"

	"github.com/db9979/sshtermy/internal/gui"
)

// Die Schriftgröße des Fensters bleibt über einen Neustart.
func TestWindowFontRemembered(t *testing.T) {
	dir := t.TempDir()
	o := withFont(dir, gui.Options{})
	if o.FontSize != 0 {
		t.Fatalf("ohne Datei: %v", o.FontSize)
	}
	o.OnFontSize(17)
	if o = withFont(dir, gui.Options{}); o.FontSize != 17 {
		t.Fatalf("gemerkt: %v", o.FontSize)
	}
}
