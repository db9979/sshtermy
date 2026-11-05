package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

// Einführungstour (beim ersten Start, später über die Palette) und der
// Spickzettel: eine Zeile über der Fußzeile, die zum aktuellen Modus passt
// und erklärt, was die Tasten dort bewirken. F9 blendet ihn aus und ein.

type tourStep struct {
	icon, title string
	text        string
	keys        [][2]string
}

var tourSteps = []tourStep{
	{"◆", "Willkommen bei sshTermy", "Hier im Verbindungs-Manager legst du deine Server und Switches an. " +
		"Passwörter und Schlüssel liegen verschlüsselt auf deinem Rechner.",
		[][2]string{{"n", "neue Verbindung"}, {"⏎", "verbinden"}, {"F10", "zwischen Manager und Terminal"}}},
	{"❯", "Eine Taste für alles", "Die Befehlspalette findet Verbindungen, Tunnel, Runbooks, Schnellbefehle und Aktionen. " +
		"Was du tippst, kann auch direkt als Ziel oder Frage an die KI gehen.",
		[][2]string{{"Strg+Space", "Befehlspalette"}, {"Tab", "in der Palette: KI fragen"}}},
	{"✦", "Die KI tippt mit", "Beim Tippen erscheint grauer Text als Vorschlag. Nach einem Befehl schlägt die KI " +
		"bis zu vier nächste Befehle vor. Eingefügt wird nur, ausgeführt erst mit deinem ⏎.",
		[][2]string{{"Tab / →", "grauen Vorschlag übernehmen"}, {"Alt+1…4", "Folgebefehl einfügen"}, {"F3", "KI eine Frage stellen"}}},
	{"◎", "Ziel-Modus", "Beschreibe ein Ziel („Docker aufräumen“), die KI schlägt Schritt für Schritt genau einen Befehl vor. " +
		"Rote Befehle sind gefährlich und brauchen eine zweite Bestätigung.",
		[][2]string{{"F2", "Ziel-Modus"}, {"⏎", "Schritt ausführen"}, {"i", "nur einfügen"}, {"x", "Ziel beenden"}}},
	{"↯", "Schnellbefehle und mehr", "Häufige Befehle liegen auf ⇧F1 bis ⇧F8, je nach Gerät vorbelegt (z. B. „show vlans“ bei Aruba). F9 zeigt die Liste. " +
		"Mit ⇧F9 erklärt die Tastenzeile unten, was die Tasten im aktuellen Bereich bewirken.",
		[][2]string{{"⇧F1…F8", "Schnellbefehl"}, {"F9", "Schnellbefehle (Liste)"}, {"F4", "Tunnel"}, {"⇧F9", "Tasten erklären"}, {"F1", "alle Tasten"}}},
}

func (a *App) tourModal() *modal { return &modal{kind: mTour, title: "Einführung"} }

// maybeTour: nach dem Entsperren beim ersten Start (oder solange noch keine
// Verbindung angelegt und die Tour nie gesehen wurde).
func (a *App) maybeTour(firstSetup bool) {
	set := a.st.Settings()
	if set.TourDone || a.modal != nil {
		return
	}
	if firstSetup || len(a.st.Connections()) == 0 {
		a.modal = a.tourModal()
	}
}

func (a *App) finishTour() {
	a.modal = nil
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.TourDone = true })
}

func (a *App) tourKey(key string) tea.Cmd {
	m := a.modal
	switch key {
	case "right", "enter", "space", "l", "n":
		if m.scroll >= len(tourSteps)-1 {
			a.finishTour()
			return nil
		}
		m.scroll++
	case "left", "backspace", "h", "b":
		m.scroll = max(0, m.scroll-1)
	case "esc", "q", "s":
		a.finishTour()
		if key != "q" {
			a.flash("Tour übersprungen · Palette „Einführungstour“ startet sie wieder")
		}
	}
	return nil
}

func (m *modal) tourView(s Styles, w int) string {
	step := tourSteps[min(m.scroll, len(tourSteps)-1)]
	cw := min(64, w-10)
	var dots strings.Builder
	for i := range tourSteps {
		if i == m.scroll {
			dots.WriteString(s.Accent.Render("●"))
		} else {
			dots.WriteString(s.Muted.Render("○"))
		}
		dots.WriteString(" ")
	}
	head := s.Accent.Render(step.icon+" "+step.title) + "  " + dots.String() +
		s.Muted.Render(" "+strconv.Itoa(m.scroll+1)+"/"+strconv.Itoa(len(tourSteps)))
	body := lipgloss.NewStyle().Width(cw).Render(s.Text.Render(step.text))
	var keys []string
	for _, k := range step.keys {
		kk := s.Key.Render(k[0])
		kk += strings.Repeat(" ", max(0, 14-lipgloss.Width(kk)))
		keys = append(keys, kk+s.Text.Render(k[1]))
	}
	return s.Modal.Render(head + "\n\n" + body + "\n\n" + strings.Join(keys, "\n"))
}

func tourButtons(m *modal) [][2]string {
	var keys [][2]string
	if m.scroll > 0 {
		keys = append(keys, [2]string{"←", "zurück"})
	}
	if m.scroll < len(tourSteps)-1 {
		keys = append(keys, [2]string{"→", "weiter"}, [2]string{"Esc", "überspringen"})
	} else {
		keys = append(keys, [2]string{"⏎", "los geht's"})
	}
	return keys
}

// ---------------------------------------------------------------- Spickzettel

func (a *App) cheatOn() bool { return a.st.Settings().ExplainKeys }

func (a *App) toggleCheat() tea.Cmd {
	on := !a.cheatOn()
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.ExplainKeys = on })
	return nil
}

// cheatTips: Tipps für den aktuellen Bereich (Taste, Erklärung).
func (a *App) cheatTips() [][2]string {
	switch a.scr {
	case scrManager:
		if len(a.st.Connections()) == 0 {
			return [][2]string{{"n", "lege deine erste Verbindung an"}, {"s", "KI-Endpunkt einrichten"}, {"Strg+Space", "findet alles"}}
		}
		return [][2]string{{"⏎", "verbindet"}, {"/", "sucht nach Name, Gruppe, Tag"}, {"q", "Schnellbefehle der Verbindung"}, {"Strg+Space", "findet alles"}}
	case scrTerminal:
	default:
		return nil
	}
	s := a.current()
	if s == nil {
		return nil
	}
	switch a.helpContext() {
	case hcFiles:
		return [][2]string{{"Tab", "wechselt die Seite"}, {"c", "kopiert hoch oder runter"}, {"⏎", "öffnet den Ordner"}, {"F5", "schließt"}}
	case hcGoal:
		if !s.goal.active {
			return [][2]string{{"⏎", "startet das Ziel"}, {"↑↓", "wählt ein gespeichertes Runbook"}, {"Esc", "zurück ins Terminal"}}
		}
		return [][2]string{{"⏎", "führt den Schritt aus"}, {"i", "fügt nur ein"}, {"s", "überspringt"}, {"x", "beendet das Ziel"}}
	case hcAsk:
		return [][2]string{{"⏎", "sendet die Frage"}, {"1…9", "fügt einen Befehl aus der Antwort ein"}, {"n", "neue Frage"}}
	case hcTunnel:
		return [][2]string{{"⏎", "startet oder stoppt"}, {"o", "öffnet im Browser"}, {"n", "neuer Tunnel"}}
	}
	switch {
	case s.state != stConnected:
		return [][2]string{{"⏎", "verbindet neu"}, {"x", "schließt den Tab"}, {"F10", "zum Manager"}}
	case s.scroll > 0:
		return [][2]string{{"Esc", "springt zurück zur Eingabe"}, {"Rad", "blättert im Verlauf"}, {"Ziehen", "markiert und kopiert"}}
	case a.visibleGhost(s) != "":
		return [][2]string{{"Tab / →", "übernimmt den grauen Vorschlag"}, {"weitertippen", "ignoriert ihn"}}
	case len(s.chips) > 0:
		return [][2]string{{"Alt+1…4", "fügt den Vorschlag ein (ohne ⏎)"}, {"rot", "= gefährlich, zweimal drücken"}}
	}
	return [][2]string{{"F9", "Schnellbefehle"}, {"F2", "Ziel an die KI"}, {"F3", "Frage an die KI"}, {"Strg+Space", "findet alles"}}
}

// cheatLine: Tastenzeile mit Erklärungen (leer, wenn aus oder nichts passt;
// dann zeigt die Fußzeile die normalen Tasten).
func (a *App) cheatLine() string {
	if !a.cheatOn() || a.modal != nil || a.pal != nil || (a.scr != scrManager && a.scr != scrTerminal) {
		return ""
	}
	tips := a.cheatTips()
	if len(tips) == 0 {
		return ""
	}
	st := a.s
	bg := st.Footer
	k := strings.ReplaceAll(a.keys.label(actCheat), "Shift+", "⇧")
	back := a.keyZone(k, st.Key.Render(k)+st.KeyDesc.Render(" Tasten"))
	// schmale Fenster: Tipps von hinten weglassen, der Rückweg bleibt
	render := func(n int) string {
		out := bg.Render(" ")
		for i, t := range tips[:n] {
			if i > 0 {
				out += bg.Render(" · ")
			}
			out += a.keyZone(t[0], st.Key.Render(t[0])) + bg.Foreground(st.T.Text).Render(" "+t[1])
		}
		return out
	}
	n := len(tips)
	for n > 1 && lipgloss.Width(render(n))+1+lipgloss.Width(back) > a.w {
		n--
	}
	out := render(n)
	gap := a.w - lipgloss.Width(out) - lipgloss.Width(back)
	if gap < 1 {
		return ansi.Truncate(out, a.w, "")
	}
	return out + bg.Render(strings.Repeat(" ", gap)) + back
}

func cheatLabel(on bool) string {
	if on {
		return "Tastenzeile: nur Tasten"
	}
	return "Tastenzeile: Tasten erklären"
}
