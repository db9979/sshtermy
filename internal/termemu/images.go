package termemu

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // Formate für iTerm2/kitty
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Bilder im Terminal
//
// Programme auf dem Server können Bilder ausgeben: Sixel (img2sixel, chafa
// -f sixel, gnuplot), das kitty-Grafikprotokoll (kitty +kitten icat, viu,
// timg) und iTerm2 Inline Images (imgcat). sshTermy fängt diese Sequenzen
// ab, bevor sie den Emulator erreichen, und zeichnet das Bild mit
// Halbblöcken (▀) in echten Farben in den Text. So geht es in jedem
// Terminal mit 24-Bit-Farbe, wandert mit dem Verlauf und überlebt die
// geteilte Ansicht. Pixelgenau ist es nicht: zwei Pixel pro Zeichenzelle.

const (
	imgMaxSeq   = 32 << 20 // größere Sequenzen gehen unverändert durch
	imgMaxPix   = 4096     // Kantenlänge, mehr wird nicht dekodiert
	imgCellW    = 9        // grobe Pixelbreite einer Zelle (für „Originalgröße“)
	imgCellH    = 18
	kittyKeep   = 16 // so viele gemerkte kitty-Bilder (a=t, später a=p)
	imgMaxRowsF = 0.8
)

// imageFilter sitzt vor dem Emulator.
type imageFilter struct {
	mu      sync.Mutex
	size    func() (cols, rows int)
	col     func() int // Spalte des Cursors (0 = Zeilenanfang)
	reply   func([]byte)
	pending []byte
	kitty   map[int]image.Image
	order   []int
	chunk   *kittyChunk // mehrteilige kitty-Übertragung
}

type kittyChunk struct {
	keys map[string]string
	data []byte
}

// filter liefert die Bytes, die der Emulator bekommen soll.
func (f *imageFilter) filter(p []byte) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 && !mayHaveImage(p) {
		return p // häufigster Fall: nichts zu tun
	}
	data := p
	if len(f.pending) > 0 {
		data = append(f.pending, p...)
		f.pending = nil
	}
	var out []byte
	i := 0
	for {
		j := bytes.IndexByte(data[i:], 0x1b)
		if j < 0 {
			out = append(out, data[i:]...)
			return out
		}
		j += i
		out = append(out, data[i:j]...)
		kind, ok := f.kind(data[j:])
		if !ok { // noch zu wenig Bytes, um es zu sagen
			f.hold(data[j:], &out)
			return out
		}
		if kind == "" {
			out = append(out, 0x1b)
			i = j + 1
			continue
		}
		end, body := seqEnd(data[j:], kind)
		if end < 0 {
			f.hold(data[j:], &out)
			return out
		}
		out = append(out, f.render(kind, body)...)
		i = j + end
	}
}

// mayHaveImage: schneller Vorab-Blick, ob p eine Bildsequenz beginnt (oder
// am Ende eine anfangen könnte).
func mayHaveImage(p []byte) bool {
	if bytes.Contains(p, []byte("\x1bP")) || bytes.Contains(p, []byte("\x1b_G")) || bytes.Contains(p, []byte("\x1b]1337;")) {
		return true
	}
	if i := bytes.LastIndexByte(p, 0x1b); i >= 0 && len(p)-i < 12 {
		var f imageFilter
		_, ok := f.kind(p[i:])
		return !ok
	}
	return false
}

// hold merkt sich einen unvollständigen Anfang; zu groß = durchreichen.
func (f *imageFilter) hold(b []byte, out *[]byte) {
	if len(b) > imgMaxSeq {
		*out = append(*out, b...)
		return
	}
	f.pending = append([]byte(nil), b...)
}

// kind: "sixel", "kitty", "iterm" oder "" (keine Bildsequenz). ok=false,
// wenn noch Bytes fehlen.
func (f *imageFilter) kind(b []byte) (string, bool) {
	if len(b) < 2 {
		return "", false
	}
	switch b[1] {
	case 'P': // DCS: Parameter, dann q = Sixel
		for k := 2; k < len(b); k++ {
			c := b[k]
			switch {
			case c >= '0' && c <= '9' || c == ';':
				continue
			case c == 'q':
				return "sixel", true
			default:
				return "", true
			}
		}
		return "", len(b) > 24
	case '_':
		if len(b) < 3 {
			return "", false
		}
		if b[2] == 'G' {
			return "kitty", true
		}
		return "", true
	case ']':
		const pre = "]1337;File="
		n := min(len(b)-1, len(pre))
		if string(b[1:1+n]) != pre[:n] {
			return "", true
		}
		if n < len(pre) {
			return "", false
		}
		return "iterm", true
	}
	return "", true
}

// seqEnd: Ende der Sequenz (Index nach dem Terminator) und Inhalt ohne
// Einleitung und Terminator.
func seqEnd(b []byte, kind string) (int, []byte) {
	start := 2
	if kind == "iterm" {
		start = len("\x1b]1337;File=")
	}
	for k := start; k < len(b); k++ {
		switch {
		case b[k] == 0x1b && k+1 < len(b) && b[k+1] == '\\':
			return k + 2, b[start:k]
		case b[k] == 0x07 && kind == "iterm":
			return k + 1, b[start:k]
		case b[k] == 0x9c:
			return k + 1, b[start:k]
		}
	}
	return -1, nil
}

func (f *imageFilter) render(kind string, body []byte) []byte {
	var img image.Image
	wantCols := 0
	switch kind {
	case "sixel":
		img = decodeSixel(body)
	case "kitty":
		img, wantCols = f.kittyImage(body)
	case "iterm":
		img, wantCols = f.itermImage(body)
	}
	if img == nil {
		return nil
	}
	cols, rows := 80, 24
	if f.size != nil {
		cols, rows = f.size()
	}
	out := halfBlocks(img, cols, rows, wantCols)
	if f.col != nil && f.col() > 0 {
		out = append([]byte("\r\n"), out...) // Bild beginnt in einer eigenen Zeile
	}
	return out
}

// ---------------------------------------------------------------- Halbblöcke

// halfBlocks zeichnet das Bild in Zellen: jede Zelle zeigt oben und unten
// ein Pixel (▀ mit Vorder- und Hintergrundfarbe). wantCols > 0 = Breite in
// Zellen, sonst etwa Originalgröße, höchstens so breit wie das Terminal.
func halfBlocks(img image.Image, cols, rows, wantCols int) []byte {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return nil
	}
	w := wantCols
	if w <= 0 {
		w = (iw + imgCellW - 1) / imgCellW
	}
	w = max(1, min(w, cols-1))
	h := (ih*w*imgCellW/iw + imgCellH - 1) / imgCellH // Zeilen bei gleichem Seitenverhältnis
	if lim := max(1, int(float64(rows)*imgMaxRowsF)); h > lim && wantCols <= 0 {
		w = max(1, w*lim/h)
		h = lim
	}
	h = max(1, h)
	px := sample(img, w, h*2)
	var out bytes.Buffer
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			top, bot := px[(2*y)*w+x], px[(2*y+1)*w+x]
			switch {
			case top.A < 128 && bot.A < 128:
				out.WriteString("\x1b[0m ")
			case bot.A < 128:
				fmt.Fprintf(&out, "\x1b[0;38;2;%d;%d;%dm▀", top.R, top.G, top.B)
			case top.A < 128:
				fmt.Fprintf(&out, "\x1b[0;38;2;%d;%d;%dm▄", bot.R, bot.G, bot.B)
			default:
				fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", top.R, top.G, top.B, bot.R, bot.G, bot.B)
			}
		}
		out.WriteString("\x1b[0m\r\n")
	}
	return out.Bytes()
}

// sample verkleinert per Mittelwert über die Fläche (vergrößert per nächstem Pixel).
func sample(img image.Image, w, h int) []color.NRGBA {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	out := make([]color.NRGBA, w*h)
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*ih/h, b.Min.Y+(y+1)*ih/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*iw/w, b.Min.X+(x+1)*iw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			stepY, stepX := max(1, (y1-y0)/8), max(1, (x1-x0)/8) // bei großen Bildern nicht jedes Pixel
			for yy := y0; yy < y1; yy += stepY {
				for xx := x0; xx < x1; xx += stepX {
					c := color.NRGBAModel.Convert(img.At(xx, yy)).(color.NRGBA)
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					bl += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					n++
				}
			}
			if a == 0 {
				continue
			}
			out[y*w+x] = color.NRGBA{uint8(r / a), uint8(g / a), uint8(bl / a), uint8(a / n)}
		}
	}
	return out
}

// ---------------------------------------------------------------- Sixel

var sixelDefault = [16]color.NRGBA{
	{0, 0, 0, 255}, {51, 51, 204, 255}, {204, 33, 33, 255}, {51, 204, 51, 255},
	{204, 51, 204, 255}, {51, 204, 204, 255}, {204, 204, 51, 255}, {135, 135, 135, 255},
	{66, 66, 66, 255}, {84, 84, 153, 255}, {153, 66, 66, 255}, {84, 153, 84, 255},
	{153, 84, 153, 255}, {84, 153, 153, 255}, {153, 153, 84, 255}, {204, 204, 204, 255},
}

// decodeSixel: Inhalt nach ESC P (Parameter, q, Daten) ohne Terminator.
// sixelMaxNum: größte Zahl in einer Sixel-Sequenz (weit über imgMaxPix).
const sixelMaxNum = 1 << 20

func decodeSixel(b []byte) image.Image {
	q := bytes.IndexByte(b, 'q')
	if q < 0 {
		return nil
	}
	params := strings.Split(string(b[:q]), ";")
	transparent := len(params) > 1 && params[1] == "1"
	data := b[q+1:]
	pal := map[int]color.NRGBA{}
	for i, c := range sixelDefault {
		pal[i] = c
	}
	cur := pal[0]
	w, h := 0, 0
	var img *image.NRGBA
	grow := func(nw, nh int) bool {
		if nw > imgMaxPix || nh > imgMaxPix {
			return false
		}
		if img != nil && nw <= img.Bounds().Dx() && nh <= img.Bounds().Dy() {
			return true
		}
		cw, ch := max(nw, 64), max(nh, 6)
		if img != nil {
			cw, ch = max(cw, img.Bounds().Dx()*2), max(ch, img.Bounds().Dy()*2)
			cw, ch = min(cw, imgMaxPix), min(ch, imgMaxPix)
		}
		n := image.NewNRGBA(image.Rect(0, 0, cw, ch))
		if !transparent {
			for i := 3; i < len(n.Pix); i += 4 {
				n.Pix[i] = 255 // Hintergrund schwarz statt durchsichtig
			}
		}
		if img != nil {
			for y := 0; y < img.Bounds().Dy(); y++ {
				copy(n.Pix[y*n.Stride:], img.Pix[y*img.Stride:y*img.Stride+img.Bounds().Dx()*4])
			}
		}
		img = n
		return true
	}
	x, y := 0, 0
	num := func(i int) (int, int) {
		v, k := 0, i
		for k < len(data) && data[k] >= '0' && data[k] <= '9' {
			// gedeckelt: sonst läuft die Zahl über (negativ) und „!“
			// wiederholt eine Spalte Milliarden Mal (Fuzz-Fund)
			v = min(v*10+int(data[k]-'0'), sixelMaxNum)
			k++
		}
		return v, k
	}
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == '"': // Rasterangaben: Pan;Pad;Ph;Pv
			var vals []int
			k := i + 1
			for {
				v, n := num(k)
				vals = append(vals, v)
				k = n
				if k < len(data) && data[k] == ';' {
					k++
					continue
				}
				break
			}
			if len(vals) >= 4 && vals[2] > 0 && vals[3] > 0 {
				if !grow(vals[2], vals[3]) {
					return nil
				}
				w, h = max(w, vals[2]), max(h, vals[3])
			}
			i = k
		case c == '#':
			var vals []int
			k := i + 1
			for {
				v, n := num(k)
				vals = append(vals, v)
				k = n
				if k < len(data) && data[k] == ';' {
					k++
					continue
				}
				break
			}
			if len(vals) >= 5 {
				var col color.NRGBA
				if vals[1] == 2 { // RGB in Prozent
					col = color.NRGBA{pct(vals[2]), pct(vals[3]), pct(vals[4]), 255}
				} else { // HLS
					col = hls(vals[2], vals[3], vals[4])
				}
				pal[vals[0]] = col
			}
			if len(vals) >= 1 {
				if c, ok := pal[vals[0]]; ok {
					cur = c
				} else {
					cur = color.NRGBA{0, 0, 0, 255}
				}
			}
			i = k
		case c == '!':
			n, k := num(i + 1)
			if k < len(data) && data[k] >= '?' && data[k] <= '~' {
				if !grow(x+n, y+6) {
					return nil
				}
				for r := 0; r < n; r++ {
					sixelCol(img, x+r, y, data[k]-'?', cur)
				}
				x += n
				w = max(w, x)
				k++
			}
			i = k
		case c == '$':
			x = 0
			i++
		case c == '-':
			x = 0
			y += 6
			i++
		case c >= '?' && c <= '~':
			if !grow(x+1, y+6) {
				return nil
			}
			sixelCol(img, x, y, c-'?', cur)
			x++
			w = max(w, x)
			i++
		default:
			i++
		}
		h = max(h, y+6)
	}
	if img == nil || w == 0 {
		return nil
	}
	return img.SubImage(image.Rect(0, 0, w, min(h, img.Bounds().Dy())))
}

func sixelCol(img *image.NRGBA, x, y int, bits byte, c color.NRGBA) {
	for k := 0; k < 6; k++ {
		if bits&(1<<k) != 0 {
			img.SetNRGBA(x, y+k, c)
		}
	}
}

func pct(v int) uint8 { return uint8(min(100, max(0, v)) * 255 / 100) }

// hls: Sixel-HLS (Farbton 0–360, Helligkeit und Sättigung in Prozent).
func hls(hh, l, s int) color.NRGBA {
	H, L, S := float64((hh+240)%360)/360, float64(l)/100, float64(s)/100 // Sixel: 0° = Blau
	if S == 0 {
		v := uint8(L * 255)
		return color.NRGBA{v, v, v, 255}
	}
	var q float64
	if L < 0.5 {
		q = L * (1 + S)
	} else {
		q = L + S - L*S
	}
	p := 2*L - q
	f := func(t float64) uint8 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(v * 255)
	}
	return color.NRGBA{f(H + 1.0/3), f(H), f(H - 1.0/3), 255}
}

// ---------------------------------------------------------------- kitty

func parseKeys(s string) map[string]string {
	m := map[string]string{}
	for _, kv := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

// kittyImage: ESC _ G keys ; base64 ESC \ (body ohne „G“-Präfix: beginnt mit G).
func (f *imageFilter) kittyImage(body []byte) (image.Image, int) {
	ks, payload, _ := strings.Cut(string(body[1:]), ";")
	keys := parseKeys(ks)
	if f.chunk != nil { // Fortsetzung: nur m= gilt
		f.chunk.data = append(f.chunk.data, payload...)
		if keys["m"] == "1" {
			if len(f.chunk.data) > imgMaxSeq {
				f.chunk = nil
			}
			return nil, 0
		}
		keys, payload = f.chunk.keys, string(f.chunk.data)
		f.chunk = nil
	} else if keys["m"] == "1" {
		f.chunk = &kittyChunk{keys: keys, data: []byte(payload)}
		return nil, 0
	}
	action := keys["a"]
	if action == "" {
		action = "t"
	}
	id, _ := strconv.Atoi(keys["i"])
	ok := func(msg string) {
		if f.reply != nil && id != 0 && keys["q"] == "" {
			f.reply([]byte(fmt.Sprintf("\x1b_Gi=%d;%s\x1b\\", id, msg)))
		}
	}
	switch action {
	case "q":
		if f.reply != nil && keys["q"] != "2" {
			f.reply([]byte(fmt.Sprintf("\x1b_Gi=%d;OK\x1b\\", id)))
		}
		return nil, 0
	case "p":
		img := f.kitty[id]
		if img == nil {
			ok("ENOENT:image not found")
			return nil, 0
		}
		c, _ := strconv.Atoi(keys["c"])
		return img, c
	case "d":
		return nil, 0
	case "t", "T":
	default:
		return nil, 0
	}
	if t := keys["t"]; t != "" && t != "d" {
		ok("EINVAL:only direct transmission")
		return nil, 0
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		ok("EINVAL:bad base64")
		return nil, 0
	}
	if keys["o"] == "z" {
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, 0
		}
		raw, err = io.ReadAll(io.LimitReader(zr, imgMaxSeq))
		if err != nil {
			return nil, 0
		}
	}
	var img image.Image
	switch keys["f"] {
	case "100":
		img, _, err = image.Decode(bytes.NewReader(raw))
		if err != nil {
			ok("EINVAL:bad png")
			return nil, 0
		}
	case "", "32", "24":
		w, _ := strconv.Atoi(keys["s"])
		h, _ := strconv.Atoi(keys["v"])
		bpp := 4
		if keys["f"] == "24" {
			bpp = 3
		}
		if w <= 0 || h <= 0 || w > imgMaxPix || h > imgMaxPix || len(raw) < w*h*bpp {
			ok("EINVAL:bad size")
			return nil, 0
		}
		n := image.NewNRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < w*h; i++ {
			copy(n.Pix[i*4:i*4+3], raw[i*bpp:i*bpp+3])
			n.Pix[i*4+3] = 255
			if bpp == 4 {
				n.Pix[i*4+3] = raw[i*4+3]
			}
		}
		img = n
	default:
		return nil, 0
	}
	if id != 0 {
		if f.kitty == nil {
			f.kitty = map[int]image.Image{}
		}
		if _, had := f.kitty[id]; !had {
			f.order = append(f.order, id)
			if len(f.order) > kittyKeep {
				delete(f.kitty, f.order[0])
				f.order = f.order[1:]
			}
		}
		f.kitty[id] = img
	}
	ok("OK")
	if action == "t" {
		return nil, 0
	}
	c, _ := strconv.Atoi(keys["c"])
	return img, c
}

// ---------------------------------------------------------------- iTerm2

// itermImage: „args:base64“ (OSC 1337;File=… ohne Präfix und Terminator).
func (f *imageFilter) itermImage(body []byte) (image.Image, int) {
	args, payload, ok := strings.Cut(string(body), ":")
	if !ok {
		return nil, 0
	}
	keys := map[string]string{}
	for _, kv := range strings.Split(args, ";") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			keys[k] = v
		}
	}
	if keys["inline"] != "1" {
		return nil, 0 // Download statt Anzeige: nicht unsere Sache
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return nil, 0
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0
	}
	cols := 0
	if f.size != nil {
		cols, _ = f.size()
	}
	want := 0
	switch w := keys["width"]; {
	case w == "" || w == "auto":
	case strings.HasSuffix(w, "%"):
		p, _ := strconv.Atoi(strings.TrimSuffix(w, "%"))
		want = cols * p / 100
	case strings.HasSuffix(w, "px"):
		p, _ := strconv.Atoi(strings.TrimSuffix(w, "px"))
		want = (p + imgCellW - 1) / imgCellW
	default:
		want, _ = strconv.Atoi(w)
	}
	return img, want
}
