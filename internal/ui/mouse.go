package ui

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Maus
//
//   - Oberfläche: Zonen (zones.go) machen Tabs, Listen, Vorschläge, Palette,
//     Formularfelder und die Tasten der Fußzeile anklickbar. Ein Klick auf
//     eine Taste der Fußzeile wirkt genau wie der Tastendruck.
//   - Terminal: Hat das Programm auf dem Server die Maus angefordert (htop,
//     mc, vim mit mouse=a, tmux), gehen Klicks, Rad und Ziehen per SSH dorthin.
//     Shift hält die Maus bei sshTermy (markieren trotz Maus-Programm).
//   - Sonst markiert sshTermy selbst: Ziehen markiert, Loslassen kopiert,
//     Doppelklick = Wort, Dreifachklick = Zeile, Rechtsklick kopiert die
//     Markierung oder fügt ein, Mittelklick fügt ein. Das Rad blättert im
//     Verlauf (im Vollbild-Programm ohne Maus: Pfeiltasten wie bei less).
//
// Abschaltbar über die Palette („Maus aus“); dann markiert wieder das
// Terminalfenster selbst.

type clickState struct {
	x, y  int
	at    time.Time
	count int
}

// mouseEnabled: Maus einfangen? (Einstellung, Standard an)
func (a *App) mouseEnabled() bool { return !a.st.Settings().NoMouse }

func (a *App) toggleMouse() tea.Cmd {
	on := !a.mouseEnabled()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoMouse = !on }); err != nil {
		a.flash("Einstellung nicht gespeichert: " + err.Error())
		return nil
	}
	if on {
		a.flash("Maus an: Klicken, Markieren, Rad. Shift+Ziehen markiert im Fenster.")
	} else {
		a.flash("Maus aus: das Terminalfenster markiert und kopiert selbst.")
	}
	return nil
}

func (a *App) mouseMode() tea.MouseMode {
	if a.scr == scrUnlock || !a.mouseEnabled() {
		return tea.MouseModeNone
	}
	if s := a.current(); a.scr == scrTerminal && s != nil && s.term != nil && a.modal == nil && a.pal == nil &&
		s.term.MouseTracking() == termemu.MouseAny {
		return tea.MouseModeAllMotion
	}
	return tea.MouseModeCellMotion
}

// mouse verteilt ein Mausereignis.
func (a *App) mouse(msg tea.MouseMsg) tea.Cmd {
	m := msg.Mouse()
	_, isClick := msg.(tea.MouseClickMsg)
	_, isWheel := msg.(tea.MouseWheelMsg)

	clicks := 0
	if isClick {
		c := &a.click
		if m.Button == tea.MouseLeft && abs(c.x-m.X) <= 1 && c.y == m.Y && time.Since(c.at) < 450*time.Millisecond {
			c.count++
		} else {
			c.count = 1
		}
		c.x, c.y, c.at = m.X, m.Y, time.Now()
		clicks = c.count
	}

	// Terminalfläche zuerst: Markieren und Weiterleiten dürfen nicht an
	// Zonen hängen bleiben (z. B. beim Ziehen über den Rand hinaus).
	s := a.current()
	if a.scr == scrTerminal && s != nil && a.modal == nil && a.pal == nil {
		if s.selecting || s.fwdHeld || (a.inTermArea(s, m.X, m.Y) && !s.files.open) {
			return a.termMouse(s, msg, clicks)
		}
	}

	if isWheel {
		up := m.Button == tea.MouseWheelUp
		if m.Button != tea.MouseWheelUp && m.Button != tea.MouseWheelDown {
			return nil
		}
		if act := a.z.at(m.X, m.Y, true); act != nil {
			return act.wheel(up)
		}
		if a.modal != nil && (a.modal.kind == mHelp || a.modal.kind == mChangelog) {
			return a.pressKey(wheelKey(up))
		}
		if a.modal != nil || a.pal != nil {
			return nil
		}
		switch a.scr {
		case scrManager:
			return a.pressKey(wheelKey(up))
		case scrTerminal:
			if s != nil && s.panel != panelNone && !s.files.open {
				s.panelFocus = true
				return a.pressKey(wheelKey(up))
			}
		}
		return nil
	}
	if isClick && a.modal != nil && (a.modal.kind == mMenu) && m.Button != tea.MouseLeft {
		a.modal = nil
		return nil
	}
	if !isClick || m.Button != tea.MouseLeft {
		return nil
	}
	if act := a.z.at(m.X, m.Y, false); act != nil {
		return act.click(m, clicks >= 2)
	}
	// Klick ins Leere
	switch {
	case a.pal != nil:
		a.pal = nil
	case a.modal != nil:
		if a.modal.kind == mHelp || a.modal.kind == mAbout || a.modal.kind == mChangelog || a.modal.kind == mMenu || a.modal.kind == mInfo {
			a.modal = nil
		}
	case a.scr == scrTerminal && s != nil && s.panel != panelNone && !s.files.open && m.X >= a.w-a.panelWidth(s):
		s.panelFocus = true
	}
	return nil
}

func wheelKey(up bool) string {
	if up {
		return "up"
	}
	return "down"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ---------------------------------------------------------------- Tasten per Klick

// keyFromLabel macht aus einer angezeigten Taste („⏎“, „Strg+S“, „F2“, „n“)
// den passenden Tastendruck. ok = false, wenn sie sich nicht eindeutig
// drücken lässt („↑↓“, „1…9“).
func keyFromLabel(label string) (tea.KeyPressMsg, bool) {
	l := strings.TrimSpace(label)
	var mod tea.KeyMod
	for {
		switch {
		case strings.HasPrefix(l, "Strg+"):
			mod |= tea.ModCtrl
			l = l[len("Strg+"):]
			continue
		case strings.HasPrefix(l, "Ctrl+"):
			mod |= tea.ModCtrl
			l = l[len("Ctrl+"):]
			continue
		case strings.HasPrefix(l, "Alt+"):
			mod |= tea.ModAlt
			l = l[len("Alt+"):]
			continue
		case strings.HasPrefix(l, "Shift+"):
			mod |= tea.ModShift
			l = l[len("Shift+"):]
			continue
		case strings.HasPrefix(l, "⇧") && len(l) > len("⇧"):
			mod |= tea.ModShift
			l = l[len("⇧"):]
			continue
		}
		break
	}
	named := map[string]rune{
		"⏎": tea.KeyEnter, "Enter": tea.KeyEnter, "Esc": tea.KeyEscape, "Tab": tea.KeyTab,
		"Space": tea.KeySpace, "Leertaste": tea.KeySpace, "Entf": tea.KeyDelete, "⌫": tea.KeyBackspace,
		"↑": tea.KeyUp, "↓": tea.KeyDown, "←": tea.KeyLeft, "→": tea.KeyRight,
		"Bild↑": tea.KeyPgUp, "Bild↓": tea.KeyPgDown, "Pos1": tea.KeyHome, "Ende": tea.KeyEnd,
		"F1": tea.KeyF1, "F2": tea.KeyF2, "F3": tea.KeyF3, "F4": tea.KeyF4, "F5": tea.KeyF5, "F6": tea.KeyF6,
		"F7": tea.KeyF7, "F8": tea.KeyF8, "F9": tea.KeyF9, "F10": tea.KeyF10, "F11": tea.KeyF11, "F12": tea.KeyF12,
	}
	if r, ok := named[l]; ok {
		return tea.KeyPressMsg{Code: r, Mod: mod}, true
	}
	if utf8.RuneCountInString(l) == 1 {
		r, _ := utf8.DecodeRuneInString(l)
		if r == '…' {
			return tea.KeyPressMsg{}, false
		}
		if mod&(tea.ModCtrl|tea.ModAlt) != 0 {
			r = unicode.ToLower(r) // „Strg+S“ heißt ctrl+s
		}
		k := tea.KeyPressMsg{Code: r, Mod: mod}
		if mod&^tea.ModShift == 0 {
			k.Text = l
		}
		return k, true
	}
	return tea.KeyPressMsg{}, false
}

// pressKey spielt einen Tastendruck ab, als wäre er getippt worden.
func (a *App) pressKey(label string) tea.Cmd {
	if k, ok := keyFromLabel(label); ok {
		return a.press(k)
	}
	if k, ok := keyFromKeyString(label); ok {
		return a.press(k)
	}
	return nil
}

func (a *App) press(k tea.KeyPressMsg) tea.Cmd {
	_, cmd := a.Update(k)
	return cmd
}

// keyFromKeyString: interne Namen wie „up“, „down“, „alt+1“.
func keyFromKeyString(s string) (tea.KeyPressMsg, bool) {
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}, true
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}, true
	}
	if strings.HasPrefix(s, "alt+") && len(s) == 5 {
		return tea.KeyPressMsg{Code: rune(s[4]), Mod: tea.ModAlt}, true
	}
	return tea.KeyPressMsg{}, false
}

// keyZone macht eine angezeigte Taste anklickbar. Mehrere Tasten mit „/“
// („F7/F8“, „⏎/←“) werden einzeln anklickbar.
func (a *App) keyZone(label, rendered string) string {
	if _, ok := keyFromLabel(label); !ok {
		return rendered
	}
	return a.z.click(rendered, func(tea.Mouse, bool) tea.Cmd { return a.pressKey(label) })
}

// keyChip: Taste + Beschreibung als ein Knopf (Fußzeile).
func (a *App) keyChip(key, desc string) string {
	parts := strings.Split(key, "/")
	if len(parts) == 1 {
		return a.keyZone(key, a.s.Key.Render(key)+a.s.KeyDesc.Render(desc))
	}
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString("/")
		}
		b.WriteString(a.keyZone(p, p))
	}
	return a.s.Key.Render(b.String()) + a.s.KeyDesc.Render(desc)
}

// button: anklickbarer Knopf für Dialoge.
func (a *App) button(key, label string) string {
	return a.keyZone(key, a.s.Key.Render(key)+a.s.KeyDesc.Render(label))
}

// ---------------------------------------------------------------- Terminal

func (a *App) inTermArea(s *session, x, y int) bool {
	cols, rows := a.termSize(s)
	return x >= 0 && x < cols && y >= topRows && y < topRows+rows
}

func (a *App) termMouse(s *session, msg tea.MouseMsg, clicks int) tea.Cmd {
	m := msg.Mouse()
	cols, rows := a.termSize(s)
	tx, ty := m.X, m.Y-topRows
	if s.term == nil || s.state != stConnected {
		return nil
	}
	_, isClick := msg.(tea.MouseClickMsg)
	_, isRelease := msg.(tea.MouseReleaseMsg)
	_, isMotion := msg.(tea.MouseMotionMsg)
	_, isWheel := msg.(tea.MouseWheelMsg)
	if isClick {
		s.panelFocus = false
	}

	// 1) Programm auf dem Server will die Maus
	lvl := s.term.MouseTracking()
	if s.fwdHeld || (lvl != termemu.MouseOff && !m.Mod.Contains(tea.ModShift) && !s.selecting) {
		ev := uv.Mouse{X: clamp(tx, 0, cols-1), Y: clamp(ty, 0, rows-1), Button: m.Button, Mod: m.Mod}
		switch {
		case isClick:
			s.fwdHeld = lvl >= termemu.MouseButtons
			s.sel, s.scroll = nil, 0
			s.term.SendMouse(uv.MouseClickEvent(ev))
		case isRelease:
			s.fwdHeld = false
			if lvl >= termemu.MouseButtons {
				s.term.SendMouse(uv.MouseReleaseEvent(ev))
			}
		case isMotion:
			if lvl == termemu.MouseAny || (lvl == termemu.MouseDrag && s.fwdHeld) {
				s.term.SendMouse(uv.MouseMotionEvent(ev))
			}
		case isWheel:
			s.term.SendMouse(uv.MouseWheelEvent(ev))
		}
		return nil
	}

	// 2) sshTermy markiert selbst
	switch {
	case isWheel:
		up := m.Button == tea.MouseWheelUp
		if m.Button != tea.MouseWheelUp && m.Button != tea.MouseWheelDown {
			return nil
		}
		if s.term.AltScreen() {
			// Vollbild-Programm ohne Maus (less, man): Rad = Pfeiltasten
			k := uv.KeyPressEvent{Code: uv.KeyDown}
			if up {
				k.Code = uv.KeyUp
			}
			for i := 0; i < 3; i++ {
				s.term.SendKey(k)
			}
			return nil
		}
		d := -3
		if up {
			d = 3
		}
		s.scroll = clamp(s.scroll+d, 0, s.term.ScrollbackLen())
		return nil

	case isClick && m.Button == tea.MouseLeft && m.Mod.Contains(tea.ModCtrl) && clicks == 1:
		if cmd, ok := a.termLinkClick(s, m, tx, ty, true); ok {
			return cmd
		}
		fallthrough

	case isClick && m.Button == tea.MouseLeft:
		p := s.term.ScreenPos(clamp(tx, 0, cols-1), clamp(ty, 0, rows-1), s.scroll)
		switch {
		case clicks == 2:
			sel := s.term.WordAt(p)
			s.sel, s.selecting = &sel, false
			return a.copySel(s)
		case clicks >= 3:
			sel := s.term.LineAt(p)
			s.sel, s.selecting = &sel, false
			return a.copySel(s)
		}
		s.sel = &termemu.Sel{A: p, B: p}
		s.selecting = true
		return nil

	case isMotion && s.selecting && s.sel != nil:
		// über den Rand ziehen blättert mit
		if ty < 0 && !s.term.AltScreen() {
			s.scroll = clamp(s.scroll+1, 0, s.term.ScrollbackLen())
		} else if ty >= rows {
			s.scroll = clamp(s.scroll-1, 0, s.term.ScrollbackLen())
		}
		s.sel.B = s.term.ScreenPos(clamp(tx, 0, cols-1), clamp(ty, 0, rows-1), s.scroll)
		return nil

	case isRelease && s.selecting:
		s.selecting = false
		if s.sel == nil || s.sel.Empty() {
			s.sel = nil
			return nil
		}
		return a.copySel(s)

	case isClick && m.Button == tea.MouseRight:
		if s.sel != nil && !s.sel.Empty() {
			cmd := a.copySel(s)
			s.sel = nil
			return cmd
		}
		if cmd, ok := a.termLinkClick(s, m, tx, ty, false); ok {
			return cmd
		}
		return a.pasteClipboard(s)

	case isClick && m.Button == tea.MouseMiddle:
		return a.pasteClipboard(s)
	}
	return nil
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// copySel kopiert die Markierung in die Zwischenablage: über das
// Betriebssystem (Windows, macOS, Linux mit xclip/wl-copy) und zusätzlich
// per OSC 52 (geht auch über SSH-in-SSH und in Windows Terminal).
func (a *App) copySel(s *session) tea.Cmd {
	if s.sel == nil || s.term == nil {
		return nil
	}
	text := s.term.Text(*s.sel)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	go func() { _ = clipboard.WriteAll(text) }()
	n := utf8.RuneCountInString(text)
	if lines := strings.Count(text, "\n") + 1; lines > 1 {
		a.flash("Kopiert: " + plural(lines, "Zeile", "Zeilen") + " · Rechtsklick fügt ein")
	} else {
		a.flash("Kopiert: " + plural(n, "Zeichen", "Zeichen") + " · Rechtsklick fügt ein")
	}
	return tea.SetClipboard(text)
}

type pasteMsg struct {
	sid  int
	text string
	err  error
}

func (a *App) pasteClipboard(s *session) tea.Cmd {
	sid := s.id
	return func() tea.Msg {
		t, err := clipboard.ReadAll()
		return pasteMsg{sid, t, err}
	}
}

func (a *App) onPaste(m pasteMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil || s.term == nil || s.state != stConnected {
		return nil
	}
	if m.err != nil || m.text == "" {
		a.flash("Zwischenablage leer oder nicht lesbar. Einfügen mit Strg+Shift+V im Terminalfenster.")
		return nil
	}
	return a.guardedPaste(s, m.text)
}

// modalButtons: Knöpfe unter dem Dialog (Klick = Taste).
func (a *App) modalButtons() string {
	var keys [][2]string
	switch a.modal.kind {
	case mHostKey:
		keys = [][2]string{{"j", "vertrauen"}, {"n", "abbrechen"}}
	case mConfirm:
		keys = [][2]string{{"j", "ja"}, {"n", "nein"}}
	case mSecret:
		keys = [][2]string{{"⏎", "senden"}, {"Esc", "abbrechen"}}
	case mAbout:
		if a.rel != nil {
			keys = append(keys, [2]string{"u", "aktualisieren"})
		} else if a.restartPending != "" {
			keys = append(keys, [2]string{"r", "neu starten"})
		} else {
			keys = append(keys, [2]string{"c", "nach Updates suchen"})
		}
		keys = append(keys, [2]string{"w", "was ist neu"}, [2]string{"Esc", "schließen"})
	case mChangelog:
		keys = [][2]string{{"Esc", "schließen"}}
	case mHelp, mInfo:
		keys = [][2]string{{"Esc", "schließen"}}
	case mTour:
		keys = tourButtons(a.modal)
	case mQuick:
		keys = [][2]string{{"⏎", "ausführen"}, {"e", "bearbeiten"}, {"Esc", "schließen"}}
	case mPaste:
		keys = [][2]string{{"⏎", "einfügen"}}
		if a.modal.paste.multi {
			keys = append(keys, [2]string{"o", "ohne letztes ⏎"})
		}
		keys = append(keys, [2]string{"Esc", "abbrechen"})
	default:
		return ""
	}
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(a.button(k[0], k[1]))
	}
	return b.String()
}

func mouseLabel(on bool) string {
	if on {
		return "Maus aus (Terminalfenster markiert selbst)"
	}
	return "Maus an (klicken, markieren, scrollen)"
}

// mouseHelp: Abschnitt für die Hilfe (F1).
const mouseHelp = `Maus         Klick auf Tabs, Listen, Vorschläge, Felder, Fußzeile · Doppelklick öffnet
             Terminal: Ziehen markiert + kopiert · Doppel-/Dreifachklick Wort/Zeile
             Rechtsklick fügt ein, auf Link/IP/Host öffnet er ein Menü · Strg+Klick öffnet URL
             Rad blättert im Verlauf (Esc zurück)
             htop, mc, vim, tmux bekommen die Maus · Shift+Ziehen markiert trotzdem
             Palette „Maus aus“: Terminalfenster markiert wieder selbst`
