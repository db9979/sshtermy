package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// fieldKind: Text, Geheimnis, Zahl, Auswahl oder Schalter.
type fieldKind int

const (
	fText fieldKind = iota
	fSecret
	fNumber
	fChoice
	fBool
)

type field struct {
	key, label, help string
	kind             fieldKind
	in               textinput.Model
	options          []string // fChoice
	opt              int
	on               bool // fBool
	hidden           func(f *form) bool
}

// form ist ein kleines, tastaturgesteuertes Formular (Tab/↑↓ wechseln,
// ←/→ ändern Auswahl und Schalter, Strg+S oder Enter im letzten Feld speichert).
type form struct {
	title  string
	fields []*field
	focus  int
	err    string
}

func newInput(val string, secret bool) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(val)
	ti.CharLimit = 4096
	ti.SetWidth(40)
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
func (f *form) toggle(key, label string, on bool, help string) *field {
	return f.add(&field{key: key, label: label, help: help, kind: fBool, on: on})
}

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
func (f *form) flag(key string) bool { return f.get(key).on }

func (f *form) visible() []int {
	var out []int
	for i, fl := range f.fields {
		if fl.hidden == nil || !fl.hidden(f) {
			out = append(out, i)
		}
	}
	return out
}

func (f *form) move(d int) tea.Cmd {
	vis := f.visible()
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
	vis := f.visible()
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
func (f *form) view(s Styles, width int, z *zones) string {
	var b strings.Builder
	if f.title != "" {
		b.WriteString(s.Accent.Render(f.title) + "\n\n")
	} else {
		b.WriteString("\n")
	}
	inW := max(20, width-26)
	for _, i := range f.visible() {
		fl := f.fields[i]
		focused := i == f.focus
		label := s.Label.Render(fl.label)
		var val string
		switch fl.kind {
		case fChoice:
			parts := make([]string, len(fl.options))
			for j, o := range fl.options {
				if j == fl.opt {
					parts[j] = s.Selected.Render(" " + o + " ")
				} else {
					parts[j] = s.Muted.Render(" " + o + " ")
				}
				if z != nil {
					i, j, fl := i, j, fl
					parts[j] = z.click(parts[j], func(tea.Mouse, bool) tea.Cmd { fl.opt = j; return f.setFocus(i) })
				}
			}
			val = strings.Join(parts, "")
		case fBool:
			if fl.on {
				val = s.OK.Render("● an")
			} else {
				val = s.Muted.Render("○ aus")
			}
		default:
			fl.in.SetWidth(inW)
			val = fl.in.View()
		}
		marker := "  "
		if focused {
			marker = s.Accent.Render("▌ ")
			label = lipgloss.NewStyle().Foreground(s.T.Text).Bold(true).Width(20).Render(fl.label)
		}
		row := marker + label + val
		if z != nil {
			i, fl := i, fl
			row = z.click(row, func(_ tea.Mouse, _ bool) tea.Cmd {
				if fl.kind == fBool {
					fl.on = !fl.on
				}
				return f.setFocus(i)
			})
		}
		b.WriteString(row + "\n")
		if focused && fl.help != "" {
			b.WriteString("  " + strings.Repeat(" ", 20) + s.Muted.Render(fl.help) + "\n")
		}
	}
	if f.err != "" {
		b.WriteString("\n" + s.Danger.Render("✗ "+f.err) + "\n")
	}
	return b.String()
}
