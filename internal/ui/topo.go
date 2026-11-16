package ui

// Netzwerk-Topologie als Bild: jeder Switch aus sshTermy ein Kasten, darunter
// die Switches, die an ihm hängen, verbunden mit Linien samt Portnummern.
// Nachbarn, die nicht in sshTermy stehen (Access Points, Telefone, fremde
// Switches), stehen gedimmt im Kasten. Passt das Bild nicht ins Fenster,
// bleibt es beim eingerückten Baum.

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/netmap"
)

const (
	topoBoxMax  = 30 // breitester Kasten (Zeichen, mit Rahmen)
	topoGap     = 3  // Abstand zwischen Nachbarkästen
	topoLeafMax = 4  // so viele fremde Nachbarn im Kasten, dann „+N weitere“
)

type topoNode struct {
	name     string
	host     string
	leaves   []string // fremde Nachbarn, schon mit Port
	kids     []*topoNode
	port     string // Verbindung zum Elternteil: „1/1/48 – 49“
	w, h     int    // Kasten
	span     int    // Breite des ganzen Teilbaums
	x, y, cx int    // Lage des Kastens, cx = Mitte
	left     int    // linker Rand des Teilbaums
}

// topoCell: ein Zeichen mit Farbe auf der Zeichenfläche.
type topoCell struct {
	r    rune
	fg   color.Color
	bold bool
}

type topoCanvas struct {
	w, h  int
	cells [][]topoCell
}

func newTopoCanvas(w, h int) *topoCanvas {
	c := &topoCanvas{w: w, h: h, cells: make([][]topoCell, h)}
	for y := range c.cells {
		c.cells[y] = make([]topoCell, w)
		for x := range c.cells[y] {
			c.cells[y][x].r = ' '
		}
	}
	return c
}

func (c *topoCanvas) put(x, y int, s string, fg color.Color, bold bool) {
	for _, r := range s {
		if x >= 0 && x < c.w && y >= 0 && y < c.h {
			c.cells[y][x] = topoCell{r, fg, bold}
		}
		x++
	}
}

func (c *topoCanvas) lines() []string {
	out := make([]string, 0, c.h)
	for _, row := range c.cells {
		var b strings.Builder
		var run []rune
		var fg color.Color
		bold := false
		flush := func() {
			if len(run) == 0 {
				return
			}
			st := lipgloss.NewStyle().Bold(bold)
			if fg != nil {
				st = st.Foreground(fg)
			}
			b.WriteString(st.Render(string(run)))
			run = run[:0]
		}
		end := len(row)
		for end > 0 && row[end-1].r == ' ' {
			end--
		}
		for _, cell := range row[:end] {
			if cell.fg != fg || cell.bold != bold {
				flush()
				fg, bold = cell.fg, cell.bold
			}
			run = append(run, cell.r)
		}
		flush()
		out = append(out, b.String())
	}
	return out
}

// topoForest: Bäume ab den Switches mit den meisten Verbindungen;
// Querverbindungen, die nicht in den Baum passen, kommen in extra.
func topoForest(devs []netmap.Device, links []netmap.Link) (roots []*topoNode, extra []string) {
	host := map[string]string{}
	order := make([]string, 0, len(devs))
	for _, d := range devs {
		host[d.Alias] = d.Host
		order = append(order, d.Alias)
	}
	type edge struct {
		to, port string
		used     *bool
	}
	adj := map[string][]edge{}
	leaves := map[string][]string{}
	deg := map[string]int{}
	for _, l := range links {
		if !l.Known {
			leaves[l.A] = append(leaves[l.A], strings.TrimSpace(netmap.ShortName(l.B)+"  "+l.APort))
			continue
		}
		u := new(bool)
		adj[l.A] = append(adj[l.A], edge{l.B, portPair(l.APort, l.BPort), u})
		adj[l.B] = append(adj[l.B], edge{l.A, portPair(l.BPort, l.APort), u})
		deg[l.A]++
		deg[l.B]++
	}
	sort.SliceStable(order, func(i, j int) bool { return deg[order[i]] > deg[order[j]] })
	done := map[string]bool{}
	mk := func(name string) *topoNode {
		n := &topoNode{name: name, host: host[name]}
		ls := leaves[name]
		if len(ls) > topoLeafMax {
			rest := len(ls) - (topoLeafMax - 1)
			ls = append(ls[:topoLeafMax-1:topoLeafMax-1], fmt.Sprintf(i18n.T("+%d weitere"), rest))
		}
		n.leaves = ls
		return n
	}
	for _, name := range order {
		if done[name] {
			continue
		}
		root := mk(name)
		done[name] = true
		queue := []*topoNode{root}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, e := range adj[p.name] {
				if done[e.to] {
					continue
				}
				*e.used = true
				done[e.to] = true
				k := mk(e.to)
				k.port = e.port
				p.kids = append(p.kids, k)
				queue = append(queue, k)
			}
		}
		roots = append(roots, root)
	}
	for _, l := range links {
		if !l.Known {
			continue
		}
		for _, e := range adj[l.A] {
			if e.to == l.B && !*e.used && e.port == portPair(l.APort, l.BPort) {
				*e.used = true // nur einmal melden
				extra = append(extra, fmt.Sprintf("%s %s  ─  %s %s", l.A, l.APort, l.B, l.BPort))
			}
		}
	}
	return roots, extra
}

func portPair(a, b string) string {
	if b == "" {
		return a
	}
	return a + " – " + b
}

// measure: Kastengrößen und Breite der Teilbäume.
func (n *topoNode) measure() {
	inner := max(runeLen(n.name), runeLen(n.host))
	for _, l := range n.leaves {
		inner = max(inner, runeLen(l)+2)
	}
	n.w = min(topoBoxMax, inner+4)
	n.h = 2 + 1 + boolInt(n.host != "") + len(n.leaves)
	kids := 0
	for i, k := range n.kids {
		k.measure()
		if i > 0 {
			kids += topoGap
		}
		kids += k.span
	}
	n.span = max(n.w, kids)
}

// place: Kasten mittig über seinen Kindern, Kinder nebeneinander.
func (n *topoNode) place(x, y int) {
	n.left = x
	if len(n.kids) == 0 {
		n.x, n.y, n.cx = x+(n.span-n.w)/2, y, x+n.span/2
		return
	}
	n.left = x
	kids := -topoGap
	for _, k := range n.kids {
		kids += k.span + topoGap
	}
	kx := x + (n.span-kids)/2
	childY := y + n.h + 2 // Balken, Linie mit Port, dann Kasten
	for _, k := range n.kids {
		k.place(kx, childY)
		kx += k.span + topoGap
	}
	mid := (n.kids[0].cx + n.kids[len(n.kids)-1].cx) / 2
	if len(n.kids)%2 == 1 {
		mid = n.kids[len(n.kids)/2].cx // genau über dem mittleren Kind
	}
	n.x, n.y, n.cx = mid-n.w/2, y, mid
	n.x = max(x, min(n.x, x+n.span-n.w))
	n.cx = n.x + n.w/2
}

func (n *topoNode) depth() int {
	d := n.y + n.h
	for _, k := range n.kids {
		d = max(d, k.depth())
	}
	return d
}

// topoLines: Bild der Topologie; nil, wenn es nicht in w Spalten passt.
func (a *App) topoLines(devs []netmap.Device, links []netmap.Link, w int) []string {
	roots, extra := topoForest(devs, links)
	t := a.s.T
	line := mixColor(t.Border, t.Accent, 0.45)
	var out []string
	for i, r := range roots {
		r.measure()
		if r.span > w {
			return nil
		}
		r.place(0, 0)
		c := newTopoCanvas(r.span+topoBoxMax, r.depth())
		a.drawTopo(c, r, i == 0, line)
		if len(out) > 0 {
			out = append(out, "")
		}
		for _, l := range c.lines() {
			if lipgloss.Width(l) > w {
				return nil
			}
			out = append(out, l)
		}
	}
	if len(extra) > 0 {
		out = append(out, "", a.s.Muted.Render(i18n.T("Weitere Verbindungen (Ring/Redundanz):")))
		for _, e := range extra {
			out = append(out, lipgloss.NewStyle().Foreground(line).Render("  "+e))
		}
	}
	return out
}

func (a *App) drawTopo(c *topoCanvas, n *topoNode, root bool, line color.Color) {
	t := a.s.T
	frame := t.Border
	if root {
		frame = t.Accent
	}
	tc, bc := frameCorners()
	inner := n.w - 2
	c.put(n.x, n.y, string(tc[0])+strings.Repeat("─", inner)+string(tc[1]), frame, false)
	for y := n.y + 1; y < n.y+n.h-1; y++ {
		c.put(n.x, y, "│"+strings.Repeat(" ", inner)+"│", frame, false)
	}
	c.put(n.x, n.y+n.h-1, string(bc[0])+strings.Repeat("─", inner)+string(bc[1]), frame, false)
	y := n.y + 1
	c.put(n.x+2, y, trunc(n.name, inner-2), t.Text, true)
	y++
	if n.host != "" {
		c.put(n.x+2, y, trunc(n.host, inner-2), t.Muted, false)
		y++
	}
	for _, l := range n.leaves {
		c.put(n.x+2, y, "· "+trunc(l, inner-4), mixColor(t.Muted, t.Bg, 0.25), false)
		y++
	}
	if len(n.kids) == 0 {
		return
	}
	// Abzweig unten am Kasten, Balken, Linie zu jedem Kind mit Port
	bottom := n.y + n.h - 1
	c.put(n.cx, bottom, "┬", frame, false)
	bar := bottom + 1
	lo, hi := n.cx, n.cx
	down := map[int]bool{}
	for _, k := range n.kids {
		lo, hi = min(lo, k.cx), max(hi, k.cx)
		down[k.cx] = true
	}
	for x := lo; x <= hi; x++ {
		c.put(x, bar, string(boxJoin(x == n.cx, down[x], x > lo, x < hi)), line, false)
	}
	for i, k := range n.kids {
		c.put(k.cx, bar+1, "│", line, false)
		room := runeLen(k.port) + 2 // der letzte darf über den Rand hinaus
		if i+1 < len(n.kids) {
			room = n.kids[i+1].cx - k.cx - 2 // bis zum nächsten Kind
		}
		if k.port != "" && room > 3 {
			c.put(k.cx+2, bar+1, trunc(k.port, room-2), t.Muted, false)
		}
		a.drawTopo(c, k, false, line)
		c.put(k.cx, k.y, "┴", t.Border, false)
	}
}

// boxJoin: Linienzeichen mit Ästen nach oben, unten, links, rechts.
func boxJoin(up, down, left, right bool) rune {
	switch {
	case up && down && left && right:
		return '┼'
	case up && down && left:
		return '┤'
	case up && down && right:
		return '├'
	case up && down:
		return '│'
	case up && left && right:
		return '┴'
	case down && left && right:
		return '┬'
	case up && left:
		return '┘'
	case up && right:
		return '└'
	case down && left:
		return '┐'
	case down && right:
		return '┌'
	}
	return '─'
}

func runeLen(s string) int { return len([]rune(s)) }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
