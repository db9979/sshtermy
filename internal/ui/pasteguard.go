package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safety"
)

// Einfügeschutz
//
// Eingefügter Text landet sonst ungesehen in der Shell, und jede Zeile mit
// Zeilenumbruch läuft sofort. Deshalb:
//
//   - Mehrzeiliges Einfügen (auch eine Zeile mit Enter am Ende) zeigt erst
//     eine Vorschau. ⏎ fügt ein, „o“ fügt ohne das letzte Enter ein, Esc
//     bricht ab. Abschaltbar über die Palette („Einfügeschutz aus“).
//   - Die festen Gefahrenregeln (internal/safety) prüfen jede eingefügte
//     Zeile. Ein Treffer färbt die Zeile rot und verlangt einen zweiten
//     Druck, auch bei einer einzelnen Zeile. Das lässt sich nicht abschalten.
//   - In Vollbild-Programmen (vim, nano, less) wird nichts ausgeführt; dort
//     fügt sshTermy direkt ein.

type pasteLine struct {
	text   string
	danger []safety.Finding
}

type pasteCheck struct {
	sid       int
	text      string
	lines     []pasteLine
	dangerous int  // Zeilen mit Treffer
	multi     bool // enthält Zeilenumbrüche (auch am Ende)
	armed     bool // gefährlich: erster Druck erfolgt
}

// checkPaste wendet die Regeln auf den Text an.
func checkPaste(text, kind string) pasteCheck {
	norm := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	pc := pasteCheck{text: text, multi: strings.Contains(norm, "\n")}
	for _, l := range strings.Split(strings.TrimRight(norm, "\n"), "\n") {
		pl := pasteLine{text: l, danger: safety.Check(l, kind)}
		if len(pl.danger) > 0 {
			pc.dangerous++
		}
		pc.lines = append(pc.lines, pl)
	}
	return pc
}

func (a *App) pasteGuardEnabled() bool { return !a.st.Settings().NoPasteGuard }

func (a *App) togglePasteGuard() tea.Cmd {
	on := !a.pasteGuardEnabled()
	if err := a.st.UpdateSettings(func(s *domain.Settings) { s.NoPasteGuard = !on }); err != nil {
		a.flash(i18n.T("Einstellung nicht gespeichert: ") + err.Error())
		return nil
	}
	if on {
		a.flash(i18n.T("Einfügeschutz an: mehrzeiliges Einfügen zeigt erst eine Vorschau."))
	} else {
		a.flash(i18n.T("Einfügeschutz aus. Gefährliche Befehle fragen trotzdem nach."))
	}
	return nil
}

func pasteGuardLabel(on bool) string {
	if on {
		return i18n.T("Einfügeschutz aus (mehrzeilig ohne Vorschau)")
	}
	return i18n.T("Einfügeschutz an (Vorschau bei mehreren Zeilen)")
}

// guardedPaste fügt ein oder zeigt zuerst die Vorschau.
func (a *App) guardedPaste(s *session, text string) tea.Cmd {
	if s.term == nil || s.state != stConnected || text == "" {
		return nil
	}
	if s.term.AltScreen() {
		s.sel, s.scroll = nil, 0
		s.term.Paste(text)
		return nil
	}
	pc := checkPaste(text, a.safetyKind(s))
	pc.sid = s.id
	if pc.dangerous == 0 && (!pc.multi || !a.pasteGuardEnabled()) {
		s.sel, s.scroll = nil, 0
		s.term.Paste(text)
		return nil
	}
	a.modal = &modal{kind: mPaste, title: i18n.T("Einfügen prüfen"), paste: &pc}
	if pc.dangerous > 0 {
		a.modal.title = i18n.T("⚠ Gefährlicher Befehl beim Einfügen")
	}
	return nil
}

func (a *App) pasteKey(key string) tea.Cmd {
	pc := a.modal.paste
	s := a.byID(pc.sid)
	do := func(text string) tea.Cmd {
		if pc.dangerous > 0 && !pc.armed {
			pc.armed = true
			return nil
		}
		a.modal = nil
		if s != nil && s.term != nil && s.state == stConnected {
			s.sel, s.scroll = nil, 0
			s.term.Paste(text)
		}
		return nil
	}
	switch key {
	case "enter", "j", "y":
		return do(pc.text)
	case "o":
		if pc.multi {
			return do(strings.TrimRight(pc.text, "\r\n"))
		}
	case "esc", "n", "q":
		a.modal = nil
	}
	return nil
}

const pastePreviewLines = 12

func (a *App) pasteView(m *modal) string {
	st := a.s
	pc := m.paste
	inner := dialogInner(a.w)
	var b strings.Builder
	d := dialog{icon: "⎘", title: m.title, width: inner, keys: a.modalKeys()}
	if pc.dangerous > 0 {
		d.icon, d.tone = "⚠", st.T.Danger
	}
	n := len(pc.lines)
	info := plural(n, i18n.T("Zeile"), i18n.T("Zeilen")) + " · " + plural(utf8.RuneCountInString(pc.text), i18n.T("Zeichen"), i18n.T("Zeichen"))
	if pc.multi {
		info += i18n.T(" · jede Zeile kann sofort laufen")
	}
	b.WriteString(st.Muted.Render(info) + "\n\n")

	// Vorschau: gefährliche Zeilen zuerst sichtbar halten
	show := make([]int, 0, pastePreviewLines)
	for i, l := range pc.lines {
		if len(l.danger) > 0 && len(show) < pastePreviewLines {
			show = append(show, i)
		}
	}
	for i := 0; i < n && len(show) < pastePreviewLines; i++ {
		if len(pc.lines[i].danger) == 0 {
			show = append(show, i)
		}
	}
	sort.Ints(show)
	numW := len(fmt.Sprint(n))
	prev := -1
	for _, i := range show {
		if prev >= 0 && i > prev+1 {
			b.WriteString(st.Muted.Render(strings.Repeat(" ", numW)+" ┆") + "\n")
		}
		prev = i
		l := pc.lines[i]
		num := st.Muted.Render(fmt.Sprintf("%*d │ ", numW, i+1))
		txt := strings.ReplaceAll(l.text, "\t", "  ")
		if len(l.danger) > 0 {
			b.WriteString(num + st.Danger.Render(trunc(txt, inner-numW-3)) + "\n")
			b.WriteString(st.Muted.Render(strings.Repeat(" ", numW)+" │ ") + st.Danger.Render("⚠ "+l.danger[0].Reason) + "\n")
		} else {
			b.WriteString(num + st.Text.Render(trunc(txt, inner-numW-3)) + "\n")
		}
	}
	if rest := n - len(show); rest > 0 {
		b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("… und %s mehr"), plural(rest, i18n.T("Zeile"), i18n.T("Zeilen")))) + "\n")
	}
	b.WriteString("\n")
	switch {
	case pc.dangerous > 0 && pc.armed:
		b.WriteString(st.Danger.Render(i18n.T("Wirklich? Nochmal ⏎ fügt ein, Esc bricht ab.")))
	case pc.dangerous > 0:
		b.WriteString(st.Warn.Render(fmt.Sprintf(i18n.T("Die Gefahrenregeln treffen auf %s zu. ⏎ zweimal drücken zum Einfügen."), plural(pc.dangerous, i18n.T("Zeile"), i18n.T("Zeilen")))))
	default:
		b.WriteString(st.Muted.Render(i18n.T("Prüfe den Text, bevor er in die Shell geht.")))
	}
	d.body = b.String()
	return st.dialog(d, a.w)
}
