package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
)

// Aussehen (Theme) und eigene Tastenkürzel. Beides liegt in den
// Einstellungen und wird mit dem Sync auf andere Rechner übertragen.

// fontHint: Die Schriftgröße gehört dem Terminalfenster, nicht dem Programm
// darin; ein TUI kann sie nicht ändern.
const fontHint = "Schriftgröße: Strg + Plus/Minus im Terminalfenster (macOS: ⌘ + / −)"

// applyLook übernimmt Theme und Tasten aus den Einstellungen.
func (a *App) applyLook() {
	set := a.st.Settings()
	sig, _ := json.Marshal([]any{set.Theme, set.Keys})
	if string(sig) == a.lookSig {
		return
	}
	a.lookSig = string(sig)
	t := ThemeByName(set.Theme)
	activeTheme = t
	a.s = NewStyles(t)
	a.keys = newKeymap(set.Keys)
	activeKeys = a.keys
	// Felder, die schon existieren, bekommen die neuen Farben.
	a.mgr.search.SetStyles(inputStyles(t))
}

func (a *App) setTheme(name string) tea.Cmd {
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Theme = name }); err != nil {
		a.flash(err.Error())
		return nil
	}
	a.applyLook()
	a.flash("Aussehen: " + ThemeByName(name).Label + " · " + fontHint)
	return nil
}

// cycleTheme: Dunkel → Hell → Hochkontrast → Dunkel.
func (a *App) cycleTheme() tea.Cmd {
	cur := ThemeByName(a.st.Settings().Theme).Name
	for i, t := range Themes {
		if t.Name == cur {
			return a.setTheme(Themes[(i+1)%len(Themes)].Name)
		}
	}
	return a.setTheme(Nacht.Name)
}

// ---------------------------------------------------------------- Tasten-Editor

type keyEditor struct {
	sel       int
	capturing bool
	err       string
}

func (a *App) openKeys() tea.Cmd {
	a.keyEd = keyEditor{}
	a.scr = scrKeys
	return nil
}

// managerLetters: Tasten, die der Manager selbst braucht.
var managerLetters = "nedtsy/jkq"

func (a *App) setKey(id action, key string) error {
	d := defOf(id)
	if key != "" {
		if why := reservedKey(key); why != "" && !(d.area == "Manager" && len([]rune(key)) == 1) {
			return fmt.Errorf("%s %s", keyLabel(key), why)
		}
		if d.area == "Manager" && len([]rune(key)) == 1 && strings.Contains(managerLetters, key) {
			return fmt.Errorf("%s ist im Manager schon belegt", key)
		}
	}
	over := a.keys.overrides()
	if over == nil {
		over = map[string]string{}
	}
	delete(over, string(id))
	if key != "" && !(len(d.def) == 1 && d.def[0] == key) {
		over[string(id)] = key
	}
	if len(over) == 0 {
		over = nil
	}
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Keys = over }); err != nil {
		return err
	}
	a.applyLook()
	return nil
}

func (a *App) keysKeys() [][2]string {
	if a.keyEd.capturing {
		return [][2]string{{"neue Taste", "drücken"}, {"Esc", "abbrechen"}}
	}
	return [][2]string{{"⏎", "neu belegen"}, {"Entf", "Standard"}, {"r", "alle zurücksetzen"}, {"Esc", "zurück"}}
}

func (a *App) updateKeys(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	e := &a.keyEd
	key := k.String()
	if e.capturing {
		e.capturing = false
		if key == "esc" {
			e.err = ""
			return nil
		}
		id := actionDefs[e.sel].id
		if err := a.setKey(id, key); err != nil {
			e.err = err.Error()
			return nil
		}
		e.err = ""
		if other, ok := a.keys.conflicts()[id]; ok {
			e.err = "Achtung: " + keyLabel(key) + " ist auch „" + defOf(other).label + "“"
		}
		return nil
	}
	switch key {
	case "esc", "q":
		a.scr = scrManager
	case "up", "k":
		e.sel = max(0, e.sel-1)
	case "down", "j":
		e.sel = min(len(actionDefs)-1, e.sel+1)
	case "enter":
		e.capturing, e.err = true, ""
	case "delete", "backspace":
		if err := a.setKey(actionDefs[e.sel].id, ""); err != nil {
			e.err = err.Error()
		}
	case "r":
		a.modal = confirmModal("Tastenkürzel", "Alle Tastenkürzel auf Standard zurücksetzen?", func() tea.Cmd {
			if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Keys = nil }); err != nil {
				a.flash(err.Error())
			}
			a.applyLook()
			return nil
		})
	}
	return nil
}

func (a *App) viewKeys() string {
	s := a.s
	e := &a.keyEd
	var lines []string
	selLine := 0
	conf := a.keys.conflicts()
	area := ""
	for i, d := range actionDefs {
		if d.area != area {
			area = d.area
			lines = append(lines, s.Muted.Render(" "+strings.ToUpper(area)))
		}
		cur := a.keys.label(d.id)
		if len(a.keys.keys[d.id]) > 1 {
			cur += " / " + keyLabel(a.keys.keys[d.id][1])
		}
		mark := ""
		if _, ok := a.keys.overrides()[string(d.id)]; ok {
			mark = s.AI.Render(" ●") + s.Muted.Render(" Standard: "+keyLabel(d.def[0]))
		}
		if o, ok := conf[d.id]; ok {
			mark += s.Danger.Render(" ⚠ auch „" + defOf(o).label + "“")
		}
		keyTxt := s.Key.Render(cur)
		if i == e.sel && e.capturing {
			keyTxt = s.BadgeAI.Render("Taste drücken …")
		}
		line := fmt.Sprintf(" %-28s ", d.label)
		if i == e.sel {
			line = s.Selected.Render("▶"+line[1:]) + keyTxt + mark
		} else {
			line = s.Text.Render(line) + keyTxt + mark
		}
		i := i
		if i == e.sel {
			selLine = len(lines)
		}
		lines = append(lines, a.z.click(line, func(_ tea.Mouse, double bool) tea.Cmd {
			e.sel = i
			if double {
				e.capturing = true
			}
			return nil
		}))
	}
	var b strings.Builder
	b.WriteString(s.Accent.Render(" ⌨ Tastenkürzel anpassen") + s.Muted.Render("   ⏎ oder Doppelklick belegt neu · werden mit dem Sync übernommen") + "\n\n")
	rows := max(5, a.h-2-2-4)
	start := 0
	if selLine >= rows {
		start = selLine - rows + 1
	}
	b.WriteString(strings.Join(lines[start:min(len(lines), start+rows)], "\n") + "\n\n")
	if e.err != "" {
		b.WriteString(" " + s.Danger.Render(e.err) + "\n")
	}
	b.WriteString(s.Muted.Render(" Alt+⏎ ist gesperrt: Windows Terminal schaltet damit auf Vollbild.\n " + fontHint))
	return b.String()
}
