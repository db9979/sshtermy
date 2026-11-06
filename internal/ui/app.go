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
	scrImport
	scrKeys
	scrBroadcast
	scrScan
	scrKeyForm // Schlüsseldatei auf diesem Rechner wählen (nach Sync)
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
	formTunnel       domain.Tunnel
	formQuickPreset  []string // Schnellbefehle: Vorbelegung zum Vergleich
	formSID          int      // Schlüsseldatei-Abfrage: Sitzung, die danach neu verbindet
	returnToTerminal bool

	sessions []*session
	cur      int
	nextSID  int
	split    splitMode // geteilte Ansicht (Strg+T)
	panes    []int     // Sitzungs-IDs je Bereich, 0 = leer

	modal      *modal
	pal        *palette
	llm        llmUI
	lh         llmHealthState // KI-Status (llmhealth.go)
	reach      reachState     // Online-Punkt im Manager (reach.go)
	toast      string
	toastUntil time.Time
	rel        *update.Release
	// restartPending: installierte, noch nicht gestartete Version.
	restartPending string
	restart        bool // nach dem Beenden neu starten (siehe Restart)
	lastKey        time.Time

	book *sessionlog.Book // Sitzungsprotokolle (Schlüssel nur bei entsperrtem Store)
	logs logsState

	sync    syncState
	wiz     *syncWizard
	imp     importState // Import aus ssh-config, PuTTY, MobaXterm
	keyEd   keyEditor   // Tastenkürzel anpassen
	keys    keymap      // aktuelle Tastenbelegung
	lookSig string      // Theme+Tasten, zuletzt übernommen

	z     zones      // klickbare Bereiche des letzten Bildes
	click clickState // für Doppel-/Dreifachklick

	pick *picker // Auswahlliste mit Suche (Jump-Host, Ordner, Tag)

	bc     *bcState                     // Broadcast an mehrere Hosts
	rbVals map[string]map[string]string // zuletzt eingegebene Runbook-Variablen (nur im Speicher)
	scan   *scanState                   // Netzwerk-Scan

	intro *introState // Startanimation, nil = aus oder vorbei
}

func New(st *store.Store) (*App, error) {
	hk, err := sshx.NewHostKeyStore(st.KnownHostsPath())
	if err != nil {
		return nil, err
	}
	a := &App{st: st, hostKeys: hk, s: NewStyles(Nacht), events: make(chan tea.Msg, 256), lastKey: time.Now(), keys: newKeymap(nil)}
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

func (a *App) flash(s string) {
	a.toast = s
	a.toastUntil = time.Now().Add(4 * time.Second)
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
		return a, tea.Batch(tick(), a.onTick(), a.llmTick(), a.reachTick())
	case llmProbeMsg:
		a.lh.probing = false
		return a, nil
	case reachMsg:
		a.onReach(m)
		return a, nil

	case outputMsg:
		if s := a.byID(m.sid); s != nil {
			s.pending.Store(false)
			s.lastOutput = time.Now()
		}
		return a, a.listen()

	case hostKeyAskMsg:
		a.modal = hostKeyModal(m)
		return a, a.listen()
	case secretAskMsg:
		a.modal = secretModal(m)
		return a, a.listen()

	case connectedMsg:
		if s := a.byID(m.sid); s != nil {
			s.client, s.shell, s.state = m.client, m.shell, stConnected
			s.connectedAt = time.Now()
			s.tunnels = sshx.NewTunnelManager(m.client)
			a.st.Touch(s.conn.ID)
			if m.client.Learned {
				a.rememberNetDevice(s)
			}
			a.profileConnected(s)
			for _, t := range a.st.Tunnels(s.conn.ID) {
				if t.AutoStart {
					if _, err := s.tunnels.Start(t); err != nil {
						a.flash("Tunnel " + t.Name + ": " + err.Error())
					}
				}
			}
			a.startLog(s)
			var cmd tea.Cmd
			if s.rc.active {
				cmd = a.onReconnected(s)
			}
			a.resize(s)
			return a, tea.Batch(cmd, a.hostOnConnect(s))
		}
		return a, nil
	case adaptMsg:
		a.onAdapt(m)
		return a, nil
	case muxProbeMsg:
		a.onMuxProbe(m)
		return a, nil
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
		}
		return a, a.listen()

	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
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
			a.flash(i18n.T("Update fehlgeschlagen: ") + m.err.Error())
			return a, nil
		}
		if a.rel != nil {
			a.restartPending = a.rel.Version
		}
		a.rel = nil
		return a, a.askRestart()
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
	if cmd, ok := a.logsMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.filesMsg(msg); ok {
		return a, cmd
	}
	if cmd, ok := a.hostMsg(msg); ok {
		return a, cmd
	}

	if k, ok := msg.(tea.KeyPressMsg); ok {
		a.lastKey = time.Now()
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
		if a.keys.is(actHelp, k.String()) && (a.pal != nil || a.scr == scrConnForm || a.scr == scrSettings || a.scr == scrTunnelForm || a.scr == scrImport || a.scr == scrKeys || a.scr == scrBroadcast || a.scr == scrScan || a.scr == scrQuickForm || a.scr == scrKeyForm) {
			a.modal = a.helpModal()
			return a, nil
		}
		if a.pal != nil {
			return a, a.paletteKey(k)
		}
		if a.keys.is(actPalette, k.String()) && (a.scr == scrTerminal || a.scr == scrManager || a.scr == scrLogs) {
			return a, a.openPalette()
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
	case scrConnForm, scrSettings, scrTunnelForm, scrQuickForm, scrKeyForm:
		return a, a.updateForm(msg)
	case scrTerminal:
		return a, a.updateTerminal(msg)
	case scrSync:
		return a, a.updateSync(msg)
	case scrLogs:
		return a, a.updateLogs(msg)
	case scrImport:
		return a, a.updateImport(msg)
	case scrKeys:
		return a, a.updateKeys(msg)
	case scrBroadcast:
		return a, a.updateBroadcast(msg)
	case scrScan:
		return a, a.updateScan(msg)
	}
	return a, nil
}

func (a *App) quit() tea.Cmd {
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
	// Auto-Sperre nur ohne offene Sitzungen (sonst würden Shells hängen bleiben).
	if set := a.st.Settings(); set.LockAfterMin > 0 && a.scr != scrUnlock && len(a.sessions) == 0 &&
		time.Since(a.lastKey) > time.Duration(set.LockAfterMin)*time.Minute {
		a.closeBook()
		a.st.Lock()
		a.unlock = newUnlock(a.st)
		a.scr = scrUnlock
		return nil
	}
	if c := a.syncTick(); c != nil {
		cmds = append(cmds, c)
	}
	for _, s := range a.sessions {
		if s.term != nil && s.term.TakeBell() {
			cmds = append(cmds, func() tea.Msg { return nil })
		}
		a.commandTick(s)
		if c := a.followTick(s); c != nil {
			cmds = append(cmds, c)
		}
		if c := a.goalTick(s); c != nil {
			cmds = append(cmds, c)
		}
		a.profileTick(s)
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
	case scrConnForm, scrTunnelForm, scrKeyForm:
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
	case scrKeys:
		body = a.frame(a.viewKeys(), a.keysKeys())
	case scrBroadcast:
		body = a.frame(a.viewBroadcast(), a.broadcastKeys())
	case scrScan:
		body = a.frame(a.viewScan(), a.scanKeys())
	}
	body = a.z.scan(body, 0, 0)
	if a.modal != nil {
		body = a.overlay(body, a.modalView())
		cur = nil
	} else if a.pick != nil {
		body = a.overlay(body, a.viewPicker())
		cur = nil
	} else if a.pal != nil {
		body = a.overlay(body, a.viewPalette())
		cur = nil
	}
	if a.intro != nil && introFits(a.w, a.h) {
		a.z.reset() // während der Animation ist nichts klickbar
		body, cur = a.viewIntro(), nil
	}
	v := tea.NewView(body)
	v.AltScreen = true
	v.MouseMode = a.mouseMode()
	v.Cursor = cur
	v.WindowTitle = version.Name
	if s := a.current(); a.scr == scrTerminal && s != nil {
		v.WindowTitle = version.Name + " · " + s.conn.Alias
	}
	v.BackgroundColor = a.s.T.Bg
	v.ForegroundColor = a.s.T.Text // sonst bliebe ungefärbter Text auf Hell unsichtbar
	return v
}

// header: Logo, Version, Credit, Tabs/Status rechts.
func (a *App) header(right string) string {
	left := a.s.Logo.Render("◆ "+version.Name) + a.s.Version.Render("v"+version.Version) + a.s.Credit.Render(i18n.T(version.Idea)+"  ")
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		right = a.s.Header.Foreground(a.s.T.Warn).Render(" " + a.toast + " ")
	} else if a.rel != nil && right == "" {
		right = a.keyZone(a.keys.label(actAbout), a.s.Header.Foreground(a.s.T.OK).Render(" ⬆ v"+a.rel.Version+i18n.T(" verfügbar · ")+a.keys.label(actAbout)+" "))
	} else if a.restartPending != "" && right == "" {
		right = a.keyZone(a.keys.label(actAbout), a.s.Header.Foreground(a.s.T.OK).Render(" ⟳ v"+a.restartPending+i18n.T(" installiert · Neustart ")+a.keys.label(actAbout)+" "))
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
	keys = append([][2]string(nil), keys...)
	for i := range keys {
		keys[i][0] = i18n.Keys(keys[i][0])
	}
	var b strings.Builder
	for _, k := range a.fitFooter(keys) {
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
	width := func(ks [][2]string) int {
		w := 0
		for _, k := range ks {
			w += lipgloss.Width(a.s.Key.Render(k[0]))
			if k[1] != "" {
				w += lipgloss.Width(a.s.KeyDesc.Render(k[1]))
			}
		}
		return w
	}
	const minKeys = 4 // so viele beschriftete Tasten bleiben mindestens
	// 1. ganze Tasten von hinten weglassen, Beschriftungen bleiben
	for i := len(out) - 1; i >= 0 && width(out) > a.w && len(out) > minKeys; i-- {
		if !keep(out[i]) {
			out = append(out[:i], out[i+1:]...)
		}
	}
	// 2. Beschreibungen von hinten weglassen (nicht die der Hilfe)
	for i := len(out) - 1; i >= 0 && width(out) > a.w; i-- {
		if out[i][0] != help {
			out[i][1] = ""
		}
	}
	// 3. weitere Tasten von hinten weglassen, nie die Hilfe
	for i := len(out) - 1; i >= 0 && width(out) > a.w; i-- {
		if out[i][0] != help {
			out = append(out[:i], out[i+1:]...)
		}
	}
	// 4. zur Not auch „Hilfe“ nur als Taste
	for i := range out {
		if width(out) > a.w && out[i][0] == help {
			out[i][1] = ""
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
	if a.modal != nil {
		if b := a.modalButtons(); b != "" {
			box = lipgloss.JoinVertical(lipgloss.Center, box, b)
		}
	}
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x, y := max(0, (a.w-bw)/2), max(0, (a.h-bh)/2)
	if m := a.modal; m != nil && m.anchored {
		x, y = clamp(m.at[0]+1, 0, max(0, a.w-bw)), clamp(m.at[1]+1, 0, max(0, a.h-bh))
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
