package ui

import (
	"image/color"
	"net"
	"strconv"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/db9979/sshtermy/internal/devfake"
	"github.com/db9979/sshtermy/internal/domain"
)

func cellsOf(s string, w, h int) uv.ScreenBuffer {
	buf := uv.NewScreenBuffer(w, h)
	uv.NewStyledString(s).Draw(buf, buf.Bounds())
	return buf
}

func sameCol(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	return hexOf(a) == hexOf(b)
}

// checkFilled: jede Zelle hat Theme-Farben statt Terminal-Standard, und was
// vorher eigens gefärbt war, bleibt genau so.
func checkFilled(t *testing.T, what, body string, th Theme, w, h int) {
	t.Helper()
	before := cellsOf(body, w, h)
	after := cellsOf(fillDefaultColors(body, th.Text, th.Bg), w, h)
	bad := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			b, a := before.CellAt(x, y), after.CellAt(x, y)
			if b == nil || a == nil || (a.Equal(&uv.EmptyCell) && b.Equal(&uv.EmptyCell)) {
				continue // hinter dem Zeilenende: nicht beschrieben
			}
			wantFg, wantBg := b.Style.Fg, b.Style.Bg
			if wantFg == nil {
				wantFg = th.Text
			}
			if wantBg == nil {
				wantBg = th.Bg
			}
			if a.Content != b.Content || !sameCol(a.Style.Fg, wantFg) || !sameCol(a.Style.Bg, wantBg) ||
				a.Style.Attrs != b.Style.Attrs || a.Style.Underline != b.Style.Underline {
				if bad < 3 {
					t.Errorf("%s/%s: Zelle %d,%d %q: fg %v bg %v, erwartet %v %v", th.Name, what, x, y, a.Content, a.Style.Fg, a.Style.Bg, wantFg, wantBg)
				}
				bad++
			}
		}
	}
}

// Helle Themes müssen auch in Terminals hell sein, die OSC 11 nicht
// umsetzen: keine Zelle darf die Standardfarbe des Terminals behalten.
func TestThemeFillsEveryCell(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal, a.intro = nil, nil
	for _, th := range Themes {
		a.setTheme(th.Name)
		screens := map[string]func(){
			"manager": func() { a.pal = nil; a.scr = scrManager },
			"palette": func() { a.scr = scrManager; a.openPalette() },
			"themes":  func() { a.pal = nil; a.openThemes() },
		}
		for name, open := range screens {
			open()
			v := a.View()
			buf := cellsOf(v.Content, a.w, a.h)
			for y := 0; y < a.h; y++ {
				for x := 0; x < a.w; x++ {
					if c := buf.CellAt(x, y); c != nil && (c.Style.Bg == nil || c.Style.Fg == nil) {
						t.Fatalf("%s/%s: Zelle %d,%d ohne Theme-Farbe", th.Name, name, x, y)
					}
				}
			}
		}
		a.pal = nil
		a.scr = scrManager
	}
}

// Cursorfarbe (Akzent) hebt sich in jedem Theme vom Grund ab.
func TestCursorContrast(t *testing.T) {
	for _, th := range Themes {
		if r := contrast(th.Accent, th.Bg); r < 3 {
			t.Errorf("%s: Cursor (Akzent) auf Grund nur %.1f:1", th.Name, r)
		}
		if r := contrast(th.Bg, th.Accent); r < 3 {
			t.Errorf("%s: Zeichen im Cursorblock nur %.1f:1", th.Name, r)
		}
	}
}

func TestFillDefaultColors(t *testing.T) {
	th := Hell
	cases := []string{
		"plain\nzwei",
		"\x1b[38;2;0;0;0mschwarz\x1b[m rest",
		"\x1b[48;5;0mnull\x1b[0;1mfett\x1b[39mx\x1b[49my",
		"\x1b[4:3mwellig\x1b[58;2;0;0;0mu\x1b[0m",
		"\x1b[?25l\x1b[7minvers\x1b[27m",
		"\x1b]8;;http://x\x07link\x1b]8;;\x07",
	}
	for i, c := range cases {
		checkFilled(t, "fall"+strconv.Itoa(i), c, th, 30, 3)
	}
	if got := fillDefaultColors("\x1b[", th.Text, th.Bg); !strings.HasSuffix(got, "\x1b[") {
		t.Errorf("abgeschnittene Folge: %q", got)
	}
}

// Ausgabe vom Server im Terminal-Bereich bekommt ebenfalls den Theme-Grund,
// eigene Farben des Servers bleiben.
func TestThemeFillsTerminal(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal, a.intro = nil, nil
	sw, err := devfake.StartSwitch("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sw.Close()
	go func() {
		for m := range a.events {
			if hk, ok := m.(hostKeyAskMsg); ok {
				hk.reply <- true
			}
		}
	}()
	host, port, _ := net.SplitHostPort(sw.Addr)
	p, _ := strconv.Atoi(port)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "sw", Host: host, Port: p, Username: "manager",
		Auth: domain.AuthPassword, Password: "x"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: 1, conn: c, chipArmed: -1}
	a.sessions, a.cur, a.scr = []*session{s}, 0, scrTerminal
	msg := a.connectCmd(s)()
	if e, ok := msg.(connectErrMsg); ok {
		t.Fatal(e.err)
	}
	a.Update(msg)
	waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })
	// Cursor: helle Themes zeichnen ihn selbst (Akzent-Block), dunkle
	// überlassen ihn dem Terminal in Akzentfarbe.
	for _, name := range []string{"hell", "kontrasthell", "nacht"} {
		a.setTheme(name)
		v := a.View()
		x, y, _ := s.term.Cursor()
		tr := a.termRect(s)
		c := cellsOf(v.Content, a.w, a.h).CellAt(tr.x+x, tr.y+y)
		if a.s.T.Dark {
			if v.Cursor == nil || !sameCol(v.Cursor.Color, a.s.T.Accent) {
				t.Errorf("%s: Terminal-Cursor fehlt oder falsche Farbe: %+v", name, v.Cursor)
			}
			continue
		}
		if v.Cursor != nil {
			t.Errorf("%s: echter Cursor sollte aus sein", name)
		}
		if c == nil || !sameCol(c.Style.Bg, a.s.T.Accent) || !sameCol(c.Style.Fg, a.s.T.Bg) {
			t.Errorf("%s: Cursorzelle nicht als Block gezeichnet: %+v", name, c)
		}
	}
	// Suche im Terminal: Feld in Theme-Farben (Cursor sichtbar)
	a.setTheme("hell")
	a.openSearch(s)
	if st := s.search.in.Styles(); !sameCol(st.Cursor.Color, Hell.Accent) || !sameCol(st.Focused.Text.GetForeground(), Hell.Text) {
		t.Errorf("Suchfeld nicht im Theme: Cursor %v Text %v", st.Cursor.Color, st.Focused.Text.GetForeground())
	}
	s.search.open = false
	for _, name := range []string{"hell", "papier", "nacht"} {
		a.setTheme(name)
		body, _ := a.viewTerminal()
		checkFilled(t, "terminal", body, a.s.T, a.w, a.h)
		buf := cellsOf(a.View().Content, a.w, a.h)
		dark := 0
		for y := 0; y < a.h; y++ {
			for x := 0; x < a.w; x++ {
				if c := buf.CellAt(x, y); c != nil && c.Style.Bg == nil {
					dark++
				}
			}
		}
		if dark > 0 {
			t.Errorf("%s: %d Zellen im Terminal ohne Theme-Grund", name, dark)
		}
	}
}

func BenchmarkFillDefaultColors(b *testing.B) {
	body := strings.Repeat("\x1b[38;2;10;20;30;48;2;1;2;3m Text \x1b[m normal \x1b[1mfett\x1b[0m"+strings.Repeat(" ", 80)+"\n", 40)
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		_ = fillDefaultColors(body, Hell.Text, Hell.Bg)
	}
}
