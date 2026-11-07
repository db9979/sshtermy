package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Auswahl der Aufteilung (Strg+T)
//
// Kacheln mit einer kleinen Skizze je Aufteilung, Tasten 1 … 7 oder Pfeile
// und ⏎. Dazu die Schalter rund um die geteilte Ansicht: ⇧+Pfeil ändert die
// Größe des aktiven Bereichs gleich hier (falls das Terminal Alt+⇧+Pfeil
// selbst belegt), „=“ macht alle gleich groß, z zoomt, d dunkelt die
// anderen Bereiche ab, w öffnet die Arbeitsplätze.

// layoutArt: Skizze je Aufteilung (gleich breit, vier Zeilen).
var layoutArt = map[splitMode][]string{
	splitOff:   {"┌────────┐", "│        │", "│        │", "└────────┘"},
	splitCols:  {"┌───┬────┐", "│   │    │", "│   │    │", "└───┴────┘"},
	splitRows:  {"┌────────┐", "├────────┤", "│        │", "└────────┘"},
	splitMain:  {"┌────┬───┐", "│    ├───┤", "│    │   │", "└────┴───┘"},
	splitCols3: {"┌──┬──┬──┐", "│  │  │  │", "│  │  │  │", "└──┴──┴──┘"},
	splitGrid:  {"┌───┬────┐", "├───┬────┤", "│   │    │", "└───┴────┘"},
	splitGrid6: {"┌──┬──┬──┐", "├──┬──┬──┤", "│  │  │  │", "└──┴──┴──┘"},
}

func (a *App) openLayout() tea.Cmd {
	sel := 0
	for i, m := range layoutOrder {
		if m == a.split {
			sel = i
		}
	}
	a.modal = &modal{kind: mLayout, title: i18n.T("Aufteilung"), sel: sel}
	return nil
}

// applyLayout wählt Aufteilung i und schließt die Auswahl.
func (a *App) applyLayout(i int) {
	if i < 0 || i >= len(layoutOrder) {
		return
	}
	m := layoutOrder[i]
	if !a.fits(m) {
		a.modal.sel = i
		a.flash(i18n.T("Fenster zu klein für „") + m.label() + "“")
		return
	}
	a.modal = nil
	if m != a.split {
		a.setSplit(m)
	}
}

func (a *App) layoutKey(key string) tea.Cmd {
	m := a.modal
	n := len(layoutOrder)
	switch key {
	case "esc", "q":
		a.modal = nil
	case "enter", "space":
		a.applyLayout(m.sel)
	case "left", "h":
		m.sel = (m.sel - 1 + n) % n
	case "right", "l", "tab":
		m.sel = (m.sel + 1) % n
	case "up", "k":
		m.sel = max(0, m.sel-4)
	case "down", "j":
		m.sel = min(n-1, m.sel+4)
	case "shift+left", "shift+right", "shift+up", "shift+down":
		a.resizeFocused(strings.TrimPrefix(key, "shift+"))
	case "=", "0":
		if a.split != splitOff {
			a.equalize()
		}
	case "z":
		a.modal = nil
		a.toggleZoom()
	case "d":
		return a.toggleDimPanes()
	case "w":
		a.modal = nil
		return a.openWorkspaces()
	default:
		if a.keys.is(actSplit, key) {
			a.modal = nil
			return nil
		}
		if len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < n {
			a.applyLayout(int(key[0] - '1'))
		}
	}
	return nil
}

func (a *App) layoutView() string {
	st := a.s
	m := a.modal
	tileW := 18
	var cells []string
	for i, mode := range layoutOrder {
		i, mode := i, mode
		c := st.Muted
		switch {
		case !a.fits(mode):
			c = st.Muted.Faint(true)
		case i == m.sel:
			c = st.Accent
		case mode == a.split:
			c = st.Text
		}
		name := map[splitMode]string{splitOff: i18n.T("eine"), splitCols: i18n.T("nebeneinander"), splitRows: i18n.T("untereinander"),
			splitMain: i18n.T("groß + zwei"), splitCols3: i18n.T("drei"), splitGrid: i18n.T("vier"), splitGrid6: i18n.T("sechs")}[mode]
		if mode == a.split {
			name = "✓ " + name
		}
		art := c.Render(strings.Join(layoutArt[mode], "\n"))
		lbl := st.Accent.Render(strconv.Itoa(i+1)) + " " + c.Render(trunc(name, tileW-3))
		box := lipgloss.NewStyle().Width(tileW).Align(lipgloss.Center).Render(art + "\n" + lbl)
		if i == m.sel {
			box = lipgloss.NewStyle().Background(st.T.Surface).Render(box)
		}
		cells = append(cells, a.z.mark(box, zoneAction{block: true, click: func(tea.Mouse, bool) tea.Cmd {
			a.applyLayout(i)
			return nil
		}}))
	}
	perRow := 4
	if dialogInner(a.w) < perRow*tileW {
		perRow = max(1, dialogInner(a.w)/tileW)
	}
	var rows []string
	for i := 0; i < len(cells); i += perRow {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells[i:min(len(cells), i+perRow)]...))
	}
	body := strings.Join(rows, "\n\n")
	info := ""
	if a.split != splitOff {
		dim := i18n.T("aus")
		if a.dimPanes() {
			dim = i18n.T("an")
		}
		info = i18n.T("Andere Bereiche abdunkeln: ") + dim
	}
	keys := [][2]string{{"1…7", i18n.T("wählen")}}
	if a.split != splitOff {
		keys = append(keys, [2]string{"⇧←→↑↓", i18n.T("Größe")}, [2]string{"=", i18n.T("gleich groß")}, [2]string{"z", i18n.T("Zoom")}, [2]string{"d", i18n.T("abdunkeln")})
	}
	keys = append(keys, [2]string{"w", i18n.T("Arbeitsplätze")}, [2]string{"Esc", i18n.T("schließen")})
	return st.dialog(dialog{icon: "▦", title: i18n.T("Aufteilung"), sub: a.keys.label(actSplit), body: body, info: info, keys: keys}, a.w)
}

func (a *App) dimPanes() bool { return a.st != nil && a.st.Settings().DimPanes }

func (a *App) toggleDimPanes() tea.Cmd {
	on := !a.dimPanes()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.DimPanes = on }); err != nil {
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Nicht aktive Bereiche werden abgedunkelt"))
	} else {
		a.flash(i18n.T("Alle Bereiche in voller Farbe"))
	}
	return nil
}
