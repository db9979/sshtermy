package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestSplitAnswer(t *testing.T) {
	ans := "Port 80 ist belegt.\n\n```sh\n$ sudo ss -tlnp | grep :80\n```\n\nDann:\n\n```\nsystemctl stop apache2\n```\n\n```nginx\nserver {\n  listen 80;\n}\n```"
	parts := splitAnswer(ans)
	if len(parts) != 5 {
		t.Fatalf("Teile: %d %#v", len(parts), parts)
	}
	cards := answerCards(ans)
	want := []card{{"sudo ss -tlnp | grep :80", "sh"}, {"systemctl stop apache2", ""}, {"server {\n  listen 80;\n}", "nginx"}}
	if len(cards) != len(want) {
		t.Fatalf("Karten: %q", cards)
	}
	for i := range want {
		if cards[i] != want[i] {
			t.Fatalf("Karte %d: %q", i, cards[i])
		}
	}
	if !cards[0].runnable() || !cards[1].runnable() || cards[2].runnable() {
		t.Fatal("einzeilige Shell = ausführbar, Dateien nur kopieren")
	}
	if (card{"{}", "json"}).runnable() || (card{"a\nb", "sh"}).runnable() {
		t.Fatal("json und mehrzeilig nicht ausführbar")
	}
	// offener Block beim Streamen zählt schon
	if c := answerCards("Prüfe:\n```sh\ndf -h"); len(c) != 1 || c[0].code != "df -h" || c[0].lang != "sh" {
		t.Fatalf("offener Block: %q", c)
	}
}

func TestHighlightKeepsText(t *testing.T) {
	st := NewStyles(Nacht)
	for _, l := range []string{
		`sudo ss -tlnp | grep ":80" && echo $HOME # Kommentar`,
		`FOO=1 docker run -d --name x -p 80:80 nginx:stable > /tmp/log 2>&1`,
		`echo "unclosed`, `$(whoami)`, ``,
	} {
		if got := ansi.Strip(highlightShell(l, st)); got != l {
			t.Errorf("Text verändert:\n%q\n%q", l, got)
		}
	}
}

func TestToasts(t *testing.T) {
	a := &App{s: NewStyles(Nacht), w: 100, h: 30}
	a.flash("Kopiert: ls")
	a.flashErr("kaputt")
	a.flash("Kopiert: ls") // gleiche Meldung: nicht doppelt
	if len(a.toasts) != 2 || a.toasts[0].kind != toastOK || a.toasts[1].kind != toastErr {
		t.Fatalf("%+v", a.toasts)
	}
	for i := 0; i < 5; i++ {
		a.flash(strings.Repeat("x", i+1))
	}
	if len(a.toasts) != maxToasts {
		t.Fatalf("höchstens %d: %d", maxToasts, len(a.toasts))
	}
	out := ansi.Strip(a.withToasts(strings.Repeat(strings.Repeat(" ", 100)+"\n", 29) + strings.Repeat(" ", 100)))
	if !strings.Contains(out, "xxxxx") {
		t.Fatalf("Toast fehlt im Bild:\n%s", out)
	}
	if a.liveToasts(time.Now().Add(9 * time.Second)) {
		t.Fatal("abgelaufene Toasts müssen verschwinden")
	}
	if len(a.toasts) != 0 {
		t.Fatal(len(a.toasts))
	}
}
