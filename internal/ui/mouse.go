package ui

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
	"github.com/db9979/sshtermy/internal/update"
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
		a.flashErr(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Maus an: Klicken, Markieren, Rad. Shift+Ziehen markiert im Fenster."))
	} else {
		a.flash(i18n.T("Maus aus: das Terminalfenster markiert und kopiert selbst."))
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
	if motionLevel() == 2 && a.modal == nil && a.pal == nil && a.pick == nil && a.scr == scrTerminal {
		return tea.MouseModeAllMotion // Hover-Karten (motion.go)
	}
	return tea.MouseModeCellMotion
}

// mouse verteilt ein Mausereignis.
func (a *App) mouse(msg tea.MouseMsg) tea.Cmd {
	m := msg.Mouse()
	_, isClick := msg.(tea.MouseClickMsg)
	_, isWheel := msg.(tea.MouseWheelMsg)
	if _, isMotion := msg.(tea.MouseMotionMsg); isMotion && m.Button == tea.MouseNone {
		// Bewegung ohne Taste kommt nur bei voller Bewegung (Hover-Karten)
		// oder wenn ein Programm im Terminal sie will.
		a.trackHover(m)
		if !a.remoteWantsMotion() {
			return nil
		}
	} else {
		a.hov = hoverState{}
	}

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

	// geteilte Ansicht: Trennlinien ziehen, Bereiche tauschen, zoomen
	if cmd, ok := a.splitMouse(msg, clicks); ok {
		return cmd
	}

	// Terminalfläche zuerst: Markieren und Weiterleiten dürfen nicht an
	// Zonen hängen bleiben (z. B. beim Ziehen über den Rand hinaus).
	s := a.current()
	if a.scr == scrTerminal && s != nil && a.modal == nil && a.pal == nil && a.pick == nil && !s.selecting && !s.fwdHeld {
		// geteilte Ansicht: Klick holt den Bereich nach vorn, das Rad
		// blättert im Bereich unter der Maus
		if p := a.paneAt(m.X, m.Y); p != nil && p != s {
			if isClick {
				a.focusSession(a.indexOf(p))
				s = p
			} else if isWheel && a.inTermArea(p, m.X, m.Y) && !p.files.open && !p.detached() {
				return a.termMouse(p, msg, 0)
			}
		}
	}
	if a.scr == scrTerminal && s != nil && a.modal == nil && a.pal == nil && a.pick == nil {
		if s.search.open && s.search.list && !s.selecting && a.inTermArea(s, m.X, m.Y) && !isWheel {
			// Fehlerliste: Zeilen sind Zonen
		} else if s.selecting || s.fwdHeld || (a.inTermArea(s, m.X, m.Y) && !s.files.open && s.state == stConnected && !s.detached()) {
			// ohne laufende Sitzung (Verbinden, Reconnect, beendet) zeigt die
			// Fläche anklickbare Hinweise, die Klicks gehen an die Zonen
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
		if a.modal != nil || a.pal != nil || a.pick != nil {
			return nil
		}
		switch a.scr {
		case scrManager, scrThemes:
			return a.pressKey(wheelKey(up))
		case scrTerminal:
			if s != nil && s.panel != panelNone && !s.files.open {
				s.panelFocus = true
				d := 3
				if up {
					d = -3
				}
				if s.scrollPanel(d) { // langer Inhalt: blättern statt Auswahl
					return nil
				}
				return a.pressKey(wheelKey(up))
			}
		}
		return nil
	}
	if isClick && a.modal != nil && (a.modal.kind == mMenu) && m.Button != tea.MouseLeft {
		a.modal = nil
		return nil
	}
	// Ziehen im Dateibrowser: Einträge auf die andere Seite kopieren
	if a.scr == scrTerminal && s != nil && s.files.open && s.files.drag != nil && a.modal == nil && a.pal == nil && a.pick == nil && !isClick {
		return a.filesDragMouse(s, msg)
	}
	// Ziehen im Manager: Verbindungen und Ordner umsortieren
	if a.scr == scrManager && a.modal == nil && a.pal == nil && a.pick == nil && a.mgr.drag != nil && !isClick {
		return a.mgrDragMouse(msg)
	}
	if isClick && (m.Button == tea.MouseRight || m.Button == tea.MouseMiddle) {
		if act := a.z.atMenu(m.X, m.Y); act != nil {
			return act.menu(m)
		}
	}
	if !isClick || m.Button != tea.MouseLeft {
		return nil
	}
	if act := a.z.at(m.X, m.Y, false); act != nil {
		cmd := act.click(m, clicks >= 2)
		a.formChanged() // Klick auf eine Auswahl wirkt wie ←/→
		return cmd
	}
	// Klick ins Leere
	switch {
	case a.pick != nil:
		a.pick = nil
	case a.pal != nil:
		a.pal = nil
	case a.modal != nil:
		if a.modal.kind == mHelp || a.modal.kind == mAbout || a.modal.kind == mChangelog || a.modal.kind == mMenu || a.modal.kind == mInfo || a.modal.kind == mLLM || a.modal.kind == mCrash || a.modal.kind == mLayout {
			a.modal = nil
		}
	case a.scr == scrTerminal && s != nil && s.panel != panelNone && !s.files.open && a.inPanel(s, m.X, m.Y):
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
		"Space": tea.KeySpace, "Leertaste": tea.KeySpace, "Entf": tea.KeyDelete, "Bksp": tea.KeyBackspace,
		"↑": tea.KeyUp, "↓": tea.KeyDown, "←": tea.KeyLeft, "→": tea.KeyRight,
		"Bild↑": tea.KeyPgUp, "Bild↓": tea.KeyPgDown, "Pos1": tea.KeyHome, "Ende": tea.KeyEnd,
		"Del": tea.KeyDelete, "PgUp": tea.KeyPgUp, "PgDn": tea.KeyPgDown, "Home": tea.KeyHome, "End": tea.KeyEnd,
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

// hints: Hinweiszeile wie „⏎ neu verbinden · x Tab schließen“, in der jede
// Taste samt Beschreibung anklickbar ist. Leere Taste = nur Text.
func (a *App) hints(style lipgloss.Style, items ...[2]string) string {
	return a.hintsIn(nil, style, items...)
}

// hintsIn: wie hints, aber für ein Seitenpanel. Ein Klick holt erst den
// Fokus ins Panel, damit die Taste dort ankommt und nicht in der Shell.
func (a *App) hintsIn(s *session, style lipgloss.Style, items ...[2]string) string {
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteString(style.Render(" · "))
		}
		key, desc := i18n.T(it[0]), it[1]
		text := desc
		if key != "" {
			text = key + " " + desc
		}
		r := style.Render(text)
		if _, ok := keyFromLabel(key); !ok || key == "" {
			b.WriteString(r)
			continue
		}
		b.WriteString(a.z.click(r, func(tea.Mouse, bool) tea.Cmd {
			if s != nil {
				s.panelFocus = true
			}
			return a.pressKey(key)
		}))
	}
	return b.String()
}

// button: anklickbarer Knopf für Dialoge.
func (a *App) button(key, label string) string {
	return a.keyZone(key, a.s.Key.Render(key)+a.s.KeyDesc.Render(label))
}

// ---------------------------------------------------------------- Terminal

func (a *App) inTermArea(s *session, x, y int) bool {
	cols, rows := a.termSize(s)
	r := a.termRect(s)
	cols = min(cols, r.w-a.panelShown(s)) // aufgeklapptes Panel verdeckt einen Teil
	return x >= r.x && x < r.x+cols && y >= r.y && y < r.y+rows
}

// inPanel: liegt (x, y) im Seitenpanel der Sitzung?
func (a *App) inPanel(s *session, x, y int) bool {
	r := a.termRect(s)
	return r.has(x, y) && x >= r.x+r.w-a.panelShown(s)
}

func (a *App) termMouse(s *session, msg tea.MouseMsg, clicks int) tea.Cmd {
	m := msg.Mouse()
	cols, rows := a.termSize(s)
	r := a.termRect(s)
	tx, ty := m.X-r.x, m.Y-r.y
	if s.term == nil || s.state != stConnected || s.detached() {
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
		a.flash(i18n.T("Kopiert: ") + plural(lines, i18n.T("Zeile"), i18n.T("Zeilen")) + i18n.T(" · Rechtsklick fügt ein"))
	} else {
		a.flash(i18n.T("Kopiert: ") + plural(n, i18n.T("Zeichen"), i18n.T("Zeichen")) + i18n.T(" · Rechtsklick fügt ein"))
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
	if s == nil || s.term == nil || s.state != stConnected || s.detached() {
		return nil
	}
	if m.err != nil || m.text == "" {
		a.flash(i18n.T("Zwischenablage leer oder nicht lesbar. Einfügen mit Strg+Shift+V im Terminalfenster."))
		return nil
	}
	return a.guardedPaste(s, m.text)
}

// modalKeys: Tastenhinweise unten im Dialog (Klick = Taste).
func (a *App) modalKeys() [][2]string {
	var keys [][2]string
	switch a.modal.kind {
	case mHostKey:
		keys = [][2]string{{yesKey(), i18n.T("vertrauen")}, {"n", i18n.T("abbrechen")}}
	case mConfirm:
		keys = [][2]string{{yesKey(), i18n.T("ja")}, {"n", i18n.T("nein")}}
	case mSecret:
		keys = [][2]string{{"⏎", i18n.T("senden")}, {"Esc", i18n.T("abbrechen")}}
	case mInput:
		keys = [][2]string{{"⏎", i18n.T("übernehmen")}, {"Esc", i18n.T("abbrechen")}}
	case mAbout:
		if a.rel != nil && update.Managed() == "" {
			keys = append(keys, [2]string{"u", i18n.T("aktualisieren")})
		} else if a.rel != nil {
			// Paketquelle: Befehl steht im Dialog, kein „u“.
		} else if a.restartPending != "" {
			keys = append(keys, [2]string{"r", i18n.T("neu starten")})
		} else {
			keys = append(keys, [2]string{"c", i18n.T("nach Updates suchen")})
		}
		keys = append(keys, [2]string{"w", i18n.T("was ist neu")}, [2]string{"b", i18n.T("♥ Kaffee spendieren")}, [2]string{"l", langTarget()}, [2]string{"Esc", i18n.T("schließen")})
	case mChangelog:
		keys = [][2]string{{"b", i18n.T("♥ Kaffee spendieren")}, {"Esc", i18n.T("schließen")}}
	case mHelp, mInfo:
		keys = [][2]string{{"Esc", i18n.T("schließen")}}
	case mCrash:
		keys = [][2]string{{"g", i18n.T("als Issue melden")}, {"o", i18n.T("Bericht öffnen")}, {"Esc", i18n.T("schließen")}}
	case mLLM:
		keys = [][2]string{{"p", i18n.T("jetzt prüfen")}, {"e", i18n.T("Einstellungen")}, {"r", i18n.T("Ersatz-Anbieter")}, {"Esc", i18n.T("schließen")}}
	case mTour:
		keys = tourButtons(a.modal)
	case mQuick:
		keys = [][2]string{{"⏎", i18n.T("ausführen")}, {"e", i18n.T("bearbeiten")}, {"Esc", i18n.T("schließen")}}
		if a.modal.scroll >= maxQuick {
			keys = [][2]string{{"⏎", i18n.T("ausführen")}, {"a", i18n.T("als Schnellbefehl")}, {"x", i18n.T("nicht vorschlagen")}, {"Esc", i18n.T("schließen")}}
		}
	case mDanger:
		keys = [][2]string{{yesKey(), i18n.T("trotzdem ausführen")}, {"Esc", i18n.T("abbrechen")}}
	case mPaste:
		keys = [][2]string{{"⏎", i18n.T("einfügen")}}
		if a.modal.paste.multi {
			keys = append(keys, [2]string{"o", i18n.T("ohne letztes ⏎")})
		}
		keys = append(keys, [2]string{"Esc", i18n.T("abbrechen")})
	}
	return keys
}

// mouseHelp: Abschnitt für die Hilfe (F1).
const mouseHelp = `Maus         Klick auf Tabs, Listen, Vorschläge, Felder, Fußzeile · Doppelklick öffnet
             Terminal: Ziehen markiert + kopiert · Doppel-/Dreifachklick Wort/Zeile
             Rechtsklick fügt ein, auf Link/IP/Host öffnet er ein Menü · Strg+Klick öffnet URL
             Rad blättert im Verlauf (Esc zurück)
             htop, mc, vim, tmux bekommen die Maus · Shift+Ziehen markiert trotzdem
             Palette „Maus aus“: Terminalfenster markiert wieder selbst`
