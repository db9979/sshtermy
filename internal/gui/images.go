package gui

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/png" // kitty f=100
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/db9979/sshtermy/internal/termemu"
)

// Bilder pixelgenau: sshTermy schickt Bilder vom Server im eigenen Fenster
// genau wie an kitty/Ghostty (termemu/kittypass.go): PNG per
// kitty-Grafikprotokoll mit virtueller Platzierung (a=T, U=1) und in den
// Zellen Platzhalter U+10EEEE, Zeile/Spalte als Akzent, Bildnummer als
// Vordergrundfarbe. imageLayer fischt die Bilddaten aus der Ausgabe, das
// Fenster zeichnet an jedem Platzhalter das passende Stück des Bildes.

const (
	placeholder = 0x10EEEE
	maxImages   = 64 // ältere Bilder fallen weg
)

type kittyImage struct {
	img        image.Image
	cols, rows int
}

type imageLayer struct {
	mu     sync.Mutex
	images map[uint32]*kittyImage
	order  []uint32
	gen    uint64 // zählt hoch, wenn ein Bild dazukommt

	// laufende Übertragung (mehrere Stücke, m=1)
	cur    *kittyImage
	curID  uint32
	data   strings.Builder
	carry  []byte // angefangene Sequenz vom letzten Write
	inSeq  bool
	seqBuf bytes.Buffer
}

func newImageLayer() *imageLayer { return &imageLayer{images: map[uint32]*kittyImage{}} }

// filter nimmt kitty-Grafiksequenzen (ESC _ G … ESC \) aus p heraus und gibt
// den Rest zurück. Sequenzen dürfen über mehrere Writes verteilt sein.
func (l *imageLayer) filter(p []byte, _ *termemu.Term) []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.carry) > 0 {
		p = append(l.carry, p...)
		l.carry = nil
	}
	var out []byte
	for len(p) > 0 {
		if l.inSeq {
			end := bytes.Index(p, []byte("\x1b\\"))
			if end < 0 {
				// ESC am Ende könnte der Anfang von ST sein
				if p[len(p)-1] == 0x1b {
					l.seqBuf.Write(p[:len(p)-1])
					l.carry = []byte{0x1b}
				} else {
					l.seqBuf.Write(p)
				}
				return out
			}
			l.seqBuf.Write(p[:end])
			l.command(l.seqBuf.String())
			l.seqBuf.Reset()
			l.inSeq = false
			p = p[end+2:]
			continue
		}
		start := bytes.Index(p, []byte("\x1b_G"))
		if start < 0 {
			// angefangenes „ESC _“ am Ende aufheben
			keep := 0
			switch {
			case bytes.HasSuffix(p, []byte("\x1b_")):
				keep = 2
			case p[len(p)-1] == 0x1b:
				keep = 1
			}
			out = append(out, p[:len(p)-keep]...)
			if keep > 0 {
				l.carry = append([]byte(nil), p[len(p)-keep:]...)
			}
			return out
		}
		out = append(out, p[:start]...)
		p = p[start+3:]
		l.inSeq = true
	}
	return out
}

// command: eine Sequenz „Schlüssel=Wert,…;Nutzdaten“.
func (l *imageLayer) command(seq string) {
	ctrl, payload, _ := strings.Cut(seq, ";")
	kv := map[string]string{}
	for _, f := range strings.Split(ctrl, ",") {
		if k, v, ok := strings.Cut(f, "="); ok {
			kv[k] = v
		}
	}
	num := func(k string) int { n, _ := strconv.Atoi(kv[k]); return n }
	switch kv["a"] {
	case "d": // löschen
		if id := uint32(num("i")); id != 0 {
			delete(l.images, id)
		} else if kv["d"] == "a" || kv["d"] == "A" || kv["d"] == "" {
			l.images = map[uint32]*kittyImage{}
			l.order = nil
		}
		l.gen++
		return
	case "T", "t", "":
	default:
		return
	}
	if l.cur == nil {
		if kv["f"] != "" && kv["f"] != "100" {
			return // nur PNG, so wie sshTermy es schickt
		}
		l.cur = &kittyImage{cols: num("c"), rows: num("r")}
		l.curID = uint32(num("i"))
		l.data.Reset()
	}
	l.data.WriteString(payload)
	if kv["m"] == "1" {
		return
	}
	cur, id := l.cur, l.curID
	l.cur = nil
	raw, err := base64.StdEncoding.DecodeString(l.data.String())
	l.data.Reset()
	if err != nil || id == 0 || cur.cols <= 0 || cur.rows <= 0 {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return
	}
	cur.img = img
	if _, ok := l.images[id]; !ok {
		l.order = append(l.order, id)
	}
	l.images[id] = cur
	for len(l.order) > maxImages {
		delete(l.images, l.order[0])
		l.order = l.order[1:]
	}
	l.gen++
}

func (l *imageLayer) get(id uint32) *kittyImage {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.images[id]
}

// placeholderCell: Bildnummer, Zeile und Spalte aus einer Platzhalterzelle.
func placeholderCell(content string, fgR, fgG, fgB uint8) (id uint32, row, col int, ok bool) {
	r, n := utf8.DecodeRuneInString(content)
	if r != placeholder {
		return 0, 0, 0, false
	}
	rest := content[n:]
	dr, n1 := utf8.DecodeRuneInString(rest)
	dc, _ := utf8.DecodeRuneInString(rest[n1:])
	row, col = diacriticIndex(dr), diacriticIndex(dc)
	if row < 0 || col < 0 {
		return 0, 0, 0, false
	}
	return uint32(fgR)<<16 | uint32(fgG)<<8 | uint32(fgB), row, col, true
}

var diacriticPos = sync.OnceValue(func() map[rune]int {
	m := map[rune]int{}
	for i, d := range termemu.KittyDiacritics() {
		m[d] = i
	}
	return m
})

func diacriticIndex(r rune) int {
	if i, ok := diacriticPos()[r]; ok {
		return i
	}
	return -1
}
