package ui

// Befehle als Blöcke (2026-11-12): Jeder fertige Befehl im Tab wird mit
// seiner Ausgabe gemerkt (cmdlog.go, auch ohne Sitzungsprotokoll; Befehle
// mit Leerzeichen davor nicht). Alt+Bild↑/↓ springt im Verlauf von Befehl zu
// Befehl, Alt+O zeigt die Liste: ⏎ springt hin, Strg+C kopiert die Ausgabe,
// Strg+K gibt Befehl und Ausgabe an „KI fragen“, Strg+L zeigt sie groß.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

type cmdBlock struct {
	cmd    string
	out    string
	at     time.Time
	dur    time.Duration
	failed bool
	exit   *int
	line   int // Zeile im Verlauf beim Start (verschiebt sich, wenn der Verlauf voll ist)
}

const maxBlocks = 200

// promptLine: Zeile des Cursors im gesamten Verlauf.
func (s *session) promptLine() int {
	if s.term == nil {
		return -1
	}
	_, y, _ := s.term.Cursor()
	return s.term.ScrollbackLen() + y
}

// addBlock (aus finishCommand).
func (s *session) addBlock(r *cmdRun, out string, failed bool, exit *int, dur time.Duration) {
	if r == nil || r.private || strings.TrimSpace(r.cmd) == "" {
		return
	}
	s.blocks = append(s.blocks, cmdBlock{cmd: r.cmd, out: out, at: r.at, dur: dur, failed: failed, exit: exit, line: r.line})
	if n := len(s.blocks); n > maxBlocks {
		s.blocks = append(s.blocks[:0:0], s.blocks[n-maxBlocks:]...)
		s.blockNav = 0
	}
}

// locateBlock: in welcher Zeile steht der Befehl jetzt? Gesucht wird die
// Zeile mit dem Befehlstext, die der gemerkten am nächsten liegt (-1 = nicht
// mehr im Verlauf).
func (s *session) locateBlock(b cmdBlock) int {
	if s.term == nil {
		return -1
	}
	q := b.cmd
	if r := []rune(q); len(r) > 60 {
		q = string(r[:60])
	}
	best, bestD := -1, 1<<30
	for _, m := range s.term.Search(q, false) {
		d := m.Line - b.line
		if d < 0 {
			d = -d
		}
		if d < bestD {
			best, bestD = m.Line, d
		}
	}
	return best
}

// jumpBlock: Alt+Bild↑ (d = -1) / Alt+Bild↓ (d = +1).
func (a *App) jumpBlock(s *session, d int) {
	if s.term == nil {
		return
	}
	n := len(s.blocks)
	if n == 0 {
		a.flash(i18n.T("Noch keine fertigen Befehle in diesem Tab"))
		return
	}
	cur := n // live
	if s.blockNav > 0 && s.scroll == s.blockScroll {
		cur = s.blockNav - 1
	}
	i := cur + d
	if i >= n {
		s.blockNav, s.scroll = 0, 0
		a.flash(i18n.T("Zurück zur Live-Ansicht"))
		return
	}
	if i < 0 {
		i = 0
	}
	a.revealBlock(s, i)
}

func (a *App) revealBlock(s *session, i int) {
	b := s.blocks[i]
	ln := s.locateBlock(b)
	if ln < 0 {
		s.blockNav, s.blockScroll = i+1, s.scroll
		a.flash(fmt.Sprintf(i18n.T("Befehl %d/%d ist nicht mehr im Verlauf: %s"), i+1, len(s.blocks), trunc(b.cmd, 40)))
		return
	}
	sb := s.term.ScrollbackLen()
	first := clamp(ln-1, 0, sb)
	s.scroll, s.sbSeen = sb-first, sb
	s.blockNav, s.blockScroll = i+1, s.scroll
	mark := "✓"
	if b.failed {
		mark = "×"
	}
	a.flash(fmt.Sprintf(i18n.T("Befehl %d/%d %s %s · Alt+O Liste"), i+1, len(s.blocks), mark, trunc(b.cmd, 50)))
}

// openBlocks: Alt+O, die Befehle dieses Tabs, neueste oben.
func (a *App) openBlocks(s *session) tea.Cmd {
	if len(s.blocks) == 0 {
		a.flash(i18n.T("Noch keine fertigen Befehle in diesem Tab"))
		return nil
	}
	var items []pickItem
	for i := len(s.blocks) - 1; i >= 0; i-- {
		b := s.blocks[i]
		hint := "✓"
		switch {
		case b.exit != nil && *b.exit != 0:
			hint = fmt.Sprintf("× %d", *b.exit)
		case b.failed:
			hint = "×"
		}
		detail := b.at.Format("15:04")
		if b.dur >= time.Second {
			detail += " · " + fmtDur(b.dur)
		}
		if l := lastLine(b.out); l != "" {
			detail += " · " + trunc(l, 40)
		}
		items = append(items, pickItem{value: strconv.Itoa(i), label: "$ " + trunc(b.cmd, max(20, a.w-50)), detail: detail, hint: hint, search: b.out})
	}
	p := newPicker(fmt.Sprintf(i18n.T("Befehle in %s · %d"), trunc(s.tabName(), 24), len(s.blocks)), i18n.T("Befehl oder Ausgabe suchen"), items, "\x00", func(it pickItem) tea.Cmd {
		if i, err := strconv.Atoi(it.value); err == nil && i < len(s.blocks) {
			a.revealBlock(s, i)
		}
		return nil
	})
	p.keys = [][2]string{{"⏎", i18n.T("hinspringen")}, {"Strg+C", i18n.T("Ausgabe kopieren")}, {"Strg+K", i18n.T("KI fragen")}, {"Strg+L", i18n.T("ganz lesen")}, {"Esc", i18n.T("zurück")}}
	p.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		if it == nil {
			return nil, false
		}
		i, err := strconv.Atoi(it.value)
		if err != nil || i >= len(s.blocks) {
			return nil, false
		}
		b := s.blocks[i]
		switch key {
		case "ctrl+c":
			a.pick = nil
			return a.copyBlock(b), true
		case "ctrl+k":
			a.pick = nil
			return a.askBlock(s, b), true
		case "ctrl+l":
			a.pick = nil
			a.readBlock(b)
			return nil, true
		}
		return nil, false
	}
	return a.openPicker(p)
}

func (a *App) copyBlock(b cmdBlock) tea.Cmd {
	text := strings.TrimRight(b.out, "\n")
	go func() { _ = clipboard.WriteAll(text) }()
	a.flash(i18n.T("Kopiert: Ausgabe von ") + trunc(b.cmd, 40) + " (" + plural(countLines(text), i18n.T("Zeile"), i18n.T("Zeilen")) + ")")
	return tea.SetClipboard(text)
}

func (a *App) readBlock(b cmdBlock) {
	w := max(20, dialog{maxW: 200}.innerMax(a.w))
	text := "$ " + b.cmd + "\n" + strings.TrimRight(b.out, "\n")
	m := textModal("$ "+trunc(b.cmd, 50), strings.Split(ansi.Wrap(text, w, ""), "\n"))
	m.copy = strings.TrimRight(b.out, "\n")
	a.modal = m
}

// askBlock: Befehl und Ausgabe an „KI fragen“.
func (a *App) askBlock(s *session, b cmdBlock) tea.Cmd {
	out := strings.TrimRight(b.out, "\n")
	if r := []rune(out); len(r) > 6000 {
		out = "…\n" + string(r[len(r)-6000:])
	}
	q := fmt.Sprintf(i18n.T("Erkläre die Ausgabe dieses Befehls und sag, ob etwas nicht stimmt:\n$ %s\n%s"), b.cmd, out)
	return a.startAsk(s, q)
}
