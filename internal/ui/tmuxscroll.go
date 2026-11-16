package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Zurückblättern in tmux (feste Sitzung oder selbst gestartet)
//
// tmux zeichnet im Alt-Bildschirm, den Verlauf hat tmux, nicht sshTermy.
// Ohne `set -g mouse on` kommt das Mausrad bei tmux nur als Pfeiltasten an
// (blättert dann durch die Shell-Historie). Deshalb schaltet sshTermy tmux
// selbst in den Copy-Mode: über einen zweiten, unsichtbaren SSH-Kanal
// (wie die Systemabfrage), ganz ohne Präfix-Taste. Läuft in tmux gerade
// ein Vollbild-Programm (less, vim), bekommt es wie bisher Pfeiltasten.
// Rad und ⇧Bild↑/↓, ⇧Pos1, ⇧Ende wirken so wie ohne tmux; Esc oder q
// verlassen den Copy-Mode, ganz nach unten blättern auch.

// tmuxOp: eine Blätter-Aktion für tmux.
type tmuxOp int

const (
	tmuxWheelUp tmuxOp = iota
	tmuxWheelDown
	tmuxPageUp
	tmuxPageDown
	tmuxTop
	tmuxBottom
)

// tmuxTarget: Ziel für -t; ok = false bei Namen, die sich nicht sicher
// in ein Shell-Skript schreiben lassen.
func tmuxTarget(name string) (string, bool) {
	if name == "" {
		return "", true
	}
	if q := shellQuote(name); q != name {
		return "", false
	}
	return " -t " + name, true
}

// tmuxScript baut das Shell-Skript für n Wiederholungen von op.
func tmuxScript(name string, op tmuxOp, n int) (string, bool) {
	t, ok := tmuxTarget(name)
	if !ok || n < 1 {
		return "", false
	}
	n = min(n, 500)
	// #{pane_in_mode}#{alternate_on}: "1…" = Copy-Mode, "01" = Vollbild-Programm
	head := fmt.Sprintf("m=$(tmux display-message -p%s '#{pane_in_mode}#{alternate_on}' 2>/dev/null) || exit 0; ", t)
	keys := func(k string) string { return strings.TrimSpace(strings.Repeat(k+" ", n)) }
	var inMode, inApp, plain string
	switch op {
	case tmuxWheelUp:
		inMode = fmt.Sprintf("tmux send-keys -X -N %d%s scroll-up", n, t)
		inApp = fmt.Sprintf("tmux send-keys%s %s", t, keys("Up"))
		plain = fmt.Sprintf("tmux copy-mode -e%s \\; send-keys -X -N %d%s scroll-up", t, n, t)
	case tmuxWheelDown:
		inMode = fmt.Sprintf("tmux send-keys -X -N %d%s scroll-down", n, t)
		inApp = fmt.Sprintf("tmux send-keys%s %s", t, keys("Down"))
	case tmuxPageUp:
		inMode = fmt.Sprintf("tmux send-keys -X -N %d%s page-up", n, t)
		inApp = fmt.Sprintf("tmux send-keys%s %s", t, keys("PPage"))
		plain = fmt.Sprintf("tmux copy-mode -e%s \\; send-keys -X -N %d%s page-up", t, n, t)
	case tmuxPageDown:
		inMode = fmt.Sprintf("tmux send-keys -X -N %d%s page-down", n, t)
		inApp = fmt.Sprintf("tmux send-keys%s %s", t, keys("NPage"))
	case tmuxTop:
		inMode = fmt.Sprintf("tmux send-keys -X%s history-top", t)
		plain = fmt.Sprintf("tmux copy-mode -e%s \\; send-keys -X%s history-top", t, t)
	case tmuxBottom:
		inMode = fmt.Sprintf("tmux send-keys -X%s cancel", t)
	}
	or := func(s string) string {
		if s == "" {
			return ":"
		}
		return s
	}
	return head + fmt.Sprintf(`case "$m" in 1*) %s;; 01) %s;; *) %s;; esac`, or(inMode), or(inApp), or(plain)), true
}

// tmuxScroller arbeitet die Blätter-Aktionen eines Tabs der Reihe nach ab;
// schnelles Drehen am Rad wird zu einer Aktion zusammengefasst.
type tmuxScroller struct {
	mu      sync.Mutex
	pending []tmuxJob
	running bool
}

type tmuxJob struct {
	op tmuxOp
	n  int
}

// add reiht op ein und startet den Arbeiter, falls er ruht.
func (q *tmuxScroller) add(c *sshx.Client, name string, op tmuxOp) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if l := len(q.pending); l > 0 && q.pending[l-1].op == op {
		q.pending[l-1].n++
	} else if l < 32 {
		q.pending = append(q.pending, tmuxJob{op, 1})
	}
	if q.running {
		return
	}
	q.running = true
	go func() {
		for {
			q.mu.Lock()
			if len(q.pending) == 0 {
				q.running = false
				q.mu.Unlock()
				return
			}
			j := q.pending[0]
			q.pending = q.pending[1:]
			q.mu.Unlock()
			if script, ok := tmuxScript(name, j.op, j.n); ok {
				_, _ = c.Output(script, 5*time.Second)
			}
		}
	}()
}

// inTmux: Der Tab zeigt gerade tmux, und sshTermy kann es fernsteuern.
func (s *session) inTmux() bool {
	if s.mux.kind != "tmux" || s.term == nil || !s.term.AltScreen() || s.client == nil || s.client.NetDevice || s.detached() {
		return false
	}
	_, ok := tmuxTarget(s.mux.name)
	return ok
}

// tmuxWheel: Rad in tmux ohne Maus-Modus. true = erledigt.
func (a *App) tmuxWheel(s *session, up bool) bool {
	if !s.inTmux() || s.term.MouseTracking() != termemu.MouseOff {
		return false
	}
	op := tmuxWheelDown
	if up {
		op = tmuxWheelUp
		a.tmuxHintOnce(s)
	}
	s.tmuxScroll.add(s.client, s.mux.name, op)
	return true
}

// tmuxKey: ⇧Bild↑/↓, ⇧Pos1, ⇧Ende in tmux. true = erledigt.
func (a *App) tmuxKey(s *session, act action) bool {
	if !s.inTmux() {
		return false
	}
	op := map[action]tmuxOp{actScrollUp: tmuxPageUp, actScrollDown: tmuxPageDown, actScrollTop: tmuxTop, actScrollBottom: tmuxBottom}[act]
	if act == actScrollUp || act == actScrollTop {
		a.tmuxHintOnce(s)
	}
	s.tmuxScroll.add(s.client, s.mux.name, op)
	return true
}

// tmuxHintOnce: einmal je Tab erklären, was passiert.
func (a *App) tmuxHintOnce(s *session) {
	if s.tmuxHinted {
		return
	}
	s.tmuxHinted = true
	a.flash(i18n.T("tmux: Verlauf im Copy-Mode · markieren kopiert · Esc/q oder ganz nach unten beendet"))
}
