package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Suche im Verlauf (Strg+F)
//
// Die Suchzeile ersetzt die Vorschlagszeile. Treffer werden gelb markiert,
// der aktuelle orange; ⏎ springt zum nächsten (nach oben, also älter),
// Shift+⏎ zurück. Strg+E schaltet den Fehlerfilter ein: Dann zählen nur
// Zeilen, die nach Fehlern aussehen, und die Fläche zeigt sie als Liste.
// Tab wechselt zwischen Liste und Verlauf, ⏎ in der Liste springt an die
// Stelle im Verlauf. Esc schließt die Suche und kehrt zur Live-Ansicht
// zurück.

type searchState struct {
	open    bool
	in      textinput.Model
	errors  bool // Fehlerfilter
	list    bool // Fehlerliste statt Verlauf
	matches []termemu.Match
	idx     int

	// für die Neuberechnung
	query   string
	qErrors bool
	at      time.Time
}

func (a *App) openSearch(s *session) tea.Cmd {
	if s.term == nil {
		return nil
	}
	if s.search.open {
		return s.search.in.Focus()
	}
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = i18n.T("Suchbegriff (Großbuchstaben = genau)")
	in.CharLimit = 200
	s.search = searchState{open: true, in: in, idx: -1}
	s.panelFocus = false
	return s.search.in.Focus()
}

func (a *App) closeSearch(s *session) {
	s.search = searchState{}
	s.scroll, s.sel = 0, nil
}

// searchKey bearbeitet Tasten bei offener Suche. ok = false: Taste gehört
// nicht der Suche (F-Tasten, Strg+Pfeil …), sie läuft normal weiter.
func (a *App) searchKey(s *session, k tea.KeyPressMsg) (tea.Cmd, bool) {
	sr := &s.search
	key := k.String()
	if k.Code >= tea.KeyF1 && k.Code <= tea.KeyF12 {
		return nil, false
	}
	if _, ok := paneDir(key); ok && a.split != splitOff {
		return nil, false
	}
	switch key {
	case "esc":
		a.closeSearch(s)
		return nil, true
	case "ctrl+f":
		// zweites Strg+F: Text markieren = neu anfangen
		sr.in.SetValue("")
		a.refreshSearch(s, true)
		return nil, true
	case "ctrl+e":
		sr.errors = !sr.errors
		sr.list = sr.errors
		a.refreshSearch(s, true)
		a.revealMatch(s)
		return nil, true
	case "tab":
		if sr.errors {
			sr.list = !sr.list
			a.revealMatch(s)
		}
		return nil, true
	case "enter", "up", "ctrl+p":
		if sr.list && key == "enter" {
			sr.list = false
			a.revealMatch(s)
			return nil, true
		}
		a.stepMatch(s, -1)
		return nil, true
	case "shift+enter", "alt+enter", "down", "ctrl+n":
		a.stepMatch(s, 1)
		return nil, true
	case "pgup":
		if sr.list {
			a.stepMatch(s, -10)
		} else if s.term != nil {
			_, h := s.term.Size()
			s.scroll = clamp(s.scroll+h-1, 0, s.term.ScrollbackLen())
		}
		return nil, true
	case "pgdown":
		if sr.list {
			a.stepMatch(s, 10)
		} else if s.term != nil {
			_, h := s.term.Size()
			s.scroll = clamp(s.scroll-h+1, 0, s.term.ScrollbackLen())
		}
		return nil, true
	}
	if k.Mod.Contains(tea.ModCtrl) && key != "ctrl+a" && key != "ctrl+e" && key != "ctrl+w" && key != "ctrl+u" &&
		key != "ctrl+k" && key != "ctrl+left" && key != "ctrl+right" && key != "ctrl+h" {
		return nil, false // Strg+Q, Strg+Space usw. bleiben global
	}
	var cmd tea.Cmd
	sr.in, cmd = sr.in.Update(k)
	a.refreshSearch(s, false)
	return cmd, true
}

func (a *App) searchPaste(s *session, m tea.PasteMsg) tea.Cmd {
	var cmd tea.Cmd
	s.search.in, cmd = s.search.in.Update(tea.PasteMsg{Content: oneLine(m.Content)})
	a.refreshSearch(s, false)
	return cmd
}

// refreshSearch rechnet die Treffer neu, wenn sich Suchbegriff, Filter oder
// die Ausgabe geändert haben (Ausgabe höchstens alle 300 ms).
func (a *App) refreshSearch(s *session, force bool) {
	sr := &s.search
	if !sr.open || s.term == nil {
		return
	}
	q := sr.in.Value()
	changed := force || q != sr.query || sr.errors != sr.qErrors
	if !changed && !(s.lastOutput.After(sr.at) && time.Since(sr.at) > 300*time.Millisecond) {
		return
	}
	var keep *termemu.Match
	atEnd := sr.idx < 0 || sr.idx >= len(sr.matches)-1
	if !changed && sr.idx >= 0 && sr.idx < len(sr.matches) {
		m := sr.matches[sr.idx]
		keep = &m
	}
	sr.query, sr.qErrors, sr.at = q, sr.errors, time.Now()
	sr.matches = s.term.Search(q, sr.errors)
	sr.idx = len(sr.matches) - 1 // neuester Treffer
	if keep != nil && !atEnd {
		for i, m := range sr.matches {
			if m == *keep {
				sr.idx = i
				break
			}
		}
	}
	if changed {
		a.revealMatch(s)
	}
}

func (a *App) stepMatch(s *session, d int) {
	sr := &s.search
	n := len(sr.matches)
	if n == 0 {
		return
	}
	if sr.list {
		sr.idx = clamp(sr.idx+d, 0, n-1)
		return
	}
	sr.idx = ((sr.idx+d)%n + n) % n
	if d < 0 && sr.idx == n-1 && n > 1 {
		a.flash(i18n.T("Suche: wieder von unten (neuester Treffer)"))
	} else if d > 0 && sr.idx == 0 && n > 1 {
		a.flash(i18n.T("Suche: wieder von oben (ältester Treffer)"))
	}
	a.revealMatch(s)
}

// revealMatch blättert so, dass der aktuelle Treffer sichtbar ist.
func (a *App) revealMatch(s *session) {
	sr := &s.search
	if s.term == nil || sr.idx < 0 || sr.idx >= len(sr.matches) {
		return
	}
	m := sr.matches[sr.idx]
	sb := s.term.ScrollbackLen()
	_, h := s.term.Size()
	first := sb - s.scroll
	if m.Line >= first && m.Line < first+h {
		return
	}
	first = clamp(m.Line-h/2, 0, sb)
	s.scroll = sb - first
	s.sbSeen = sb
}

// searchMarks: Treffer im sichtbaren Ausschnitt.
func (a *App) searchMarks(s *session) []termemu.Mark {
	sr := &s.search
	sb := s.term.ScrollbackLen()
	_, h := s.term.Size()
	first := sb - min(s.scroll, sb)
	var out []termemu.Mark
	for i, m := range sr.matches {
		if m.Line < first || m.Line >= first+h {
			continue
		}
		out = append(out, termemu.Mark{Line: m.Line, Col: m.Col, Len: m.Len, Current: i == sr.idx})
	}
	return out
}

func (a *App) markStyle() termemu.MarkStyle {
	t := a.s.T
	return termemu.MarkStyle{
		Hit:     uv.Style{Fg: t.Bg, Bg: t.Warn},
		Current: uv.Style{Fg: t.Bg, Bg: t.Danger, Attrs: uv.AttrBold},
	}
}

// viewErrorList: nur die Fehlerzeilen, älteste oben, aktuelle markiert.
func (a *App) viewErrorList(s *session, w, h int) string {
	st := a.s
	sr := &s.search
	if len(sr.matches) == 0 {
		msg := i18n.T("Keine Fehlerzeilen im Verlauf")
		if sr.in.Value() != "" {
			msg = i18n.T("Keine Fehlerzeilen mit „") + sr.in.Value() + "“"
		}
		return fitBlock("\n "+st.OK.Render("✓ "+msg)+"\n\n "+a.hints(st.Muted, [2]string{"Tab", i18n.T("zeigt den Verlauf")}, [2]string{"Strg+E", i18n.T("Filter aus")}, [2]string{"Esc", i18n.T("schließen")}), w, h)
	}
	head := st.Accent.Render(fmt.Sprintf(i18n.T(" ⚠ %s im Verlauf"), plural(len(sr.matches), i18n.T("Fehlerzeile"), i18n.T("Fehlerzeilen")))) +
		st.Muted.Render("  ") + a.hints(st.Muted, [2]string{"↑↓", i18n.T("wählen")}, [2]string{"⏎", i18n.T("im Verlauf zeigen")}, [2]string{"Tab", i18n.T("Verlauf")})
	rows := max(1, h-1)
	start := clamp(sr.idx-rows/2, 0, max(0, len(sr.matches)-rows))
	numW := len(fmt.Sprint(s.term.Lines()))
	lines := []string{head}
	for i := start; i < len(sr.matches) && len(lines) < h; i++ {
		m := sr.matches[i]
		num := fmt.Sprintf(" %*d │ ", numW, m.Line+1)
		text := ansi.Truncate(s.term.PlainLine(m.Line), max(1, w-len([]rune(num))-1), "…")
		line := st.Muted.Render(num) + st.Danger.UnsetBold().Render(text)
		if i == sr.idx {
			line = st.Selected.Render(padRight(num+text, w))
		}
		i := i
		lines = append(lines, a.z.click(line, func(_ tea.Mouse, double bool) tea.Cmd {
			sr.idx = i
			if double {
				sr.list = false
				a.revealMatch(s)
			}
			return nil
		}))
	}
	return fitBlock(strings.Join(lines, "\n"), w, h)
}

// searchBar: Suchzeile anstelle der Vorschläge.
func (a *App) searchBar(s *session) string {
	st := a.s
	sr := &s.search
	label := i18n.T(" ⌕ Suche: ")
	if sr.errors {
		label = i18n.T(" ⚠ Fehler: ")
	}
	count := st.Muted.Render(i18n.T(" keine Treffer "))
	if n := len(sr.matches); n > 0 {
		count = st.Accent.Render(fmt.Sprintf(" %d/%d ", sr.idx+1, n))
	} else if sr.in.Value() == "" && !sr.errors {
		count = ""
	}
	right := ""
	if sr.errors {
		right = a.s.Warn.Render(i18n.T("nur Fehlerzeilen "))
	}
	sr.in.SetWidth(max(10, a.w-ansi.StringWidth(label)-ansi.StringWidth(count)-ansi.StringWidth(right)-2))
	out := st.Accent.Render(label) + sr.in.View() + count
	gap := a.w - ansi.StringWidth(out) - ansi.StringWidth(right)
	if gap > 0 {
		out += strings.Repeat(" ", gap) + right
	}
	return ansi.Truncate(padRight(out, a.w), a.w, "")
}
