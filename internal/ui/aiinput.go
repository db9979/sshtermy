package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// aiInputMaxRows: so hoch wächst ein KI-Eingabefeld höchstens, danach
// scrollt es innerhalb des Felds.
const aiInputMaxRows = 8

// aiInput ist das Eingabefeld für Ziele, Fragen und die Palette: Der Text
// bricht automatisch um und das Feld wächst mit, statt horizontal zu
// scrollen. ⏎ wertet der Aufrufer aus (senden), Shift+⏎ / Strg+J (frei belegbar)
// fügt eine neue Zeile ein.
type aiInput struct {
	textarea.Model
	singleLine bool // z. B. Befehl bearbeiten: umbrechen ja, Zeilenumbruch nein
	gen        int  // Theme-Stand der Farben (themeGen)
}

// aiInputStyles: Farben des Feldes passend zum Theme.
func aiInputStyles(t Theme) textarea.Styles {
	sty := textarea.DefaultStyles(t.Dark)
	sty.Focused.Text = lipgloss.NewStyle().Foreground(t.Text)
	sty.Blurred.Text = lipgloss.NewStyle().Foreground(t.Muted)
	sty.Focused.Placeholder = lipgloss.NewStyle().Foreground(t.Muted)
	sty.Blurred.Placeholder = sty.Focused.Placeholder
	sty.Cursor.Color = t.Accent
	plain := lipgloss.NewStyle()
	sty.Focused.CursorLine = plain
	sty.Focused.EndOfBuffer = plain
	sty.Blurred.CursorLine = plain
	sty.Blurred.EndOfBuffer = plain
	return sty
}

// restyle: Theme wurde gewechselt, während das Feld schon bestand (Ziel-,
// Frage-, Vorgangsfeld leben so lange wie der Tab). Sonst bliebe z. B. die
// helle Schrift von „Nacht“ auf einem hellen Theme stehen.
func (in *aiInput) restyle() {
	if in.gen != themeGen {
		in.Model.SetStyles(aiInputStyles(activeTheme))
		in.gen = themeGen
	}
}

func (in aiInput) View() string {
	in.restyle()
	return in.Model.View()
}

func newAIInput(placeholder string) aiInput {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.Placeholder = placeholder
	ta.CharLimit = 8000
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = aiInputMaxRows
	ta.MaxContentHeight = 400
	ta.EndOfBufferCharacter = ' '
	// Kein Alt+⏎: das schaltet Windows Terminal auf Vollbild.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys(activeKeys.keys[actNewline]...))

	ta.SetStyles(aiInputStyles(activeTheme))
	ta.SetWidth(40)
	return aiInput{Model: ta, gen: themeGen}
}

// newCommandInput: bricht um, erlaubt aber keine Zeilenumbrüche (ein Befehl).
func newCommandInput() aiInput {
	in := newAIInput("")
	in.singleLine = true
	in.KeyMap.InsertNewline.SetEnabled(false)
	return in
}

func (in aiInput) Update(msg tea.Msg) (aiInput, tea.Cmd) {
	in.restyle()
	var cmd tea.Cmd
	in.Model, cmd = in.Model.Update(msg)
	if in.singleLine {
		if v := in.Model.Value(); strings.ContainsAny(v, "\r\n") {
			in.Model.SetValue(strings.Join(strings.Fields(v), " "))
		}
	}
	return in, cmd
}

// CursorEnd setzt den Cursor ans Ende des gesamten Texts (wie bei textinput).
func (in *aiInput) CursorEnd() { in.Model.MoveToEnd() }

// promptBlock setzt ein Präfix wie „› “ vor ein (evtl. mehrzeiliges) Feld,
// sodass Folgezeilen bündig eingerückt bleiben.
func promptBlock(prefix, view string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, prefix, view)
}

// oneLine fasst mehrzeiligen Text für Listen-Beschriftungen zusammen.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
