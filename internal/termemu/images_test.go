package termemu

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func redPNG(t *testing.T, w, h int) string {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Set(i%w, i/w, color.NRGBA{255, 0, 0, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

// feed schreibt in kleinen Stücken (Sequenzen über Paketgrenzen hinweg).
func feed(t *Term, s string, step int) {
	for i := 0; i < len(s); i += step {
		_, _ = t.Write([]byte(s[i:min(len(s), i+step)]))
	}
}

func TestImageITerm2(t *testing.T) {
	tm := New(40, 10, &lockedBuf{})
	feed(tm, "vorher\r\n\x1b]1337;File=name=eA==;inline=1;width=4:"+redPNG(t, 8, 8)+"\x07nachher", 7)
	txt := tm.PlainLines(10)
	if !strings.Contains(txt, "vorher") || !strings.Contains(txt, "nachher") || !strings.Contains(txt, "▀▀▀▀") {
		t.Fatalf("%q", txt)
	}
	if strings.Contains(txt, "1337") || strings.Contains(txt, "iVBOR") {
		t.Fatalf("Sequenz sichtbar: %q", txt)
	}
}

func TestImageSixel(t *testing.T) {
	tm := New(40, 10, &lockedBuf{})
	// 4 Pixel breit, 12 hoch, rot
	feed(tm, "a\x1bPq\"1;1;4;12#1;2;100;0;0#1~~~~-~~~~\x1b\\b", 3)
	txt := tm.PlainLines(10)
	if !strings.Contains(txt, "▀") || !strings.Contains(txt, "a") || !strings.Contains(txt, "b") || strings.Contains(txt, "~~") {
		t.Fatalf("%q", txt)
	}
	img := decodeSixel([]byte("q\"1;1;4;12#1;2;100;0;0#1~~~~-~~~~"))
	if img == nil || img.Bounds().Dx() != 4 || img.Bounds().Dy() != 12 {
		t.Fatalf("%v", img)
	}
	if c := color.NRGBAModel.Convert(img.At(1, 7)).(color.NRGBA); c.R != 255 || c.G != 0 {
		t.Errorf("%v", c)
	}
	if c := color.NRGBAModel.Convert(hls(120, 50, 100)).(color.NRGBA); c.R < 200 || c.G > 50 { // Sixel-HLS: 120° = Rot
		t.Errorf("HLS rot: %v", c)
	}
}

func TestImageKitty(t *testing.T) {
	host := &lockedBuf{}
	tm := New(40, 10, host)
	raw := base64.StdEncoding.EncodeToString([]byte{0, 255, 0, 0, 255, 0, 0, 255, 0, 0, 255, 0})
	// Abfrage: sshTermy antwortet OK
	feed(tm, "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\", 5)
	if !strings.Contains(host.String(), "\x1b_Gi=31;OK\x1b\\") {
		t.Fatalf("keine Antwort: %q", host.String())
	}
	// in zwei Teilen übertragen und anzeigen
	feed(tm, "x\x1b_Ga=T,f=24,s=2,v=2,q=2,m=1;"+raw[:8]+"\x1b\\\x1b_Gm=0;"+raw[8:]+"\x1b\\y", 4)
	txt := tm.PlainLines(10)
	if !strings.Contains(txt, "▀") || !strings.Contains(txt, "x") || !strings.Contains(txt, "y") {
		t.Fatalf("%q", txt)
	}
	// gemerkt und später platziert
	feed(tm, "\x1b_Ga=t,i=7,f=100,q=2;"+redPNG(t, 4, 4)+"\x1b\\\x1b_Ga=p,i=7,c=3\x1b\\", 50)
	if n := strings.Count(tm.PlainLines(10), "▀"); n < 3 {
		t.Fatalf("a=p nicht gezeichnet: %q", tm.PlainLines(10))
	}
}

func TestImageOtherSequencesUntouched(t *testing.T) {
	tm := New(40, 5, &lockedBuf{})
	in := []byte("\x1b[31mrot\x1b[0m \x1b]0;titel\x07ok")
	var f imageFilter
	if got := f.filter(in); !bytes.Equal(got, in) {
		t.Fatalf("%q", got)
	}
	feed(tm, "\x1b[31mrot\x1b[0m \x1bP$qm\x1b\\ok \x1b_Xfoo\x1b\\da", 2)
	if txt := tm.PlainLines(5); !strings.Contains(txt, "rot") || !strings.Contains(txt, "da") {
		t.Fatalf("%q", txt)
	}
}
