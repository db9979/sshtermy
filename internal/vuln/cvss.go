package vuln

import (
	"math"
	"strings"
)

// Schwere aus OSV-Einträgen: CVSS v3.x wird nach der Spezifikation
// berechnet, v2 ebenso; v4 wird näherungsweise über die v3-Formel geschätzt
// (genau genug für die Einordnung kritisch/hoch/mittel/niedrig). Ubuntu
// liefert statt eines Vektors eine eigene Stufe.

// Level: Einordnung wie bei NVD.
type Level int

const (
	LevelUnknown Level = iota
	LevelLow
	LevelMedium
	LevelHigh
	LevelCritical
)

// LevelOf: Stufe zu einem CVSS-Wert.
func LevelOf(score float64) Level {
	switch {
	case score >= 9:
		return LevelCritical
	case score >= 7:
		return LevelHigh
	case score >= 4:
		return LevelMedium
	case score > 0:
		return LevelLow
	}
	return LevelUnknown
}

// ScoreOf: CVSS-Wert einer Schwere-Angabe.
func ScoreOf(s Severity) (float64, bool) {
	v := strings.TrimSpace(s.Score)
	switch strings.ToUpper(s.Type) {
	case "CVSS_V3":
		return cvss3(v, false)
	case "CVSS_V4":
		return cvss3(v, true)
	case "CVSS_V2":
		return cvss2(v)
	case "UBUNTU":
		switch strings.ToLower(v) {
		case "critical":
			return 9.5, true
		case "high":
			return 8, true
		case "medium":
			return 5.5, true
		case "low":
			return 3, true
		case "negligible":
			return 0.5, true
		}
	}
	return 0, false
}

func metrics(v string) map[string]string {
	m := map[string]string{}
	for _, p := range strings.Split(v, "/") {
		if k, x, ok := strings.Cut(p, ":"); ok {
			m[k] = x
		}
	}
	return m
}

// roundUp wie CVSS 3.1 (kleinste Zahl mit einer Nachkommastelle ≥ x).
func roundUp(x float64) float64 {
	n := int64(math.Round(x * 100000))
	if n%10000 == 0 {
		return float64(n) / 100000
	}
	return float64(n/10000+1) / 10
}

func cvss3(v string, v4 bool) (float64, bool) {
	m := metrics(v)
	av := map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}[m["AV"]]
	ac := map[string]float64{"L": 0.77, "H": 0.44}[m["AC"]]
	ui := map[string]float64{"N": 0.85, "R": 0.62, "P": 0.62, "A": 0.62}[m["UI"]]
	cia := map[string]float64{"H": 0.56, "L": 0.22, "N": 0}
	c, i, a := m["C"], m["I"], m["A"]
	changed := m["S"] == "C"
	if v4 {
		c, i, a = m["VC"], m["VI"], m["VA"]
		if m["AT"] == "P" {
			ac = 0.44
		}
		changed = m["SC"] == "H" || m["SI"] == "H" || m["SA"] == "H"
	}
	pr := map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}[m["PR"]]
	if changed {
		pr = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.5}[m["PR"]]
	}
	if av == 0 || ac == 0 || ui == 0 || pr == 0 {
		return 0, false
	}
	cv, ok1 := cia[c]
	iv, ok2 := cia[i]
	avv, ok3 := cia[a]
	if !ok1 || !ok2 || !ok3 {
		return 0, false
	}
	iss := 1 - (1-cv)*(1-iv)*(1-avv)
	var impact float64
	if changed {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}
	if impact <= 0 {
		return 0, true
	}
	expl := 8.22 * av * ac * pr * ui
	if changed {
		return roundUp(math.Min(1.08*(impact+expl), 10)), true
	}
	return roundUp(math.Min(impact+expl, 10)), true
}

func cvss2(v string) (float64, bool) {
	m := metrics(v)
	av := map[string]float64{"L": 0.395, "A": 0.646, "N": 1}[m["AV"]]
	ac := map[string]float64{"H": 0.35, "M": 0.61, "L": 0.71}[m["AC"]]
	au := map[string]float64{"M": 0.45, "S": 0.56, "N": 0.704}[m["Au"]]
	cia := map[string]float64{"N": 0, "P": 0.275, "C": 0.66}
	if av == 0 || ac == 0 || au == 0 {
		return 0, false
	}
	c, ok1 := cia[m["C"]]
	i, ok2 := cia[m["I"]]
	a, ok3 := cia[m["A"]]
	if !ok1 || !ok2 || !ok3 {
		return 0, false
	}
	impact := 10.41 * (1 - (1-c)*(1-i)*(1-a))
	f := 1.176
	if impact == 0 {
		f = 0
	}
	score := (0.6*impact + 0.4*20*av*ac*au - 1.5) * f
	return math.Round(score*10) / 10, true
}
