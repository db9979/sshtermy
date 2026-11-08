package termemu

// Bilder pixelgenau durchreichen. Läuft sshTermy in einem Terminal, das das
// kitty-Grafikprotokoll mit Unicode-Platzhaltern kann (kitty, Ghostty),
// geht das Bild als PNG an dieses Terminal (a=T, U=1: virtuelle
// Platzierung), und in die Zellen schreibt der Emulator Platzhalter
// U+10EEEE. Zeile und Spalte stehen als Akzente daran, die Bildnummer als
// Vordergrundfarbe. Die Platzhalter sind normaler Text: sie wandern mit
// dem Verlauf, überstehen Neuzeichnen und geteilte Ansicht, und das
// Terminal zeigt an ihrer Stelle die echten Pixel.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

const (
	kittyChunkLen = 4096
	kittyMaxPix   = 2048 // längere Kante wird vor dem Senden verkleinert
	kittyIDBase   = 0x5e0000
)

// SetPixelImages: seq bekommt die Sequenzen fürs äußere Terminal (nil = aus,
// dann Halbblöcke). Aufruf vor dem ersten Write.
func (t *Term) SetPixelImages(seq func([]byte)) {
	t.img.mu.Lock()
	t.img.pixels = seq
	t.img.mu.Unlock()
}

// passThrough: Bild hinausschicken und Platzhalter zurückgeben (nil =
// geht nicht, dann Halbblöcke).
func (f *imageFilter) passThrough(img image.Image, cols, rows, wantCols int) []byte {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return nil
	}
	w, h := imageCells(iw, ih, cols, rows, wantCols)
	n := len(kittyDiacritics)
	if w > n || h > n {
		return nil
	}
	if iw > kittyMaxPix || ih > kittyMaxPix {
		k := float64(kittyMaxPix) / float64(max(iw, ih))
		img = nrgbaOf(sample(img, max(1, int(float64(iw)*k)), max(1, int(float64(ih)*k))), max(1, int(float64(iw)*k)), max(1, int(float64(ih)*k)))
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return nil
	}
	f.nextID++
	id := kittyIDBase + f.nextID%0xffff
	f.pixels(kittyTransmit(id, pngBuf.Bytes(), w, h))
	return kittyPlaceholders(id, w, h)
}

// kittyTransmit: PNG in Stücken, mit virtueller Platzierung w×h Zellen.
func kittyTransmit(id uint32, pngData []byte, w, h int) []byte {
	enc := base64.StdEncoding.EncodeToString(pngData)
	var out bytes.Buffer
	first := true
	for len(enc) > 0 {
		part := enc[:min(kittyChunkLen, len(enc))]
		enc = enc[len(part):]
		more := 0
		if len(enc) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&out, "\x1b_Ga=T,U=1,f=100,q=2,i=%d,c=%d,r=%d,m=%d;%s\x1b\\", id, w, h, more, part)
			first = false
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, part)
		}
	}
	return out.Bytes()
}

// kittyPlaceholders: w×h Zellen U+10EEEE mit Zeile/Spalte als Akzent und
// der Bildnummer als 24-Bit-Vordergrundfarbe.
func kittyPlaceholders(id uint32, w, h int) []byte {
	var out bytes.Buffer
	for y := 0; y < h; y++ {
		fmt.Fprintf(&out, "\x1b[0;38;2;%d;%d;%dm", (id>>16)&0xff, (id>>8)&0xff, id&0xff)
		for x := 0; x < w; x++ {
			out.WriteRune(0x10EEEE)
			out.WriteRune(kittyDiacritics[y])
			out.WriteRune(kittyDiacritics[x])
		}
		out.WriteString("\x1b[0m\r\n")
	}
	return out.Bytes()
}

// nrgbaOf: Pixel aus sample() wieder als Bild.
func nrgbaOf(px []color.NRGBA, w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, px[y*w+x])
		}
	}
	return img
}

// PixelImagesSupported: kann das Terminal, in dem sshTermy läuft,
// kitty-Grafik mit Platzhaltern? (kitty, Ghostty; nicht in tmux/screen.)
func PixelImagesSupported(getenv func(string) string) bool {
	if getenv("SSHTERMY_NO_PIXELS") != "" || getenv("TMUX") != "" || getenv("STY") != "" {
		return false
	}
	switch {
	case getenv("KITTY_WINDOW_ID") != "", getenv("TERM") == "xterm-kitty":
		return true
	case getenv("TERM_PROGRAM") == "ghostty", getenv("TERM") == "xterm-ghostty", getenv("GHOSTTY_RESOURCES_DIR") != "":
		return true
	}
	return false
}
