package update

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{{"1.0.10", "1.0.9", true}, {"1.0.9", "1.0.10", false}, {"1.1.0", "dev", true}, {"x", "1.0.0", false}, {"1.0.0", "1.0.0", false}}
	for _, c := range cases {
		if Newer(c.a, c.b) != c.want {
			t.Errorf("Newer(%s,%s)", c.a, c.b)
		}
	}
}
