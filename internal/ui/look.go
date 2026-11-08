package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Aussehen (Theme, Symbole) und eigene Tastenkürzel. Alles liegt in den
// Einstellungen und wird mit dem Sync auf andere Rechner übertragen.

// fontHint: Die Schriftgröße gehört dem Terminalfenster, nicht dem Programm
// darin; ein TUI kann sie nicht ändern.
func fontHint() string {
	return i18n.T("Schriftgröße: Strg + Plus/Minus im Terminalfenster (macOS: ⌘ + / −)")
}

// setStyles: Farben des Themes, Knöpfe in Dialogen werden klickbar.
func (a *App) setStyles(t Theme) {
	a.s = NewStyles(t)
	a.s.zoneKey = a.keyZone
}

// applyLook übernimmt Theme und Tasten aus den Einstellungen.
func (a *App) applyLook() {
	a.applyLang()
	set := a.st.Settings()
	_ = syncIntroFile(a.st.Dir, set.Intro)
	accessible = set.Accessible
	motionSetting = set.Motion
	sig, _ := json.Marshal([]any{set.Theme, set.Keys, set.Colors, set.Icons, set.NerdFont})
	if string(sig) == a.lookSig {
		return
	}
	first := a.lookSig == ""
	a.lookSig = string(sig)
	t := themeOf(set)
	if iconSet != set.IconsNow() && set.Icons == iconNerd && set.NerdFont == "nein" && !first {
		// Nerd-Font-Symbole kamen per Sync von einem anderen Rechner.
		a.flash(i18n.T("Nerd-Font-Symbole sind auf einem anderen Rechner an; hier erst nach der Probe (Einstellungen › Symbole)"))
	}
	iconSet = set.IconsNow()
	activeTheme = t
	a.setStyles(t)
	a.keys = newKeymap(set.Keys)
	activeKeys = a.keys
	// Felder, die schon existieren, bekommen die neuen Farben.
	a.mgr.search.SetStyles(inputStyles(t))
}

func (a *App) setTheme(name string) tea.Cmd {
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Theme = name }); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.applyLook()
	a.flash(i18n.T("Aussehen: ") + i18n.T(ThemeByName(name).Label) + " · " + fontHint())
	return nil
}

// cycleTheme: Dunkel → Hell → Hochkontrast → Hochkontrast hell → Dunkel.
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
			return fmt.Errorf(i18n.T("%s ist im Manager schon belegt"), key)
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
		return [][2]string{{i18n.T("neue Taste"), i18n.T("drücken")}, {"Esc", i18n.T("abbrechen")}}
	}
	return [][2]string{{"⏎", i18n.T("neu belegen")}, {i18n.T("Entf"), i18n.T("Standard")}, {"r", i18n.T("alle zurücksetzen")}, {"Esc", i18n.T("zurück")}}
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
			e.err = i18n.T("Achtung: ") + keyLabel(key) + i18n.T(" ist auch „") + i18n.T(defOf(other).label) + i18n.T("“")
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
		a.modal = confirmModal(i18n.T("Tastenkürzel"), i18n.T("Alle Tastenkürzel auf Standard zurücksetzen?"), func() tea.Cmd {
			if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Keys = nil }); err != nil {
				a.flashErr(err.Error())
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
			lines = append(lines, s.Muted.Render(" "+strings.ToUpper(i18n.T(area))))
		}
		cur := a.keys.label(d.id)
		if len(a.keys.keys[d.id]) > 1 {
			cur += " / " + keyLabel(a.keys.keys[d.id][1])
		}
		mark := ""
		if _, ok := a.keys.overrides()[string(d.id)]; ok {
			mark = s.AI.Render(" ●") + s.Muted.Render(i18n.T(" Standard: ")+keyLabel(d.def[0]))
		}
		if o, ok := conf[d.id]; ok {
			mark += s.Danger.Render(i18n.T(" ▲ auch „") + i18n.T(defOf(o).label) + i18n.T("“"))
		}
		keyTxt := s.Key.Render(cur)
		if i == e.sel && e.capturing {
			keyTxt = s.BadgeAI.Render(i18n.T("Taste drücken …"))
		}
		line := fmt.Sprintf(" %-28s ", i18n.T(d.label))
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
	b.WriteString(s.Accent.Render(i18n.T(" ▪ Tastenkürzel anpassen")) + s.Muted.Render(i18n.T("   ⏎ oder Doppelklick belegt neu · werden mit dem Sync übernommen")) + "\n\n")
	rows := max(5, a.h-2-2-4)
	start := 0
	if selLine >= rows {
		start = selLine - rows + 1
	}
	b.WriteString(strings.Join(lines[start:min(len(lines), start+rows)], "\n") + "\n\n")
	if e.err != "" {
		b.WriteString(" " + s.Danger.Render(e.err) + "\n")
	}
	b.WriteString(s.Muted.Render(i18n.T(" Alt+⏎ ist gesperrt: Windows Terminal schaltet damit auf Vollbild.\n ") + fontHint()))
	return b.String()
}
