package ui

import (
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestDialogFitsAndIsRectangular(t *testing.T) {
	s := NewStyles(Nacht)
	long := strings.Repeat("sehr langer Text ohne Ende ", 10)
	for _, w := range []int{24, 40, 60, 80, 120, 200} {
		d := dialog{icon: "?", title: "Eine Frage mit sehr langem Titel, der nicht passt", sub: "host",
			body: "kurz\n" + long, keys: [][2]string{{"j", "ja"}, {"n", "nein"}, {"Esc", "abbrechen"}, {"", "Hinweis"}}}
		out := s.dialog(d, w)
		lines := strings.Split(out, "\n")
		want := lipgloss.Width(lines[0])
		if want > w {
			t.Errorf("w=%d: Dialog %d breit", w, want)
		}
		for i, l := range lines {
			if lipgloss.Width(l) != want {
				t.Errorf("w=%d: Zeile %d ist %d statt %d breit: %q", w, i, lipgloss.Width(l), want, ansi.Strip(l))
			}
		}
		if !strings.Contains(ansi.Strip(lines[0]), "?") || !strings.Contains(ansi.Strip(lines[0]), "×") {
			t.Errorf("w=%d: Titel oder × fehlt im Rahmen: %q", w, ansi.Strip(lines[0]))
		}
	}
}

func TestDialogKeysWrap(t *testing.T) {
	s := NewStyles(Nacht)
	keys := [][2]string{{"u", "aktualisieren"}, {"w", "was ist neu"}, {"b", "Kaffee spendieren"}, {"l", "English"}, {"Esc", "schließen"}}
	rows := s.keyRows("", keys, 30)
	if len(rows) < 2 {
		t.Fatalf("Tasten müssen umbrechen: %d Zeilen", len(rows))
	}
	for _, r := range rows {
		if lipgloss.Width(r) > 30 {
			t.Errorf("Tastenzeile zu breit: %q", ansi.Strip(r))
		}
	}
}

func TestFadeANSI(t *testing.T) {
	th := Nacht
	in := lipgloss.NewStyle().Foreground(th.Accent).Background(th.Surface).Render("Hallo") + " Welt"
	if fadeANSI(in, th, 1) != in {
		t.Fatal("k=1 muss unverändert bleiben")
	}
	bg := rgbOf(th.Bg)
	out := fadeANSI(in, th, 0)
	for _, m := range sgrRe.FindAllStringSubmatch(out, -1) {
		ps := strings.Split(m[1], ";")
		for i := 0; i+4 < len(ps); i++ {
			if (ps[i] == "38" || ps[i] == "48") && ps[i+1] == "2" {
				got := ps[i+2] + ";" + ps[i+3] + ";" + ps[i+4]
				if want := sgrRGB(0, bg)[4:]; got != want {
					t.Errorf("k=0: Farbe %s statt Hintergrund %s", got, want)
				}
			}
		}
	}
	if ansi.Strip(out) != ansi.Strip(in) {
		t.Fatal("Text darf sich beim Blenden nicht ändern")
	}
	if !strings.Contains(fadeANSI("\x1b[71001z", th, 0.5), "\x1b[71001z") {
		t.Fatal("Zonen-Marker müssen erhalten bleiben")
	}
}

func TestTabStripFitsNarrowHeader(t *testing.T) {
	a := &App{s: NewStyles(Nacht), keys: newKeymap(nil)}
	for i := 0; i < 7; i++ {
		a.sessions = append(a.sessions, &session{id: i + 1, conn: domain.Connection{Alias: "produktion-server-" + string(rune('a'+i))}, chipArmed: -1})
	}
	for _, w := range []int{60, 80, 100, 140, 200} {
		for _, cur := range []int{0, 3, 6} {
			a.w, a.cur = w, cur
			h := a.header(a.tabStrip(a.w - lipgloss.Width(a.headerLeft()) - 1))
			if lipgloss.Width(h) > w {
				t.Errorf("w=%d: Kopfzeile %d breit", w, lipgloss.Width(h))
			}
			plain := ansi.Strip(zoneRe.ReplaceAllString(h, ""))
			if strings.Contains(plain, "Dominik") {
				t.Errorf("w=%d: Credit gehört ins Über-Fenster, nicht in die Kopfzeile: %q", w, plain)
			}
			act := a.sessions[cur].conn.Alias
			if !strings.Contains(plain, act[:3]) && !strings.Contains(plain, " "+string(rune('1'+cur))+" ") {
				t.Errorf("w=%d cur=%d: aktiver Tab nicht sichtbar: %q", w, cur, plain)
			}
		}
	}
}

func TestNarrowPanelCollapses(t *testing.T) {
	a := splitApp(1)
	a.w = 80
	s := a.sessions[0]
	s.panel = panelAsk
	if !a.panelCollapsed(s) || a.panelWidth(s) != panelRailW {
		t.Fatal("bei 80 Spalten muss das Panel einklappen")
	}
	s.panelFocus = true
	if pw := a.panelShown(s); pw < 44 || pw > a.w-8 {
		t.Fatalf("aufgeklapptes Panel %d breit", pw)
	}
	a.w = 140
	if a.panelCollapsed(s) || a.panelWidth(s) != 48 {
		t.Fatal("im breiten Fenster bleibt das Panel daneben")
	}
}

// Kontrast nach WCAG 2: Text mindestens 4,5:1, in den Hochkontrast-Themes 7:1.
func TestThemeContrast(t *testing.T) {
	lum := func(c [3]int) float64 {
		f := func(v int) float64 {
			x := float64(v) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*f(c[0]) + 0.7152*f(c[1]) + 0.0722*f(c[2])
	}
	ratio := func(a, b [3]int) float64 {
		la, lb := lum(a)+0.05, lum(b)+0.05
		return math.Max(la, lb) / math.Min(la, lb)
	}
	for _, th := range Themes {
		need := 4.5
		if strings.HasPrefix(th.Name, "kontrast") {
			need = 7
		}
		bg := rgbOf(th.Bg)
		for name, c := range map[string][3]int{"Text": rgbOf(th.Text), "Muted": rgbOf(th.Muted), "Accent": rgbOf(th.Accent), "Danger": rgbOf(th.Danger), "AI": rgbOf(th.AI), "OK": rgbOf(th.OK), "Warn": rgbOf(th.Warn)} {
			min := need
			if name == "Muted" && need < 7 {
				min = 3.5 // Nebensachen dürfen in den normalen Themes leiser sein
			}
			if r := ratio(c, bg); r < min {
				t.Errorf("%s: %s hat nur %.1f:1", th.Name, name, r)
			}
		}
	}
}
