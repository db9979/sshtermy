package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Hintergrund und Schrift des Themes in jede Zelle schreiben.
//
// Die Oberfläche lässt die Grundfarben meist offen („Standardfarbe des
// Terminals“) und meldet sie dem Terminal nur per OSC 10/11. Viele Terminals
// übernehmen das nicht (Windows-Konsole, ConPTY, Terminal.app, ältere Windows
// Terminal), dann blieben helle Themes auf dunklem Grund. fillDefaultColors
// ersetzt darum jede Rückkehr zur Standardfarbe (SGR 0, 39, 49) durch die
// Theme-Farbe und setzt sie einmal am Anfang. Ausgabe vom Server im
// Terminal-Bereich bekommt so ebenfalls den Theme-Grund.

// sgrColor: SGR-Parameter für eine Vorder- (38) oder Hintergrundfarbe (48).
func sgrColor(base int, c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("%d;2;%d;%d;%d", base, r>>8, g>>8, b>>8)
}

func fillDefaultColors(s string, fg, bg color.Color) string {
	if fg == nil || bg == nil {
		return s
	}
	fgP, bgP := sgrColor(38, fg), sgrColor(48, bg)
	var b strings.Builder
	b.Grow(len(s) + len(s)/8 + 32)
	b.WriteString("\x1b[" + fgP + ";" + bgP + "m")
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		s = s[i:]
		// Ende der CSI-Folge: erstes Byte 0x40–0x7E nach den Parametern
		j := 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		if j >= len(s) {
			b.WriteString(s)
			break
		}
		params := s[2:j]
		if s[j] == 'm' && plainSGR(params) {
			b.WriteString("\x1b[")
			b.WriteString(rewriteSGR(params, fgP, bgP))
			b.WriteByte('m')
		} else {
			b.WriteString(s[:j+1])
		}
		s = s[j+1:]
	}
	return b.String()
}

// plainSGR: nur Ziffern, ; und : (keine privaten Präfixe wie „?“ oder „>“).
func plainSGR(p string) bool {
	for i := 0; i < len(p); i++ {
		if c := p[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// rewriteSGR: 0/leer → Reset plus Theme-Farben, 39 → Theme-Schrift,
// 49 → Theme-Grund. Farbangaben 38/48/58;5;n und ;2;r;g;b bleiben, auch
// wenn darin eine 0 steht.
func rewriteSGR(params, fgP, bgP string) string {
	parts := strings.Split(params, ";")
	out := make([]string, 0, len(parts)+2)
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if strings.Contains(p, ":") {
			out = append(out, p)
			continue
		}
		n := 0
		if p != "" {
			v, err := strconv.Atoi(p)
			if err != nil {
				out = append(out, p)
				continue
			}
			n = v
		}
		switch n {
		case 0:
			out = append(out, "0", fgP, bgP)
		case 39:
			out = append(out, fgP)
		case 49:
			out = append(out, bgP)
		case 38, 48, 58:
			k := 0
			if i+1 < len(parts) {
				switch parts[i+1] {
				case "5":
					k = 2
				case "2":
					k = 4
				}
			}
			end := min(len(parts), i+1+k)
			out = append(out, parts[i:end]...)
			i = end - 1
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, ";")
}

// paintCursor: Cursorzelle selbst als Block in Akzentfarbe zeichnen. Auf
// hellen Themes nötig, weil viele Terminals die Cursorfarbe (OSC 12) nicht
// übernehmen und ihren eigenen, meist weißen Cursor zeigen – auf hellem Grund
// unsichtbar. View blendet den echten Cursor dann aus, damit nicht zwei
// Cursor übereinanderliegen.
func paintCursor(body string, x, y int, t Theme) string {
	lines := strings.Split(body, "\n")
	if y < 0 || y >= len(lines) || x < 0 {
		return body
	}
	ch := ansi.Strip(ansi.Cut(lines[y], x, x+1))
	if ch == "" || ansi.StringWidth(ch) != 1 {
		ch = " "
	}
	block := lipgloss.NewStyle().Background(t.Accent).Foreground(t.Bg).Render(ch)
	lines[y] = stampLine(lines[y], block, x, 0)
	return strings.Join(lines, "\n")
}
