package vuln

import (
	"strings"
)

// Versionsvergleich je Paketformat. OSV-Bereiche geben „introduced“ und
// „fixed“ als Versionen des jeweiligen Ökosystems an; verglichen wird nach
// den Regeln von dpkg, rpm bzw. apk. Ergebnis wie strings.Compare.

// Compare vergleicht zwei Versionen nach den Regeln des Formats.
func Compare(f Format, a, b string) int {
	switch f {
	case FormatRPM:
		return CompareRPM(a, b)
	case FormatAPK:
		return CompareAPK(a, b)
	}
	return CompareDeb(a, b)
}

// ---------------------------------------------------------------- dpkg

// CompareDeb: [epoch:]upstream[-revision] wie dpkg --compare-versions.
func CompareDeb(a, b string) int {
	ea, ua, ra := splitDeb(a)
	eb, ub, rb := splitDeb(b)
	if c := cmpNum(ea, eb); c != 0 {
		return c
	}
	if c := debPart(ua, ub); c != 0 {
		return c
	}
	return debPart(ra, rb)
}

func splitDeb(v string) (epoch, up, rev string) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		epoch, v = v[:i], v[i+1:]
	}
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		v, rev = v[:i], v[i+1:]
	}
	return epoch, v, rev
}

// debOrder: ~ vor allem (auch vor dem Ende), Buchstaben vor Sonderzeichen.
func debOrder(c byte, ok bool) int {
	switch {
	case !ok:
		return 0
	case c == '~':
		return -1
	case isDigit(c):
		return 0
	case isAlpha(c):
		return int(c)
	}
	return int(c) + 256
}

func debPart(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		// nicht-numerischer Teil
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			var ca, cb byte
			okA, okB := i < len(a) && !isDigit(a[i]), j < len(b) && !isDigit(b[j])
			if okA {
				ca = a[i]
			}
			if okB {
				cb = b[j]
			}
			oa, ob := debOrder(ca, okA), debOrder(cb, okB)
			if oa != ob {
				return sign(oa - ob)
			}
			if okA {
				i++
			}
			if okB {
				j++
			}
		}
		// numerischer Teil
		si := i
		for i < len(a) && isDigit(a[i]) {
			i++
		}
		sj := j
		for j < len(b) && isDigit(b[j]) {
			j++
		}
		if c := cmpNum(a[si:i], b[sj:j]); c != 0 {
			return c
		}
	}
	return 0
}

// ---------------------------------------------------------------- rpm

// CompareRPM: [epoch:]version[-release] wie rpmvercmp (mit ~ und ^).
func CompareRPM(a, b string) int {
	ea, va, ra := splitRPM(a)
	eb, vb, rb := splitRPM(b)
	if c := cmpNum(ea, eb); c != 0 {
		return c
	}
	if c := rpmvercmp(va, vb); c != 0 {
		return c
	}
	if ra == "" || rb == "" {
		return 0 // ohne Release gilt jede Release als gleich
	}
	return rpmvercmp(ra, rb)
}

func splitRPM(v string) (epoch, ver, rel string) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		epoch, v = v[:i], v[i+1:]
	}
	if epoch == "(none)" {
		epoch = ""
	}
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		v, rel = v[:i], v[i+1:]
	}
	return epoch, v, rel
}

func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for i < len(a) && !isAlnum(a[i]) && a[i] != '~' && a[i] != '^' {
			i++
		}
		for j < len(b) && !isAlnum(b[j]) && b[j] != '~' && b[j] != '^' {
			j++
		}
		// Tilde: kleiner als alles, auch als das Ende
		ta, tb := i < len(a) && a[i] == '~', j < len(b) && b[j] == '~'
		if ta || tb {
			if !ta {
				return 1
			}
			if !tb {
				return -1
			}
			i++
			j++
			continue
		}
		// Caret: größer als das Ende, kleiner als alles andere
		ca, cb := i < len(a) && a[i] == '^', j < len(b) && b[j] == '^'
		if ca || cb {
			if i >= len(a) {
				return -1
			}
			if j >= len(b) {
				return 1
			}
			if !ca {
				return 1
			}
			if !cb {
				return -1
			}
			i++
			j++
			continue
		}
		if i >= len(a) || j >= len(b) {
			break
		}
		si, sj := i, j
		num := isDigit(a[i])
		if num {
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
		} else {
			for i < len(a) && isAlpha(a[i]) {
				i++
			}
			for j < len(b) && isAlpha(b[j]) {
				j++
			}
		}
		pa, pb := a[si:i], b[sj:j]
		if pb == "" {
			// Zahl gegen Buchstaben: Zahl ist neuer
			if num {
				return 1
			}
			return -1
		}
		if num {
			if c := cmpNum(pa, pb); c != 0 {
				return c
			}
		} else if c := strings.Compare(pa, pb); c != 0 {
			return c
		}
	}
	switch {
	case i >= len(a) && j >= len(b):
		return 0
	case i >= len(a):
		return -1
	}
	return 1
}

// ---------------------------------------------------------------- apk

// CompareAPK: Alpine-Versionen wie apk version -t (1.2.3a_rc1_p2-r4).
func CompareAPK(a, b string) int {
	va, ra := splitAPK(a)
	vb, rb := splitAPK(b)
	if c := apkVer(va, vb); c != 0 {
		return c
	}
	return cmpNum(ra, rb)
}

func splitAPK(v string) (ver, rel string) {
	v = strings.TrimSpace(v)
	if i := strings.LastIndex(v, "-r"); i >= 0 && allDigits(v[i+2:]) {
		return v[:i], v[i+2:]
	}
	return v, ""
}

// apkSuffix: Rang der Endungen; ohne Endung = 0.
var apkSuffix = map[string]int{"alpha": -4, "beta": -3, "pre": -2, "rc": -1, "cvs": 1, "svn": 2, "git": 3, "hg": 4, "p": 5}

type apkTok struct {
	kind int // 0 Zahl, 1 Buchstabe, 2 Endung
	s    string
	n    int
}

func apkTokens(v string) []apkTok {
	var out []apkTok
	i := 0
	for i < len(v) {
		c := v[i]
		switch {
		case isDigit(c):
			s := i
			for i < len(v) && isDigit(v[i]) {
				i++
			}
			out = append(out, apkTok{kind: 0, s: v[s:i]})
		case c == '_':
			i++
			s := i
			for i < len(v) && isAlpha(v[i]) {
				i++
			}
			r, ok := apkSuffix[v[s:i]]
			if !ok {
				r = 0
			}
			out = append(out, apkTok{kind: 2, n: r})
		case isAlpha(c):
			out = append(out, apkTok{kind: 1, s: string(c)})
			i++
		default:
			i++
		}
	}
	return out
}

func apkVer(a, b string) int {
	ta, tb := apkTokens(a), apkTokens(b)
	for k := 0; k < len(ta) || k < len(tb); k++ {
		if k >= len(ta) {
			// b ist länger: Endung wie _rc macht b älter, sonst ist b neuer
			if tb[k].kind == 2 && tb[k].n < 0 {
				return 1
			}
			return -1
		}
		if k >= len(tb) {
			if ta[k].kind == 2 && ta[k].n < 0 {
				return -1
			}
			return 1
		}
		x, y := ta[k], tb[k]
		if x.kind != y.kind {
			// Endung < Zahl/Buchstabe, wenn die Endung „vor“ steht
			if x.kind == 2 {
				return sign(x.n*2 - 1)
			}
			if y.kind == 2 {
				return -sign(y.n*2 - 1)
			}
			return sign(x.kind - y.kind)
		}
		var c int
		switch x.kind {
		case 0:
			c = cmpNum(x.s, y.s)
		case 1:
			c = strings.Compare(x.s, y.s)
		case 2:
			c = sign(x.n - y.n)
		}
		if c != 0 {
			return c
		}
	}
	return 0
}

// ---------------------------------------------------------------- Hilfen

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isAlnum(c byte) bool { return isDigit(c) || isAlpha(c) }

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// cmpNum vergleicht Ziffernfolgen beliebiger Länge (leer = 0).
func cmpNum(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return sign(len(a) - len(b))
	}
	return strings.Compare(a, b)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
