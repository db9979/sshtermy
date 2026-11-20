package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Panel andocken: Das Seitenpanel (Ziel, KI fragen, Begleiter, Tunnel, F6)
// steht rechts (Standard), links oder unten. Breite bzw. Höhe zieht man mit
// der Maus am Rand des Panels; sshTermy merkt sie sich (je Rechner). Unten
// geht nur, wenn genug Zeilen da sind, sonst rechts. In schmalen Fenstern
// klappt das Panel wie bisher rechts zu einer Leiste ein.

const (
	dockRight  = ""
	dockLeft   = "left"
	dockBottom = "bottom"

	dockBottomMinH = 18 // darunter steht ein Panel unten nicht sinnvoll
	panelMinW      = 30
	panelMinH      = 6
)

var dockOrder = []string{dockRight, dockLeft, dockBottom}

func dockLabel(d string) string {
	switch d {
	case dockLeft:
		return i18n.T("links")
	case dockBottom:
		return i18n.T("unten")
	}
	return i18n.T("rechts")
}

func (a *App) dockSetting() string {
	if a.st == nil {
		return dockRight
	}
	return a.st.Settings().PanelDock
}

// panelDock: wo das Panel dieser Sitzung gerade steht.
func (a *App) panelDock(s *session) string {
	d := a.dockSetting()
	if s == nil || s.panel == panelNone || a.panelCollapsed(s) {
		return dockRight
	}
	if d == dockBottom && a.termRect(s).h < dockBottomMinH {
		return dockRight
	}
	if s.panel == panelHost && s.host.live.wide {
		return dockRight // F6 groß liegt ohnehin über allem
	}
	return d
}

// panelHeight: Zeilen des Panels unten.
func (a *App) panelHeight(s *session) int {
	h := a.termRect(s).h
	if _, n := a.panelSizes(); n > 0 {
		return clamp(n, panelMinH, max(panelMinH, h-5))
	}
	return clamp(h*2/5, 8, max(8, h-5))
}

// termBox: Fläche des Terminals ohne Panel.
func (a *App) termBox(s *session) rect {
	r := a.termRect(s)
	if a.filesBeside(s) || a.logBeside(s) {
		return rect{r.x, r.y, r.w - a.filesWidth(s), r.h}
	}
	if s == nil || s.panel == panelNone {
		return r
	}
	switch a.panelDock(s) {
	case dockLeft:
		pw := a.panelWidth(s)
		return rect{r.x + pw, r.y, max(20, r.w-pw), r.h}
	case dockBottom:
		return rect{r.x, r.y, r.w, max(3, r.h-a.panelHeight(s))}
	}
	return rect{r.x, r.y, max(20, r.w-a.panelWidth(s)), r.h}
}

// panelBox: Fläche, die das Panel gerade belegt (aufgeklappt mitgerechnet).
func (a *App) panelBox(s *session) rect {
	r := a.termRect(s)
	if s == nil || s.panel == panelNone {
		return rect{}
	}
	switch a.panelDock(s) {
	case dockLeft:
		return rect{r.x, r.y, a.panelWidth(s), r.h}
	case dockBottom:
		ph := a.panelHeight(s)
		return rect{r.x, r.y + r.h - ph, r.w, ph}
	}
	pw := a.panelShown(s)
	return rect{r.x + r.w - pw, r.y, pw, r.h}
}

// dockCompose: Terminal und Panel zusammensetzen (links oder unten).
func (a *App) dockCompose(s *session, body string) string {
	pb := a.panelBox(s)
	switch a.panelDock(s) {
	case dockLeft:
		return beside(a.viewPanel(s, pb.w, pb.h), pb.w, body) // perf.go
	case dockBottom:
		tb := a.termBox(s)
		lines := strings.Split(body, "\n")
		if len(lines) > tb.h {
			lines = lines[:tb.h]
		}
		for len(lines) < tb.h {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n") + "\n" + a.viewPanel(s, pb.w, pb.h)
	}
	return beside(body, a.termBox(s).w, a.viewPanel(s, pb.w, pb.h))
}

func (a *App) cycleDock(dir int) tea.Cmd {
	next := dockOrder[cycleIndex(dockIndex(a.dockSetting()), len(dockOrder), dir)]
	_ = a.st.UpdateSettings(func(x *domain.Settings) { x.PanelDock = next })
	a.resizeAll()
	a.flash(i18n.T("Panel steht jetzt ") + dockLabel(next))
	return nil
}

// ---------------------------------------------------------------- Größe ziehen

// panelEdge: liegt (x, y) auf dem Rand des Panels zum Terminal?
func (a *App) panelEdge(s *session, x, y int) bool {
	if s == nil || s.panel == panelNone || a.panelCollapsed(s) || s.files.open {
		return false
	}
	pb := a.panelBox(s)
	if pb.w <= 0 || !a.termRect(s).has(x, y) {
		return false
	}
	switch a.panelDock(s) {
	case dockLeft:
		return x == pb.x+pb.w-1 && y >= pb.y && y < pb.y+pb.h
	case dockBottom:
		return y == pb.y && x >= pb.x && x < pb.x+pb.w
	}
	return x == pb.x && y >= pb.y && y < pb.y+pb.h
}

// panelDragMouse: Panelrand mit der Maus ziehen. ok = Ereignis verbraucht.
func (a *App) panelDragMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	m := msg.Mouse()
	s := a.current()
	if a.scr != scrTerminal || s == nil || a.modal != nil || a.pal != nil || a.pick != nil {
		a.panelDrag = false
		return nil, false
	}
	_, isClick := msg.(tea.MouseClickMsg)
	_, isRelease := msg.(tea.MouseReleaseMsg)
	_, isMotion := msg.(tea.MouseMotionMsg)
	if !a.panelDrag {
		if isClick && m.Button == tea.MouseLeft && a.panelEdge(s, m.X, m.Y) {
			a.panelDrag = true
			return nil, true
		}
		return nil, false
	}
	switch {
	case isMotion:
		r := a.termRect(s)
		switch a.panelDock(s) {
		case dockLeft:
			a.setPanelSize(clamp(m.X-r.x+1, panelMinW, max(panelMinW, r.w-20)), 0)
		case dockBottom:
			a.setPanelSize(0, clamp(r.y+r.h-m.Y, panelMinH, max(panelMinH, r.h-5)))
		default:
			a.setPanelSize(clamp(r.x+r.w-m.X, panelMinW, max(panelMinW, r.w-20)), 0)
		}
		return nil, true
	case isRelease:
		a.panelDrag = false
		_ = a.st.UpdateSettings(func(x *domain.Settings) { x.PanelW, x.PanelH = a.dragW, a.dragH })
		return nil, true
	}
	return nil, true
}

// setPanelSize: während des Ziehens nur im Speicher, gespeichert beim Loslassen.
func (a *App) setPanelSize(w, h int) {
	a.dragW, a.dragH = a.panelSizes()
	if w > 0 {
		a.dragW = w
	}
	if h > 0 {
		a.dragH = h
	}
	a.resizeAll()
}

// resetPanelSize: Breite und Höhe wieder automatisch.
func (a *App) resetPanelSize() tea.Cmd {
	_ = a.st.UpdateSettings(func(x *domain.Settings) { x.PanelW, x.PanelH = 0, 0 })
	a.resizeAll()
	a.flash(i18n.T("Panelgröße wieder automatisch"))
	return nil
}

// panelSizes: gewünschte Breite/Höhe (0 = automatisch); beim Ziehen die
// gerade gezogene, gespeichert wird erst beim Loslassen.
func (a *App) panelSizes() (w, h int) {
	if a.panelDrag {
		return a.dragW, a.dragH
	}
	if a.st == nil {
		return 0, 0
	}
	st := a.st.Settings()
	return st.PanelW, st.PanelH
}

func dockIndex(d string) int {
	for i, x := range dockOrder {
		if x == d {
			return i
		}
	}
	return 0
}

// ---------------------------------------------------------------- F5 daneben

// filesBesideMinW: so breit muss die Fläche sein, damit F5 neben dem
// Terminal steht (sonst wie bisher über dem Terminal).
const filesBesideMinW = 140

func (s *session) filesFocus() bool { return s.files.open && !s.files.blur }

func (a *App) filesBesideOn() bool { return a.st != nil && a.st.Settings().FilesBeside }

// filesBeside: F5 steht gerade neben dem Terminal.
func (a *App) filesBeside(s *session) bool {
	return s != nil && s.files.open && a.filesBesideOn() && a.termRect(s).w >= filesBesideMinW && !s.detached()
}

func (a *App) filesWidth(s *session) int {
	w := a.termRect(s).w
	return clamp(w*11/20, 70, w-60)
}

// filesRect: Fläche der Dateiansicht.
func (a *App) filesRect(s *session) rect {
	r := a.termRect(s)
	if a.filesBeside(s) {
		fw := a.filesWidth(s)
		return rect{r.x + r.w - fw, r.y, fw, r.h}
	}
	return r
}

func (a *App) toggleFilesBeside() tea.Cmd {
	on := !a.filesBesideOn()
	_ = a.st.UpdateSettings(func(x *domain.Settings) { x.FilesBeside = on })
	a.resizeAll()
	if on {
		a.flash(i18n.T("F5 und Log-Blick stehen in breiten Fenstern neben dem Terminal"))
	} else {
		a.flash(i18n.T("F5 und Log-Blick füllen wieder die ganze Fläche"))
	}
	return nil
}

// fitCurrent: Terminalgröße nachziehen, wenn F5 oder der Log-Blick daneben
// auf- oder zugeht (resize tut nichts, wenn die Größe stimmt).
func (a *App) fitCurrent() {
	if s := a.current(); s != nil {
		a.resize(s)
	}
}

// logBeside: Log-Blick steht neben dem Terminal der Sitzung (aus dem
// Terminal geöffnet, breites Fenster, keine geteilte Ansicht).
func (a *App) logBeside(s *session) bool {
	return s != nil && a.scr == scrLogScan && a.lscan.back == scrTerminal && a.lscan.sid == s.id &&
		a.filesBesideOn() && a.split == splitOff && a.termRect(s).w >= filesBesideMinW && !s.detached()
}

// logPaneView: Log-Blick in w×h (für daneben).
func (a *App) logPaneView(w, h int) string {
	ow, oh := a.w, a.h
	a.w, a.h = w+2, h+4 // viewLogScan rechnet mit dem ganzen Fenster
	out := a.viewLogScan()
	a.w, a.h = ow, oh
	lines := strings.Split(out, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		l = ansi.Truncate(l, w-2, "")
		lines[i] = "│ " + l + strings.Repeat(" ", max(0, w-2-ansi.StringWidth(l)))
	}
	for len(lines) < h {
		lines = append(lines, "│"+strings.Repeat(" ", w-1))
	}
	return strings.Join(lines, "\n")
}
