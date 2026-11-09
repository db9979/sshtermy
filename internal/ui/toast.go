package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Toasts: kurze Einblendungen oben rechts („verbunden“, „kopiert“, Fehler),
// die von selbst verschwinden. Sie liegen über dem Bild, verdrängen also
// weder die Tabs in der Kopfzeile noch den Terminalinhalt dauerhaft.
// Neueste oben, höchstens maxToasts gleichzeitig; ein Klick schließt.

type toastKind int

const (
	toastInfo toastKind = iota
	toastOK
	toastWarn
	toastErr
)

const (
	maxToasts = 3
	toastFade = 700 * time.Millisecond // so lange vor dem Ende blasst er ab
	toastIn   = 250 * time.Millisecond // so lange gleitet er herein
)

type toastItem struct {
	text        string
	kind        toastKind
	born, until time.Time
}

// toastLife: Fehler bleiben länger stehen, man will sie lesen können.
func toastLife(k toastKind) time.Duration {
	switch k {
	case toastErr:
		return 8 * time.Second
	case toastWarn:
		return 6 * time.Second
	}
	return 4 * time.Second
}

// flash zeigt eine Meldung als Toast. Die Art (Erfolg, Warnung) wird aus
// dem Anfang des Textes erraten; Fehler melden flashErr.
func (a *App) flash(s string) { a.pushToast(guessToastKind(s), s) }

// flashErr zeigt einen Fehler (rot, bleibt länger).
func (a *App) flashErr(s string) { a.pushToast(toastErr, s) }

// flashOK zeigt eine Erfolgsmeldung (grün).
func (a *App) flashOK(s string) { a.pushToast(toastOK, s) }

func (a *App) pushToast(k toastKind, s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	now := time.Now()
	// gleiche Meldung nochmal: nur verlängern statt stapeln
	for i := range a.toasts {
		if a.toasts[i].text == s {
			a.toasts[i].until, a.toasts[i].kind = now.Add(toastLife(k)), k
			return
		}
	}
	a.toasts = append(a.toasts, toastItem{text: s, kind: k, born: now, until: now.Add(toastLife(k))})
	if len(a.toasts) > maxToasts {
		a.toasts = a.toasts[len(a.toasts)-maxToasts:]
	}
}

// clearToasts entfernt alle Einblendungen (z. B. nach dem Ausführen eines
// Befehls, vor dem gewarnt wurde).
func (a *App) clearToasts() { a.toasts = nil }

// liveToasts wirft abgelaufene weg und meldet, ob noch welche stehen.
func (a *App) liveToasts(now time.Time) bool {
	keep := a.toasts[:0]
	for _, t := range a.toasts {
		if now.Before(t.until) {
			keep = append(keep, t)
		}
	}
	a.toasts = keep
	return len(a.toasts) > 0
}

// guessToastKind: Erfolgs- und Warnmeldungen an ihrem Anfang erkennen
// (in der aktiven Sprache), damit nicht jede Stelle im Code die Art angeben muss.
func guessToastKind(s string) toastKind {
	for _, p := range []string{"Vorsicht: ", "Broadcast wird abgebrochen …"} {
		if strings.HasPrefix(s, i18n.T(p)) {
			return toastWarn
		}
	}
	for _, p := range []string{"Update fehlgeschlagen: ", "Einstellung nicht gespeichert: ", "Browser ließ sich nicht öffnen: "} {
		if strings.HasPrefix(s, i18n.T(p)) {
			return toastErr
		}
	}
	for _, p := range []string{"Kopiert: ", "Exportiert: ", "Web-Service bereit: ", "Tunnel gestoppt: ",
		"Key eingebettet (verschlüsselt, geht mit dem Sync mit)", "Schlüsseldatei für diesen Rechner gemerkt",
		"Verbunden: "} {
		if strings.HasPrefix(s, i18n.T(p)) {
			return toastOK
		}
	}
	return toastInfo
}

type toastTickMsg struct{}

// toastTick: solange Toasts stehen, öfter neu zeichnen (Herein-Gleiten,
// Abblassen, pünktliches Verschwinden). Ohne Toasts läuft nichts extra.
func (a *App) toastTick() tea.Cmd {
	if a.toastTicking || len(a.toasts) == 0 {
		return nil
	}
	a.toastTicking = true
	return tea.Tick(125*time.Millisecond, func(time.Time) tea.Msg { return toastTickMsg{} })
}

// toastBox zeichnet einen Toast: farbiger Rand und Symbol je Art,
// in den letzten Augenblicken blass.
func (a *App) toastBox(t toastItem, w int, now time.Time) string {
	st := a.s
	icon, col := "i", st.T.Accent
	switch t.kind {
	case toastOK:
		icon, col = "✓", st.T.OK
	case toastWarn:
		icon, col = "▲", st.T.Warn
	case toastErr:
		icon, col = "×", st.T.Danger
	}
	text := st.T.Text
	if t.until.Sub(now) < toastFade {
		col, text = st.T.Border, st.T.Muted // sanft ausblenden
	}
	inner := max(8, w-6)
	body := lipgloss.NewStyle().Foreground(text).Background(st.T.Surface).Width(inner).Render(t.text)
	lines := strings.Split(body, "\n")
	if len(lines) > 3 {
		lines = lines[:3]
		lines[2] = ansi.Truncate(lines[2], inner-1, "") + "…"
	}
	ic := lipgloss.NewStyle().Foreground(col).Background(st.T.Surface).Bold(true).Render(icon + " ")
	pad := lipgloss.NewStyle().Background(st.T.Surface).Render("  ")
	for i := range lines {
		if i == 0 {
			lines[i] = ic + lines[i]
		} else {
			lines[i] = pad + lines[i]
		}
	}
	return lipgloss.NewStyle().Border(frameBorder).BorderForeground(col).
		BorderBackground(st.T.Bg).Background(st.T.Surface).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

// toastWidth: so breit wie die längste Meldung, höchstens 56 Spalten und
// nie mehr als die Hälfte des Fensters (bei schmalen Fenstern fast alles).
func (a *App) toastWidth() int {
	maxW := min(56, max(a.w/2, a.w-4))
	if a.w >= 100 {
		maxW = min(56, a.w/2)
	}
	w := 0
	for _, t := range a.toasts {
		w = max(w, lipgloss.Width(t.text)+6)
	}
	return clamp(w, 20, max(20, maxW))
}

// withToasts legt die Toasts oben rechts (unter die Kopfzeile) über das
// fertige Bild. Ein Klick auf einen Toast schließt ihn.
func (a *App) withToasts(base string) string {
	now := time.Now()
	if !a.liveToasts(now) || a.w < 24 || a.h < 6 {
		return base
	}
	w := a.toastWidth()
	y := 1
	if a.scr == scrTerminal {
		y = 2 // Sitzungszeile frei lassen
	}
	for i := len(a.toasts) - 1; i >= 0 && y < a.h-3; i-- {
		t := a.toasts[i]
		text := t.text
		box := a.toastBox(t, w, now)
		box = a.z.mark(box, zoneAction{block: true, click: func(tea.Mouse, bool) tea.Cmd {
			a.dropToast(text)
			return nil
		}})
		bw, bh := lipgloss.Width(box), lipgloss.Height(box)
		x := max(0, a.w-bw-1)
		if pr := a.perf.rect; pr.w > 0 && y < pr.y+pr.h && x < pr.x+pr.w && pr.x < x+bw {
			y = pr.y + pr.h // unter die Leistungsanzeige (perf.go)
		}
		if age := now.Sub(t.born); age < toastIn { // von rechts hereingleiten
			x += int(float64(bw) * (1 - float64(age)/float64(toastIn)) / 2)
		}
		if x+bw > a.w { // beim Hereingleiten rechts abschneiden, sonst wird das Bild breiter als das Fenster
			ls := strings.Split(box, "\n")
			for j := range ls {
				ls[j] = ansi.Truncate(ls[j], a.w-x, "")
			}
			box = strings.Join(ls, "\n")
		}
		box = a.z.scan(box, x, y)
		base = stamp(base, box, x, y, a.w) // stamp.go
		y += bh
	}
	return base
}

func (a *App) dropToast(text string) {
	for i, t := range a.toasts {
		if t.text == text {
			a.toasts = append(a.toasts[:i], a.toasts[i+1:]...)
			return
		}
	}
}
