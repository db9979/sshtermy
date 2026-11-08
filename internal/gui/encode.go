// Package gui ist das eigene Fenster von sshTermy: Die Oberfläche läuft
// unverändert als bubbletea-Programm, schreibt ihre Ausgabe aber nicht in ein
// fremdes Terminal, sondern in den eingebauten Emulator (internal/termemu).
// Ein Ebiten-Fenster zeichnet dessen Zellraster mit eingebauter Schrift und
// schickt Tasten, Maus und Einfügen als Terminal-Sequenzen zurück.
//
// Dieser Teil (Tastenkodierung, Farben, Rahmenzeichen) braucht kein Fenster
// und ist auf allen Plattformen testbar. Das Fenster selbst steckt in
// window.go: Windows, macOS und Linux (auch armhf/riscv64), dank Ebitens
// purego ohne cgo, also mit denselben Builds wie bisher.
package gui

import (
	"strconv"
	"unicode"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
)

// encodeKey übersetzt einen Tastendruck in die Bytes, die ein xterm schicken
// würde. Wo xterm mehrdeutig ist (Strg+Leertaste mit Shift, Shift+Enter,
// Strg+Shift+Buchstabe), nimmt es CSI u: bubbletea versteht das immer.
// Dadurch kommen Tasten an, die fremde Terminals gern selbst schlucken
// (Alt+Enter, ⇧F-Tasten, Strg+Leertaste).
func encodeKey(k uv.Key) []byte {
	mod := k.Mod & (uv.ModShift | uv.ModAlt | uv.ModCtrl)
	// reiner Text (auch Umlaute, AltGr-Zeichen, tote Tasten)
	if k.Text != "" && mod&(uv.ModCtrl|uv.ModAlt) == 0 {
		return []byte(k.Text)
	}
	param := 1
	if mod&uv.ModShift != 0 {
		param++
	}
	if mod&uv.ModAlt != 0 {
		param += 2
	}
	if mod&uv.ModCtrl != 0 {
		param += 4
	}
	esc := func(b ...byte) []byte {
		if mod&uv.ModAlt != 0 {
			return append([]byte{0x1b}, b...)
		}
		return b
	}
	csiu := func(code rune) []byte {
		return []byte("\x1b[" + strconv.Itoa(int(code)) + ";" + strconv.Itoa(param) + "u")
	}
	// CSI 1;m X (Pfeile, Pos1, Ende, F1–F4)
	letter := func(final byte, ss3 bool) []byte {
		if param == 1 {
			if ss3 {
				return []byte{0x1b, 'O', final}
			}
			return []byte{0x1b, '[', final}
		}
		return []byte("\x1b[1;" + strconv.Itoa(param) + string(final))
	}
	// CSI n ~ / CSI n;m ~
	tilde := func(n int) []byte {
		if param == 1 {
			return []byte("\x1b[" + strconv.Itoa(n) + "~")
		}
		return []byte("\x1b[" + strconv.Itoa(n) + ";" + strconv.Itoa(param) + "~")
	}

	switch k.Code {
	case uv.KeyUp:
		return letter('A', false)
	case uv.KeyDown:
		return letter('B', false)
	case uv.KeyRight:
		return letter('C', false)
	case uv.KeyLeft:
		return letter('D', false)
	case uv.KeyHome:
		return letter('H', false)
	case uv.KeyEnd:
		return letter('F', false)
	case uv.KeyInsert:
		return tilde(2)
	case uv.KeyDelete:
		return tilde(3)
	case uv.KeyPgUp:
		return tilde(5)
	case uv.KeyPgDown:
		return tilde(6)
	case uv.KeyF1, uv.KeyF2, uv.KeyF3, uv.KeyF4:
		return letter(byte('P'+k.Code-uv.KeyF1), true)
	case uv.KeyF5:
		return tilde(15)
	case uv.KeyF6, uv.KeyF7, uv.KeyF8, uv.KeyF9, uv.KeyF10:
		return tilde(17 + int(k.Code-uv.KeyF6))
	case uv.KeyF11, uv.KeyF12:
		return tilde(23 + int(k.Code-uv.KeyF11))
	case uv.KeyEnter, uv.KeyKpEnter:
		if mod&(uv.ModShift|uv.ModCtrl) != 0 {
			return csiu(13)
		}
		return esc('\r')
	case uv.KeyTab:
		switch {
		case mod == uv.ModShift:
			return []byte("\x1b[Z")
		case mod&(uv.ModShift|uv.ModCtrl) != 0:
			return csiu(9)
		}
		return esc('\t')
	case uv.KeyBackspace:
		switch {
		case mod&uv.ModShift != 0:
			return csiu(127)
		case mod&uv.ModCtrl != 0:
			return esc(0x08)
		}
		return esc(0x7f)
	case uv.KeyEscape:
		if mod&(uv.ModShift|uv.ModCtrl) != 0 {
			return csiu(27)
		}
		return esc(0x1b)
	case uv.KeySpace:
		switch {
		case mod&uv.ModShift != 0:
			return csiu(' ')
		case mod&uv.ModCtrl != 0:
			return esc(0)
		}
		return esc(' ')
	}

	r := k.Code
	if r < 0x20 || r > utf8.MaxRune || !utf8.ValidRune(r) {
		return nil
	}
	if mod&uv.ModCtrl != 0 {
		// Strg+Buchstabe ohne Shift: klassisches Steuerzeichen
		if mod&uv.ModShift == 0 {
			lr := unicode.ToLower(r)
			switch {
			case lr >= 'a' && lr <= 'z':
				return esc(byte(lr-'a') + 1)
			case r == '[' || r == '\\' || r == ']' || r == '^' || r == '_':
				return esc(byte(r) - 0x40)
			}
		}
		return csiu(unicode.ToLower(r))
	}
	// nur Alt (bzw. Alt+Shift): ESC + Zeichen
	txt := k.Text
	if txt == "" {
		if mod&uv.ModShift != 0 {
			r = unicode.ToUpper(r)
		}
		txt = string(r)
	}
	return esc([]byte(txt)...)
}
