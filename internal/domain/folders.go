package domain

import (
	"sort"
	"strings"
	"time"
)

// Ordner
//
// Eine Verbindung liegt in dem Ordner, den ihr Feld Group nennt. Ordner
// lassen sich verschachteln, die Stufen trennt „ / “ (wie beim
// MobaXterm-Import): "Produktion / Web / Frontend". Ein Ordner existiert,
// sobald eine Verbindung darin liegt; ein Folder-Eintrag merkt sich zusätzlich
// Reihenfolge und Auf-/Zugeklappt und hält leere Ordner fest. Beim Sync wird
// er wie eine Verbindung pro Eintrag zusammengeführt.

// PathSep trennt die Stufen eines Ordnerpfads.
const PathSep = " / "

// Folder: Ordner im Verbindungs-Manager.
type Folder struct {
	ID        string  `json:"id"`
	Path      string  `json:"path"`            // normalisiert, siehe NormPath
	Order     float64 `json:"order,omitempty"` // unter Geschwistern, 0 = alphabetisch
	Collapsed bool    `json:"collapsed,omitempty"`
	// AIRules: Hausregeln für die KI, gelten für alle Verbindungen in diesem
	// Ordner und seinen Unterordnern (zusätzlich zu LLMConfig.Rules).
	AIRules   string    `json:"ai_rules,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Rev: Zeitstempel für den Sync.
func (f Folder) Rev() time.Time {
	if f.UpdatedAt.IsZero() {
		return f.CreatedAt
	}
	return f.UpdatedAt
}

// NormPath macht aus „Prod/Web “, „Prod / Web“ oder „Prod\Web“ einheitlich
// „Prod / Web“. Leere Stufen fallen weg.
func NormPath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	var parts []string
	for _, s := range strings.Split(p, "/") {
		if s = strings.Join(strings.Fields(s), " "); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, PathSep)
}

// PathParts zerlegt einen (normalisierten) Pfad.
func PathParts(p string) []string {
	if p == "" {
		return nil
	}
	return strings.Split(p, PathSep)
}

// PathParent: übergeordneter Ordner ("" = oberste Ebene).
func PathParent(p string) string {
	if i := strings.LastIndex(p, PathSep); i >= 0 {
		return p[:i]
	}
	return ""
}

// PathBase: letzter Teil des Pfads (Anzeigename).
func PathBase(p string) string {
	if i := strings.LastIndex(p, PathSep); i >= 0 {
		return p[i+len(PathSep):]
	}
	return p
}

// PathJoin hängt name an parent an.
func PathJoin(parent, name string) string {
	name = NormPath(name)
	if parent == "" {
		return name
	}
	if name == "" {
		return parent
	}
	return parent + PathSep + name
}

// InFolder: liegt path in folder oder darunter? (folder "" = überall)
func InFolder(path, folder string) bool {
	return folder == "" || path == folder || strings.HasPrefix(path, folder+PathSep)
}

// Reparent ersetzt den Anfang old in path durch neu (Umbenennen/Verschieben
// eines Ordners samt Inhalt). ok = false, wenn path nicht unter old liegt.
func Reparent(path, old, neu string) (string, bool) {
	if path == old {
		return neu, true
	}
	if strings.HasPrefix(path, old+PathSep) {
		return PathJoin(neu, path[len(old)+len(PathSep):]), true
	}
	return path, false
}

// ---------------------------------------------------------------- Baum

// TreeNode ist ein Ordner im Baum. Die Wurzel hat Path "".
type TreeNode struct {
	Path      string
	Name      string
	Order     float64
	Collapsed bool
	Known     bool // es gibt einen Folder-Eintrag
	Folders   []*TreeNode
	Conns     []Connection
	Total     int // Verbindungen in diesem Ordner und darunter
}

// BuildTree ordnet Verbindungen und Ordner zu einem Baum. Ordner kommen vor
// den Verbindungen, beide sortiert nach Order, dann alphabetisch.
func BuildTree(conns []Connection, folders []Folder) *TreeNode {
	root := &TreeNode{}
	byPath := map[string]*TreeNode{"": root}
	var ensure func(p string) *TreeNode
	ensure = func(p string) *TreeNode {
		if n, ok := byPath[p]; ok {
			return n
		}
		parent := ensure(PathParent(p))
		n := &TreeNode{Path: p, Name: PathBase(p)}
		byPath[p] = n
		parent.Folders = append(parent.Folders, n)
		return n
	}
	for _, f := range folders {
		p := NormPath(f.Path)
		if p == "" {
			continue
		}
		n := ensure(p)
		n.Order, n.Collapsed, n.Known = f.Order, f.Collapsed, true
	}
	for _, c := range conns {
		n := ensure(NormPath(c.Group))
		n.Conns = append(n.Conns, c)
	}
	var walk func(n *TreeNode) int
	walk = func(n *TreeNode) int {
		sort.SliceStable(n.Folders, func(i, j int) bool {
			a, b := n.Folders[i], n.Folders[j]
			if a.Order != b.Order {
				return orderLess(a.Order, b.Order)
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
		SortConns(n.Conns)
		t := len(n.Conns)
		for _, f := range n.Folders {
			t += walk(f)
		}
		n.Total = t
		return t
	}
	walk(root)
	return root
}

// orderLess: gesetzte Reihenfolge vor 0 (= noch nie umsortiert).
func orderLess(a, b float64) bool {
	if a == 0 {
		return false
	}
	if b == 0 {
		return true
	}
	return a < b
}

// SortConns sortiert Verbindungen eines Ordners (Order, dann Alias).
func SortConns(cs []Connection) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Order != cs[j].Order {
			return orderLess(cs[i].Order, cs[j].Order)
		}
		return strings.ToLower(cs[i].Alias) < strings.ToLower(cs[j].Alias)
	})
}

// Find sucht einen Ordner im Baum.
func (n *TreeNode) Find(path string) *TreeNode {
	if n.Path == path {
		return n
	}
	for _, f := range n.Folders {
		if InFolder(path, f.Path) {
			return f.Find(path)
		}
	}
	return nil
}

// AllConns: alle Verbindungen in Baumreihenfolge (wie im Manager).
func (n *TreeNode) AllConns() []Connection {
	var out []Connection
	for _, f := range n.Folders {
		out = append(out, f.AllConns()...)
	}
	return append(out, n.Conns...)
}

// AllPaths: alle Ordnerpfade in Baumreihenfolge.
func (n *TreeNode) AllPaths() []string {
	var out []string
	for _, f := range n.Folders {
		out = append(out, f.Path)
		out = append(out, f.AllPaths()...)
	}
	return out
}

// ---------------------------------------------------------------- Jump-Hosts

// JumpChain: die Stationen bis zur Verbindung id, vom ersten Sprung bis zu
// ihr selbst (z. B. [bastion, jump-lan, ziel]). cycle = true, wenn die Kette
// in einen Kreis läuft (dann endet sie vor der Wiederholung).
func JumpChain(id string, get func(string) (Connection, bool)) (chain []Connection, cycle bool) {
	seen := map[string]bool{}
	for cur := id; cur != ""; {
		if seen[cur] {
			cycle = true
			break
		}
		seen[cur] = true
		c, ok := get(cur)
		if !ok {
			break
		}
		chain = append([]Connection{c}, chain...)
		cur = c.JumpHostID
	}
	return chain, cycle
}

// JumpCreatesCycle: würde „self über jump“ einen Kreis bilden? (jump führt
// selbst irgendwann über self, oder jump == self)
func JumpCreatesCycle(self, jump string, get func(string) (Connection, bool)) bool {
	if jump == "" {
		return false
	}
	seen := map[string]bool{}
	for cur := jump; cur != ""; {
		if cur == self || seen[cur] {
			return true
		}
		seen[cur] = true
		c, ok := get(cur)
		if !ok {
			return false
		}
		cur = c.JumpHostID
	}
	return false
}
