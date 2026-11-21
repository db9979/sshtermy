package ui

// KI-Stufen: sshTermy zeigt nur, was mit der angebundenen KI auch geht.
//
//   - keine KI (nichts eingerichtet, Endpunkt seit dem Start nie erreicht,
//     KI-Datenschutz „aus“ oder Tag ki-aus): alle KI-Funktionen sind
//     ausgeblendet. Was ohne KI geht, bleibt: Ghost-Text aus dem Verlauf,
//     Befehle prüfen und reparieren, Befehl erklären aus der man-Seite.
//   - kleine Offline-KI (lxask-Modell, offlineai): springt ein, wenn keine
//     andere KI da ist. Sie kann einen Satz in einen Befehl verwandeln und
//     einfache Befehlsfragen beantworten. Ziel-, Agent- und Plan-Modus,
//     Begleiter, Vorgang, Fehler erklären, KI-Ghost-Text, Folgebefehle und
//     die Gegenprüfung brauchen eine größere KI und bleiben aus.
//   - volle KI: alles.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/offlineai"
	"github.com/db9979/sshtermy/internal/store"
)

type aiTierKind int

const (
	tierNone  aiTierKind = iota // keine KI: nur Funktionen ohne KI
	tierSmall                   // kleine Offline-KI: Satz → Befehl, einfache Fragen
	tierFull                    // volle KI: alles
)

// offlineEngine: die Offline-KI dieses Rechners (Dateien im Konfigurationsordner).
func (a *App) offlineEngine() *offlineai.Engine {
	if a.offline == nil {
		dir, err := store.Dir()
		if err != nil {
			return nil
		}
		a.offline = offlineai.New(filepath.Join(dir, "offline-ki"))
	}
	return a.offline
}

// offlineReady: Die Offline-KI ist heruntergeladen (Ergebnis kurz gemerkt,
// die Abfrage schaut auf die Platte).
func (a *App) offlineReady() bool {
	if a.offTest != nil {
		return *a.offTest
	}
	now := time.Now()
	if now.Sub(a.offAt) < 5*time.Second {
		return a.offOK
	}
	e := a.offlineEngine()
	a.offOK, a.offAt = e != nil && e.Installed(), now
	return a.offOK
}

// mainAIUsable: Die eingerichtete KI kommt für diese Datenschutzstufe in
// Frage und war nicht von Anfang an unerreichbar.
func (a *App) mainAIUsable(level string) bool {
	if a.st == nil {
		return false
	}
	cfg := a.st.LLM()
	if level == llm.PrivacyOff || cfg.BaseURL == "" || cfg.Model == "" {
		return false
	}
	if level == llm.PrivacyLocal {
		own := false
		for _, c := range cfg.Chain() {
			if llm.IsOwnEndpoint(llm.BaseURL(c), c.OwnHosts) {
				own = true
			}
		}
		if !own {
			return false
		}
	}
	if a.aiDownTest {
		return false
	}
	h := a.llmHealth()
	if h.level == lhError && len(cfg.Chain()) == 1 {
		if _, ok := llm.HealthOf(cfg.Chain()[0]); ok.At.IsZero() {
			return false // seit dem Start nie geantwortet: gilt als keine KI
		}
	}
	return true
}

// aiTier: was die KI für diese Sitzung kann (s darf nil sein: allgemein).
func (a *App) aiTier(s *session) aiTierKind {
	if a.st == nil {
		return tierNone
	}
	level := a.st.LLM().Privacy
	if s != nil {
		level = a.aiLevel(s.conn)
	}
	if level == llm.PrivacyOff {
		return tierNone
	}
	if a.mainAIUsable(level) {
		return tierFull
	}
	if a.offlineReady() {
		return tierSmall
	}
	return tierNone
}

func (a *App) curTier() aiTierKind {
	if s := a.current(); s != nil {
		return a.aiTier(s)
	}
	return a.aiTier(nil)
}

// offlineLLM: Einstellung für den Endpunkt der Offline-KI.
func (a *App) offlineLLM() domain.LLMConfig {
	e := a.offlineEngine()
	port, key := 0, ""
	if e != nil {
		port, key = e.Port(), e.Key()
	}
	return domain.LLMConfig{
		Provider: domain.ProviderCompat, BaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", port), APIKey: domain.Secret(key), Model: offlineai.Model,
		MaxTokens: 200, Temperature: 0.1, TimeoutSec: 120, SendScreen: false,
	}
}

// errNoAI: Text, wenn eine Funktion eine KI braucht.
func errNoAI() error {
	return errors.New(i18n.T("Keine KI eingerichtet: Einstellungen › KI (oder die kleine Offline-KI herunterladen)"))
}

func errNeedBigAI() error {
	return errors.New(i18n.T("Das braucht eine größere KI als die kleine Offline-KI (Einstellungen › KI)"))
}

// needAI prüft vor einer KI-Funktion die Stufe. full = braucht volle KI.
// false = geht nicht, Hinweis steht in der Meldungszeile.
func (a *App) needAI(s *session, full bool) bool {
	switch a.aiTier(s) {
	case tierNone:
		a.flash(errNoAI().Error())
		return false
	case tierSmall:
		if full {
			a.flash(errNeedBigAI().Error())
			return false
		}
	}
	return true
}

// smallAgent: Zugang zur Offline-KI (startet den Server beim ersten Gebrauch).
func (a *App) smallAgent() (*agent.Agent, error) {
	c, err := llm.New(a.offlineLLM())
	if err != nil {
		return nil, err
	}
	return &agent.Agent{C: c, Small: true}, nil
}

// offlineEnsure: im Hintergrund vor einer Anfrage an die Offline-KI
// aufrufen; startet llama-server und wartet, bis das Modell geladen ist.
func (a *App) offlineEnsure(ag *agent.Agent) func(ctx context.Context) error {
	e := a.offlineEngine()
	if ag == nil || !ag.Small || e == nil {
		return func(context.Context) error { return nil }
	}
	return func(ctx context.Context) error {
		err := e.Ensure(ctx)
		e.Touch()
		return err
	}
}

// offlineTick (jede Sekunde): Server nach langer Ruhe beenden.
func (a *App) offlineTick(now time.Time) {
	if a.offline != nil {
		a.offline.StopIdle(now)
	}
}

// ---------------------------------------------------------------- Herunterladen

type offlineDLState struct {
	busy    bool
	prog    offlineai.Progress
	err     string
	cancel  context.CancelFunc
	started time.Time
}

type offlineProgMsg struct{ p offlineai.Progress }
type offlineDoneMsg struct{ err error }

// offlineDownload: Knopf „Kleine Offline-KI herunterladen“.
func (a *App) offlineDownload() tea.Cmd {
	e := a.offlineEngine()
	if e == nil {
		a.flash(i18n.T("Kein Konfigurationsordner"))
		return nil
	}
	if a.offDL.busy {
		a.flash(a.offlineStatus())
		return nil
	}
	if e.Installed() {
		a.flashOK(i18n.T("Offline-KI ist schon da"))
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.offDL = offlineDLState{busy: true, cancel: cancel, started: time.Now()}
	ev := a.events
	a.pushToast(toastInfo, i18n.T("Offline-KI wird heruntergeladen (ca. 1 GB) …"))
	return func() tea.Msg {
		err := e.Download(ctx, func(p offlineai.Progress) {
			select {
			case ev <- offlineProgMsg{p}:
			default:
			}
		})
		return offlineDoneMsg{err}
	}
}

func (a *App) offlineMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case offlineProgMsg:
		a.offDL.prog = m.p
		return a.listen(), true
	case offlineDoneMsg:
		a.offDL.busy = false
		a.offAt = time.Time{}
		if m.err != nil {
			if errors.Is(m.err, context.Canceled) {
				a.flash(i18n.T("Herunterladen abgebrochen"))
				return nil, true
			}
			a.offDL.err = m.err.Error()
			a.pushToast(toastWarn, i18n.T("Offline-KI: ")+trunc(m.err.Error(), 120))
			return nil, true
		}
		a.pushToast(toastOK, i18n.T("Offline-KI bereit: springt ein, wenn keine andere KI da ist"))
		return nil, true
	}
	return nil, false
}

// offlineStatus: eine Zeile zum Stand (Palette, Meldung).
func (a *App) offlineStatus() string {
	d := a.offDL
	switch {
	case d.busy && d.prog.Total > 0:
		return fmt.Sprintf(i18n.T("lädt %s: %d %%"), d.prog.Step, d.prog.Done*100/d.prog.Total)
	case d.busy:
		return i18n.T("lädt …")
	case a.offlineReady():
		return fmt.Sprintf(i18n.T("bereit · %d MB"), a.offlineEngine().Size()>>20)
	case d.err != "":
		return i18n.T("Fehler: ") + trunc(d.err, 60)
	}
	return i18n.T("ca. 1 GB, nicht installiert")
}

// offlineRemove: Offline-KI löschen.
func (a *App) offlineRemove() tea.Cmd {
	if a.offDL.busy && a.offDL.cancel != nil {
		a.offDL.cancel()
		return nil
	}
	e := a.offlineEngine()
	if e == nil || !e.Installed() {
		a.flash(i18n.T("Offline-KI ist nicht installiert"))
		return nil
	}
	if err := e.Remove(); err != nil {
		a.flash(i18n.T("Fehler: ") + err.Error())
		return nil
	}
	a.offAt = time.Time{}
	a.flashOK(i18n.T("Offline-KI entfernt"))
	return nil
}

// aiHide blendet KI-Einträge der Palette aus, die mit der Stufe nicht gehen.
func (a *App) aiHide(s *session, items []palItem) []palItem {
	t := a.aiTier(s)
	if t == tierFull {
		return items
	}
	out := items[:0]
	for _, it := range items {
		if it.area == "ai" && !it.noAI && (t == tierNone || !it.small) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// startSmallAsk: „KI fragen“ mit der kleinen Offline-KI. Sie schreibt nur
// einen Befehl; die Karte prüft ihn wie jede andere (cmdcheck.go), und e
// erklärt ihn aus der man-Seite.
func (a *App) startSmallAsk(s *session, text string) tea.Cmd {
	q := &s.ask
	ag, err := a.smallAgent()
	if err != nil {
		q.err = err.Error()
		return nil
	}
	ctx, cancel := q.begin(text)
	q.history = nil
	q.input.SetValue(q.question)
	sid, seq, ev := s.id, q.seq, a.events
	c := a.agentContext(s, false)
	question := q.question
	ensure := a.offlineEnsure(ag)
	return func() tea.Msg {
		defer cancel()
		if err := ensure(ctx); err != nil {
			return askDoneMsg{sid, seq, err}
		}
		cmd, err := ag.WishCommand(ctx, question, c)
		if err != nil {
			return askDoneMsg{sid, seq, err}
		}
		ev <- askDeltaMsg{sid, seq, i18n.T("Befehl der kleinen Offline-KI (lxask), vorab geprüft; e erklärt ihn aus der man-Seite.") + "\n\n```bash\n" + cmd + "\n```\n"}
		return askDoneMsg{sid, seq, nil}
	}
}

// offlinePalItem: Eintrag in Einstellungen › KI.
func (a *App) offlinePalItem() palItem {
	it := palItem{group: pgSetSetup, menu: "settings/ki", icon: ic("ai"), keep: true, hint: a.offlineStatus(),
		keys:  "offline ki ai lokal local klein small lxask llama llama.cpp modell model herunterladen download ohne internet",
		label: i18n.T("Kleine Offline-KI herunterladen (ca. 1 GB)"), run: func() tea.Cmd { return a.offlineDownload() }}
	switch {
	case a.offDL.busy:
		it.label = i18n.T("Offline-KI: Herunterladen abbrechen")
		it.run = func() tea.Cmd { return a.offlineRemove() }
	case a.offlineReady():
		it.label = i18n.T("Offline-KI entfernen")
		it.run = func() tea.Cmd { return a.offlineRemove() }
	}
	return it
}
