package gui

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// Jede Taste muss in bubbletea als genau die Taste ankommen, die die
// Tastenbelegung (internal/ui/keymap.go) erwartet.
func TestEncodeKeyRoundTrip(t *testing.T) {
	cases := []struct {
		k    uv.Key
		want string
	}{
		{uv.Key{Code: 'a', Text: "a"}, "a"},
		{uv.Key{Code: 'ä', Text: "ä"}, "ä"},
		{uv.Key{Code: '@', Text: "@"}, "@"},
		{uv.Key{Code: uv.KeyEnter}, "enter"},
		{uv.Key{Code: uv.KeyEnter, Mod: uv.ModAlt}, "alt+enter"},
		{uv.Key{Code: uv.KeyEnter, Mod: uv.ModShift}, "shift+enter"},
		{uv.Key{Code: uv.KeySpace, Mod: uv.ModCtrl}, "ctrl+space"},
		{uv.Key{Code: uv.KeyF1}, "f1"},
		{uv.Key{Code: uv.KeyF1, Mod: uv.ModShift}, "shift+f1"},
		{uv.Key{Code: uv.KeyF5, Mod: uv.ModShift}, "shift+f5"},
		{uv.Key{Code: uv.KeyF10, Mod: uv.ModShift}, "shift+f10"},
		{uv.Key{Code: uv.KeyF12}, "f12"},
		{uv.Key{Code: 'q', Mod: uv.ModCtrl}, "ctrl+q"},
		{uv.Key{Code: 'n', Mod: uv.ModAlt}, "alt+n"},
		{uv.Key{Code: 't', Mod: uv.ModAlt | uv.ModShift}, "alt+shift+t"},
		{uv.Key{Code: 'p', Mod: uv.ModCtrl | uv.ModShift}, "ctrl+shift+p"},
		{uv.Key{Code: uv.KeyUp}, "up"},
		{uv.Key{Code: uv.KeyUp, Mod: uv.ModShift}, "shift+up"},
		{uv.Key{Code: uv.KeyLeft, Mod: uv.ModCtrl}, "ctrl+left"},
		{uv.Key{Code: uv.KeyTab, Mod: uv.ModShift}, "shift+tab"},
		{uv.Key{Code: uv.KeyPgDown}, "pgdown"},
		{uv.Key{Code: uv.KeyDelete}, "delete"},
		{uv.Key{Code: uv.KeyBackspace}, "backspace"},
		{uv.Key{Code: uv.KeyEscape}, "esc"},
		{uv.Key{Code: uv.KeyHome}, "home"},
	}
	for _, c := range cases {
		b := encodeKey(c.k)
		var d uv.EventDecoder
		n, ev := d.Decode(b)
		if n != len(b) {
			t.Errorf("%q: nur %d von %d Bytes gelesen", b, n, len(b))
			continue
		}
		kp, ok := ev.(uv.KeyPressEvent)
		if !ok {
			t.Errorf("%q: kein Tastendruck, sondern %T", b, ev)
			continue
		}
		if got := kp.String(); got != c.want {
			t.Errorf("%+v → %q → %q, erwartet %q", c.k, b, got, c.want)
		}
	}
}
