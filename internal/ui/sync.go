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

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/syncx"
)

// Sync zwischen Rechnern: Assistent (Taste y im Manager) und Hintergrund-Abgleich.
//
//	Start/Entsperren ─▶ Abgleich
//	lokale Änderung  ─▶ 3 s warten ─▶ Abgleich
//	alle 5 Minuten   ─▶ Abgleich
//
// Der Abgleich läuft im Hintergrund und blockiert die Oberfläche nie.

const (
	syncDebounce = 3 * time.Second
	syncInterval = 5 * time.Minute
)

type syncState struct {
	running bool
	again   bool
	gen     int
	next    time.Time
	lastErr string
}

type syncChangedMsg struct{}
type syncKickMsg struct{ gen int }
type syncDoneMsg struct {
	merged *syncx.Payload
	rev    uint64
	stats  syncx.Stats
	arch   archiveStats
	err    error
}

func deviceName() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return i18n.T("unbekannt")
}

func backendFor(c domain.SyncConfig) (syncx.Backend, error) {
	switch c.Backend {
	case domain.SyncFolder:
		return syncx.Folder{Dir: syncx.ExpandHome(c.Folder)}, nil
	case domain.SyncWebDAV:
		return &syncx.WebDAV{URL: c.URL, User: c.User, Password: c.Password.Reveal(), CertPin: c.CertPin}, nil
	}
	return nil, errors.New(i18n.T("Sync ist nicht eingerichtet"))
}

// hookSync verbindet Store-Änderungen mit dem Abgleich.
func (a *App) hookSync() {
	ev := a.events
	a.st.OnChange = func() {
		select {
		case ev <- syncChangedMsg{}:
		default:
		}
	}
}

func (a *App) scheduleSync(d time.Duration) tea.Cmd {
	if !a.st.SyncConfig().Enabled() || !a.st.SyncConfig().Auto {
		return nil
	}
	a.sync.gen++
	gen := a.sync.gen
	return tea.Tick(d, func(time.Time) tea.Msg { return syncKickMsg{gen} })
}

// runSync startet einen Abgleich (oder merkt ihn vor, falls einer läuft).
func (a *App) runSync() tea.Cmd {
	cfg := a.st.SyncConfig()
	if !cfg.Enabled() {
		return nil
	}
	if a.sync.running {
		a.sync.again = true
		return nil
	}
	d, rev, err := a.st.Snapshot()
	if err != nil {
		return nil
	}
	var kh []string
	if cfg.HostKeys {
		kh = a.hostKeys.Lines()
	}
	local := syncx.FromData(d, kh)
	var book *sessionlog.Book
	if cfg.Archive {
		book, _ = a.logBook() // gesperrt/Fehler: nur Sicherungen
	}
	st := a.st
	a.sync.running, a.sync.again = true, false
	a.sync.next = time.Now().Add(syncInterval)
	return func() tea.Msg {
		b, err := backendFor(cfg)
		if err != nil {
			return syncDoneMsg{err: err}
		}
		k, err := syncx.ParseKey(cfg.Key.Reveal())
		if err != nil {
			return syncDoneMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		m, stats, err := syncx.Run(ctx, b, k, local, deviceName())
		done := syncDoneMsg{merged: m, rev: rev, stats: stats, err: err}
		if err == nil && cfg.Archive {
			actx, acancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer acancel()
			done.arch = runArchive(actx, b, k, st, book, m.Deleted, cfg.ArchiveDays)
		}
		return done
	}
}

func (a *App) syncMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case syncChangedMsg:
		return tea.Batch(a.listen(), a.scheduleSync(syncDebounce)), true
	case syncKickMsg:
		if m.gen == a.sync.gen {
			return a.runSync(), true
		}
		return nil, true
	case syncDoneMsg:
		a.sync.running = false
		cmd := a.syncDone(m)
		if a.sync.again {
			cmd = tea.Batch(cmd, a.runSync())
		}
		return cmd, true
	case syncCheckMsg:
		return a.wizCheckDone(m), true
	case syncSetupMsg:
		return a.wizSetupDone(m), true
	}
	return nil, false
}

func (a *App) syncDone(m syncDoneMsg) tea.Cmd {
	now := time.Now().UTC()
	if m.err != nil {
		e := m.err.Error()
		if e != a.sync.lastErr {
			a.flash(i18n.T("Sync: ") + e)
		}
		a.sync.lastErr = e
		cfg := a.st.SyncConfig()
		cfg.LastError = e
		_ = a.st.SetSyncConfig(cfg)
		a.wizFirstSync(m)
		return nil
	}
	a.sync.lastErr = ""
	cfg := a.st.SyncConfig()
	changed, err := a.st.ApplySync(m.rev, func(d *domain.Data) {
		cur := syncx.FromData(d, nil)
		syncx.Merge(cur, m.merged, now).ApplyTo(d)
		d.Sync.LastSync, d.Sync.LastError = now, ""
	})
	if err != nil {
		return nil
	}
	if cfg.HostKeys {
		_, _ = a.hostKeys.AddLines(m.merged.KnownHosts)
	}
	if n := len(a.st.Connections()); a.mgr.sel >= n {
		a.mgr.sel = max(0, n-1)
	}
	if n := m.stats.Added + m.stats.Changed + m.stats.Gone; n > 0 && a.wiz == nil {
		var parts []string
		if m.stats.Added > 0 {
			parts = append(parts, fmt.Sprintf(i18n.T("%d neu"), m.stats.Added))
		}
		if m.stats.Changed > 0 {
			parts = append(parts, fmt.Sprintf(i18n.T("%d geändert"), m.stats.Changed))
		}
		if m.stats.Gone > 0 {
			parts = append(parts, fmt.Sprintf(i18n.T("%d entfernt"), m.stats.Gone))
		}
		a.flash(i18n.T("Sync: ") + strings.Join(parts, ", "))
	}
	if n := archiveNote(m.arch); n != "" && a.wiz == nil {
		a.flash(n)
	}
	if (m.arch.dropped > 0 || m.arch.logs > 0) && a.scr == scrLogs {
		a.reloadLogs()
	}
	if m.arch.err != nil {
		if e := i18n.T("Sync-Archiv: ") + m.arch.err.Error(); e != a.sync.lastErr {
			a.flash(e)
			a.sync.lastErr = e
		}
	}
	a.wizFirstSync(m)
	if changed {
		return a.scheduleSync(syncDebounce)
	}
	return nil
}

func (a *App) syncTick() tea.Cmd {
	cfg := a.st.SyncConfig()
	if cfg.Enabled() && cfg.Auto && !a.sync.running && !a.sync.next.IsZero() && time.Now().After(a.sync.next) {
		return a.runSync()
	}
	return nil
}

// syncStatus: eine Zeile für den Manager.
func (a *App) syncStatus() string {
	s := a.s
	cfg := a.st.SyncConfig()
	if !cfg.Enabled() {
		return s.Muted.Render(i18n.T("nicht eingerichtet · y = Assistent"))
	}
	where := i18n.T("Ordner")
	if cfg.Backend == domain.SyncWebDAV {
		where = i18n.T("NAS ") + syncx.PresetLabel(cfg.Preset)
	}
	switch {
	case a.sync.running:
		return s.AI.Render(i18n.T("⠿ gleicht ab …")) + s.Muted.Render(" · "+where)
	case cfg.LastError != "":
		return s.Danger.Render("× "+trunc(cfg.LastError, 60)) + s.Muted.Render(" · y")
	case !cfg.LastSync.IsZero():
		return s.OK.Render("✓ "+ago(cfg.LastSync)) + s.Muted.Render(" · "+where)
	}
	return s.Muted.Render(i18n.T("eingerichtet · ") + where)
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return i18n.T("gerade eben")
	case d < time.Hour:
		return fmt.Sprintf(i18n.T("vor %d min"), int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf(i18n.T("vor %d h"), int(d.Hours()))
	}
	return t.Local().Format("02.01. 15:04")
}

// ---------------------------------------------------------------- Assistent

const (
	modeFolder = "Ordner (Cloud)"
	modeNAS    = "NAS / WebDAV"
	modeOff    = "aus"
)

type syncWizard struct {
	f         *form
	rep       *syncx.Report
	repSig    string
	checking  bool
	gen       int
	pin       string
	askedPin  string
	saveAfter bool
	busy      string
	first     *syncx.Step // Ergebnis des ersten Abgleichs nach dem Speichern
	awaitSync bool
	cfg       domain.SyncConfig // Stand beim Öffnen
}

type syncCheckMsg struct {
	gen int
	sig string
	rep *syncx.Report
}

type syncSetupMsg struct {
	cfg domain.SyncConfig
	err error
}

func (a *App) openSync() tea.Cmd {
	cfg := a.st.SyncConfig()
	w := &syncWizard{cfg: cfg, pin: cfg.CertPin}
	f := &form{}
	mode := modeFolder
	switch {
	case cfg.Backend == domain.SyncWebDAV:
		mode = modeNAS
	case cfg.Backend == domain.SyncOff && cfg.Preset != "":
		mode = modeOff
	}
	f.choice("mode", i18n.T("Speicherort"), []string{modeFolder, modeNAS, modeOff}, mode,
		i18n.T("Ordner = OneDrive, Dropbox, iCloud, Syncthing · NAS = Synology, QNAP, Nextcloud direkt"))
	folder := cfg.Folder
	help := i18n.T("Ordner, den dein Cloud-Dienst abgleicht, z. B. ~/OneDrive/sshtermy")
	if sug := syncx.SuggestFolders(); len(sug) > 0 {
		if folder == "" {
			folder = sug[0]
		}
		help = i18n.T("Gefunden: ") + strings.Join(sug, " · ")
	}
	notFolder := func(f *form) bool { return f.sel("mode") != modeFolder }
	notNAS := func(f *form) bool { return f.sel("mode") != modeNAS }
	off := func(f *form) bool { return f.sel("mode") == modeOff }
	f.text("folder", i18n.T("Ordner"), folder, help).hidden = notFolder
	labels := make([]string, len(syncx.Presets))
	cur := "Synology"
	for i, p := range syncx.Presets {
		labels[i] = syncx.PresetLabel(p)
		if p == cfg.Preset {
			cur = labels[i]
		}
	}
	f.choice("preset", i18n.T("Gerät"), labels, cur, i18n.T("wählt Ports, Pfade und Hilfetexte passend aus")).hidden = notNAS
	f.text("addr", i18n.T("Adresse"), cfg.Address, i18n.T("Name oder IP, z. B. diskstation oder 192.168.1.20 · Port und https sucht sshTermy selbst")).hidden = notNAS
	f.text("user", i18n.T("Benutzer"), cfg.User, i18n.T("dein Konto auf dem NAS")).hidden = notNAS
	f.secret("pass", i18n.T("Passwort"), cfg.Password.Reveal(), i18n.T("wird nur lokal verschlüsselt gespeichert")).hidden = notNAS
	f.text("remote", i18n.T("Ordner am NAS"), cfg.Remote, "").hidden = notNAS
	f.secret("spw", i18n.T("Sync-Passwort"), "", "").hidden = off
	f.secret("spw2", i18n.T("Wiederholen"), "", i18n.T("Sync-Passwort noch einmal")).hidden = func(f *form) bool {
		return off(f) || w.rep == nil || !w.rep.OK() || w.rep.RemoteExists || (w.keepKey() && f.raw("spw") == "")
	}
	f.toggle("hostkeys", i18n.T("Host-Keys"), cfg.HostKeys || !cfg.Enabled(), i18n.T("bekannte Server-Fingerprints mitnehmen (weniger Rückfragen)")).hidden = off
	f.toggle("archive", i18n.T("Archiv"), cfg.Archive, i18n.T("Config-Sicherungen und Sitzungsprotokolle mitnehmen (verschlüsselt, eigener Unterordner)")).hidden = off
	days := cfg.ArchiveDays
	if days <= 0 {
		days = syncx.DefaultArchiveDays
	}
	dayLabels := make([]string, len(archiveDayOptions))
	for i, d := range archiveDayOptions {
		dayLabels[i] = archiveDaysLabel(d)
	}
	f.choice("archivedays", i18n.T("Aufbewahren"), dayLabels, archiveDaysLabel(days), i18n.T("so lange bleiben Einträge im Archiv; lokale Kopien bleiben")).hidden = func(f *form) bool {
		return off(f) || !f.flag("archive")
	}
	f.toggle("auto", i18n.T("Automatisch"), cfg.Auto || !cfg.Enabled(), i18n.T("beim Start, nach Änderungen und alle 5 Minuten")).hidden = off
	w.f = f
	a.wiz, a.form, a.scr = w, f, scrSync
	w.refreshHelp()
	return tea.Batch(f.setFocus(0), a.wizAutoCheck())
}

func (w *syncWizard) presetKey() string {
	l := w.f.sel("preset")
	for _, p := range syncx.Presets {
		if syncx.PresetLabel(p) == l {
			return p
		}
	}
	return "andere"
}

// sig: Fingerabdruck der Eingaben, die der Live-Test prüft.
func (w *syncWizard) sig() string {
	f := w.f
	switch f.sel("mode") {
	case modeFolder:
		return "f\x00" + f.str("folder")
	case modeNAS:
		return strings.Join([]string{"w", w.presetKey(), f.str("addr"), f.str("user"), f.raw("pass"), f.str("remote"), w.pin}, "\x00")
	}
	return "off"
}

func (w *syncWizard) complete() bool {
	f := w.f
	switch f.sel("mode") {
	case modeFolder:
		return f.str("folder") != ""
	case modeNAS:
		return f.str("addr") != "" && f.str("user") != "" && f.raw("pass") != ""
	}
	return false
}

// keepKey: gleicher Ort wie bisher, vorhandener Schlüssel kann bleiben.
func (w *syncWizard) keepKey() bool {
	c := w.cfg
	if !c.Enabled() {
		return false
	}
	f := w.f
	switch f.sel("mode") {
	case modeFolder:
		return c.Backend == domain.SyncFolder && syncx.ExpandHome(c.Folder) == syncx.ExpandHome(f.str("folder"))
	case modeNAS:
		return c.Backend == domain.SyncWebDAV && w.rep != nil && w.rep.URL == c.URL
	}
	return false
}

func (w *syncWizard) refreshHelp() {
	f := w.f
	p := w.presetKey()
	f.get("remote").help = i18n.T("leer = ") + syncx.DefaultRemote(p)
	switch p {
	case "synology":
		f.get("remote").help += i18n.T(" · muss in einem freigegebenen Ordner liegen")
	case "nextcloud":
		f.get("remote").help = i18n.T("relativ zu deinen Dateien, leer = sshtermy · bei 2FA: App-Passwort")
	}
	sp := f.get("spw")
	switch {
	case w.rep != nil && w.rep.RemoteExists && w.keepKey():
		sp.help = i18n.T("leer lassen = bisherigen Schlüssel behalten")
	case w.rep != nil && w.rep.RemoteExists:
		sp.help = i18n.T("das Sync-Passwort, das du auf dem anderen Rechner vergeben hast")
	case w.keepKey():
		sp.help = i18n.T("leer lassen = bisherigen Schlüssel behalten")
	default:
		sp.help = i18n.T("neu, mind. 8 Zeichen · verschlüsselt alles Ende-zu-Ende · gleich auf allen Rechnern")
	}
}

func (a *App) wizAutoCheck() tea.Cmd {
	w := a.wiz
	if w == nil || !w.complete() || w.sig() == w.repSig || w.checking {
		return nil
	}
	return a.wizCheck()
}

func (a *App) wizCheck() tea.Cmd {
	w := a.wiz
	f := w.f
	w.gen++
	w.checking = true
	gen, sig := w.gen, w.sig()
	mode := f.sel("mode")
	folder := f.str("folder")
	in := syncx.WebDAVInput{Preset: w.presetKey(), Address: f.str("addr"), User: f.str("user"),
		Password: f.raw("pass"), Remote: f.str("remote"), CertPin: w.pin}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		var r *syncx.Report
		if mode == modeFolder {
			r = syncx.CheckFolder(ctx, folder)
		} else {
			r = syncx.CheckWebDAV(ctx, in)
		}
		return syncCheckMsg{gen: gen, sig: sig, rep: r}
	}
}

func (a *App) wizCheckDone(m syncCheckMsg) tea.Cmd {
	w := a.wiz
	if w == nil || m.gen != w.gen {
		return nil
	}
	w.checking = false
	w.rep, w.repSig = m.rep, m.sig
	w.refreshHelp()
	if c := m.rep.Cert; c != nil && c.Pin != "" && c.Pin != w.askedPin {
		w.askedPin = c.Pin
		pin := c.Pin
		name := ""
		if c.Subject != "" {
			name = i18n.T("Ausgestellt für: ") + c.Subject + "\n"
		}
		a.modal = confirmModal(i18n.T("Eigenes Zertifikat vertrauen?"),
			i18n.T("Der Server zeigt ein Zertifikat, das keine bekannte Stelle bestätigt\n(")+c.Reason+i18n.T("). Bei einem NAS ist das normal.\n\n")+
				name+i18n.T("Fingerprint (SHA-256):\n")+c.Fingerprint+"\n\n"+
				i18n.T("Vergleiche den Fingerprint mit der NAS-Oberfläche. sshTermy merkt sich\ngenau dieses Zertifikat und warnt, wenn es sich ändert."),
			func() tea.Cmd {
				w.pin = pin
				return a.wizCheck()
			})
		w.saveAfter = false
		return nil
	}
	if w.saveAfter {
		w.saveAfter = false
		if m.rep.OK() {
			return a.wizSave()
		}
		w.f.err = i18n.T("Bitte erst die rot markierten Punkte beheben (Strg+T testet erneut).")
	}
	return nil
}

func (a *App) updateSync(msg tea.Msg) tea.Cmd {
	w := a.wiz
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			a.wiz, a.form, a.scr = nil, nil, scrManager
			return nil
		case "ctrl+t":
			w.f.err = ""
			return a.wizCheck()
		}
		if w.busy != "" {
			return nil
		}
		if w.first != nil && k.String() == "enter" {
			a.wiz, a.form, a.scr = nil, nil, scrManager
			return nil
		}
	}
	before := w.sig()
	mode := w.f.sel("mode")
	submit, cmd := w.f.update(msg)
	w.refreshHelp()
	if submit {
		w.f.err = ""
		return tea.Batch(cmd, a.wizSave())
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "tab", "shift+tab", "up", "down", "enter":
			return tea.Batch(cmd, a.wizAutoCheck()) // Feld verlassen: live prüfen
		case "left", "right", "space":
			if w.sig() != before || w.f.sel("mode") != mode {
				w.rep, w.repSig = nil, ""
				return tea.Batch(cmd, a.wizAutoCheck())
			}
		}
	}
	return cmd
}

func (a *App) wizSave() tea.Cmd {
	w := a.wiz
	f := w.f
	cfg := w.cfg
	if f.sel("mode") == modeOff {
		preset := cfg.Preset
		if preset == "" {
			preset = "andere"
		}
		cfg = domain.SyncConfig{Preset: preset} // Ort und Schlüssel vergessen
		if err := a.st.SetSyncConfig(cfg); err != nil {
			f.err = err.Error()
			return nil
		}
		a.wiz, a.form, a.scr = nil, nil, scrManager
		a.flash(i18n.T("Sync ausgeschaltet (die Sync-Datei bleibt am Speicherort liegen)"))
		return nil
	}
	if !w.complete() {
		f.err = i18n.T("Bitte die Felder oben ausfüllen.")
		return nil
	}
	if w.rep == nil || w.repSig != w.sig() || w.checking {
		w.saveAfter = true
		if w.checking {
			return nil
		}
		return a.wizCheck()
	}
	if !w.rep.OK() {
		f.err = i18n.T("Bitte erst die rot markierten Punkte beheben (Strg+T testet erneut).")
		return nil
	}
	spw, spw2 := f.raw("spw"), f.raw("spw2")
	rep := w.rep

	nc := cfg
	nc.Preset, nc.HostKeys, nc.Auto = w.presetKey(), f.flag("hostkeys"), f.flag("auto")
	nc.Archive = f.flag("archive")
	for _, d := range archiveDayOptions {
		if archiveDaysLabel(d) == f.sel("archivedays") {
			nc.ArchiveDays = d
		}
	}
	nc.LastError = ""
	if f.sel("mode") == modeFolder {
		nc.Backend, nc.Folder = domain.SyncFolder, f.str("folder")
	} else {
		nc.Backend, nc.Address, nc.User = domain.SyncWebDAV, f.str("addr"), f.str("user")
		nc.Password, nc.Remote, nc.URL, nc.CertPin = domain.Secret(f.raw("pass")), f.str("remote"), rep.URL, w.pin
	}

	// Schlüssel: behalten, vorhandene Datei öffnen oder neu anlegen
	if spw == "" && w.keepKey() {
		if k, err := syncx.ParseKey(cfg.Key.Reveal()); err == nil {
			if !rep.RemoteExists {
				return func() tea.Msg { return syncSetupMsg{cfg: nc} }
			}
			if _, err := syncx.Decode(k, rep.RemoteRaw); err == nil {
				return func() tea.Msg { return syncSetupMsg{cfg: nc} }
			}
		}
		f.err = i18n.T("Die Sync-Datei passt nicht mehr zum gespeicherten Schlüssel: bitte das Sync-Passwort eingeben.")
		return nil
	}
	if rep.RemoteExists {
		if spw == "" {
			f.err = i18n.T("Bitte das Sync-Passwort der vorhandenen Sync-Datei eingeben.")
			return nil
		}
		w.busy = i18n.T("Sync-Passwort wird geprüft (Argon2id) …")
		raw := rep.RemoteRaw
		return func() tea.Msg {
			k, err := syncx.UnlockKey(raw, []byte(spw))
			if err != nil {
				return syncSetupMsg{err: err}
			}
			nc.Key = domain.Secret(k.Marshal())
			return syncSetupMsg{cfg: nc}
		}
	}
	switch {
	case len(spw) < 8:
		f.err = i18n.T("Das Sync-Passwort braucht mindestens 8 Zeichen.")
		return nil
	case spw != spw2:
		f.err = i18n.T("Die Sync-Passwörter stimmen nicht überein.")
		return nil
	}
	w.busy = i18n.T("Sync-Schlüssel wird erzeugt (Argon2id) …")
	return func() tea.Msg {
		k, err := syncx.NewKey([]byte(spw))
		if err != nil {
			return syncSetupMsg{err: err}
		}
		nc.Key = domain.Secret(k.Marshal())
		return syncSetupMsg{cfg: nc}
	}
}

func (a *App) wizSetupDone(m syncSetupMsg) tea.Cmd {
	w := a.wiz
	if w == nil {
		return nil
	}
	w.busy = ""
	if m.err != nil {
		w.f.err = m.err.Error()
		if errors.Is(m.err, syncx.ErrWrongPass) {
			w.f.err += i18n.T(". Es ist das Passwort, das beim ersten Einrichten des Syncs vergeben wurde (nicht das NAS-Passwort).")
		}
		return nil
	}
	if err := a.st.SetSyncConfig(m.cfg); err != nil {
		w.f.err = err.Error()
		return nil
	}
	w.cfg = m.cfg
	w.awaitSync = true
	w.busy = i18n.T("Erster Abgleich läuft …")
	return a.runSync()
}

// wizFirstSync zeigt das Ergebnis des ersten Abgleichs im Assistenten.
func (a *App) wizFirstSync(m syncDoneMsg) {
	w := a.wiz
	if w == nil || !w.awaitSync {
		return
	}
	w.awaitSync, w.busy = false, ""
	if m.err != nil {
		w.first = &syncx.Step{Title: "Erster Abgleich", State: syncx.StepFail, Detail: m.err.Error(), Hint: i18n.T("Einstellungen sind gespeichert; Strg+T testet erneut, Esc schließt.")}
		return
	}
	d := fmt.Sprintf(i18n.T("%s und %s abgeglichen"), plural(m.stats.Connections, i18n.T("Verbindung"), i18n.T("Verbindungen")), plural(m.stats.Tunnels, i18n.Pick("Tunnel", "tunnel"), i18n.Pick("Tunnel", "tunnels")))
	if m.stats.Added > 0 {
		d += fmt.Sprintf(i18n.T(", %d von anderen Rechnern übernommen"), m.stats.Added)
	}
	w.first = &syncx.Step{Title: "Erster Abgleich", State: syncx.StepOK, Detail: d,
		Hint: i18n.T("Fertig. Auf jedem weiteren Rechner: y drücken, denselben Ort und dasselbe Sync-Passwort angeben.")}
}

func (a *App) viewSync() string {
	w := a.wiz
	if w == nil {
		return ""
	}
	s := a.s
	width := min(110, a.w-4)
	var b strings.Builder
	b.WriteString(s.Accent.Render(i18n.T("↕ Sync zwischen Rechnern")) + "\n")
	b.WriteString(s.Muted.Render(i18n.T("Verbindungen, Tunnel, Runbooks, KI und alle Einstellungen landen als Ende-zu-Ende verschlüsselte Datei am\n")+
		i18n.T("gewählten Ort. Der Speicherort sieht nie Klartext; DPAPI und Master-Passwort bleiben pro Rechner.")) + "\n")
	b.WriteString(w.f.view(s, width-4, &a.z))

	b.WriteString("\n" + s.PanelTitle.Foreground(s.T.Accent).Render(i18n.T("Live-Test")) + "\n")
	steps := []syncx.Step{}
	if w.rep != nil && w.repSig == w.sig() {
		steps = append(steps, w.rep.Steps...)
	}
	if w.first != nil {
		steps = append(steps, *w.first)
	}
	switch {
	case w.checking:
		b.WriteString(s.AI.Render(i18n.T("⠿ prüfe Verbindung …")) + "\n")
	case w.busy != "":
		b.WriteString(s.AI.Render("⠿ "+w.busy) + "\n")
	case len(steps) == 0 && w.f.sel("mode") != modeOff:
		b.WriteString(s.Muted.Render(i18n.T("startet automatisch, sobald die Angaben vollständig sind (oder Strg+T)")) + "\n")
	}
	hintW := width - 10
	for _, st := range steps {
		icon, style := "✓", s.OK
		switch st.State {
		case syncx.StepWarn:
			icon, style = "▲", s.Warn
		case syncx.StepFail:
			icon, style = "×", s.Danger
		}
		b.WriteString(" " + style.Render(icon) + " " + s.Text.Render(lipgloss.NewStyle().Width(20).Render(i18n.T(st.Title))) + s.Muted.Render(trunc(st.Detail, width-30)) + "\n")
		if st.Hint != "" && (st.State != syncx.StepOK || st.Title == "Erster Abgleich" || (st.Title == "Sync-Datei" && !w.keepKey())) {
			fg := s.T.Warn
			if st.State == syncx.StepOK {
				fg = s.T.Muted
			}
			b.WriteString(lipgloss.NewStyle().PaddingLeft(4).Width(hintW).Foreground(fg).Render(st.Hint) + "\n")
		}
	}
	return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, s.Panel.Width(width).Render(b.String()))
}

func (a *App) syncKeys() [][2]string {
	if a.wiz != nil && a.wiz.first != nil {
		return [][2]string{{"⏎/Esc", i18n.T("fertig")}, {"Strg+T", i18n.T("erneut testen")}}
	}
	return [][2]string{{"Tab", i18n.T("weiter")}, {"←/→", i18n.T("wählen")}, {"Strg+T", i18n.T("testen")}, {"Strg+S", i18n.T("speichern")}, {"Esc", i18n.T("schließen")}}
}
