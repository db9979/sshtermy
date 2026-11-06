package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Auswahlliste mit Suche (Jump-Host, Ordner, Tag-Filter). Liegt als Overlay
// über dem aktuellen Bildschirm; Tippen filtert, ↑↓ wählt, ⏎ übernimmt.
// Gesperrte Einträge (z. B. ein Jump-Host, der einen Kreis bilden würde)
// stehen grau in der Liste und nennen beim Auswählen den Grund.

type pickItem struct {
	value    string
	label    string
	detail   string // grau hinter dem Namen
	hint     string // rechts (Status)
	search   string // zusätzlicher Suchtext
	disabled string // Grund, warum nicht wählbar ("" = wählbar)
	indent   int
}

type picker struct {
	title   string
	help    string
	in      textinput.Model
	items   []pickItem
	sel     int
	top     int
	current string // aktuell gesetzter Wert (Markierung ✓)
	onPick  func(it pickItem) tea.Cmd
	// extra: zusätzlicher Eintrag aus dem Suchtext (z. B. „Neuer Ordner …“)
	extra func(q string) *pickItem
}

func newPicker(title, placeholder string, items []pickItem, current string, onPick func(pickItem) tea.Cmd) *picker {
	in := newInput("", false)
	in.Placeholder = placeholder
	p := &picker{title: title, in: in, items: items, current: current, onPick: onPick}
	for i, it := range p.visible() {
		if it.value == current {
			p.sel = i
		}
	}
	return p
}

func (p *picker) visible() []pickItem {
	q := strings.ToLower(strings.TrimSpace(p.in.Value()))
	var out []pickItem
	if p.extra != nil {
		if it := p.extra(strings.TrimSpace(p.in.Value())); it != nil {
			out = append(out, *it)
		}
	}
	words := strings.Fields(q)
	for _, it := range p.items {
		hay := strings.ToLower(it.label + " " + it.detail + " " + it.hint + " " + it.search)
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, it)
		}
	}
	return out
}

func (a *App) openPicker(p *picker) tea.Cmd {
	a.pick = p
	return p.in.Focus()
}

func (a *App) pickerChoose(it pickItem) tea.Cmd {
	if it.disabled != "" {
		a.flash(strings.TrimSpace(it.label) + ": " + it.disabled)
		return nil
	}
	p := a.pick
	a.pick = nil
	if p.onPick != nil {
		return p.onPick(it)
	}
	return nil
}

func (a *App) pickerKey(k tea.KeyPressMsg) tea.Cmd {
	p := a.pick
	vis := p.visible()
	switch k.String() {
	case "esc":
		a.pick = nil
		return nil
	case "up", "ctrl+p":
		p.sel = max(0, p.sel-1)
		return nil
	case "down", "ctrl+n", "tab":
		p.sel = min(len(vis)-1, p.sel+1)
		return nil
	case "pgup":
		p.sel = max(0, p.sel-p.rows(a.h))
		return nil
	case "pgdown":
		p.sel = min(len(vis)-1, p.sel+p.rows(a.h))
		return nil
	case "enter":
		if p.sel >= 0 && p.sel < len(vis) {
			return a.pickerChoose(vis[p.sel])
		}
		return nil
	}
	var cmd tea.Cmd
	before := p.in.Value()
	p.in, cmd = p.in.Update(k)
	if p.in.Value() != before {
		p.sel, p.top = 0, 0
	}
	return cmd
}

func (p *picker) rows(h int) int { return max(3, min(16, h-12)) }

func (a *App) viewPicker() string {
	p := a.pick
	s := a.s
	w := min(96, a.w-6)
	vis := p.visible()
	p.sel = clamp(p.sel, 0, max(0, len(vis)-1))
	rows := p.rows(a.h)
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+rows {
		p.top = p.sel - rows + 1
	}
	p.top = clamp(p.top, 0, max(0, len(vis)-rows))

	var b strings.Builder
	p.in.SetWidth(w - 8)
	b.WriteString(s.Accent.Render("/ ") + p.in.View() + "\n\n")
	var list strings.Builder
	for i := p.top; i < len(vis) && i < p.top+rows; i++ {
		it := vis[i]
		mark := "  "
		if it.value == p.current && p.current != "\x00" {
			mark = s.OK.Render("✓ ")
		}
		name := strings.Repeat("  ", it.indent) + it.label
		hint := it.hint
		left := mark + name
		if it.detail != "" {
			left += "  " + s.Muted.Render(it.detail)
		}
		if it.disabled != "" {
			left = mark + s.Muted.Render(name+"  "+it.detail)
			hint = "⊘ " + trunc(it.disabled, 34)
		}
		room := w - 6 - lipgloss.Width(hint) - 1
		left = ansi.Truncate(left, max(10, room), "…")
		line := left + strings.Repeat(" ", max(1, room-lipgloss.Width(left)+1)) + s.Muted.Render(hint)
		if i == p.sel {
			line = s.Selected.Width(w - 4).Render(ansi.Strip(line))
		}
		i, it := i, it
		line = a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
			p.sel = i
			return a.pickerChoose(it)
		})
		list.WriteString(line + "\n")
	}
	if len(vis) == 0 {
		list.WriteString(s.Muted.Render(i18n.T("  nichts gefunden · Esc zurück, Rücktaste ändert die Suche")) + "\n")
	}
	b.WriteString(a.z.mark(list.String(), zoneAction{wheel: func(up bool) tea.Cmd {
		if up {
			p.sel = max(0, p.sel-1)
		} else {
			p.sel = min(len(vis)-1, p.sel+1)
		}
		return nil
	}, block: true}))
	more := ""
	if len(vis) > rows {
		more = strconv.Itoa(p.top+1) + "–" + strconv.Itoa(min(len(vis), p.top+rows)) + i18n.T(" von ") + strconv.Itoa(len(vis))
	}
	keys := [][2]string{{"⏎", i18n.T("übernehmen")}, {"Esc", i18n.T("abbrechen")}, {"", i18n.T("tippen filtert")}}
	if p.help != "" {
		keys = append(keys, [2]string{"", p.help})
	}
	return s.dialog(dialog{title: p.title, sub: more, body: strings.TrimRight(b.String(), "\n"), keys: keys, width: w - 6, maxW: 100}, a.w)
}

// ---------------------------------------------------------------- Texteingabe

// inputModal: eine Zeile Text abfragen (Ordnername).
func inputModal(title, body, val string, submit func(string) tea.Cmd) *modal {
	in := newInput(val, false)
	in.SetWidth(50)
	in.Placeholder = "Name"
	in.CursorEnd()
	in.Focus()
	return &modal{kind: mInput, title: title, body: body, in: in, onText: submit}
}

func (a *App) inputKey(k tea.KeyPressMsg) tea.Cmd {
	m := a.modal
	switch k.String() {
	case "esc":
		a.modal = nil
		return nil
	case "enter":
		a.modal = nil
		if m.onText != nil {
			return m.onText(strings.TrimSpace(m.in.Value()))
		}
		return nil
	}
	var cmd tea.Cmd
	m.in, cmd = m.in.Update(k)
	return cmd
}

var _ = textinput.EchoNormal
