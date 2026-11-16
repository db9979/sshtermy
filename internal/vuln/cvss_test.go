package vuln

import "testing"

func TestScores(t *testing.T) {
	for _, c := range []struct {
		typ, vec string
		want     float64
	}{
		{"CVSS_V3", "CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:H", 8.1},
		{"CVSS_V3", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8},
		{"CVSS_V3", "CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H", 7.8},
		{"CVSS_V3", "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:H/I:L/A:L", 8.8},
		{"CVSS_V3", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H", 10},
		{"CVSS_V3", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N", 0},
		{"CVSS_V3", "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:N/A:N", 4.3},
		{"CVSS_V2", "AV:N/AC:L/Au:N/C:P/I:P/A:P", 7.5},
		{"Ubuntu", "high", 8},
	} {
		got, ok := ScoreOf(Severity{Type: c.typ, Score: c.vec})
		if !ok || got != c.want {
			t.Errorf("%s %s = %v %v, want %v", c.typ, c.vec, got, ok, c.want)
		}
	}
	if got, ok := ScoreOf(Severity{Type: "CVSS_V4", Score: "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"}); !ok || LevelOf(got) != LevelCritical {
		t.Errorf("v4 = %v", got)
	}
}
