package vuln

import "testing"

func TestCompareDeb(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1:9.2p1-2+deb12u2", "1:9.2p1-2+deb12u3", -1},
		{"1:9.2p1-2+deb12u3", "1:9.2p1-2+deb12u3", 0},
		{"9.9", "1:1.0", -1},
		{"1.0~rc1", "1.0", -1},
		{"1.0", "1.0+b1", -1},
		{"3.0.11-1~deb12u2", "3.0.15-1~deb12u1", -1},
		{"2.5.0-1", "2.5.0-1+deb12u1", -1},
		{"1.2.10", "1.2.9", 1},
		{"1.0-1", "1.0", 1},
		{"1.0a", "1.0+", -1},
		{"7.88.1-10+deb12u8", "7.88.1-10+deb12u7", 1},
	} {
		if got := CompareDeb(c.a, c.b); got != c.want {
			t.Errorf("CompareDeb(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
		if got := CompareDeb(c.b, c.a); got != -c.want {
			t.Errorf("CompareDeb(%q,%q)=%d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestCompareRPM(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0:1.20.11-28.el8_10.5", "0:1.20.11-28.el8_10.6", -1},
		{"1.20.11-28.el8_10.6", "0:1.20.11-28.el8_10.6", 0},
		{"(none):3.0.7-27.el9", "1:3.0.7-27.el9", -1},
		{"2:3.32.2-1.el10_2", "2:3.5.0-1.el10", 1},
		{"1.0~rc1-1", "1.0-1", -1},
		{"1.0^git1-1", "1.0-1", 1},
		{"1.0^git1-1", "1.0.1-1", -1},
		{"1.0a-1", "1.0-1", 1},
		{"1.0.1-1", "1.0a-1", 1},
		{"4.16.7_18-150400.4.98.1", "4.16.7_18-150400.4.95.1", 1},
	} {
		if got := CompareRPM(c.a, c.b); got != c.want {
			t.Errorf("CompareRPM(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
		if got := CompareRPM(c.b, c.a); got != -c.want {
			t.Errorf("CompareRPM(%q,%q)=%d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestCompareAPK(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.26.10-r0", "1.26.11-r0", -1},
		{"3.3.2-r0", "3.3.2-r1", -1},
		{"1.2.3_rc1-r0", "1.2.3-r0", -1},
		{"1.2.3_p1-r0", "1.2.3-r0", 1},
		{"1.2.3a-r0", "1.2.3-r0", 1},
		{"1.2.3-r0", "1.2.3.1-r0", -1},
		{"1.2.3_alpha-r0", "1.2.3_beta-r0", -1},
		{"8.11.1-r0", "8.9.1-r5", 1},
	} {
		if got := CompareAPK(c.a, c.b); got != c.want {
			t.Errorf("CompareAPK(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
		if got := CompareAPK(c.b, c.a); got != -c.want {
			t.Errorf("CompareAPK(%q,%q)=%d, want %d", c.b, c.a, got, -c.want)
		}
	}
}
