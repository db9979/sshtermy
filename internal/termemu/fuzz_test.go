package termemu

import (
	"io"
	"testing"
)

// FuzzImageFilter: Bildsequenzen (Sixel, kitty, iTerm2) kommen ungeprüft vom
// Server. Kein Absturz, kein Hängen, auch wenn sie in zwei Teilen ankommen.
func FuzzImageFilter(f *testing.F) {
	f.Add([]byte("\x1bPq#0;2;100;0;0#0~~~~-~~~~\x1b\\"), 5)
	f.Add([]byte("\x1b_Ga=T,f=100;iVBORw0KGgo=\x1b\\"), 3)
	f.Add([]byte("\x1b_Ga=t,i=7,m=1;AAAA\x1b\\\x1b_Gm=0;AAAA\x1b\\\x1b_Ga=p,i=7\x1b\\"), 20)
	f.Add([]byte("\x1b]1337;File=inline=1;width=5:R0lGODlhAQABAAAAACw=\x07"), 9)
	f.Add([]byte("\x1bP0;1;0q\"1;1;4000;4000#1;1;360;50;100!4000~\x1b\\"), 1)
	f.Fuzz(func(t *testing.T, b []byte, cut int) {
		fl := &imageFilter{
			size:  func() (int, int) { return 80, 24 },
			col:   func() int { return 0 },
			reply: func([]byte) {},
		}
		if cut < 0 {
			cut = -cut
		}
		cut %= len(b) + 1
		fl.filter(b[:cut])
		fl.filter(b[cut:])
		decodeSixel(b)
	})
}

// FuzzTerm: beliebige Bytes vom Server durch den ganzen Emulator, mit
// Größenänderung dazwischen und Lesen des Bildschirms danach.
func FuzzTerm(f *testing.F) {
	f.Add([]byte("\x1b[2J\x1b[1;1Hhallo\x1b[38;2;1;2;3mfarbe\x1b[0m\r\n\x1b[?1049h\x1b[5n\x1b]52;c;aGFsbG8=\x07"), uint8(40), uint8(10))
	f.Add([]byte("\x1b[999;999H\x1b[3J\x1b]0;titel\x07\x1b[?2004h\x1b[c\x1b]133;D;1\x07"), uint8(1), uint8(1))
	f.Fuzz(func(t *testing.T, b []byte, w, h uint8) {
		tm := New(80, 24, io.Discard)
		defer tm.Close()
		half := len(b) / 2
		_, _ = tm.Write(b[:half])
		tm.Resize(int(w%200)+1, int(h%100)+1)
		_, _ = tm.Write(b[half:])
		tm.View("", func(s string) string { return s })
		tm.PlainLines(50)
		tm.TypedInput()
		tm.TakeClipboard()
	})
}
