// Package notify zeigt eine Desktop-Benachrichtigung des Betriebssystems
// (Windows-Infobereich, macOS-Mitteilungszentrale, Linux notify-send).
// sshTermy nutzt das, wenn ein langer Befehl in einem Tab fertig ist, während
// man in einem anderen Fenster arbeitet. Es gibt keine Abhängigkeit und kein
// cgo: jede Plattform ruft ihr eigenes Bordmittel auf. Fehlt es, passiert
// einfach nichts.
package notify

import (
	"strings"
	"unicode/utf8"
)

// Send zeigt title und body an und kehrt sofort zurück.
func Send(title, body string) {
	title, body = clean(title, 64), clean(body, 200)
	go func() { _ = send(title, body) }()
}

// clean: einzeilig, ohne Steuerzeichen, höchstens n Zeichen.
func clean(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > n {
		r := []rune(s)
		s = string(r[:n-1]) + "…"
	}
	return s
}

// quoteAS: Zeichenkette für AppleScript ("…" mit \" und \\).
func quoteAS(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// quotePS: Zeichenkette für PowerShell in einfachen Anführungszeichen.
func quotePS(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// xmlEscape für den Text im Toast-XML.
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}
