package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// fieldKind: Text, Geheimnis, Zahl, Auswahl oder Schalter.
type fieldKind int

const (
	fText fieldKind = iota
	fSecret
	fNumber
	fChoice
	fBool
	fPick   // Auswahl über eine durchsuchbare Liste (picker.go)
	fHeader // Abschnittsüberschrift; mit fold einklappbar und fokussierbar
)

type field struct {
	key, label, help string
	kind             fieldKind
	in               textinput.Model
	options          []string // fChoice
	opt              int
	on               bool // fBool
	hidden           func(f *form) bool

	// fPick: gewählter Wert, Anzeige und Öffnen der Auswahlliste
	value   string
	display func(value string) string
	open    func() tea.Cmd

	// fHeader: fold = einklappbar (dann fokussierbar), on = aufgeklappt,
	// summary zeigt im zugeklappten Zustand, was darin eingestellt ist.
	fold    bool
	summary func(f *form) string
	tint    func(t Theme) color.Color // Abschnittsfarbe (fHeader)

	// swatch (fChoice, optional): Farbpunkt vor der Option, z. B. bei „Farbe“
	swatch func(t Theme, opt string) color.Color
}

// form ist ein kleines, tastaturgesteuertes Formular (Tab/↑↓ wechseln,
// ←/→ ändern Auswahl und Schalter, Strg+S oder Enter im letzten Feld speichert).
type form struct {
	title  string
	fields []*field
	focus  int
	err    string
	minH   int // höchste bisher gezeigte Höhe: das Formular schrumpft nicht (kein Springen)
}

func newInput(val string, secret bool) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(val)
	ti.CharLimit = 4096
	ti.SetWidth(40)
	ti.SetStyles(inputStyles(activeTheme))
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	return ti
}

func (f *form) add(fl *field) *field { f.fields = append(f.fields, fl); return fl }

func (f *form) text(key, label, val, help string) *field {
	return f.add(&field{key: key, label: label, help: help, kind: fText, in: newInput(val, false)})
}
func (f *form) secret(key, label, val, help string) *field {
	return f.add(&field{key: key, label: label, help: help, kind: fSecret, in: newInput(val, true)})
}
func (f *form) number(key, label string, val int, help string) *field {
	s := ""
	if val != 0 {
		s = strconv.Itoa(val)
	}
	return f.add(&field{key: key, label: label, help: help, kind: fNumber, in: newInput(s, false)})
}
func (f *form) choice(key, label string, opts []string, cur string, help string) *field {
	fl := &field{key: key, label: label, help: help, kind: fChoice, options: opts}
	for i, o := range opts {
		if o == cur {
			fl.opt = i
		}
	}
	return f.add(fl)
}
func (f *form) pick(key, label, val string, display func(string) string, open func() tea.Cmd, help string) *field {
	return f.add(&field{key: key, label: label, help: help, kind: fPick, value: val, display: display, open: open})
}
func (f *form) toggle(key, label string, on bool, help string) *field {
	return f.add(&field{key: key, label: label, help: help, kind: fBool, on: on})
}

// section beginnt einen Abschnitt mit Überschrift. Felder darunter gehören
// dazu, bis der nächste Abschnitt beginnt.
func (f *form) section(key, label string, tint func(Theme) color.Color) *field {
	return f.add(&field{key: key, label: label, kind: fHeader, tint: tint})
}

// folded beginnt einen einklappbaren Abschnitt (zu, solange open false).
func (f *form) folded(key, label string, open bool, summary func(f *form) string, help string) *field {
	return f.add(&field{key: key, label: label, kind: fHeader, fold: true, on: open, summary: summary, help: help})
}

func (fl *field) focusable() bool { return fl.kind != fHeader || fl.fold }

func (f *form) get(key string) *field {
	for _, fl := range f.fields {
		if fl.key == key {
			return fl
		}
	}
	return &field{}
}
func (f *form) str(key string) string { return strings.TrimSpace(f.get(key).in.Value()) }
func (f *form) raw(key string) string { return f.get(key).in.Value() }
func (f *form) num(key string) int    { n, _ := strconv.Atoi(f.str(key)); return n }
func (f *form) sel(key string) string {
	fl := f.get(key)
	if len(fl.options) == 0 {
		return ""
	}
	return fl.options[fl.opt]
}
func (f *form) flag(key string) bool  { return f.get(key).on }
func (f *form) val(key string) string { return f.get(key).value }

// visible: sichtbare Felder in Reihenfolge. Felder in einem zugeklappten
// Abschnitt sind unsichtbar; eine Überschrift erscheint nur, wenn ihr
// Abschnitt (aufgeklappt) mindestens ein sichtbares Feld hätte.
func (f *form) visible() []int {
	var out []int
	head, headUsed := -1, false
	for i, fl := range f.fields {
		if fl.kind == fHeader {
			head, headUsed = i, false
			continue
		}
		if fl.hidden != nil && fl.hidden(f) {
			continue
		}
		if head >= 0 && !headUsed {
			out = append(out, head)
			headUsed = true
		}
		if head >= 0 && f.fields[head].fold && !f.fields[head].on {
			continue
		}
		out = append(out, i)
	}
	return out
}

func (f *form) focusables() []int {
	var out []int
	for _, i := range f.visible() {
		if f.fields[i].focusable() {
			out = append(out, i)
		}
	}
	return out
}

func (f *form) move(d int) tea.Cmd {
	vis := f.focusables()
	pos := 0
	for i, v := range vis {
		if v == f.focus {
			pos = i
		}
	}
	pos = (pos + d + len(vis)) % len(vis)
	return f.setFocus(vis[pos])
}

func (f *form) setFocus(i int) tea.Cmd {
	// Feste Überschriften bekommen keinen Fokus: zum nächsten Feld weiter.
	for i < len(f.fields)-1 && !f.fields[i].focusable() {
		i++
	}
	for j, fl := range f.fields {
		if j != i {
			fl.in.Blur()
		}
	}
	f.focus = i
	fl := f.fields[i]
	if fl.kind == fText || fl.kind == fSecret || fl.kind == fNumber {
		return fl.in.Focus()
	}
	return nil
}

func (f *form) isLast() bool {
	vis := f.focusables()
	return len(vis) > 0 && vis[len(vis)-1] == f.focus
}

// update: submit = true, wenn gespeichert werden soll.
func (f *form) update(msg tea.Msg) (submit bool, cmd tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if ok {
		fl := f.fields[f.focus]
		switch k.String() {
		case "tab", "down":
			return false, f.move(1)
		case "shift+tab", "up":
			return false, f.move(-1)
		case "ctrl+s":
			return true, nil
		case "enter", "space", "right", "left":
			if fl.kind == fPick && fl.open != nil {
				return false, fl.open()
			}
			if fl.kind == fHeader {
				fl.on = k.String() == "right" || (k.String() != "left" && !fl.on)
				return false, nil
			}
		}
		switch k.String() {
		case "enter":
			if f.isLast() {
				return true, nil
			}
			return false, f.move(1)
		case "left", "right", "space":
			if fl.kind == fChoice {
				d := 1
				if k.String() == "left" {
					d = -1
				}
				fl.opt = (fl.opt + d + len(fl.options)) % len(fl.options)
				return false, nil
			}
			if fl.kind == fBool {
				fl.on = !fl.on
				return false, nil
			}
		}
		if fl.kind == fNumber && len(k.Text) == 1 && (k.Text[0] < '0' || k.Text[0] > '9') {
			return false, nil
		}
	}
	fl := f.fields[f.focus]
	if fl.kind == fText || fl.kind == fSecret || fl.kind == fNumber {
		fl.in, cmd = fl.in.Update(msg)
	}
	return false, cmd
}

// z (optional) macht Felder anklickbar: Klick setzt den Fokus, wählt eine
// Option oder schaltet einen Schalter um.
//
// Ruhig gehalten: Auswahlfelder zeigen ohne Fokus nur den gewählten Wert,
// die Hilfe steht in einer festen Zeile unten (statt sich unter das Feld
// zu schieben) und das Formular wird nie wieder kleiner, solange es offen ist.
func (f *form) view(s Styles, width int, z *zones) string {
	var b strings.Builder
	lines := 0
	put := func(str string) { b.WriteString(str + "\n"); lines++ }
	if f.title != "" {
		put(s.Accent.Render(f.title))
	}
	const labelW = 22
	inW := max(20, width-labelW-4)
	first := true
	var tint color.Color // Farbe des aktuellen Abschnitts (nil = ohne)
	for _, i := range f.visible() {
		fl := f.fields[i]
		focused := i == f.focus
		if fl.kind == fHeader {
			if !first || f.title != "" {
				put("")
			}
			first = false
			tint = s.T.Muted
			if fl.tint != nil {
				tint = fl.tint(s.T)
			}
			put(f.headerRow(s, i, fl, focused, tint, width, z))
			continue
		}
		first = false
		label := s.Label.Width(labelW).Render(fl.label)
		var val string
		switch fl.kind {
		case fChoice:
			val = f.choiceView(s, i, fl, focused, tint, inW, z)
		case fPick:
			v := fl.value
			if fl.display != nil {
				v = fl.display(fl.value)
			}
			val = " " + ansi.Truncate(v, max(10, inW-3), "…") + s.Muted.Render(" ▾")
		case fBool:
			if fl.on {
				val = " " + s.OK.Render(i18n.T("● an"))
			} else {
				val = " " + s.Muted.Render(i18n.T("○ aus"))
			}
		default:
			fl.in.SetWidth(inW - 1)
			val = " " + fl.in.View()
		}
		marker := "  "
		if tint != nil {
			marker = lipgloss.NewStyle().Foreground(tint).Faint(true).Render("│ ")
		}
		if focused {
			fc := s.T.Accent
			if tint != nil {
				fc = tint
			}
			marker = lipgloss.NewStyle().Foreground(fc).Render("▌ ")
			label = lipgloss.NewStyle().Foreground(s.T.Text).Bold(true).Width(labelW).Render(fl.label)
		}
		row := marker + label + val
		if z != nil {
			i, fl := i, fl
			row = z.click(row, func(_ tea.Mouse, _ bool) tea.Cmd {
				if fl.kind == fBool {
					fl.on = !fl.on
				}
				if fl.kind == fPick && fl.open != nil {
					f.setFocus(i)
					return fl.open()
				}
				return f.setFocus(i)
			})
		}
		put(row)
	}
	// Feste Hilfezone: immer zwei Zeilen, damit nichts nachrutscht.
	help := ""
	if f.focus < len(f.fields) {
		help = f.fields[f.focus].help
	}
	hl := strings.Split(lipgloss.NewStyle().Width(max(20, width-2)).Render(help), "\n")
	for len(hl) < 2 {
		hl = append(hl, "")
	}
	put("")
	put(s.Muted.Render(strings.Repeat("─", max(10, width))))
	for _, l := range hl[:2] {
		put(s.Muted.Render(strings.TrimRight(l, " ")))
	}
	if f.err != "" {
		put("")
		put(s.Danger.Render("× " + f.err))
	}
	for lines < f.minH {
		put("")
	}
	f.minH = lines
	return b.String()
}

// choiceView: mit Fokus alle Optionen (passt es nicht, ‹ Wert › zum
// Durchschalten), ohne Fokus nur der gewählte Wert.
func (f *form) choiceView(s Styles, i int, fl *field, focused bool, tint color.Color, inW int, z *zones) string {
	if fl.opt >= len(fl.options) {
		return ""
	}
	cur := i18n.T(fl.options[fl.opt])
	if !focused {
		if fl.swatch != nil {
			if c := fl.swatch(s.T, fl.options[fl.opt]); c != nil {
				return " " + lipgloss.NewStyle().Foreground(c).Render("● "+cur)
			}
		}
		if tint == nil {
			tint = s.T.Text
		}
		return " " + lipgloss.NewStyle().Foreground(tint).Render(cur)
	}
	parts := make([]string, len(fl.options))
	for j, o := range fl.options {
		var sw color.Color
		if fl.swatch != nil {
			sw = fl.swatch(s.T, o)
		}
		switch {
		case j == fl.opt:
			parts[j] = s.Selected.Render(" " + i18n.T(o) + " ")
			if sw != nil {
				parts[j] = s.Selected.Foreground(sw).Render(" " + i18n.T(o) + " ")
			}
		case sw != nil:
			parts[j] = lipgloss.NewStyle().Foreground(sw).Faint(true).Render(" " + i18n.T(o) + " ")
		default:
			parts[j] = s.Muted.Render(" " + i18n.T(o) + " ")
		}
		if z != nil {
			j := j
			parts[j] = z.click(parts[j], func(tea.Mouse, bool) tea.Cmd { fl.opt = j; return f.setFocus(i) })
		}
	}
	val := strings.Join(parts, "")
	if lipgloss.Width(val) > inW {
		val = s.Muted.Render("‹ ") + s.Selected.Render(" "+cur+" ") + s.Muted.Render(" ›") +
			s.Muted.Render(fmt.Sprintf("  %d/%d", fl.opt+1, len(fl.options)))
	}
	return val
}

// headerRow: „── Anmeldung ──────“; einklappbar mit ▸/▾ und Zusammenfassung.
func (f *form) headerRow(s Styles, i int, fl *field, focused bool, tint color.Color, width int, z *zones) string {
	tc := lipgloss.NewStyle().Foreground(tint)
	title := tc.Bold(true).Render(fl.label)
	lead := tc.Render("━━ ")
	extra := ""
	if fl.fold {
		arrow := "▸ "
		if fl.on {
			arrow = "▾ "
		} else if fl.summary != nil {
			if sm := fl.summary(f); sm != "" {
				extra = "  " + s.Muted.Render(sm)
			}
		}
		lead = tc.Render(" " + arrow)
		if focused {
			lead = s.Accent.Render("▌") + tc.Render(arrow)
		}
	}
	row := lead + title + extra + " "
	if rest := width - lipgloss.Width(row); rest > 0 {
		row += tc.Faint(true).Render(strings.Repeat("─", rest))
	}
	if fl.fold && z != nil {
		row = z.click(row, func(tea.Mouse, bool) tea.Cmd { fl.on = !fl.on; return f.setFocus(i) })
	}
	return row
}
