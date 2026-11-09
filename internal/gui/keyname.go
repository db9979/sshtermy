package gui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
)

// parseKey liest Tastennamen wie in der Tastenbelegung („ctrl+space“,
// „shift+f10“, „alt+n“). Für Skripte und Tests.
func parseKey(s string) (uv.Key, bool) {
	parts := strings.Split(strings.ToLower(s), "+")
	var k uv.Key
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "ctrl":
			k.Mod |= uv.ModCtrl
		case "alt":
			k.Mod |= uv.ModAlt
		case "shift":
			k.Mod |= uv.ModShift
		default:
			return k, false
		}
	}
	name := parts[len(parts)-1]
	named := map[string]rune{
		"enter": uv.KeyEnter, "tab": uv.KeyTab, "esc": uv.KeyEscape, "space": uv.KeySpace,
		"backspace": uv.KeyBackspace, "up": uv.KeyUp, "down": uv.KeyDown, "left": uv.KeyLeft,
		"right": uv.KeyRight, "home": uv.KeyHome, "end": uv.KeyEnd, "pgup": uv.KeyPgUp,
		"pgdown": uv.KeyPgDown, "insert": uv.KeyInsert, "delete": uv.KeyDelete,
	}
	if r, ok := named[name]; ok {
		k.Code = r
		return k, true
	}
	if len(name) >= 2 && name[0] == 'f' {
		if n, err := strconv.Atoi(name[1:]); err == nil && n >= 1 && n <= 12 {
			k.Code = uv.KeyF1 + rune(n-1)
			return k, true
		}
	}
	if r, size := utf8.DecodeRuneInString(name); size == len(name) && size > 0 {
		k.Code = r
		if k.Mod == 0 || k.Mod == uv.ModShift {
			k.Text = string(r)
		}
		return k, true
	}
	return k, false
}
