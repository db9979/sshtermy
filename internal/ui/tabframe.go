package ui

// Farbige Rahmen je Tab: Jedes Terminal (allein oder als Bereich der
// geteilten Ansicht) bekommt einen Rahmen in der Farbe seiner Verbindung,
// und der Name oben (Tab-Leiste, Bereichstitel) steht in derselben Farbe.
//
// Farbe: eigene Wahl im Verbindungsformular, sonst die Umgebung
// (prod rot, test grün, labor blau, accent.go), sonst bekommt jeder Tab
// automatisch eine eigene Farbe. Rot vergibt die Automatik nie, Rot heißt
// also immer „produktiv“ (oder ausdrücklich so gewählt). Abschaltbar in den
// Einstellungen (Ansicht); dann wie früher nur der Farbstreifen.

import (
	"hash/fnv"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// autoTabColors: Reihenfolge der automatischen Farben. Ohne Rot (= produktiv);
// Grün und Blau (test/labor) zuletzt, damit sie selten doppelt belegt sind.
var autoTabColors = []string{"cyan", "purple", "yellow", "orange", "blue", "green"}

func (a *App) tabFramesOn() bool { return a.st == nil || !a.st.Settings().NoTabFrames }

func (a *App) toggleTabFrames() tea.Cmd {
	on := !a.tabFramesOn()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoTabFrames = !on }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	a.resizeAll()
	if on {
		a.flash(i18n.T("Jeder Tab hat jetzt einen Rahmen in seiner Farbe"))
	} else {
		a.flash(i18n.T("Rahmen um die Tabs aus"))
	}
	return nil
}

// framed: Terminals werden gerade umrahmt (Einstellung an, Fenster groß genug).
func (a *App) framed() bool { return a.tabFramesOn() && a.w >= 40 && a.h >= 12 }

// tabColorID: Farb-ID des Tabs ("" = keine Farbe).
func (a *App) tabColorID(s *session) string {
	if s == nil {
		return ""
	}
	if !a.tabFramesOn() || s.conn.Color == accentNone {
		return connAccent(s.conn)
	}
	if id := connAccent(s.conn); id != "" {
		return id
	}
	if s.autoCol == "" {
		s.autoCol = a.pickAutoColor(s)
	}
	return s.autoCol
}

// tabColor: Farbe des Tabs im aktuellen Theme (nil = keine).
func (a *App) tabColor(s *session) color.Color {
	return accentColor(a.s.T, a.tabColorID(s))
}

// pickAutoColor: dieselbe Verbindung bekommt möglichst dieselbe Farbe
// (Streuwert aus ID/Alias), aber keine, die ein anderer offener Tab schon hat.
func (a *App) pickAutoColor(s *session) string {
	used := map[string]bool{}
	for _, x := range a.sessions {
		if x == s {
			continue
		}
		if id := connAccent(x.conn); id != "" {
			used[id] = true
		} else if x.autoCol != "" {
			used[x.autoCol] = true
		}
	}
	return autoColorFor(s.conn, used)
}

func autoColorFor(c domain.Connection, used map[string]bool) string {
	h := fnv.New32a()
	key := c.ID
	if key == "" {
		key = c.Alias + "\x00" + c.Host
	}
	_, _ = h.Write([]byte(key))
	n := len(autoTabColors)
	start := int(h.Sum32() % uint32(n))
	for i := 0; i < n; i++ {
		if id := autoTabColors[(start+i)%n]; !used[id] {
			return id
		}
	}
	return autoTabColors[start]
}

// frameColor: Rahmenfarbe; nicht aktive Bereiche etwas gedämpft.
func (a *App) frameColor(s *session, focused bool) color.Color {
	t := a.s.T
	col := a.tabColor(s)
	if col == nil {
		if focused {
			return t.Accent
		}
		return t.Border
	}
	if !focused {
		return mixColor(t.Bg, col, 0.6)
	}
	return col
}

// frameGlyphs: links/rechts, waagerecht, Ecken oben links/rechts, unten links/rechts.
// Der aktive Bereich hat dicke Linien (Form statt nur Farbe).
func frameGlyphs(focused bool) (v, h, tl, tr, bl, br string) {
	switch {
	case focused:
		return "┃", "━", "┏", "┓", "┗", "┛"
	case framesSetting == framesRund:
		return "│", "─", "╭", "╮", "╰", "╯"
	}
	return "│", "─", "┌", "┐", "└", "┘"
}

// frameBody umrahmt body: w×h ist die Größe samt Rahmen. top = mit oberer
// Kante (sonst übernimmt die Titelzeile darüber diese Rolle); label steht
// dann links in der oberen Kante.
func (a *App) frameBody(body string, w, h int, s *session, focused, top bool, label string) string {
	col := a.frameColor(s, focused)
	ln := lipgloss.NewStyle().Foreground(col)
	v, hz, tl, tr, bl, br := frameGlyphs(focused)
	ih := h - 1
	if top {
		ih--
	}
	iw := max(0, w-2)
	inner := strings.Split(fitBlock(body, iw, max(0, ih)), "\n")
	side := ln.Render(v)
	var b strings.Builder
	if top {
		lbl := ""
		if label != "" {
			lbl = ln.Bold(focused).Render(" " + ansi.Truncate(label, max(0, iw-4), "…") + " ")
		}
		rest := max(0, iw-1-ansi.StringWidth(lbl))
		b.WriteString(ln.Render(tl+hz) + lbl + ln.Render(strings.Repeat(hz, rest)+tr))
		b.WriteByte('\n')
	}
	for _, l := range inner {
		if ih <= 0 {
			break
		}
		b.WriteString(side + l + side + "\n")
	}
	b.WriteString(ln.Render(bl + strings.Repeat(hz, iw) + br))
	return b.String()
}
