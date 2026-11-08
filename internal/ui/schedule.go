package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/netmap"
	"github.com/db9979/sshtermy/internal/runbook"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/store"
)

// Zeitpläne (Manager: z, Palette „Zeitpläne“)
//
// Ein Zeitplan lässt einen Broadcast oder ein Runbook zu einer Uhrzeit auf
// einem Ordner oder einzelnen Verbindungen laufen, täglich oder an
// bestimmten Wochentagen, z. B. „3:00 Mo–Fr: show running-config auf allen
// Switches“. Er läuft, solange sshTermy offen ist (auch gesperrt); einen
// verpassten Termin holt sshTermy innerhalb von 12 Stunden nach.
//
// Unbeaufsichtigt heißt: keine Rückfragen. Unbekannte Host-Keys und
// fehlende Passwörter lassen den Host scheitern, gefährliche Befehle und
// Runbooks mit Variablen oder Skripten lassen sich gar nicht einplanen.

const schedStepSep = ";;"

var dayChoices = []struct {
	label string
	days  []int
}{
	{"täglich", nil},
	{"Mo–Fr", []int{1, 2, 3, 4, 5}},
	{"Sa + So", []int{6, 0}},
	{"Montag", []int{1}}, {"Dienstag", []int{2}}, {"Mittwoch", []int{3}}, {"Donnerstag", []int{4}},
	{"Freitag", []int{5}}, {"Samstag", []int{6}}, {"Sonntag", []int{0}},
}

func daysLabel(days []int) string {
	for _, d := range dayChoices {
		if fmt.Sprint(d.days) == fmt.Sprint(days) || (len(d.days) == 0 && len(days) == 0) {
			return d.label
		}
	}
	return "täglich"
}

func daysFromLabel(l string) []int {
	for _, d := range dayChoices {
		if i18n.T(d.label) == l || d.label == l {
			return append([]int(nil), d.days...)
		}
	}
	return nil
}

// ---------------------------------------------------------------- Liste

func (a *App) schedules() []domain.Schedule { return a.st.Settings().Schedules }

func (a *App) scheduleItems() []pickItem {
	var out []pickItem
	for _, sc := range a.schedules() {
		hint := i18n.T("noch nie gelaufen")
		if !sc.LastRun.IsZero() {
			hint = sc.LastRun.Format("02.01. 15:04") + fmt.Sprintf(" ✓%d", sc.LastOK)
			if sc.LastFail > 0 {
				hint += fmt.Sprintf(" ×%d", sc.LastFail)
			}
		}
		if a.schedRun[sc.ID] {
			hint = i18n.T("läuft …")
		}
		if sc.Off {
			hint = i18n.T("aus")
		}
		out = append(out, pickItem{value: sc.ID, label: sc.At + " " + i18n.T(daysLabel(sc.Days)) + " · " + sc.Name,
			detail: a.schedTargetText(sc), hint: hint})
	}
	return out
}

func (a *App) schedTargetText(sc domain.Schedule) string {
	if sc.Folder != "" {
		return "▸ " + sc.Folder
	}
	var names []string
	for _, id := range sc.Conns {
		if c, ok := a.st.Connection(id); ok {
			names = append(names, c.Alias)
		}
	}
	return strings.Join(names, ", ")
}

func (a *App) openSchedules() tea.Cmd {
	p := newPicker(i18n.T("Zeitpläne"), i18n.T("Name, Zeit oder Ziel"), a.scheduleItems(), "\x00", func(it pickItem) tea.Cmd {
		return a.showScheduleResult(it.value)
	})
	p.help = i18n.T("Läuft, solange sshTermy offen ist; Verpasstes holt es innerhalb von 12 h nach. Nur Hosts mit gespeichertem Passwort oder Key.")
	p.reload = a.scheduleItems
	p.keys = [][2]string{{"⏎", i18n.T("letztes Ergebnis")}, {"Strg+N", i18n.T("neu")}, {"Strg+E", i18n.T("bearbeiten")},
		{"Strg+R", i18n.T("jetzt ausführen")}, {"Strg+O", i18n.T("an/aus")}, {"Strg+D", i18n.T("löschen")}, {"Esc", i18n.T("zurück")}}
	p.onKey = func(key string, it *pickItem) (tea.Cmd, bool) {
		if key == "ctrl+n" {
			a.pick = nil
			return a.openScheduleForm(domain.Schedule{At: "03:00"}), true
		}
		if it == nil {
			return nil, false
		}
		sc, ok := a.scheduleByID(it.value)
		if !ok {
			return nil, false
		}
		switch key {
		case "ctrl+e":
			a.pick = nil
			return a.openScheduleForm(sc), true
		case "ctrl+r":
			cmd := a.startSchedule(sc)
			p.items = a.scheduleItems()
			return cmd, true
		case "ctrl+o":
			a.updateSchedule(sc.ID, func(x *domain.Schedule) { x.Off = !x.Off })
			p.items = a.scheduleItems()
			return nil, true
		case "ctrl+d", "delete":
			_ = a.st.UpdateSettings(func(s *domain.Settings) {
				for i, x := range s.Schedules {
					if x.ID == sc.ID {
						s.Schedules = append(s.Schedules[:i], s.Schedules[i+1:]...)
						break
					}
				}
			})
			p.items = a.scheduleItems()
			p.sel = clamp(p.sel, 0, max(0, len(p.items)-1))
			a.flash(i18n.T("Zeitplan gelöscht: ") + sc.Name)
			return nil, true
		}
		return nil, false
	}
	if len(p.items) == 0 {
		p.extra = func(string) *pickItem {
			return &pickItem{value: "\x00new", label: i18n.T("+ Neuer Zeitplan (Strg+N)")}
		}
		inner := p.onPick
		p.onPick = func(it pickItem) tea.Cmd {
			if it.value == "\x00new" {
				return a.openScheduleForm(domain.Schedule{At: "03:00"})
			}
			return inner(it)
		}
	}
	return a.openPicker(p)
}

func (a *App) scheduleByID(id string) (domain.Schedule, bool) {
	for _, sc := range a.schedules() {
		if sc.ID == id {
			return sc, true
		}
	}
	return domain.Schedule{}, false
}

func (a *App) updateSchedule(id string, fn func(*domain.Schedule)) {
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		for i := range s.Schedules {
			if s.Schedules[i].ID == id {
				fn(&s.Schedules[i])
			}
		}
	})
}

func (a *App) showScheduleResult(id string) tea.Cmd {
	sc, ok := a.scheduleByID(id)
	if !ok {
		return nil
	}
	if sc.LastRun.IsZero() {
		a.flash(i18n.T("Noch nie gelaufen · Strg+R führt ihn jetzt aus"))
		return a.openSchedules()
	}
	title := fmt.Sprintf(i18n.T("%s · %s · %d ok, %d Fehler"), sc.Name, sc.LastRun.Format("02.01.2006 15:04"), sc.LastOK, sc.LastFail)
	a.modal = textModal(title, strings.Split(sc.LastText, "\n"))
	return nil
}

// ---------------------------------------------------------------- Formular

func (a *App) openScheduleForm(sc domain.Schedule) tea.Cmd {
	title := i18n.T("Neuer Zeitplan")
	if sc.ID != "" {
		title = i18n.T("Zeitplan: ") + sc.Name
	}
	f := &form{title: title}
	f.text("name", "Name", sc.Name, i18n.T("z. B. „Switch-Configs sichern“"))
	kinds := []string{i18n.T("Befehle oder Runbook"), i18n.T("Config-Sicherung (Switches)")}
	kindCur := kinds[0]
	if sc.Kind == domain.ScheduleBackup {
		kindCur = kinds[1]
	}
	f.choice("kind", i18n.T("Art"), kinds, kindCur, i18n.T("Config-Sicherung: show running-config je Switch, verschlüsselt gespeichert, mit den Regeln für Switch-Configs geprüft"))
	isBackup := func(f *form) bool { return f.sel("kind") == kinds[1] }
	f.text("at", i18n.T("Uhrzeit"), sc.At, i18n.T("HH:MM, Ortszeit dieses Rechners"))
	var days []string
	for _, d := range dayChoices {
		days = append(days, i18n.T(d.label))
	}
	f.choice("days", i18n.T("Tage"), days, i18n.T(daysLabel(sc.Days)), "")
	folders := []string{i18n.T("— einzelne Verbindungen")}
	var walk func(n *domain.TreeNode)
	walk = func(n *domain.TreeNode) {
		for _, fo := range n.Folders {
			folders = append(folders, fo.Path)
			walk(fo)
		}
	}
	walk(a.st.Tree())
	cur := folders[0]
	if sc.Folder != "" {
		cur = sc.Folder
	}
	f.choice("folder", i18n.T("Ordner"), folders, cur, i18n.T("alle Verbindungen darin, auch in Unterordnern"))
	var aliases []string
	for _, id := range sc.Conns {
		if c, ok := a.st.Connection(id); ok {
			aliases = append(aliases, c.Alias)
		}
	}
	f.text("conns", i18n.T("Verbindungen"), strings.Join(aliases, ", "), i18n.T("Aliasse durch Komma getrennt (nur ohne Ordner)")).hidden = func(f *form) bool {
		return f.sel("folder") != folders[0]
	}
	rbs := []string{i18n.T("— eigene Befehle")}
	rbIDs := map[string]string{}
	rbCur := rbs[0]
	for _, rb := range a.st.Runbooks("") {
		label := rb.Name
		rbs = append(rbs, label)
		rbIDs[label] = rb.ID
		if rb.ID == sc.RunbookID {
			rbCur = label
		}
	}
	a.schedRBIDs = rbIDs
	f.choice("runbook", "Runbook", rbs, rbCur, i18n.T("Runbooks ohne Variablen und ohne Skripte")).hidden = isBackup
	f.text("steps", i18n.T("Befehle"), strings.Join(sc.Steps, " "+schedStepSep+" "), i18n.T("mehrere Schritte mit ;; trennen · {{host}} {{alias}} werden je Host eingesetzt · Gefährliches geht nie")).hidden = func(f *form) bool {
		return isBackup(f) || f.sel("runbook") != rbs[0]
	}
	f.get("conns").in.Placeholder = "sw-core, sw-etage1"
	f.get("steps").in.Placeholder = "show running-config ;; show vlan"
	a.formSched = sc
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.scr = f, scrScheduleForm
	return f.setFocus(0)
}

func (a *App) saveScheduleForm() error {
	f := a.form
	sc := a.formSched
	sc.Name = f.str("name")
	h, m, ok := domain.ParseClock(f.str("at"))
	if !ok {
		return errors.New(i18n.T("Uhrzeit als HH:MM, z. B. 03:00"))
	}
	sc.At = fmt.Sprintf("%02d:%02d", h, m)
	sc.Days = daysFromLabel(f.sel("days"))
	sc.Folder, sc.Conns = "", nil
	if fo := f.sel("folder"); !strings.HasPrefix(fo, "—") {
		sc.Folder = fo
	} else {
		for _, alias := range strings.Split(f.str("conns"), ",") {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			c, ok := a.connByAlias(alias)
			if !ok {
				return fmt.Errorf(i18n.T("Verbindung „%s“ gibt es nicht"), alias)
			}
			sc.Conns = append(sc.Conns, c.ID)
		}
	}
	sc.Kind = ""
	if f.sel("kind") == i18n.T("Config-Sicherung (Switches)") {
		sc.Kind = domain.ScheduleBackup
	}
	sc.RunbookID, sc.Steps = a.schedRBIDs[f.sel("runbook")], nil
	if sc.Kind == domain.ScheduleBackup {
		sc.RunbookID = ""
	} else if sc.RunbookID == "" {
		for _, st := range strings.Split(f.str("steps"), schedStepSep) {
			if st = strings.TrimSpace(st); st != "" {
				sc.Steps = append(sc.Steps, st)
			}
		}
	}
	if sc.Name == "" && sc.Kind == domain.ScheduleBackup {
		sc.Name = i18n.T("Switch-Configs sichern")
	}
	if sc.Name == "" {
		sc.Name = orDefault(firstOf(sc.Steps), i18n.T("Zeitplan"))
	}
	if len(a.schedConns(sc)) == 0 {
		if sc.Kind == domain.ScheduleBackup {
			return errors.New(i18n.T("Kein Switch im Ziel (Server werden bei der Config-Sicherung übersprungen)"))
		}
		return errors.New(i18n.T("Kein Ziel: Ordner wählen oder Verbindungen eintragen"))
	}
	if _, err := a.schedSteps(sc); err != nil {
		return err
	}
	if sc.ID == "" {
		sc.ID = fmt.Sprintf("sch-%d", time.Now().UnixNano())
		// erst ab jetzt: nicht sofort einen „verpassten“ Termin von heute nachholen
		sc.LastRun = time.Now()
	}
	return a.st.UpdateSettings(func(s *domain.Settings) {
		for i := range s.Schedules {
			if s.Schedules[i].ID == sc.ID {
				s.Schedules[i] = sc
				return
			}
		}
		s.Schedules = append(s.Schedules, sc)
	})
}

func firstOf(x []string) string {
	if len(x) == 0 {
		return ""
	}
	return trunc(x[0], 30)
}

func (a *App) connByAlias(alias string) (domain.Connection, bool) {
	for _, c := range a.st.Connections() {
		if strings.EqualFold(c.Alias, alias) {
			return c, true
		}
	}
	return domain.Connection{}, false
}

// schedConns: Ziel-Verbindungen (nur SSH; Telnet/Konsole/Lokal brauchen
// einen Tab).
func (a *App) schedConns(sc domain.Schedule) []domain.Connection {
	var list []domain.Connection
	if sc.Folder != "" {
		for _, c := range a.st.Connections() {
			if domain.InFolder(c.Group, sc.Folder) {
				list = append(list, c)
			}
		}
	} else {
		for _, id := range sc.Conns {
			if c, ok := a.st.Connection(id); ok {
				list = append(list, c)
			}
		}
	}
	out := list[:0]
	for _, c := range list {
		if c.Protocol.Raw() {
			continue
		}
		if sc.Kind == domain.ScheduleBackup && !maybeNetConn(c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// maybeNetConn: kein bekannter Server (für die Config-Sicherung).
func maybeNetConn(c domain.Connection) bool {
	if isNetConn(c) {
		return true
	}
	if c.Device == domain.DeviceServer {
		return false
	}
	return c.Profile == nil || c.Profile.Family == ""
}

// schedSteps: die Schritte, geprüft für einen Lauf ohne Aufsicht.
func (a *App) schedSteps(sc domain.Schedule) ([]string, error) {
	if sc.Kind == domain.ScheduleBackup {
		return []string{"show running-config"}, nil
	}
	steps := sc.Steps
	if sc.RunbookID != "" {
		var found bool
		for _, rb := range a.st.Runbooks("") {
			if rb.ID == sc.RunbookID {
				if len(rb.Files) > 0 {
					return nil, errors.New(i18n.T("Runbooks mit Skripten des Agenten lassen sich nicht einplanen"))
				}
				steps, found = rb.Steps, true
			}
		}
		if !found {
			return nil, errors.New(i18n.T("Das Runbook gibt es nicht mehr"))
		}
	}
	if len(steps) == 0 {
		return nil, errors.New(i18n.T("Keine Befehle"))
	}
	if v := runbook.Vars(steps); len(v) > 0 {
		return nil, fmt.Errorf(i18n.T("Variable {{%s}} kann ohne Aufsicht niemand ausfüllen"), v[0].Name)
	}
	if d := broadcast.Dangers(steps); len(d) > 0 {
		return nil, fmt.Errorf(i18n.T("Gefährlicher Befehl läuft nie nach Zeitplan: %s (%s)"), trunc(d[0].Command, 40), strings.Join(d[0].Reasons, ", "))
	}
	return steps, nil
}

// ---------------------------------------------------------------- Ausführen

type schedDoneMsg struct {
	id      string
	results []broadcast.Result
	err     error
}

// scheduleTick: fällige Zeitpläne starten (jede Sekunde aus onTick).
func (a *App) scheduleTick() tea.Cmd {
	if a.st == nil || a.scr == scrUnlock {
		return nil
	}
	now := time.Now()
	var cmds []tea.Cmd
	for _, sc := range a.schedules() {
		if sc.Due(now) && !a.schedRun[sc.ID] {
			cmds = append(cmds, a.startSchedule(sc))
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) startSchedule(sc domain.Schedule) tea.Cmd {
	if a.schedRun[sc.ID] {
		a.flash(i18n.T("Läuft schon: ") + sc.Name)
		return nil
	}
	a.updateSchedule(sc.ID, func(x *domain.Schedule) { x.LastRun = time.Now() })
	steps, err := a.schedSteps(sc)
	conns := a.schedConns(sc)
	if err == nil && len(conns) == 0 {
		err = errors.New(i18n.T("keine SSH-Verbindung im Ziel"))
	}
	if err != nil {
		return func() tea.Msg { return schedDoneMsg{id: sc.ID, err: err} }
	}
	if a.schedRun == nil {
		a.schedRun = map[string]bool{}
	}
	a.schedRun[sc.ID] = true
	job := broadcast.Job{Conns: conns, Steps: steps}
	var backup *schedBackup
	if sc.Kind == domain.ScheduleBackup {
		backup = &schedBackup{st: a.st, rules: a.configRules()}
		for _, c := range conns {
			cmds := append(netmap.PagingOff(connVendor(c)), "show running-config")
			job.PerHost = append(job.PerHost, cmds)
		}
	} else if runbook.HasBuiltins(steps) {
		for _, c := range conns {
			job.PerHost = append(job.PerHost, runbook.Fill(steps, nil, c))
		}
	}
	dial := schedDialer(a)
	a.flash(i18n.T("Zeitplan startet: ") + sc.Name)
	id := sc.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		res := broadcast.Run(ctx, job, dial, func(int, broadcast.Result) {})
		if backup != nil {
			backup.apply(conns, job.PerHost, res)
		}
		return schedDoneMsg{id: id, results: res}
	}
}

// schedDialer: in Tests austauschbar.
var schedDialer = func(a *App) broadcast.Dialer { return a.quietDialer() }

// quietDialer: wie der Broadcast, aber ohne Rückfragen. Unbekannte Host-Keys
// und fehlende Passwörter lassen den Host scheitern.
func (a *App) quietDialer() broadcast.Dialer {
	hk, st := a.hostKeys, a.st
	relayDial := a.relayDialer()
	return func(ctx context.Context, c domain.Connection) (broadcast.Executor, error) {
		opts := sshx.Options{
			HostKeys: hk,
			Resolve:  st.Connection,
			Relay:    relayDial,
			Prompt:   func(string, string, string) bool { return false },
			AskSecret: func(label string) (string, error) {
				return "", fmt.Errorf(i18n.T("braucht eine Eingabe (%s), Zeitpläne fragen nie"), label)
			},
		}
		cl, err := sshx.Dial(ctx, c, opts)
		if err != nil {
			return nil, err
		}
		return broadcast.SSH(cl, sshx.ShellTiming{}), nil
	}
}

func (a *App) scheduleMsg(msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(schedDoneMsg)
	if !ok {
		return nil, false
	}
	delete(a.schedRun, m.id)
	sc, ok := a.scheduleByID(m.id)
	if !ok {
		return nil, true
	}
	if m.err != nil {
		a.updateSchedule(m.id, func(x *domain.Schedule) {
			x.LastOK, x.LastFail, x.LastText = 0, 1, "× "+m.err.Error()
		})
		a.flashErr(i18n.T("Zeitplan „") + sc.Name + "“: " + m.err.Error())
		a.schedDesktop(sc.Name, m.err.Error())
		return nil, true
	}
	okN, failN, _ := broadcast.Summary(m.results)
	text := scheduleReport(m.results)
	a.updateSchedule(m.id, func(x *domain.Schedule) { x.LastOK, x.LastFail, x.LastText = okN, failN, text })
	msgText := fmt.Sprintf(i18n.T("Zeitplan „%s“ fertig: %d ok, %d Fehler"), sc.Name, okN, failN)
	if failN > 0 {
		a.flashErr(msgText)
	} else {
		a.flashOK(msgText)
	}
	a.schedDesktop(sc.Name, fmt.Sprintf(i18n.T("%d ok, %d Fehler"), okN, failN))
	if a.pick != nil && a.pick.title == i18n.T("Zeitpläne") {
		a.pick.items = a.scheduleItems()
	}
	return nil, true
}

func (a *App) schedDesktop(name, body string) {
	if (a.blurred || a.lock != nil) && a.desktopNotifyOn() && a.notifyOn() {
		if a.lock != nil {
			body = i18n.T("Zeitplan ist gelaufen")
		}
		a.sendDesktop("sshTermy · "+name, body)
	}
}

// scheduleReport: Ergebnis je Host, Ausgabe gekürzt (bleibt in den
// Einstellungen, also klein halten).
func scheduleReport(rs []broadcast.Result) string {
	var b strings.Builder
	per := max(400, 12000/max(1, len(rs)))
	for _, r := range rs {
		mark := "✓"
		if !r.OK() {
			mark = "×"
		}
		fmt.Fprintf(&b, "%s %s", mark, r.Alias)
		if r.Exit > 0 {
			fmt.Fprintf(&b, " (exit %d)", r.Exit)
		}
		if r.Err != "" {
			b.WriteString(": " + r.Err)
		}
		b.WriteString("\n")
		if out := strings.TrimSpace(r.Output); out != "" {
			if len(out) > per {
				out = "…" + out[len(out)-per:]
			}
			for _, l := range strings.Split(out, "\n") {
				b.WriteString("    " + strings.TrimRight(l, "\r") + "\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---------------------------------------------------------------- Config-Sicherung

// schedBackup: nach dem Lauf die running-config jedes Switches sichern
// (nur wenn sie sich geändert hat) und gegen die Regeln prüfen. Die Ausgabe
// im Ergebnis wird durch eine Zeile Zusammenfassung ersetzt.
type schedBackup struct {
	st    *store.Store
	rules []hostinfo.ConfigRule
}

func (b *schedBackup) apply(conns []domain.Connection, cmds [][]string, res []broadcast.Result) {
	for i := range res {
		r := &res[i]
		if r.State != broadcast.Done || r.Err != "" {
			continue
		}
		if !r.Net {
			r.Output, r.Err = "", i18n.T("kein Netzwerkgerät, übersprungen")
			continue
		}
		sec, _ := netmap.Sections(r.Output, cmds[i])
		cfg := strings.TrimSpace(sec["show running-config"])
		if err := checkConfig(cfg); err != nil {
			r.Output, r.Err = "", err.Error()
			continue
		}
		cfg += "\n"
		note, err := b.save(conns[i].ID, cfg)
		if err != nil {
			r.Output, r.Err = "", err.Error()
			continue
		}
		r.Output = note
		if vs := hostinfo.CheckConfig(cfg, b.rules); len(vs) > 0 {
			r.Err = fmt.Sprintf(i18n.T("Regeln verletzt (%d): "), len(vs)) + violationText(vs)
		}
	}
}

func (b *schedBackup) save(connID, cfg string) (string, error) {
	if list := b.st.Backups(connID); len(list) > 0 {
		if old, err := b.st.LoadBackup(connID, list[0].ID); err == nil {
			if hostinfo.SameConfig(old, cfg) {
				return i18n.T("unverändert seit ") + localTime(list[0].Taken), nil
			}
			_, add, rem := hostinfo.Diff(hostinfo.ConfigLines(old), hostinfo.ConfigLines(cfg), 0)
			if _, err := b.st.SaveBackup(connID, cfg); err != nil {
				return "", err
			}
			return fmt.Sprintf(i18n.T("gesichert, +%d −%d Zeilen (F6 d zeigt den Diff)"), add, rem), nil
		}
	}
	if _, err := b.st.SaveBackup(connID, cfg); err != nil {
		return "", err
	}
	return i18n.T("erste Sicherung gespeichert"), nil
}
