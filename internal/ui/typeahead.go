package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Vorausgetippt: Wer gleich nach der Auswahl eines Servers lostippt, ist
// schneller als der Verbindungsaufbau. Früher gingen diese Tasten
// verloren (die erste Eingabe „wurde nicht angenommen“), ein „x“ schloss
// sogar den Tab und ⏎ startete die Verbindung ein zweites Mal.
//
// Jetzt: Text wird gesammelt und auf der Karte „Verbinde mit …“ gezeigt.
// Steht der Prompt, tippt sshTermy ihn dort ein. ⏎ wird nie vorausgeschickt
// (der Gefahren-Check sieht die Zeile erst am Prompt), Esc bricht ab.
// Kommt statt eines Prompts eine Passwort- oder Login-Abfrage, wird der
// Text verworfen statt dort zu landen.

const (
	aheadMax  = 256              // Zeichen
	aheadWait = 8 * time.Second // länger kein Prompt: verwerfen
)

// connectingKey: Taste, während die Verbindung noch aufgebaut wird.
func (a *App) connectingKey(s *session, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		a.closeTab(s)
		return nil
	case "backspace":
		if n := len(s.ahead); n > 0 {
			s.ahead = s.ahead[:n-1]
		}
		return nil
	case "ctrl+u":
		s.ahead = nil
		return nil
	}
	if k.Text == "" || k.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModMeta|tea.ModSuper) != 0 {
		return nil // ⏎, Pfeile, Kürzel: erst nach dem Verbinden
	}
	for _, r := range k.Text {
		if r >= 0x20 && r != 0x7f && len(s.ahead) < aheadMax {
			s.ahead = append(s.ahead, r)
		}
	}
	return nil
}

// flushAhead: Vorausgetipptes an den Prompt, sobald er steht (Sekundentakt
// und neue Ausgabe). Startbefehl, Abfragen und tmux/screen haben Vorrang.
func (a *App) flushAhead(s *session) {
	if len(s.ahead) == 0 {
		return
	}
	if s.state != stConnected && s.state != stConnecting {
		s.ahead = nil // Verbindung gescheitert oder Tab zu
		return
	}
	if s.state != stConnected || s.term == nil {
		return
	}
	if s.detached() {
		s.ahead = nil // eigenes Fenster: dort wird getippt
		return
	}
	busy := s.startPending || s.muxPending || s.cap.active()
	if !busy && s.term.AtIdlePrompt() && !agent.LooksLikeSecretPrompt(s.term.CursorLine()) &&
		time.Since(s.lastOutput) > 250*time.Millisecond {
		txt := string(s.ahead)
		s.ahead = nil
		s.term.SendText(txt)
		return
	}
	if !s.connectedAt.IsZero() && time.Since(s.connectedAt) > aheadWait {
		s.ahead = nil
		a.flash(i18n.T("Vorausgetippter Text verworfen: kein Prompt erkannt"))
	}
}

type aheadMsg struct{ sid int }

// aheadTick: nach neuer Ausgabe kurz warten (Prompt fertig?) und dann
// prüfen, statt bis zum nächsten Sekundentakt.
func (a *App) aheadTick(s *session) tea.Cmd {
	if len(s.ahead) == 0 || s.aheadTicking {
		return nil
	}
	s.aheadTicking = true
	sid := s.id
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return aheadMsg{sid} })
}

func (a *App) onAhead(m aheadMsg) tea.Cmd {
	s := a.byID(m.sid)
	if s == nil {
		return nil
	}
	s.aheadTicking = false
	a.flushAhead(s)
	return nil
}
