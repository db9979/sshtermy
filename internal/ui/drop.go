package ui

import (
	"net/url"
	"os"
	"runtime"
	"strings"
	"unicode"
)

// Dateien ins Fenster ziehen
//
// Zieht man Dateien aus dem Explorer, Finder oder Dateimanager in ein
// Terminalfenster, fügt das Terminal ihre Pfade als Text ein. Jedes Terminal
// macht das ein wenig anders:
//
//	Windows Terminal   "C:\Users\me\My File.txt" "C:\b.txt"
//	macOS Terminal     /Users/me/My\ File.txt /Users/me/b.txt
//	iTerm2             /Users/me/My\ File.txt
//	GNOME/Konsole      '/home/me/My File.txt' '/home/me/b.txt'
//	manche Linux-DEs   file:///home/me/My%20File.txt
//
// droppedPaths erkennt das: Nur wenn der eingefügte Text ausschließlich aus
// absoluten Pfaden besteht, die es auf diesem Rechner gibt, gilt er als
// „abgelegte Dateien“. Sonst ist es normaler Text.

const maxDropped = 500

// droppedPaths: die abgelegten Dateien, oder nil, wenn der Text keine ist.
func droppedPaths(text string) []string {
	toks := splitDropped(text, runtime.GOOS == "windows")
	if len(toks) == 0 || len(toks) > maxDropped {
		return nil
	}
	for _, p := range toks {
		if _, err := os.Stat(p); err != nil {
			return nil
		}
	}
	return toks
}

// splitDropped zerlegt den Text in Pfade (ohne Prüfung, ob es sie gibt).
// nil, wenn ein Teil kein absoluter Pfad ist. win: Backslash ist dann
// Pfadtrenner und kein Fluchtzeichen.
func splitDropped(text string, win bool) []string {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if text == "" {
		return nil
	}
	var (
		out     []string
		cur     strings.Builder
		inTok   bool
		quote   rune // 0, '\'' oder '"'
		escaped bool
	)
	flush := func() {
		if inTok {
			out = append(out, cur.String())
		}
		cur.Reset()
		inTok = false
	}
	for _, r := range text {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' && !win {
				escaped = true
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inTok = r, true
		case r == '\\' && !win:
			escaped, inTok = true, true
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if quote != 0 || escaped {
		return nil // unvollständig: kein Dateipfad
	}
	flush()
	for i, p := range out {
		if strings.HasPrefix(p, "file://") {
			u, err := url.Parse(p)
			if err != nil || (u.Host != "" && u.Host != "localhost") {
				return nil
			}
			p = u.Path
			if win && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
				p = p[1:] // /C:/x → C:/x
			}
		}
		if !isAbsPath(p, win) {
			return nil
		}
		out[i] = p
	}
	return out
}

func isAbsPath(p string, win bool) bool {
	if !win {
		return strings.HasPrefix(p, "/")
	}
	if strings.HasPrefix(p, `\\`) {
		return len(p) > 2
	}
	return len(p) >= 3 && unicode.IsLetter(rune(p[0])) && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}
