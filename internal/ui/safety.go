package ui

import (
	"reflect"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/safe"
)

// Absturzschutz (internal/safe): Ein Fehler in Update, View, einem Kommando
// oder einer Hintergrund-Goroutine beendet nicht mehr das ganze Programm mit
// allen Verbindungen. Stattdessen entsteht ein Absturzbericht, oben rechts
// erscheint eine Meldung, und alles andere läuft weiter. Häufen sich die
// Fehler, endet sshTermy wie früher (safe.StormN).

// caughtMsg: eine Panik wurde abgefangen, der Bericht liegt unter path.
type caughtMsg struct {
	where string
	path  string
	err   string
}

// CatchPanics schaltet den Absturzschutz ein (main, keep). Tests lassen ihn
// aus, damit Fehler dort sofort auffallen.
func (a *App) CatchPanics() {
	safe.SetHandler(func(c safe.Caught) {
		// Bericht nebenher schreiben: die Panik kann mitten in einer Sperre
		// passiert sein (Store), dann darf der Handler nicht warten.
		go func() {
			defer func() { _ = recover() }()
			msg := caughtMsg{where: c.Where}
			if r, err := crash.Write(a.st.Dir, crash.KindCaught, c.Raw(), a.st.Sensitive()); err == nil {
				msg.path = r.Path
			} else {
				msg.err = err.Error()
			}
			select {
			case a.events <- msg:
			default:
			}
		}()
	})
}

func (a *App) onCaught(m caughtMsg) {
	text := i18n.T("Interner Fehler abgefangen, sshTermy läuft weiter.")
	if m.path != "" {
		text += " " + i18n.T("Bericht: ") + m.path
	} else if m.err != "" {
		text += " " + i18n.T("Absturzbericht ließ sich nicht schreiben: ") + m.err
	}
	a.pushToast(toastErr, text)
}

func (a *App) Init() tea.Cmd {
	var c tea.Cmd
	if !safe.Run("ui.Init", func() { c = a.init0() }) {
		return a.listen()
	}
	return guardCmd(c)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(caughtMsg); ok {
		a.onCaught(m)
		return a, a.listen()
	}
	var (
		model tea.Model = a
		c     tea.Cmd
	)
	if !safe.Run("ui.Update", func() { model, c = a.update0(msg) }) {
		return a, a.rearm(msg)
	}
	return model, guardCmd(c)
}

// rearm: update0 ist mittendrin abgebrochen und hat die Ketten nicht neu
// angestoßen, die sonst jede Nachricht verlängert: den Sekundentakt und den
// Empfang aus a.events (Ausgabe der Sitzungen, Meldungen).
func (a *App) rearm(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	if _, ok := msg.(tickMsg); ok {
		cmds = append(cmds, tick())
	}
	if a.listening.Load() == 0 {
		cmds = append(cmds, a.listen())
	}
	return tea.Batch(cmds...)
}

// View: gemessen, bei Ausgabeflut gedrosselt (perf.go). Scheitert das
// Zeichnen, bleibt das letzte gute Bild stehen.
func (a *App) View() tea.View {
	var v tea.View
	if !safe.Run("ui.View", func() { v = a.view() }) {
		return a.lastView
	}
	a.lastView = v
	return v
}

// guardCmd schützt ein Kommando und alle, die es über Batch/Sequence
// nachreicht.
func guardCmd(c tea.Cmd) tea.Cmd {
	if c == nil || !safe.Active() {
		return c
	}
	return func() tea.Msg {
		var m tea.Msg
		if !safe.Run("ui.Cmd", func() { m = c() }) {
			return nil
		}
		return guardMsg(m)
	}
}

var cmdType = reflect.TypeFor[tea.Cmd]()

// guardMsg: BatchMsg und das (nicht exportierte) sequenceMsg sind Listen von
// Kommandos; jedes einzeln schützen, Typ und Reihenfolge bleiben.
func guardMsg(m tea.Msg) tea.Msg {
	if m == nil {
		return nil
	}
	v := reflect.ValueOf(m)
	if v.Kind() != reflect.Slice || v.Type().Elem() != cmdType {
		return m
	}
	out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
	for i := 0; i < v.Len(); i++ {
		c, _ := v.Index(i).Interface().(tea.Cmd)
		out.Index(i).Set(reflect.ValueOf(guardCmd(c)))
	}
	return out.Interface()
}
