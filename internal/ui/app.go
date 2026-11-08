// Package ui ist die Oberfläche (bubbletea v2 + lipgloss v2).
//
// Ablauf (Interaction Flow):
//
//	Start ─▶ Entsperren (DPAPI automatisch / Master-Passwort / Ersteinrichtung)
//	      ─▶ Verbindungs-Manager (Liste, Details, Tunnel, Suche)
//	           ├─ n/e  Verbindung anlegen/bearbeiten  ─▶ Formular
//	           ├─ s    KI-Einstellungen (Endpunkt, Modell, Test)
//	           ├─ m    Broadcast: Befehl/Runbook auf mehreren Hosts, Ergebnisse nebeneinander
//	           ├─ f    Netzwerk-Scan: Subnetz nach SSH-Geräten durchsuchen, Funde anlegen
//	           └─ ⏎    Verbinden ─▶ (Host-Key prüfen) ─▶ Terminal (Tab)
//	                     ├─ Ghost-Text beim Tippen      Tab/→ übernimmt
//	                     ├─ Folgebefehle nach Enter     Alt+1…4 fügt ein
//	                     ├─ F2 Ziel-Modus (Agent)       ⏎ ausführen · i einfügen · s überspringen
//	                     ├─ F3 Frage an die KI          1…9 Befehl aus der Antwort einfügen
//	                     ├─ F4 Tunnel                   ⏎ an/aus · o im Browser öffnen
//	                     ├─ F6 Host-Übersicht           r/⏎ abfragen · b Switch-Config sichern · d Diff
//	                     └─ F10 zurück zum Manager (Sitzungen laufen weiter), F7/F8 Tabs
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/update"
	"github.com/db9979/sshtermy/internal/vault"
	"github.com/db9979/sshtermy/internal/version"
)

type screen int

const (
	scrUnlock screen = iota
	scrManager
	scrConnForm
	scrSettings
	scrTunnelForm
	scrTerminal
	scrSync
	scrLogs
	scrQuickForm
	scrMonitorForm // Grenzwerte der Überwachung (monitor.go)
	scrImport
	scrKeys
	scrBroadcast
	scrScan
	scrKeyForm       // Schlüsseldatei auf diesem Rechner wählen (nach Sync)
	scrSSHKeyForm    // SSH-Schlüssel erzeugen / übertragen (sshkey.go)
	scrRelayForm     // Vermittler koppeln (relay.go)
	scrColors        // Farben anpassen (colors.go)
	scrTeam          // Team-Freigabe: Auswahl bzw. Vorschau (team.go)
	scrTeamForm      // Team-Freigabe: Passwort, Datei, Optionen
	scrHighlightForm // Farbregeln (highlight.go)
	scrScheduleForm  // Zeitplan (schedule.go)
)

// App ist das Wurzel-Modell.
type App struct {
	inputLog *os.File // nur mit SSHTERMY_DEBUG=1
	st       *store.Store
	hostKeys *sshx.HostKeyStore
	s        Styles
	w, h     int
	scr      screen
	events   chan tea.Msg

	unlock           unlockState
	mgr              managerState
	form             *form
	formID           string // ID des bearbeiteten Datensatzes
	formBase         domain.Connection
	formKC           [2]string // Verbindungsformular: Werte aus dem Schlüsselbund beim Öffnen
	formTunnel       domain.Tunnel
	formQuickPreset  []string // Schnellbefehle: Vorbelegung zum Vergleich
	formSID          int      // Schlüsseldatei-Abfrage: Sitzung, die danach neu verbindet
	returnToTerminal bool

	sessions []*session
	cur      int
	nextSID  int
	split    splitMode // geteilte Ansicht (Strg+T)
	syncIn   bool      // Eingabe an alle Bereiche (syncinput.go)
	panes    []int     // Sitzungs-IDs je Bereich, 0 = leer
	fracX    []float64 // Spaltenanteile der geteilten Ansicht (nil = gleich)
	fracY    []float64 // Zeilenanteile (nil = gleich)
	zoom     bool      // aktiver Bereich aufs ganze Fenster (Alt+Z)
	sdrag    *splitDrag
	dropOn   int // Titel wird gezogen: Zielbereich+1

	modal        *modal
	pal          *palette
	llm          llmUI
	lh           llmHealthState // KI-Status (llmhealth.go)
	reach        reachState     // Online-Punkt im Manager (reach.go)
	toasts       []toastItem    // Einblendungen oben rechts (toast.go)
	toastTicking bool
	rel          *update.Release
	// restartPending: installierte, noch nicht gestartete Version.
	restartPending string
	restart        bool // nach dem Beenden neu starten (siehe Restart)
	lastKey        time.Time

	hl    hlState       // Farbregeln (highlight.go)
	edits []*remoteEdit // Server-Dateien im eigenen Editor (editor.go)

	formSched  domain.Schedule   // Zeitplan im Formular (schedule.go)
	schedRBIDs map[string]string // Runbook-Name → ID im Formular
	schedRun   map[string]bool   // laufende Zeitpläne

	book *sessionlog.Book // Sitzungsprotokolle (Schlüssel nur bei entsperrtem Store)
	logs logsState

	sync    syncState
	wiz     *syncWizard
	imp     importState   // Import aus ssh-config, PuTTY, MobaXterm
	team    teamState     // Team-Freigabe (team.go)
	tl      teamLinkState // Team-Ordner: veröffentlichen/abonnieren (teamlink.go)
	keyEd   keyEditor     // Tastenkürzel anpassen
	colEd   colorEditor   // Farben anpassen
	keys    keymap        // aktuelle Tastenbelegung
	lookSig string        // Theme+Tasten, zuletzt übernommen

	z     zones      // klickbare Bereiche des letzten Bildes
	click clickState // für Doppel-/Dreifachklick

	pick *picker // Auswahlliste mit Suche (Jump-Host, Ordner, Tag)

	bc     *bcState                     // Broadcast an mehrere Hosts
	rbVals map[string]map[string]string // zuletzt eingegebene Runbook-Variablen (nur im Speicher)
	scan   *scanState                   // Netzwerk-Scan

	intro  *introState // Startanimation, nil = aus oder vorbei
	relays relayCache  // Systemlisten der Vermittler (relay.go)
	fx     fadeFx      // Ein-/Ausblenden der Popups (dialog.go)

	crashTest string // sshtermy -crash-test (crash.go)

	blurred bool       // Terminalfenster nicht im Vordergrund (Fokus-Meldungen)
	lock    *lockState // Bildschirmsperre bei offenen Sitzungen (lock.go)
}

func New(st *store.Store) (*App, error) {
	hk, err := sshx.NewHostKeyStore(st.KnownHostsPath())
	if err != nil {
		return nil, err
	}
	a := &App{st: st, hostKeys: hk, events: make(chan tea.Msg, 256), lastKey: time.Now(), keys: newKeymap(nil)}
	a.setStyles(Nacht)
	a.unlock = newUnlock(st)
	a.inputLog = openInputLog(st.Dir)
	a.hookSync()
	if IntroEnabled(st.Dir) {
		a.intro = &introState{}
	}
	return a, nil
}

func (a *App) listen() tea.Cmd {
	ev := a.events
	return func() tea.Msg { return <-ev }
}

type tickMsg time.Time

func tick() tea.Cmd { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }

type updateMsg struct {
	rel *update.Release
	err error
}

func checkUpdate() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		r, err := update.Check(ctx)
		return updateMsg{r, err}
	}
}

func (a *App) Init() tea.Cmd {
	var intro tea.Cmd
	if a.intro != nil {
		a.intro.start = time.Now()
		intro = introFrame()
	}
	return tea.Batch(a.listen(), tick(), a.tryDPAPI(), intro)
}

func (a *App) current() *session {
	if a.cur >= 0 && a.cur < len(a.sessions) {
		return a.sessions[a.cur]
	}
	return nil
}

func (a *App) byID(id int) *session {
	for _, s := range a.sessions {
		if s.id == id {
			return s
		}
	}
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case fadeMsg:
		if a.fadeRunning() {
			return a, fadeTick()
		}
		return a, nil
	case toastTickMsg:
		a.toastTicking = false
		a.liveToasts(time.Now())
		return a, a.toastTick()
	}
	m, cmd := a.update(msg)
	a.clearNotices()
	if c := a.trackFade(); c != nil {
		cmd = tea.Batch(cmd, c)
	}
	if t := a.toastTick(); t != nil {
		cmd = tea.Batch(cmd, t)
	}
	return m, cmd
}

func (a *App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	a.logInput(msg)
	if cmd, ok := a.introMsg(msg); ok {
		return a, cmd
	}
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = m.Width, m.Height
		for _, s := range a.sessions {
			a.resize(s)
		}
		return a, nil

	case tickMsg:
		return a, tea.Batch(tick(), a.onTick(), a.llmTick(), a.reachTick(), a.crashTestTick())
	case crashTestMsg:
		panic("sshTermy crash test (ui) password=hunter2")
	case llmProbeMsg:
		a.lh.probing = false
		return a, nil
	case reachMsg:
		a.onReach(m)
		return a, nil

	case outputMsg:
		var clip tea.Cmd
		if s := a.byID(m.sid); s != nil {
			s.pending.Store(false)
			s.lastOutput = time.Now()
			clip = a.takeClipboard(s)
		}
		return a, tea.Batch(a.listen(), clip)
	case tea.FocusMsg:
		a.blurred = false
		a.clearNotices()
		return a, nil
	case tea.BlurMsg:
		a.blurred = true
		return a, nil

	case hostKeyAskMsg:
		a.modal = hostKeyModal(m)
		return a, a.listen()
	case secretAskMsg:
		a.modal = secretModal(m)
		return a, a.listen()
	case connectNoticeMsg:
		if s := a.byID(m.sid); s != nil {
			s.connNote = m.text
		}
		return a, a.listen()
	case sshKeyDoneMsg:
		return a, a.sshKeyDone(m)
	case relayMsg:
		return a, m.apply(a)
	case teamLinkMsg:
		return a, m.apply(a)

	case connectedMsg:
		if s := a.byID(m.sid); s != nil {
			s.client, s.shell, s.state, s.connNote = m.client, m.shell, stConnected, ""
			s.connectedAt = time.Now()
			if m.client != nil {
				s.tunnels = sshx.NewTunnelManager(m.client)
			}
			a.st.Touch(s.conn.ID)
			if m.note != "" {
				a.flash(m.note)
			}
			if m.client != nil && m.client.Learned {
				a.rememberNetDevice(s)
			}
			a.profileConnected(s)
			a.learnKeychain(s)
			a.flashOK(i18n.T("Verbunden: ") + s.conn.Alias)
			for _, t := range a.st.Tunnels(s.conn.ID) {
				if t.AutoStart && s.tunnels != nil {
					if _, err := s.tunnels.Start(t); err != nil {
						a.flashErr("Tunnel " + t.Name + ": " + err.Error())
					}
				}
			}
			a.startLog(s)
			if s.detached() {
				s.detachNotice(i18n.T("Wieder verbunden."))
			}
			var cmd tea.Cmd
			if s.rc.active {
				cmd = a.onReconnected(s)
			}
			a.resize(s)
			return a, tea.Batch(cmd, a.hostOnConnect(s), a.persistOnConnect(s))
		}
		return a, nil
	case adaptMsg:
		a.onAdapt(m)
		return a, nil
	case muxProbeMsg:
		a.onMuxProbe(m)
		return a, nil
	case persistMsg:
		return a, a.onPersist(m)
	case connectErrMsg:
		if s := a.byID(m.sid); s != nil {
			if s.rc.active {
				a.onReconnectErr(s, m.err)
			} else {
				s.state, s.err = stClosed, m.err.Error()
				var km *sshx.KeyMissingError
				if errors.As(m.err, &km) && a.form == nil && a.modal == nil && a.pick == nil && a.pal == nil && a.scr == scrTerminal {
					return a, a.openKeyFix(s.id, km)
				}
			}
		}
		return a, nil
	case shellClosedMsg:
		if s := a.byID(m.sid); s != nil {
			a.onShellClosed(s, m)
			if s.state == stReconnect {
				s.detachNotice(i18n.T("Verbindung abgerissen, verbinde neu …"))
			}
		}
		return a, a.listen()
	case detachConnMsg:
		a.onDetachConn(m)
		return a, a.listen()
	case detachResizeMsg:
		a.onDetachResize(m)
		return a, a.listen()
	case detachEndMsg:
		a.onDetachEnd(m)
		return a, a.listen()

	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
		if a.lock != nil {
			return a, nil
		}
		return a, a.mouse(msg.(tea.MouseMsg))
	case pasteMsg:
		return a, a.onPaste(m)
	case pingDoneMsg:
		a.onPingDone(m)
		return a, nil

	case updateMsg:
		if m.rel != nil {
			a.rel = m.rel
			a.flash(i18n.T("Update auf v") + m.rel.Version + i18n.T(" verfügbar (") + a.keys.label(actAbout) + ")")
		}
		return a, nil
	case applyDoneMsg:
		a.modal = nil
		if m.err != nil {
			a.flashErr(i18n.T("Update fehlgeschlagen: ") + m.err.Error())
			return a, nil
		}
		if a.rel != nil {
			a.restartPending = a.rel.Version
		}
		a.rel = nil
		return a, a.askRestart()
	}

	if cmd, ok := a.trzMsg(msg); ok {
		return a, cmd
	}
	// Broadcast und Netzwerk-Scan (Fortschritt kommt über a.events)
	if cmd, ok := a.bcMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.scanMsg(msg); ok {
		return a, cmd
	}

	// Sync zwischen Rechnern
	if cmd, ok := a.syncMsg(msg); ok {
		return a, cmd
	}

	// Nachrichten der Sitzungen (KI, Ziel, Frage)
	if cmd, ok := a.sessionMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.llmMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.reportMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.logsMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.scheduleMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.editMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.filesMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.hostMsg(msg); ok {
		return a, cmd
	}

	if cmd, ok := a.lockMsg(msg); ok {
		return a, cmd
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		a.lastKey = time.Now()
		if a.scr == scrColors && a.colEd.editing && a.modal == nil {
			return a, a.updateColors(k) // Hex-Eingabe: Buchstaben gehören dem Feld
		}
		if a.scr == scrKeys && a.keyEd.capturing && a.modal == nil {
			return a, a.updateKeys(k) // neue Taste: nichts anderes darf sie abfangen
		}
		if a.keys.is(actQuit, k.String()) {
			return a, a.quit()
		}
		if a.modal != nil {
			return a, a.modalKey(k)
		}
		if a.pick != nil {
			return a, a.pickerKey(k)
		}
		if a.keys.is(actHelp, k.String()) && (a.pal != nil || a.scr == scrConnForm || a.scr == scrSettings || a.scr == scrTunnelForm || a.scr == scrImport || a.scr == scrKeys || a.scr == scrBroadcast || a.scr == scrScan || a.scr == scrQuickForm || a.scr == scrMonitorForm || a.scr == scrHighlightForm || a.scr == scrScheduleForm || a.scr == scrKeyForm || a.scr == scrSSHKeyForm || a.scr == scrRelayForm) {
			a.modal = a.helpModal()
			return a, nil
		}
		if a.pal != nil {
			return a, a.paletteKey(k)
		}
		if a.keys.is(actPalette, k.String()) && (a.scr == scrTerminal || a.scr == scrManager || a.scr == scrLogs) {
			return a, a.openPalette()
		}
		if a.keys.is(actLocal, k.String()) && (a.scr == scrTerminal || a.scr == scrManager) && a.lock == nil {
			return a, a.openLocal("")
		}
	}
	if pm, ok := msg.(tea.PasteMsg); ok && a.modal != nil {
		// Einfügen in einen offenen Dialog: nur das Eingabefeld nimmt es an.
		if a.modal.kind == mSecret || a.modal.kind == mInput {
			var cmd tea.Cmd
			a.modal.in, cmd = a.modal.in.Update(pm)
			return a, cmd
		}
		return a, nil
	}
	if pm, ok := msg.(tea.PasteMsg); ok && a.pick != nil {
		var cmd tea.Cmd
		a.pick.in, cmd = a.pick.in.Update(pm)
		a.pick.sel, a.pick.top = 0, 0
		return a, cmd
	}
	if pm, ok := msg.(tea.PasteMsg); ok && a.pal != nil && a.modal == nil {
		var cmd tea.Cmd
		a.pal.in, cmd = a.pal.in.Update(pm)
		a.pal.sel, a.pal.armed = 0, -1
		return a, cmd
	}

	switch a.scr {
	case scrUnlock:
		return a, a.updateUnlock(msg)
	case scrManager:
		return a, a.updateManager(msg)
	case scrConnForm, scrSettings, scrTunnelForm, scrQuickForm, scrMonitorForm, scrHighlightForm, scrScheduleForm, scrKeyForm, scrSSHKeyForm, scrRelayForm:
		return a, a.updateForm(msg)
	case scrTerminal:
		return a, a.updateTerminal(msg)
	case scrSync:
		return a, a.updateSync(msg)
	case scrLogs:
		return a, a.updateLogs(msg)
	case scrImport:
		return a, a.updateImport(msg)
	case scrTeam:
		return a, a.updateTeam(msg)
	case scrTeamForm:
		return a, a.updateTeamForm(msg)
	case scrKeys:
		return a, a.updateKeys(msg)
	case scrColors:
		return a, a.updateColors(msg)
	case scrBroadcast:
		return a, a.updateBroadcast(msg)
	case scrScan:
		return a, a.updateScan(msg)
	}
	return a, nil
}

func (a *App) quit() tea.Cmd {
	a.rememberLayout() // geteilte Ansicht als „Zuletzt“ (workspace.go)
	a.cleanupEdits()
	sshx.StopSKAgent() // eigener ssh-agent für Sicherheitsschlüssel
	for _, s := range a.sessions {
		a.endLog(s)
		a.stopRecording(s)
		s.close()
	}
	a.closeBook()
	a.st.Lock()
	return tea.Quit
}

func (a *App) onTick() tea.Cmd {
	var cmds []tea.Cmd
	a.applyLook() // Theme/Tasten können per Sync von einem anderen Rechner kommen
	// Auto-Sperre: ohne offene Sitzungen wird der Store gesperrt; mit
	// Sitzungen nur der Bildschirm (die Shells laufen weiter, lock.go).
	if set := a.st.Settings(); set.LockAfterMin > 0 && a.scr != scrUnlock && a.lock == nil &&
		time.Since(a.lastKey) > time.Duration(set.LockAfterMin)*time.Minute {
		if len(a.sessions) == 0 {
			a.closeBook()
			a.st.Lock()
			a.unlock = newUnlock(a.st)
			a.scr = scrUnlock
			return nil
		}
		cmds = append(cmds, a.lockScreen())
	}
	if c := a.scheduleTick(); c != nil {
		cmds = append(cmds, c)
	}
	if c := a.editTick(); c != nil {
		cmds = append(cmds, c)
	}
	if c := a.syncTick(); c != nil {
		cmds = append(cmds, c)
	}
	if c := a.teamLinkTick(); c != nil {
		cmds = append(cmds, c)
	}
	for _, s := range a.sessions {
		if s.term != nil && s.term.TakeBell() {
			a.noticeBell(s)
		}
		a.alertTick(s)
		if c := a.takeClipboard(s); c != nil {
			cmds = append(cmds, c)
		}
		a.commandTick(s)
		if c := a.followTick(s); c != nil {
			cmds = append(cmds, c)
		}
		if c := a.goalTick(s); c != nil {
			cmds = append(cmds, c)
		}
		a.profileTick(s)
		if c := a.monitorTick(s); c != nil {
			cmds = append(cmds, c)
		}
		if c := a.reconnectTick(s); c != nil {
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

// ---------------------------------------------------------------- Layout

func (a *App) View() tea.View {
	a.z.reset()
	var body string
	var cur *tea.Cursor
	switch a.scr {
	case scrUnlock:
		body = a.viewUnlock()
	case scrManager:
		body = a.frame(a.viewManager(), a.managerKeys())
	case scrTunnelForm:
		k := [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+S", i18n.T("speichern")}}
		if a.formTunnel.ID != "" {
			k = append(k, [2]string{"Strg+D", i18n.T("löschen")})
		}
		body = a.frame(a.viewForm(), append(k, [2]string{"Esc", i18n.T("abbrechen")}))
	case scrConnForm, scrKeyForm, scrSSHKeyForm, scrRelayForm, scrMonitorForm, scrHighlightForm, scrScheduleForm:
		body = a.frame(a.viewForm(), [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+S", i18n.T("speichern")}, {"Esc", i18n.T("abbrechen")}})
	case scrQuickForm:
		body = a.frame(a.viewForm(), [][2]string{{"Tab", i18n.T("weiter")}, {"Strg+R", i18n.T("Vorbelegung")}, {"Strg+S", i18n.T("speichern")}, {"Esc", i18n.T("abbrechen")}})
	case scrSettings:
		body = a.frame(a.viewForm(), [][2]string{{"Strg+T", i18n.T("Endpunkt testen")}, {"Strg+L", i18n.T("Modell wählen")}, {"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+S", i18n.T("speichern")}, {"Esc", i18n.T("abbrechen")}})
	case scrTerminal:
		body, cur = a.viewTerminal()
	case scrSync:
		body = a.frame(a.viewSync(), a.syncKeys())
	case scrLogs:
		body = a.frame(a.viewLogs(), a.logsKeys())
	case scrImport:
		body = a.frame(a.viewImport(), a.importKeys())
	case scrTeam:
		body = a.frame(a.viewTeam(), a.teamKeys())
	case scrTeamForm:
		body = a.frame(a.viewForm(), [][2]string{{"Tab", i18n.T("weiter")}, {"Space", i18n.T("an/aus")}, {"Strg+S", i18n.T("weiter")}, {"Esc", i18n.T("zurück")}})
	case scrKeys:
		body = a.frame(a.viewKeys(), a.keysKeys())
	case scrColors:
		body = a.frame(a.viewColors(), a.colorsKeys())
	case scrBroadcast:
		body = a.frame(a.viewBroadcast(), a.broadcastKeys())
	case scrScan:
		body = a.frame(a.viewScan(), a.scanKeys())
	}
	body = a.z.scan(body, 0, 0)
	if a.lock != nil {
		a.z.reset() // gesperrt: nichts klickbar, nichts zu sehen
		body, cur = a.viewLock(), nil
	} else if a.modal != nil {
		body = a.overlay(body, a.modalView())
		cur = nil
	} else if a.pick != nil {
		body = a.overlay(body, a.viewPicker())
		cur = nil
	} else if a.pal != nil {
		body = a.overlay(body, a.viewPalette())
		cur = nil
	} else if a.fx.out && a.fx.ghost != "" {
		// gerade geschlossenes Popup blendet aus (nicht mehr klickbar)
		if k, run := a.fadeLevel(); run {
			c := lipgloss.NewCompositor(lipgloss.NewLayer(body), lipgloss.NewLayer(fadeANSI(a.fx.ghost, a.s.T, k)).X(a.fx.x).Y(a.fx.y).Z(1))
			body = c.Render()
		}
	}
	if a.lock == nil {
		body = a.withToasts(body)
	}
	if a.intro != nil && introFits(a.w, a.h) {
		a.z.reset() // während der Animation ist nichts klickbar
		body, cur = a.viewIntro(), nil
	}
	v := tea.NewView(body)
	v.AltScreen = true
	v.MouseMode = a.mouseMode()
	v.Cursor = cur
	v.ReportFocus = true // Fenster im Hintergrund? (Meldungen, notify.go)
	v.WindowTitle = version.Name
	if s := a.current(); a.scr == scrTerminal && s != nil && a.lock == nil {
		v.WindowTitle = version.Name + " · " + s.conn.Alias
	}
	v.BackgroundColor = a.s.T.Bg
	v.ForegroundColor = a.s.T.Text // sonst bliebe ungefärbter Text auf Hell unsichtbar
	return v
}

// header: Logo, Version, Credit, Tabs/Status rechts.
// headerLeft: Logo, Version und Credit. Im schmalen Fenster fällt zuerst die
// Version weg (steht im Über-Dialog), der Credit bleibt immer sichtbar.
func (a *App) headerLeft() string {
	if a.w < 90 {
		return a.s.Logo.Render(">_ "+version.Name) + a.s.Credit.Render(" "+i18n.T(version.Idea)+" ")
	}
	return a.s.Logo.Render(">_ "+version.Name) + a.s.Version.Render("v"+version.Version) + a.s.Credit.Render(i18n.T(version.Idea)+"  ")
}

func (a *App) header(right string) string {
	left := a.headerLeft()
	if a.rel != nil && right == "" {
		right = a.keyZone(a.keys.label(actAbout), a.s.Header.Foreground(a.s.T.OK).Render(" ↑ v"+a.rel.Version+i18n.T(" verfügbar · ")+a.keys.label(actAbout)+" "))
	} else if a.restartPending != "" && right == "" {
		right = a.keyZone(a.keys.label(actAbout), a.s.Header.Foreground(a.s.T.OK).Render(" ◌ v"+a.restartPending+i18n.T(" installiert · Neustart ")+a.keys.label(actAbout)+" "))
	}
	if a.scr != scrTerminal && a.scr != scrUnlock {
		right += a.llmChip() // im Terminal steht der Status in der Sitzungszeile
	}
	gap := a.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		right = ansi.Truncate(right, max(0, a.w-lipgloss.Width(left)), "…")
		gap = max(0, a.w-lipgloss.Width(left)-lipgloss.Width(right))
	}
	return left + a.s.Header.Render(strings.Repeat(" ", gap)) + right
}

func (a *App) footer(keys [][2]string) string {
	if c := a.cheatLine(); c != "" {
		return c // F9: Tastenzeile mit Erklärungen für den aktuellen Bereich
	}
	shown := a.sortedFooter(keys)
	var b strings.Builder
	for _, k := range shown {
		if k[1] == "" {
			b.WriteString(a.keyZone(k[0], a.s.Key.Render(k[0])))
		} else {
			b.WriteString(a.keyChip(k[0], k[1]))
		}
	}
	out := b.String()
	if w := lipgloss.Width(out); w < a.w {
		out += a.s.Footer.Render(strings.Repeat(" ", a.w-w))
	}
	return ansi.Truncate(out, a.w, "")
}

// sortedFooter: Tasten übersetzen, nach Wichtigkeit kürzen (die Aufrufer
// liefern die wichtigsten vorn) und erst dann für die Anzeige sortieren:
// Palette, F1 … F12, ⇧F-Tasten, Rest (Strg+…, Buchstaben).
func (a *App) sortedFooter(keys [][2]string) [][2]string {
	keys = append([][2]string(nil), keys...)
	for i := range keys {
		keys[i][0] = i18n.Keys(keys[i][0])
	}
	shown := a.fitFooter(keys)
	pal := strings.ReplaceAll(i18n.Keys(a.keys.label(actPalette)), "Shift+", "⇧")
	for _, k := range shown {
		if k[0] == pal { // nur Hauptleisten; Panel-Leisten bleiben wie sie sind
			return sortFKeys(shown, pal)
		}
	}
	return shown
}

// fitFooter passt die Tastenzeile an schmale Fenster an. Eine Taste ohne
// Beschriftung sagt niemandem etwas, deshalb fallen zuerst ganze Tasten von
// hinten weg (die wichtigsten stehen vorn), solange noch einige mit Text
// übrig bleiben. Erst danach verlieren die restlichen ihre Beschreibung.
// „F1 Hilfe“ und die Erklär-Taste bleiben bis zuletzt sichtbar, dort steht alles.
func (a *App) fitFooter(keys [][2]string) [][2]string {
	out := make([][2]string, len(keys))
	help, explain := a.keys.label(actHelp), strings.ReplaceAll(a.keys.label(actCheat), "Shift+", "⇧")
	for i, k := range keys {
		out[i] = [2]string{strings.ReplaceAll(k[0], "Shift+", "⇧"), k[1]} // kürzer: ⇧F9
	}
	keep := func(k [2]string) bool { return k[0] == help || k[0] == explain }
	// Breiten einmal messen und die Summe mitführen: lipgloss.Width über
	// gerenderte Stile ist teuer, und die Schleifen unten fragten früher
	// bei jedem Schritt die ganze Zeile neu ab (pro Bild Hunderte Mal).
	kw, dw := make([]int, len(out)), make([]int, len(out))
	total := 0
	for i, k := range out {
		kw[i] = lipgloss.Width(a.s.Key.Render(k[0]))
		if k[1] != "" {
			dw[i] = lipgloss.Width(a.s.KeyDesc.Render(k[1]))
		}
		total += kw[i] + dw[i]
	}
	drop := func(i int) {
		total -= kw[i] + dw[i]
		out = append(out[:i], out[i+1:]...)
		kw = append(kw[:i], kw[i+1:]...)
		dw = append(dw[:i], dw[i+1:]...)
	}
	noDesc := func(i int) {
		total -= dw[i]
		dw[i] = 0
		out[i][1] = ""
	}
	const minKeys = 4 // so viele beschriftete Tasten bleiben mindestens
	// 1. ganze Tasten von hinten weglassen, Beschriftungen bleiben
	for i := len(out) - 1; i >= 0 && total > a.w && len(out) > minKeys; i-- {
		if !keep(out[i]) {
			drop(i)
		}
	}
	// 2. Beschreibungen von hinten weglassen (nicht die der Hilfe)
	for i := len(out) - 1; i >= 0 && total > a.w; i-- {
		if out[i][0] != help {
			noDesc(i)
		}
	}
	// 3. weitere Tasten von hinten weglassen, nie die Hilfe
	for i := len(out) - 1; i >= 0 && total > a.w; i-- {
		if out[i][0] != help {
			drop(i)
		}
	}
	// 4. zur Not auch „Hilfe“ nur als Taste
	for i := range out {
		if total > a.w && out[i][0] == help {
			noDesc(i)
		}
	}
	return out
}

func (a *App) frame(content string, keys [][2]string) string {
	h := max(1, a.h-2)
	box := lipgloss.NewStyle().Width(a.w).Height(h).MaxHeight(h).Render(content)
	return lipgloss.JoinVertical(lipgloss.Left, a.header(""), box, a.footer(keys))
}

// overlay legt ein Modal mittig über den Inhalt (Compositing mit lipgloss v2).
// Klickbar ist dann nur noch das Modal (die Zonen darunter verfallen).
func (a *App) overlay(base, box string) string {
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x, y := max(0, (a.w-bw)/2), max(0, (a.h-bh)/2)
	if m := a.modal; m != nil && m.anchored {
		x, y = clamp(m.at[0]+1, 0, max(0, a.w-bw)), clamp(m.at[1]+1, 0, max(0, a.h-bh))
	}
	a.fx.ghost, a.fx.x, a.fx.y = zoneRe.ReplaceAllString(box, ""), x, y
	if k, run := a.fadeLevel(); run && !a.fx.out {
		box = fadeANSI(box, a.s.T, k)
	}
	a.z.rects = a.z.rects[:0]
	box = a.z.scan(box, x, y)
	c := lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(box).X(x).Y(y).Z(1))
	return c.Render()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

var _ = vault.ErrLocked
