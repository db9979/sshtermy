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
	"image/color"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/mcp"
	"github.com/db9979/sshtermy/internal/netwatch"
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
	scrBulkForm    // Sammeländerung an markierten Verbindungen (bulk.go)
	scrFolderForm  // Ordner bearbeiten: Name und KI-Hausregeln (mgrtree.go)
	scrImport
	scrKeys
	scrBroadcast
	scrScan
	scrKeyForm       // Schlüsseldatei auf diesem Rechner wählen (nach Sync)
	scrSSHKeyForm    // SSH-Schlüssel erzeugen / übertragen (sshkey.go)
	scrRelayForm     // Vermittler koppeln (relay.go)
	scrShareForm     // Sitzung teilen: Weg und Freigabe (sharedialog.go)
	scrRelaySetup    // Vermittler-Assistent: Server und Adresse (relaysetup.go)
	scrMasterForm    // Master-Passwort ändern, DPAPI (master.go)
	scrBackupForm    // Gesamtsicherung speichern/einspielen (fullbackup.go)
	scrColors        // Farben anpassen (colors.go)
	scrThemes        // Farbschema wählen mit Vorschau (themepick.go)
	scrTeam          // Team-Freigabe: Auswahl bzw. Vorschau (team.go)
	scrTeamForm      // Team-Freigabe: Passwort, Datei, Optionen
	scrHighlightForm // Farbregeln (highlight.go)
	scrScheduleForm  // Zeitplan (schedule.go)
	scrRulesForm     // Regeln für Switch-Configs (configrules.go)
	scrLogScan       // Log-Blick (logscan.go)
	scrCmdSets       // Befehlssätze (cmdsets.go)
	scrCmdSetForm    // eigener Befehlssatz bearbeiten
)

// App ist das Wurzel-Modell.
type App struct {
	// asyncModals: Rückfragen, die von selbst kamen, während schon ein Dialog
	// offen war (modalguard.go). Sie überschreiben ihn nicht, sie warten.
	asyncModals []*modal

	// heldEat: Taste (⏎ oder j), die gerade einen riskanten Befehl bestätigt
	// hat. Ihre weiteren Wiederholungen werden verschluckt (dangerguard.go).
	heldEat heldKeyEat

	lscan         logScanState      // Log-Blick (logscan.go)
	cmds          cmdsetState       // Befehlssätze (cmdsets.go)
	lwatch        logWatchState     // Logwache + Syslog-Empfang (logwatch.go)
	fresh         storeFresh        // zweites sshTermy hat gespeichert (storefresh.go)
	quickVals     map[string]string // zuletzt eingegebene Platzhalter-Werte (quickvars.go)
	lfold         folderScanState   // Ordner-Scan (logfolder.go)
	compUse       companionUsage    // Begleiter: Anfragen, Tokens (companion.go)
	recap         *recapState       // Begleiter: laufender Rückblick
	windowed      bool              // im eigenen Fenster (internal/gui), SetWindowed
	inputLog      *os.File          // nur mit SSHTERMY_DEBUG=1
	audit         *audit.Log        // audit.go
	nw            *netwatch.Watcher // Ruhezustand/Netzwechsel (netwatch.go)
	nwAt          time.Time
	mcpHost       *mcp.Host // mcp.go
	debugForced   bool      // --debug: Debug-Protokoll für diesen Start (debugreport.go)
	startNotice   string    // Meldung nach dem Entsperren (fullbackup.go)
	startNoticeOK bool
	st            *store.Store
	hostKeys      *sshx.HostKeyStore
	s             Styles
	w, h          int
	scr           screen
	events        chan tea.Msg

	unlock           unlockState
	mgr              managerState
	form             *form
	formID           string // ID des bearbeiteten Datensatzes
	formBase         domain.Connection
	formKC           [2]string // Verbindungsformular: Werte aus dem Schlüsselbund beim Öffnen
	formTunnel       domain.Tunnel
	formFolder       string   // Pfad des Ordners im Ordner-Formular
	formBulk         []string // Verbindungen der Sammeländerung (bulk.go)
	searchAllLast    string   // letzter Suchbegriff der Suche über alle Tabs
	tabDrag          *tabDrag // Tab wird mit der Maus gezogen (tabs.go)
	formQuickPreset  []string // Schnellbefehle: Vorbelegung zum Vergleich
	formSID          int      // Schlüsseldatei-Abfrage: Sitzung, die danach neu verbindet
	returnToTerminal bool
	shareForm        shareFormState // Teilen-Dialog (sharedialog.go)
	undo             *undoState     // Löschen rückgängig machen (undo.go)

	sessions []*session
	cur      int
	nextSID  int
	split    splitMode // geteilte Ansicht (Strg+T)
	syncIn   bool      // Eingabe an alle Bereiche (syncinput.go)
	panes    []int     // Sitzungs-IDs je Bereich, 0 = leer
	fracX    []float64 // Spaltenanteile der geteilten Ansicht (nil = gleich)
	fracY    []float64 // Zeilenanteile (nil = gleich)
	zoom     bool      // aktiver Bereich aufs ganze Fenster (Alt+Z)
	sfx      splitFx   // gleitender Rahmen (splitfx.go)
	sdrag    *splitDrag
	dropOn   int // Titel wird gezogen: Zielbereich+1

	modal        *modal
	pal          *palette
	llm          llmUI
	lh           llmHealthState // KI-Status (llmhealth.go)
	reach        reachState     // Online-Punkt im Manager (reach.go)
	toasts       []toastItem    // Einblendungen oben rechts (toast.go)
	perf         perfState      // Bildrate und Leistungsanzeige (perf.go)
	styleGen     int            // zählt Theme-Wechsel (termView)
	toastTicking bool
	animTicking  bool       // Takt für Bewegung (motion.go)
	hov          hoverState // Karte unter der Maus (motion.go)
	rel          *update.Release
	// restartPending: installierte, noch nicht gestartete Version.
	restartPending string
	restart        bool // nach dem Beenden neu starten (siehe Restart)
	lastKey        time.Time

	hl      hlState       // Farbregeln (highlight.go)
	edits   []*remoteEdit // Server-Dateien im eigenen Editor (editor.go)
	openTmp []string      // heruntergeladen zum Öffnen mit dem Standardprogramm (fileview.go)

	formSched     domain.Schedule   // Zeitplan im Formular (schedule.go)
	schedRBIDs    map[string]string // Runbook-Name → ID im Formular
	schedMachines map[string]string // Rechner-Auswahl im Zeitplan-Formular → Name
	schedRun      map[string]bool   // laufende Zeitpläne
	netData       *netCache         // letzte Abfrage der Switches (netfind.go)
	netBusy       bool
	netScope      string // zuletzt gewähltes Netz (Ordner) für Gerät finden/Topologie

	book *sessionlog.Book // Sitzungsprotokolle (Schlüssel nur bei entsperrtem Store)
	logs logsState

	sync      syncState
	wiz       *syncWizard
	imp       importState   // Import aus ssh-config, PuTTY, MobaXterm
	team      teamState     // Team-Freigabe (team.go)
	tl        teamLinkState // Team-Ordner: veröffentlichen/abonnieren (teamlink.go)
	keyEd     keyEditor     // Tastenkürzel anpassen
	colEd     colorEditor   // Farben anpassen
	themePick themePicker   // Farbschema wählen
	keys      keymap        // aktuelle Tastenbelegung
	lookSig   string        // Theme+Tasten, zuletzt übernommen

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
	vg      *vorgang   // Vorgang über mehrere Fenster (vorgang.go)
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
	a.hookFresh()
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

// checkUpdate fragt nach einer neuen Version. Mit dem Kanal „stabil“
// (Einstellungen, updatechannel.go) zählen nur Versionen, die schon eine
// Weile veröffentlicht sind.
func (a *App) checkUpdate() tea.Cmd {
	wait := time.Duration(a.st.Settings().UpdateWaitH) * time.Hour
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		r, err := checkRelease(ctx, wait)
		return updateMsg{r, err}
	}
}

// checkRelease: Variable für Tests.
var checkRelease = update.CheckAged

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
	a.perf.skip = false // jede Nachricht außer Ausgabeflut zeichnet sofort (perf.go)
	switch msg.(type) {
	case WindowClosedMsg:
		return a, a.askQuit() // quitask.go
	case perfRedrawMsg:
		a.perf.redraw = false
		return a, nil
	case perfTickMsg:
		if a.perf.on {
			return a, a.perfTick()
		}
		return a, nil
	case fadeMsg:
		if a.fadeRunning() {
			return a, fadeTick()
		}
		return a, nil
	case toastTickMsg:
		a.toastTicking = false
		a.expireUndo(time.Now())
		a.liveToasts(time.Now())
		return a, a.toastTick()
	case animMsg:
		a.animTicking = false
		for _, s := range a.sessions {
			s.ask.typeOn(time.Now())
		}
		return a, tea.Batch(a.dangerHoldTick(), a.animTick())
	}
	if _, ok := msg.(tea.KeyPressMsg); ok {
		a.hov = hoverState{} // Karte weg, sobald getippt wird
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && a.heldEat.eat(k, time.Now()) {
		return a, nil // Taste noch vom Bestätigen gehalten: nicht ans Terminal
	}
	if k, ok := msg.(tea.KeyReleaseMsg); ok {
		a.heldEat.release(k)
	}
	if k, ok := msg.(tea.KeyReleaseMsg); ok && a.modal != nil && a.modal.kind == mDanger && k.String() == "enter" {
		a.modal.danger.hold.release(time.Now())
		return a, a.animTick()
	}
	a.debugMsg(msg)
	a.auditMsg(msg)
	m, cmd := a.update(msg)
	a.popAsyncModal() // modalguard.go
	a.clearNotices()
	if c := a.trackFade(); c != nil {
		cmd = tea.Batch(cmd, c)
	}
	if t := a.toastTick(); t != nil {
		cmd = tea.Batch(cmd, t)
	}
	if a.scr == scrTerminal && a.split != splitOff {
		a.trackFocusGlide(a.current()) // Rahmen gleitet zum neuen Bereich (splitfx.go)
	}
	if t := a.animTick(); t != nil {
		cmd = tea.Batch(cmd, t)
	}
	return m, cmd
}

func (a *App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	a.logInput(msg)
	if cmd, ok := a.introMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.coreMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.featureMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.overlayMsg(msg); ok {
		return a, cmd
	}
	return a, a.screenMsg(msg)
}

// coreMsg: Fenster, Takt, Verbindungsaufbau, Maus, Update. ok = false:
// die Nachricht gehört einem der Bereiche in featureMsg oder der Ansicht.
func (a *App) coreMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = m.Width, m.Height
		for _, s := range a.sessions {
			a.resize(s)
		}
		return nil, true

	case tickMsg:
		a.freshTick()
		return tea.Batch(tick(), a.onTick(), a.llmTick(), a.reachTick(), a.crashTestTick(), a.netWatchTick()), true
	case crashTestMsg:
		panic("sshTermy crash test (ui) password=hunter2")
	case llmProbeMsg:
		a.lh.probing = false
		return nil, true
	case reachMsg:
		a.onReach(m)
		return nil, true

	case outputMsg:
		var clip tea.Cmd
		if s := a.byID(m.sid); s != nil {
			s.pending.Store(false)
			s.lastOutput = time.Now()
			if !a.sessionVisible(s) {
				s.unseen = true
			}
			clip = a.takeClipboard(s)
		}
		return tea.Batch(a.listen(), clip, a.onOutputFrame(time.Now()), a.takePixels(m.sid)), true
	case tea.FocusMsg:
		a.blurred = false
		a.clearNotices()
		return nil, true
	case tea.BlurMsg:
		a.blurred = true
		return nil, true

	case mcpCallMsg:
		return tea.Batch(a.onMCP(m), a.listen()), true
	case connTestMsg:
		if a.modal != nil && a.modal.kind == mBusy {
			a.modal = nil
		}
		a.connTestResult(m)
		return nil, true
	case hostKeyAskMsg:
		a.showAsync(hostKeyModal(m))
		return a.listen(), true
	case secretAskMsg:
		a.showAsync(secretModal(m))
		return a.listen(), true
	case connectStepMsg:
		if s := a.byID(m.sid); s != nil && m.step >= 0 && m.step < len(s.connStepAt) {
			s.connStep = m.step
			if m.step == sshx.StepConnect {
				s.connStepAt = [4]time.Time{} // neuer Versuch
			}
			if s.connStepAt[m.step].IsZero() {
				s.connStepAt[m.step] = time.Now()
			}
		}
		return a.listen(), true
	case connectNoticeMsg:
		if s := a.byID(m.sid); s != nil {
			s.connNote = m.text
		}
		return a.listen(), true
	case sshKeyDoneMsg:
		return a.sshKeyDone(m), true
	case relayMsg:
		return m.apply(a), true
	case teamLinkMsg:
		return m.apply(a), true
	case macLearnedMsg:
		a.onMACLearned(m)
		return nil, true
	case wakeDoneMsg:
		a.onWakeDone(m)
		return nil, true

	case connectedMsg:
		if s := a.byID(m.sid); s != nil {
			s.client, s.state, s.connNote, s.errv = m.client, stConnected, "", nil
			s.setShell(m.shell)
			s.connectedAt = time.Now()
			s.flashTab(a.s.T.OK)
			if !s.rc.active {
				s.revealAt = s.connectedAt // Zeilen blenden ein (motion2.go)
			}
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
			if m.client != nil {
				if w := m.client.WeakNote(); w != "" {
					a.flash(w) // „Vorsicht: …“ = gelber Toast
				}
			}
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
			a.armStartCmd(s)
			return tea.Batch(cmd, a.hostOnConnect(s), a.persistOnConnect(s), a.learnMAC(s)), true
		}
		return nil, true
	case adaptMsg:
		a.onAdapt(m)
		return nil, true
	case muxProbeMsg:
		a.onMuxProbe(m)
		return nil, true
	case persistMsg:
		return a.onPersist(m), true
	case connectErrMsg:
		if s := a.byID(m.sid); s != nil {
			if s.rc.active {
				a.onReconnectErr(s, m.err)
			} else {
				s.state, s.err, s.errv = stClosed, m.err.Error(), m.err
				s.flashTab(a.s.T.Danger)
				var km *sshx.KeyMissingError
				if errors.As(m.err, &km) && a.form == nil && a.modal == nil && a.pick == nil && a.pal == nil && a.scr == scrTerminal {
					return a.openKeyFix(s.id, km), true
				}
			}
		}
		return nil, true
	case shellClosedMsg:
		if s := a.byID(m.sid); s != nil {
			a.onShellClosed(s, m)
			if s.state == stReconnect {
				s.detachNotice(i18n.T("Verbindung abgerissen, verbinde neu …"))
			}
		}
		return a.listen(), true
	case detachConnMsg:
		a.onDetachConn(m)
		return a.listen(), true
	case detachResizeMsg:
		a.onDetachResize(m)
		return a.listen(), true
	case detachEndMsg:
		a.onDetachEnd(m)
		return a.listen(), true

	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
		if a.lock != nil {
			return nil, true
		}
		return a.mouse(msg.(tea.MouseMsg)), true
	case pasteMsg:
		return a.onPaste(m), true
	case pingDoneMsg:
		a.onPingDone(m)
		return nil, true

	case updateMsg:
		if m.rel != nil {
			a.rel = m.rel
			a.flash(i18n.T("Update auf v") + m.rel.Version + i18n.T(" verfügbar (") + a.keys.label(actAbout) + ")")
		}
		return nil, true
	case applyDoneMsg:
		a.modal = nil
		if m.err != nil {
			a.flashErr(i18n.T("Update fehlgeschlagen: ") + m.err.Error())
			return nil, true
		}
		if a.rel != nil {
			a.restartPending = a.rel.Version
		}
		a.rel = nil
		return a.askRestart(), true
	}
	return nil, false
}

// featureMsg reicht die Nachricht an die Bereiche weiter (Broadcast, Sync,
// KI, Dateien …); jeder nimmt nur seine eigenen.
func (a *App) featureMsg(msg tea.Msg) (tea.Cmd, bool) {
	if cmd, ok := a.trzMsg(msg); ok {
		return cmd, true
	}
	// Broadcast und Netzwerk-Scan (Fortschritt kommt über a.events)
	if cmd, ok := a.bcMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.scanMsg(msg); ok {
		return cmd, true
	}

	// Sync zwischen Rechnern
	if cmd, ok := a.syncMsg(msg); ok {
		return cmd, true
	}

	// Nachrichten der Sitzungen (KI, Ziel, Frage)
	if cmd, ok := a.vgMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.companionMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.sessionMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.llmMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.reportMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.cmdsetMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.logsMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.scheduleMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.netMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.schedLogsMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.syslogSetupMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.folderScanMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.watchMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.logScanResult(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.containerMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.shareMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.editMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.filesMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.sudoMsg(msg); ok {
		return cmd, true
	}
	if cmd, ok := a.hostMsg(msg); ok {
		return cmd, true
	}

	if cmd, ok := a.lockMsg(msg); ok {
		return cmd, true
	}
	return nil, false
}

// overlayMsg: Tasten und Einfügen, die vor der Ansicht kommen (Beenden,
// Dialog, Auswahlliste, Palette, Hilfe).
func (a *App) overlayMsg(msg tea.Msg) (tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		a.lastKey = time.Now()
		if a.scr == scrColors && a.colEd.editing && a.modal == nil {
			return a.updateColors(k), true // Hex-Eingabe: Buchstaben gehören dem Feld
		}
		if a.scr == scrKeys && a.keyEd.capturing && a.modal == nil {
			return a.updateKeys(k), true // neue Taste: nichts anderes darf sie abfangen
		}
		if a.keys.is(actQuit, k.String()) {
			if a.modal != nil && a.modal.quit {
				return a.quit(), true // zweimal Strg+Q = beenden
			}
			return a.askQuit(), true
		}
		if a.modal != nil {
			return a.modalKey(k), true
		}
		if a.pick != nil {
			return a.pickerKey(k), true
		}
		if a.keys.is(actHelp, k.String()) && (a.pal != nil || a.scr == scrConnForm || a.scr == scrSettings || a.scr == scrTunnelForm || a.scr == scrImport || a.scr == scrKeys || a.scr == scrBroadcast || a.scr == scrScan || a.scr == scrQuickForm || a.scr == scrMonitorForm || a.scr == scrFolderForm || a.scr == scrHighlightForm || a.scr == scrScheduleForm || a.scr == scrRulesForm || a.scr == scrCmdSetForm || a.scr == scrKeyForm || a.scr == scrSSHKeyForm || a.scr == scrRelayForm || a.scr == scrShareForm || a.scr == scrRelaySetup || a.scr == scrMasterForm || a.scr == scrBackupForm || a.scr == scrBulkForm) {
			a.modal = a.helpModal()
			return nil, true
		}
		if a.pal != nil {
			return a.paletteKey(k), true
		}
		if a.keys.is(actPalette, k.String()) && (a.scr == scrTerminal || a.scr == scrManager || a.scr == scrLogs || a.scr == scrLogScan) {
			return a.openPalette(), true
		}
		if a.keys.is(actLocal, k.String()) && (a.scr == scrTerminal || a.scr == scrManager) && a.lock == nil {
			return a.openLocal(""), true
		}
	}
	if pm, ok := msg.(tea.PasteMsg); ok && a.modal != nil {
		// Einfügen in einen offenen Dialog: nur das Eingabefeld nimmt es an.
		if a.modal.kind == mSecret || a.modal.kind == mInput {
			var cmd tea.Cmd
			a.modal.in, cmd = a.modal.in.Update(pm)
			return cmd, true
		}
		return nil, true
	}
	if pm, ok := msg.(tea.PasteMsg); ok && a.pick != nil {
		var cmd tea.Cmd
		a.pick.in, cmd = a.pick.in.Update(pm)
		a.pick.sel, a.pick.top = 0, 0
		return cmd, true
	}
	if pm, ok := msg.(tea.PasteMsg); ok && a.pal != nil && a.modal == nil {
		var cmd tea.Cmd
		a.pal.in, cmd = a.pal.in.Update(pm)
		a.pal.sel, a.pal.armed = 0, -1
		return cmd, true
	}

	return nil, false
}

// screenMsg gibt die Nachricht an die Ansicht, die gerade zu sehen ist.
func (a *App) screenMsg(msg tea.Msg) tea.Cmd {
	switch a.scr {
	case scrUnlock:
		return a.updateUnlock(msg)
	case scrManager:
		return a.updateManager(msg)
	case scrConnForm, scrSettings, scrTunnelForm, scrQuickForm, scrMonitorForm, scrFolderForm, scrHighlightForm, scrScheduleForm, scrRulesForm, scrCmdSetForm, scrKeyForm, scrSSHKeyForm, scrRelayForm, scrShareForm, scrRelaySetup, scrMasterForm, scrBackupForm, scrBulkForm:
		return a.updateForm(msg)
	case scrTerminal:
		return a.updateTerminal(msg)
	case scrSync:
		return a.updateSync(msg)
	case scrLogs:
		return a.updateLogs(msg)
	case scrLogScan:
		return a.updateLogScan(msg)
	case scrCmdSets:
		return a.updateCmdSets(msg)
	case scrImport:
		return a.updateImport(msg)
	case scrTeam:
		return a.updateTeam(msg)
	case scrTeamForm:
		return a.updateTeamForm(msg)
	case scrKeys:
		return a.updateKeys(msg)
	case scrColors:
		return a.updateColors(msg)
	case scrThemes:
		return a.updateThemes(msg)
	case scrBroadcast:
		return a.updateBroadcast(msg)
	case scrScan:
		return a.updateScan(msg)
	}
	return nil
}

// WindowClosedMsg: Das eigene Fenster (internal/gui) wurde geschlossen.
// sshTermy beendet sich dann wie mit Strg+Q: Sitzungen, Protokolle und
// Aufzeichnungen werden sauber geschlossen.
type WindowClosedMsg struct{}

func (a *App) quit() tea.Cmd {
	a.finishUndo()     // offenes Löschen endgültig machen (undo.go)
	a.rememberLayout() // geteilte Ansicht als „Zuletzt“ (workspace.go)
	a.cleanupEdits()
	a.cleanupOpenTmp()
	sshx.StopSKAgent() // eigener ssh-agent für Sicherheitsschlüssel
	for _, s := range a.sessions {
		a.endLog(s)
		a.stopRecording(s)
		s.close()
	}
	a.stopWatch()
	a.closeBook()
	a.stopMCP()
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
	a.watchTick()
	if c := a.vgTick(); c != nil {
		cmds = append(cmds, c)
	}
	for _, s := range a.sessions {
		if s.term != nil && s.term.TakeBell() {
			a.noticeBell(s)
		}
		a.alertTick(s)
		s.recordLatency(time.Now())
		a.shareTick(s)
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
		if c := a.companionTick(s); c != nil {
			cmds = append(cmds, c)
		}
		if c := a.cardFixTick(s); c != nil {
			cmds = append(cmds, c)
		}
		a.profileTick(s)
		if c := a.liveTick(s); c != nil {
			cmds = append(cmds, c)
		}
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

// View: gemessen, bei Ausgabeflut gedrosselt (perf.go).
func (a *App) View() tea.View { return a.view() }

func (a *App) viewFrame() tea.View {
	a.z.reset()
	var body string
	var cur *tea.Cursor
	switch a.scr {
	case scrUnlock:
		body = a.viewUnlock()
	case scrManager:
		body = a.frame(a.viewManager(), a.simpleKeys(a.managerKeys()))
	case scrTunnelForm:
		k := [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+S", i18n.T("speichern")}}
		if a.formTunnel.ID != "" {
			k = append(k, [2]string{"Strg+D", i18n.T("löschen")})
		}
		body = a.frame(a.viewForm(), append(k, [2]string{"Esc", i18n.T("abbrechen")}))
	case scrConnForm:
		body = a.frame(a.viewForm(), [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+T", i18n.T("testen")}, {"Strg+S", i18n.T("speichern")}, {"Esc", i18n.T("abbrechen")}})
	case scrShareForm:
		body = a.frame(a.viewForm(), [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+S", i18n.T("Einladung erzeugen")}, {"Esc", i18n.T("abbrechen")}})
	case scrRelaySetup:
		body = a.frame(a.viewForm(), [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+S", i18n.T("weiter zu den Schritten")}, {"Esc", i18n.T("abbrechen")}})
	case scrKeyForm, scrSSHKeyForm, scrRelayForm, scrMasterForm, scrBackupForm, scrMonitorForm, scrFolderForm, scrHighlightForm, scrScheduleForm, scrRulesForm, scrCmdSetForm, scrBulkForm:
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
	case scrLogScan:
		body = a.frame(a.viewLogScan(), a.logScanKeys())
	case scrCmdSets:
		body = a.frame(a.viewCmdSets(), a.cmdSetsKeys())
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
	case scrThemes:
		body = a.frame(a.viewThemes(), a.themesKeys())
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
			body = stamp(body, fadeANSI(a.fx.ghost, a.s.T, k), a.fx.x, a.fx.y, a.w)
		}
	}
	if a.lock == nil {
		body = a.withHover(body)
		body = a.withToasts(body)
		body = a.withPerf(body)
	}
	if a.intro != nil && introFits(a.w, a.h) {
		a.z.reset() // während der Animation ist nichts klickbar
		body, cur = a.viewIntro(), nil
	}
	// Grundfarben in jede Zelle (fillbg.go): sonst bleiben helle Themes in
	// Terminals ohne OSC 11 dunkel.
	v := tea.NewView(fillDefaultColors(body, a.s.T.Text, a.s.T.Bg))
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

// header: Logo links, Tabs/Status rechts. Version und Credit stehen im
// Über-Dialog (F12), im Startbild und beim Entsperren, nicht in der Kopfzeile:
// dort kosteten sie im schmalen Fenster die Tabnamen.
func (a *App) headerLeft() string {
	return a.logoChip(">_ " + version.Name) // gradient.go
}

// headerTint: Kopfzeile links mit einem Hauch Akzent, der nach rechts
// ausläuft (gradient.go).
func (a *App) headerTint() func(int) color.Color {
	return barTint(a.s.T.Accent, a.s.T.Surface, 0.38, max(24, a.w/3))
}

func (a *App) header(right string) string {
	if a.rel != nil && right == "" {
		right = a.keyZone(a.keys.label(actAbout), a.s.Header.Foreground(a.s.T.OK).Render(" ↑ v"+a.rel.Version+i18n.T(" verfügbar · ")+a.keys.label(actAbout)+" "))
	} else if a.restartPending != "" && right == "" {
		right = a.keyZone(a.keys.label(actAbout), a.s.Header.Foreground(a.s.T.OK).Render(" ◌ v"+a.restartPending+i18n.T(" installiert · Neustart ")+a.keys.label(actAbout)+" "))
	}
	if a.scr != scrTerminal && a.scr != scrUnlock {
		right += a.llmChip() // im Terminal steht der Status in der Sitzungszeile
	}
	if a.scr == scrManager {
		right = a.syncChip() + right // manager.go
	}
	return a.headerRow(right)
}

// headerRow: Logo links, right rechtsbündig, dazwischen die getönte Fläche.
func (a *App) headerRow(right string) string {
	left := a.headerLeft()
	gap := a.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		right = ansi.Truncate(right, max(0, a.w-lipgloss.Width(left)), "…")
		gap = max(0, a.w-lipgloss.Width(left)-lipgloss.Width(right))
	}
	return left + tintRun(strings.Repeat(" ", gap), lipgloss.Width(left), a.s.Header, a.headerTint()) + right
}

func (a *App) footer(keys [][2]string) string {
	if c := a.cheatLine(); c != "" {
		return c // F9: Tastenzeile mit Erklärungen für den aktuellen Bereich
	}
	shown := a.sortedFooter(keys)
	fbar := a.isMainBar(keys)
	var b strings.Builder
	for _, k := range shown {
		if fbar && isFKeyLabel(k[0]) {
			b.WriteString(a.fkeyChip(k[0], k[1]))
			continue
		}
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
	shown := a.fitFooterAs(keys, a.isMainBar(keys))
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
func (a *App) fitFooter(keys [][2]string) [][2]string { return a.fitFooterAs(keys, false) }

// isMainBar: Hauptleiste (Manager, Terminal) erkennt man an der Palette-Taste.
// Dort stehen F-Tasten kurz als Ziffer + Wort (fkeyChip).
func (a *App) isMainBar(keys [][2]string) bool {
	pal := i18n.Keys(a.keys.label(actPalette))
	for _, k := range keys {
		if i18n.Keys(k[0]) == pal || strings.ReplaceAll(k[0], "Shift+", "⇧") == strings.ReplaceAll(pal, "Shift+", "⇧") {
			return true
		}
	}
	return false
}

// isFKeyLabel: „F2“, „F10“ oder „F7/F8“ (ohne Umschalt).
func isFKeyLabel(k string) bool {
	if k == "" {
		return false
	}
	for _, p := range strings.Split(k, "/") {
		if len(p) < 2 || len(p) > 3 || p[0] != 'F' {
			return false
		}
		for _, r := range p[1:] {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// fkeyChip: F-Taste als Leiste wie in klassischen Werkzeugen: die Ziffer auf
// Akzentfarbe, dahinter das Wort. „F2 Ziel“ wird „2Ziel“ und braucht so drei
// Zeichen weniger; damit passen alle F-Tasten auch in 80 Spalten.
func (a *App) fkeyChip(key, desc string) string {
	t := a.s.T
	num := lipgloss.NewStyle().Background(t.Accent).Foreground(t.Bg).Bold(true)
	parts := strings.Split(key, "/")
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(num.Render("/"))
		}
		b.WriteString(a.keyZone(p, num.Render(strings.TrimPrefix(p, "F"))))
	}
	lbl := lipgloss.NewStyle().Background(t.Surface2).Foreground(t.Text)
	if desc == "" {
		return b.String() + a.s.Footer.Render(" ")
	}
	return b.String() + a.keyZone(parts[0], lbl.Render(desc)) + a.s.Footer.Render(" ")
}

func fkeyChipWidth(key, desc string) int {
	return len([]rune(key)) - strings.Count(key, "F") + len([]rune(desc)) + 1
}

func (a *App) fitFooterAs(keys [][2]string, fbar bool) [][2]string {
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
		if fbar && isFKeyLabel(k[0]) {
			kw[i] = fkeyChipWidth(k[0], "")
			dw[i] = len([]rune(k[1]))
			total += kw[i] + dw[i]
			continue
		}
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
	box := frameBlock(content, a.w, h) // stamp.go
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
	layers := []*lipgloss.Layer{lipgloss.NewLayer(base)}
	if shadowsOn() {
		layers = a.shadowLayers(base, x, y, bw, bh) // frames.go
	}
	c := lipgloss.NewCompositor(append(layers, lipgloss.NewLayer(box).X(x).Y(y).Z(2))...)
	return c.Render()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

var _ = vault.ErrLocked
