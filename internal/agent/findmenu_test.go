package agent

import "testing"

func TestParseMenuPick(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want int
		why  string
	}{
		{"Nr: 3\nWarum: Tunnel leiten Ports weiter.", 5, 2, "Tunnel leiten Ports weiter."},
		{"**Nr:** 12.\nWarum: passt", 20, 11, "passt"},
		{"Nr: 0\nWarum: gibt es nicht", 5, -1, "gibt es nicht"},
		{"Nr: 99\nWarum: x", 5, -1, "x"},
		{"keine Ahnung", 5, -1, ""},
	}
	for _, c := range cases {
		p := ParseMenuPick(c.in, c.n)
		if p.Index != c.want || p.Why != c.why {
			t.Errorf("%q: %+v, wollte %d/%q", c.in, p, c.want, c.why)
		}
	}
}
