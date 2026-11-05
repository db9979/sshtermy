package hostinfo

import "github.com/db9979/sshtermy/internal/i18n"
import "fmt"

// DiffOp: Art einer Diff-Zeile.
type DiffOp byte

const (
	DiffSame DiffOp = ' '
	DiffDel  DiffOp = '-'
	DiffAdd  DiffOp = '+'
	DiffHunk DiffOp = '@' // Abschnittskopf "@@ Zeile 12 @@"
)

// DiffLine ist eine Zeile der Unified-Ansicht.
type DiffLine struct {
	Op   DiffOp
	Text string
}

// Diff vergleicht zwei Zeilenlisten und liefert eine Unified-Ansicht mit ctx
// Zeilen Kontext sowie die Zahl hinzugefügter und entfernter Zeilen.
//
// Gemeinsamer Anfang und gemeinsames Ende werden vorab abgeschnitten; der Rest
// wird per LCS verglichen. Ist dieser Rest riesig (anderes Gerät, ganz neue
// Konfiguration), gilt er als komplett ersetzt, damit der Speicher klein bleibt.
func Diff(a, b []string, ctx int) (lines []DiffLine, added, removed int) {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]

	// Ablauf als Folge von Operationen über alle Zeilen.
	type op struct {
		op   DiffOp
		text string
		ln   int // Zeilennummer im neuen Stand (1-basiert)
	}
	ops := make([]op, 0, len(a)+len(b))
	nb := 0
	for i := 0; i < pre; i++ {
		nb++
		ops = append(ops, op{DiffSame, a[i], nb})
	}
	if int64(len(ma))*int64(len(mb)) <= 4_000_000 {
		n, m := len(ma), len(mb)
		L := make([]int32, (n+1)*(m+1))
		at := func(i, j int) int32 { return L[i*(m+1)+j] }
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				v := at(i+1, j)
				if ma[i] == mb[j] {
					v = at(i+1, j+1) + 1
				} else if w := at(i, j+1); w > v {
					v = w
				}
				L[i*(m+1)+j] = v
			}
		}
		i, j := 0, 0
		for i < n || j < m {
			switch {
			case i < n && j < m && ma[i] == mb[j]:
				nb++
				ops = append(ops, op{DiffSame, ma[i], nb})
				i++
				j++
			case i < n && (j == m || at(i+1, j) >= at(i, j+1)):
				ops = append(ops, op{DiffDel, ma[i], nb}) // erst Entferntes, dann Neues
				i++
			default:
				nb++
				ops = append(ops, op{DiffAdd, mb[j], nb})
				j++
			}
		}
	} else {
		for _, l := range ma {
			ops = append(ops, op{DiffDel, l, nb})
		}
		for _, l := range mb {
			nb++
			ops = append(ops, op{DiffAdd, l, nb})
		}
	}
	for i := len(a) - suf; i < len(a); i++ {
		nb++
		ops = append(ops, op{DiffSame, a[i], nb})
	}

	// Nur Änderungen mit ctx Zeilen Umgebung behalten.
	keep := make([]bool, len(ops))
	for i, o := range ops {
		if o.op == DiffSame {
			continue
		}
		if o.op == DiffAdd {
			added++
		} else {
			removed++
		}
		for k := max(0, i-ctx); k <= min(len(ops)-1, i+ctx); k++ {
			keep[k] = true
		}
	}
	last := -2
	for i, o := range ops {
		if !keep[i] {
			continue
		}
		if i != last+1 {
			ln := o.ln
			if o.op == DiffDel {
				ln++
			}
			lines = append(lines, DiffLine{DiffHunk, fmt.Sprintf(i18n.T("@@ Zeile %d @@"), max(1, ln))})
		}
		lines = append(lines, DiffLine{o.op, o.text})
		last = i
	}
	return lines, added, removed
}
