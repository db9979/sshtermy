package highlight

import (
	"testing"

	"github.com/db9979/sshtermy/internal/i18n"
)

func colorAt(spans []Span, line, word string) Color {
	for _, sp := range spans {
		if line[sp.Start:sp.End] == word {
			return sp.Color
		}
	}
	return None
}

func TestDefaults(t *testing.T) {
	s, err := New(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		line, word string
		want       Color
	}{
		{"Nov  8 %LINK-3-UPDOWN: Interface Gi1/0/3, changed state to down", "%LINK-3-UPDOWN", Red},
		{"Nov  8 %LINK-3-UPDOWN: Interface Gi1/0/3, changed state to down", "down", Red},
		{"%SYS-4-CONFIG_I: Configured from console", "%SYS-4-CONFIG_I", Yellow},
		{"E: Unable to locate package foo, error 3", "error", Red},
		{"Active: active (running) since Mon", "active (running)", Green},
		{"inet 192.168.10.20/24 brd 192.168.10.255", "192.168.10.20/24", Blue},
		{"link/ether 00:1a:2b:3c:4d:5e brd ff:ff:ff:ff:ff:ff", "00:1a:2b:3c:4d:5e", Purple},
		{"  1   0011.2233.4455  DYNAMIC  Gi1/0/1", "0011.2233.4455", Purple},
		{"  1   001122-334455  5", "001122-334455", Purple},
		{"WARNING: apt does not have a stable CLI", "WARNING", Yellow},
	}
	for _, c := range cases {
		if got := colorAt(s.Spans(c.line), c.line, c.word); got != c.want {
			t.Errorf("%q in %q: %v, want %v", c.word, c.line, got, c.want)
		}
	}
	if sp := s.Spans("shutdown -r now; download done"); len(sp) != 0 {
		t.Errorf("Wortgrenzen: %v", sp)
	}
	if sp := s.Spans("version 999.300.1.1"); colorAt(sp, "version 999.300.1.1", "999.300.1.1") != None {
		t.Errorf("keine IP: %v", sp)
	}
}

func TestOwnRulesWinAndNotify(t *testing.T) {
	s, err := New([]string{"!grün error 42", "# Kommentar", ""}, true)
	if err != nil {
		t.Fatal(err)
	}
	line := "got error 42 today"
	if c := colorAt(s.Spans(line), line, "error 42"); c != Green {
		t.Fatalf("eigene Regel zuerst: %v", s.Spans(line))
	}
	if r, ok := s.Alert(line); !ok || r.Color != Green {
		t.Fatal("Meldung fehlt")
	}
	if _, ok := s.Alert("got error 41"); ok {
		t.Fatal("falsche Meldung")
	}
	if !s.HasNotify() {
		t.Fatal("HasNotify")
	}
}

func TestParse(t *testing.T) {
	r, ok, err := Parse("  !Red  %LINK-3  ")
	if err != nil || !ok || !r.Notify || r.Color != Red || r.Expr != "%LINK-3" || r.String() != "!rot %LINK-3" {
		t.Fatalf("%+v %v %v", r, ok, err)
	}
	// Großbuchstaben = genau
	if !r.rx.MatchString("%LINK-3") || r.rx.MatchString("%link-3") {
		t.Fatal("smartcase")
	}
	for _, bad := range []string{"pink foo", "rot", "rot (", "rot a*"} {
		if _, _, err := Parse(bad); err == nil {
			t.Errorf("%q: Fehler erwartet", bad)
		}
	}
	s, err := New([]string{"rot ("}, false)
	if err == nil || len(s.Rules()) != 0 {
		t.Fatal("kaputte Regel muss gemeldet und übersprungen werden")
	}
}

func TestNameFollowsLanguage(t *testing.T) {
	defer i18n.Set(i18n.DE)
	r, _, _ := Parse("!rot fehler")
	i18n.Set(i18n.EN)
	if r.String() != "!red fehler" {
		t.Fatalf("englisch: %q", r.String())
	}
	if r2, ok, err := Parse(r.String()); !ok || err != nil || r2.Color != Red {
		t.Fatal("englische Regel nicht lesbar")
	}
	i18n.Set(i18n.DE)
	if r.String() != "!rot fehler" {
		t.Fatalf("deutsch: %q", r.String())
	}
}
