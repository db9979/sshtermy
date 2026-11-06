package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/broadcast"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/runbook"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Broadcast (Manager: m = an mehrere Hosts): ein Befehl oder ein Runbook läuft parallel auf
// mehreren Verbindungen, die Ergebnisse stehen nebeneinander.
//
// Ablauf: Hosts und Befehl wählen ─▶ Bestätigen (rot bei Gefahr: zweimal ⏎)
//         ─▶ Ergebnisse in Spalten (←/→ Host, ↑↓ blättern, z groß, r nochmal)

type bcPhase int

const (
	bcSetup bcPhase = iota
	bcVars          // Variablen ausfüllen (broadcast_rb.go)
	bcConfirm
	bcRun
)

const (
	bcFocusHosts = iota
	bcFocusSource
	bcFocusCmd
)

type bcState struct {
	phase  bcPhase
	focus  int
	conns  []domain.Connection // alle, nach Gruppe sortiert
	picked map[string]bool
	cur    int // Cursor in der Host-Liste
	src    int // 0 = eigener Befehl, sonst Runbook src-1
	books  []domain.Runbook
	cmd    aiInput
	armed  bool // Gefahr: erster ⏎ gedrückt
	err    string

	// Runbooks (broadcast_rb.go)
	editID, editName string            // Befehlsfeld kommt aus diesem Runbook
	vals             map[string]string // Werte der Variablen
	vars             []runbook.Var
	varIn            []textinput.Model
	varCur           int
	review           map[string]*hostReview // KI-Prüfung pro Verbindung
	reviewSeq        int
	useAdapted       bool // Vorschläge der KI ausführen

	// Lauf
	targets []domain.Connection
	steps   []string   // Vorlage (für die Kopfzeile)
	per     [][]string // tatsächliche Schritte je Host
	results []broadcast.Result
	cancel  context.CancelFunc
	seq     int
	running bool
	started time.Time
	sel     int // ausgewählte Spalte
	first   int // erste sichtbare Spalte
	scroll  int
	zoom    bool
}

type bcUpdateMsg struct {
	seq int
	i   int
	r   broadcast.Result
}
type bcDoneMsg struct{ seq int }

func (a *App) openBroadcast() tea.Cmd {
	var conns []domain.Connection
	for _, c := range a.st.Connections() {
		if !c.Protocol.Raw() { // Broadcast läuft über SSH-Exec
			conns = append(conns, c)
		}
	}
	sort.SliceStable(conns, func(i, j int) bool { return conns[i].Group < conns[j].Group })
	b := &bcState{conns: conns, picked: map[string]bool{}, books: a.st.Runbooks(""), cmd: newAIInput(i18n.T("Befehl, z. B. uptime · ⇧⏎ = weiterer Schritt"))}
	b.cmd.MaxHeight = 5
	// Vorauswahl: Ordner der markierten Verbindung (sonst nur sie selbst);
	// ist ein Ordner markiert, alles darin samt Unterordnern.
	if r, ok := a.mgrCur(); ok && r.isFolder() {
		first := true
		for i, c := range conns {
			if domain.InFolder(c.Group, r.node.Path) {
				b.picked[c.ID] = true
				if first {
					b.cur, first = i, false
				}
			}
		}
	} else if sel, ok := a.selectedConn(); ok {
		for i, c := range conns {
			if c.ID == sel.ID {
				b.cur = i
			}
			if c.ID == sel.ID || (sel.Group != "" && c.Group == sel.Group) {
				b.picked[c.ID] = true
			}
		}
	}
	if a.bc != nil && a.bc.seq > 0 { // letzte Eingabe behalten
		b.cmd.SetValue(a.bc.cmd.Value())
		b.seq = a.bc.seq
	}
	if a.bc != nil { // Variablenwerte und Bearbeitung behalten
		b.vals, b.editID, b.editName = a.bc.vals, a.bc.editID, a.bc.editName
	}
	a.bc = b
	a.scr = scrBroadcast
	if len(conns) == 0 {
		b.err = i18n.T("Noch keine Verbindungen. Mit n anlegen oder mit f im Netz suchen.")
	}
	b.focus = bcFocusCmd
	return b.cmd.Focus()
}

func (b *bcState) chosen() []domain.Connection {
	var out []domain.Connection
	for _, c := range b.conns {
		if b.picked[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// stepList: Runbook-Schritte oder die Zeilen des Befehlsfelds.
func (b *bcState) stepList() []string {
	if b.src > 0 && b.src <= len(b.books) {
		return b.books[b.src-1].Steps
	}
	var out []string
	for _, l := range strings.Split(b.cmd.Value(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (b *bcState) sourceNames() []string {
	out := []string{i18n.T("eigener Befehl")}
	for _, r := range b.books {
		out = append(out, "▶ "+r.Name)
	}
	return out
}

func (b *bcState) setFocus(f int) tea.Cmd {
	if f == bcFocusCmd && b.src > 0 {
		f = bcFocusHosts
	}
	b.focus = f
	if f == bcFocusCmd {
		return b.cmd.Focus()
	}
	b.cmd.Blur()
	return nil
}

func (a *App) updateBroadcast(msg tea.Msg) tea.Cmd {
	b := a.bc
	if b == nil {
		a.scr = scrManager
		return nil
	}
	if pm, ok := msg.(tea.PasteMsg); ok && b.phase == bcSetup && b.focus == bcFocusCmd {
		var cmd tea.Cmd
		b.cmd, cmd = b.cmd.Update(pm)
		return cmd
	}
	if pm, ok := msg.(tea.PasteMsg); ok && b.phase == bcVars && b.varCur < len(b.varIn) {
		var cmd tea.Cmd
		b.varIn[b.varCur], cmd = b.varIn[b.varCur].Update(pm)
		return cmd
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch b.phase {
	case bcVars:
		return a.bcVarsKey(k)
	case bcConfirm:
		return a.bcConfirmKey(k)
	case bcRun:
		return a.bcRunKey(k)
	}
	key := k.String()
	switch key {
	case "esc":
		a.scr = scrManager
		return nil
	case "tab":
		return b.setFocus((b.focus + 1) % 3)
	case "shift+tab":
		return b.setFocus((b.focus + 2) % 3)
	case "ctrl+s", "enter":
		if key == "enter" && b.focus == bcFocusHosts {
			if b.cur < len(b.conns) {
				id := b.conns[b.cur].ID
				b.picked[id] = !b.picked[id]
			}
			return nil
		}
		return a.bcToConfirm()
	case "ctrl+r":
		return a.bcSaveBook()
	case "f1":
		a.modal = a.helpModal()
		return nil
	}
	switch b.focus {
	case bcFocusHosts:
		switch key {
		case "up", "k":
			b.cur = max(0, b.cur-1)
		case "down", "j":
			b.cur = min(len(b.conns)-1, b.cur+1)
		case "space", "x":
			if b.cur < len(b.conns) {
				id := b.conns[b.cur].ID
				b.picked[id] = !b.picked[id]
			}
		case "g":
			if b.cur < len(b.conns) {
				g := b.conns[b.cur].Group
				on := !b.picked[b.conns[b.cur].ID]
				for _, c := range b.conns {
					if c.Group == g {
						b.picked[c.ID] = on
					}
				}
			}
		case "a":
			all := len(b.chosen()) < len(b.conns)
			for _, c := range b.conns {
				b.picked[c.ID] = all
			}
		}
		b.err = ""
	case bcFocusSource:
		n := len(b.books) + 1
		switch key {
		case "left", "up":
			b.src = (b.src + n - 1) % n
		case "right", "down", "space":
			b.src = (b.src + 1) % n
		case "e":
			return a.bcEditBook()
		case "ctrl+d", "delete":
			return a.bcDeleteBook()
		}
	case bcFocusCmd:
		var cmd tea.Cmd
		b.cmd, cmd = b.cmd.Update(k)
		b.err = ""
		return cmd
	}
	return nil
}

func (a *App) bcToConfirm() tea.Cmd {
	b := a.bc
	if len(b.chosen()) == 0 {
		b.err = i18n.T("Keine Hosts gewählt (Leertaste, g = ganzer Ordner, a = alle)")
		return nil
	}
	if len(b.stepList()) == 0 {
		b.err = i18n.T("Kein Befehl")
		return b.setFocus(bcFocusCmd)
	}
	b.err, b.armed, b.phase = "", false, bcConfirm
	b.cmd.Blur()
	if len(b.varsOf()) > 0 {
		return a.bcToVars()
	}
	return nil
}

func (a *App) bcConfirmKey(k tea.KeyPressMsg) tea.Cmd {
	b := a.bc
	switch k.String() {
	case "enter", "j", "y":
		hosts := b.chosen()
		per := b.perHost(hosts)
		if len(allDangers(per)) > 0 && !b.armed {
			b.armed = true // rot: zweiter Druck nötig
			return nil
		}
		return a.bcStart(hosts, b.stepList(), per)
	case "k":
		return a.bcReview()
	case "a":
		if b.review != nil {
			b.useAdapted, b.armed = !b.useAdapted, false
		}
	case "v":
		if len(b.varsOf()) > 0 {
			return a.bcToVars()
		}
	case "esc", "n", "e":
		b.phase = bcSetup
		return b.setFocus(b.focus)
	}
	return nil
}

func (a *App) bcStart(targets []domain.Connection, steps []string, per [][]string) tea.Cmd {
	b := a.bc
	if b.cancel != nil {
		b.cancel()
	}
	b.seq++
	seq := b.seq
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel, b.running, b.started = cancel, true, time.Now()
	b.targets, b.steps, b.per = targets, steps, per
	b.results = make([]broadcast.Result, len(targets))
	for i, c := range targets {
		b.results[i] = broadcast.Result{ConnID: c.ID, Alias: c.Alias, Exit: -1}
	}
	b.phase, b.sel, b.first, b.scroll, b.zoom = bcRun, 0, 0, 0, false
	if b.src > 0 && b.src <= len(b.books) {
		a.st.UseRunbook(b.books[b.src-1].ID)
	}
	ev := a.events
	dial := a.bcDialer()
	job := broadcast.Job{Conns: targets, Steps: steps, PerHost: per}
	go func() {
		broadcast.Run(ctx, job, dial, func(i int, r broadcast.Result) { ev <- bcUpdateMsg{seq, i, r} })
		ev <- bcDoneMsg{seq}
	}()
	return nil
}

// bcDialer verbindet wie ein Tab, aber Rückfragen (Host-Key, Passwort)
// kommen nacheinander, nie zwei Dialoge gleichzeitig.
func (a *App) bcDialer() broadcast.Dialer {
	ev, hk, st := a.events, a.hostKeys, a.st
	relayDial := a.relayDialer()
	var mu sync.Mutex
	answered := map[string]bool{}
	return func(ctx context.Context, c domain.Connection) (broadcast.Executor, error) {
		opts := sshx.Options{
			HostKeys: hk,
			Resolve:  st.Connection,
			Relay:    relayDial,
			Prompt: func(host, fp, typ string) bool {
				mu.Lock()
				defer mu.Unlock()
				if v, ok := answered[host+" "+fp]; ok {
					return v // gleicher Host über mehrere Verbindungen: nur einmal fragen
				}
				if ctx.Err() != nil {
					return false
				}
				r := make(chan bool, 1)
				ev <- hostKeyAskMsg{host, fp, typ, r}
				select {
				case v := <-r:
					answered[host+" "+fp] = v
					return v
				case <-ctx.Done():
					return false
				}
			},
			AskSecret: func(label string) (string, error) {
				mu.Lock()
				defer mu.Unlock()
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				r := make(chan string, 1)
				ev <- secretAskMsg{c.Alias + ": " + label, r}
				select {
				case v := <-r:
					if v == "\x00cancel" {
						return "", context.Canceled
					}
					return v, nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			},
		}
		cl, err := sshx.Dial(ctx, c, opts)
		if err != nil {
			return nil, err
		}
		return broadcast.SSH(cl, sshx.ShellTiming{}), nil
	}
}

// bcMsg: Fortschritt des Laufs (kommt über a.events, also danach weiter lauschen).
func (a *App) bcMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case bcUpdateMsg:
		if b := a.bc; b != nil && m.seq == b.seq && m.i < len(b.results) {
			b.results[m.i] = m.r
			if m.r.Learned && m.r.Finished() {
				a.rememberNetDeviceID(m.r.ConnID)
			}
		}
		return a.listen(), true
	case bcReviewMsg:
		a.bcReviewDone(m)
		return nil, true
	case bcDoneMsg:
		if b := a.bc; b != nil && m.seq == b.seq {
			b.running = false
			ok, failed, _ := broadcast.Summary(b.results)
			a.flash(fmt.Sprintf(i18n.T("Broadcast fertig: %d ok, %d Fehler"), ok, failed))
		}
		return a.listen(), true
	}
	return nil, false
}

// rememberNetDeviceID: wie rememberNetDevice, nur ohne Tab.
func (a *App) rememberNetDeviceID(id string) {
	c, ok := a.st.Connection(id)
	if !ok || c.Device != domain.DeviceAuto {
		return
	}
	c.Device = domain.DeviceNetwork
	_, _ = a.st.UpsertConnection(c)
}

func (a *App) bcRunKey(k tea.KeyPressMsg) tea.Cmd {
	b := a.bc
	n := len(b.results)
	switch k.String() {
	case "esc", "q":
		if b.zoom {
			b.zoom = false
			return nil
		}
		if b.running {
			b.cancel()
		}
		a.scr = scrManager
	case "left", "h", "shift+tab":
		b.sel = max(0, b.sel-1)
	case "right", "l", "tab":
		b.sel = min(n-1, b.sel+1)
	case "up", "k":
		b.scroll = max(0, b.scroll-1)
	case "down", "j":
		b.scroll++
	case "pgup":
		b.scroll = max(0, b.scroll-10)
	case "pgdown", "space":
		b.scroll += 10
	case "home":
		b.scroll = 0
	case "end":
		b.scroll = 1 << 20
	case "z", "enter":
		b.zoom = !b.zoom
	case "x":
		if b.running {
			b.cancel()
			a.flash(i18n.T("Broadcast wird abgebrochen …"))
		}
	case "r":
		if !b.running {
			b.phase, b.armed = bcConfirm, false
		}
	case "e":
		if !b.running {
			b.phase = bcSetup
			return b.setFocus(bcFocusCmd)
		}
	case "c":
		if b.sel < n {
			text := b.results[b.sel].Output
			go func() { _ = clipboard.WriteAll(text) }()
			a.flash(i18n.T("Ausgabe von ") + b.results[b.sel].Alias + i18n.T(" kopiert"))
			return tea.SetClipboard(text)
		}
	case "f1":
		a.modal = a.helpModal()
	}
	return nil
}

func (a *App) broadcastKeys() [][2]string {
	b := a.bc
	if b == nil {
		return nil
	}
	switch b.phase {
	case bcVars:
		return [][2]string{{"⏎", i18n.T("nächstes Feld")}, {"Strg+S", i18n.T("weiter")}, {"Esc", i18n.T("zurück")}}
	case bcConfirm:
		k := [][2]string{{"⏎", i18n.T("ausführen")}, {"k", i18n.T("KI prüft gegen Systemprofil")}}
		if b.review != nil {
			k = append(k, [2]string{"a", i18n.T("KI-Vorschläge an/aus")})
		}
		if len(b.varsOf()) > 0 {
			k = append(k, [2]string{"v", i18n.T("Variablen")})
		}
		return append(k, [2]string{"Esc", i18n.T("zurück")})
	case bcRun:
		k := [][2]string{{"←/→", "Host"}, {"↑/↓", i18n.T("blättern")}, {"z", i18n.T("groß")}, {"c", i18n.T("kopieren")}}
		if b.running {
			k = append(k, [2]string{"x", i18n.T("abbrechen")})
		} else {
			k = append(k, [2]string{"r", i18n.T("nochmal")}, [2]string{"e", i18n.T("ändern")})
		}
		return append(k, [2]string{"Esc", i18n.T("zum Manager")}, [2]string{a.keys.label(actHelp), i18n.T("Hilfe")})
	}
	if b.focus == bcFocusSource && b.src > 0 {
		return [][2]string{{"←/→", "Runbook"}, {"e", i18n.T("bearbeiten")}, {"Strg+D", i18n.T("löschen")},
			{"Strg+S", i18n.T("weiter")}, {"Esc", i18n.T("abbrechen")}, {a.keys.label(actHelp), i18n.T("Hilfe")}}
	}
	return [][2]string{{"Tab", i18n.T("Hosts/Quelle/Befehl")}, {"Space", i18n.T("Host wählen")}, {"g", i18n.T("Ordner")}, {"a", i18n.T("alle")},
		{"Strg+R", i18n.T("als Runbook")}, {"Strg+S", i18n.T("weiter")}, {"Esc", i18n.T("abbrechen")}, {a.keys.label(actHelp), i18n.T("Hilfe")}}
}

// ---------------------------------------------------------------- Ansicht

func (a *App) viewBroadcast() string {
	b := a.bc
	if b == nil {
		return ""
	}
	switch b.phase {
	case bcVars:
		return a.viewBcVars()
	case bcConfirm:
		return a.viewBcConfirm()
	case bcRun:
		return a.viewBcRun()
	}
	return a.viewBcSetup()
}

func (a *App) viewBcSetup() string {
	b, s := a.bc, a.s
	w := min(110, a.w-4)
	var o strings.Builder
	o.WriteString(s.Accent.Render(i18n.T("» Broadcast")) + s.Muted.Render(i18n.T("  ein Befehl oder Runbook parallel auf mehreren Hosts")) + "\n\n")

	head := func(focus int, title string) string {
		m := "  "
		st := s.Label
		if b.focus == focus {
			m = s.Accent.Render("▌ ")
			st = lipgloss.NewStyle().Foreground(s.T.Text).Bold(true).Width(20)
		}
		return m + a.z.click(st.Render(title), func(tea.Mouse, bool) tea.Cmd { return b.setFocus(focus) })
	}

	// Hosts
	picked := len(b.chosen())
	o.WriteString(head(bcFocusHosts, "Hosts") + s.Text.Render(fmt.Sprintf(i18n.T("%d von %d gewählt"), picked, len(b.conns))) +
		s.Muted.Render(i18n.T("   Leertaste wählen · g Ordner · a alle")) + "\n")
	steps := b.stepList()
	stepsH := len(steps)
	if b.src == 0 {
		stepsH = b.cmd.Height()
	}
	rows := max(3, a.h-2-10-stepsH-len(broadcast.Dangers(steps))*2)
	start := 0
	if b.cur >= rows {
		start = b.cur - rows + 1
	}
	group := "\x00"
	shown := 0
	for i := start; i < len(b.conns) && shown < rows; i++ {
		c := b.conns[i]
		if c.Group != group {
			group = c.Group
			g := group
			if g == "" {
				g = i18n.T("Allgemein")
			}
			if shown > 0 || i == start {
				o.WriteString("    " + s.Muted.Render(strings.ToUpper(g)) + "\n")
				shown++
			}
		}
		box := s.Muted.Render("[ ]")
		if b.picked[c.ID] {
			box = s.OK.Render("[x]")
		}
		dev := ""
		if c.Device == domain.DeviceNetwork {
			dev = s.Muted.Render(i18n.T(" · Netzwerkgerät"))
		}
		line := fmt.Sprintf("    %s %-20s %s%s", box, trunc(c.Alias, 20), s.Muted.Render(trunc(c.Username+"@"+c.Host, 30)), dev)
		if i == b.cur && b.focus == bcFocusHosts {
			line = s.Selected.Width(w - 4).Render(ansi.Strip(line))
		}
		i, id := i, c.ID
		line = a.z.click(line, func(tea.Mouse, bool) tea.Cmd {
			b.cur, b.picked[id] = i, !b.picked[id]
			return b.setFocus(bcFocusHosts)
		})
		o.WriteString(line + "\n")
		shown++
	}
	o.WriteString("\n")

	// Quelle
	var parts []string
	for i, n := range b.sourceNames() {
		p := s.Muted.Render(" " + trunc(n, 24) + " ")
		if i == b.src {
			p = s.Selected.Render(" " + trunc(n, 24) + " ")
		}
		i := i
		parts = append(parts, a.z.click(p, func(tea.Mouse, bool) tea.Cmd { b.src = i; return b.setFocus(bcFocusSource) }))
	}
	src := strings.Join(parts, "")
	if lipgloss.Width(src) > w-24 {
		src = ansi.Truncate(src, w-24, "…")
	}
	o.WriteString(head(bcFocusSource, i18n.T("Quelle")) + src + "\n")

	// Befehl bzw. Runbook-Schritte
	if b.src == 0 {
		b.cmd.SetWidth(w - 26)
		o.WriteString(promptBlock(head(bcFocusCmd, i18n.T("Befehl")), b.cmd.View()) + "\n")
		if b.editName != "" {
			o.WriteString("  " + strings.Repeat(" ", 20) + s.Muted.Render(i18n.T("bearbeitet „")+trunc(b.editName, w-50)+i18n.T("“ · Strg+R speichert")) + "\n")
		}
	} else {
		rb := b.books[b.src-1]
		o.WriteString("  " + s.Label.Render("Runbook") + s.Muted.Render(trunc(rb.Goal, w-26)) + "\n")
		for i, st := range rb.Steps {
			o.WriteString("  " + strings.Repeat(" ", 20) + s.Muted.Render(fmt.Sprintf("%d. ", i+1)) + s.Code.Render(trunc(st, w-28)) + "\n")
		}
		if len(rb.Steps) == 0 {
			o.WriteString("  " + strings.Repeat(" ", 20) + s.Warn.Render(i18n.T("Runbook hat keine gespeicherten Schritte")) + "\n")
		}
	}
	if vs := runbook.Vars(steps); len(vs) > 0 {
		var names []string
		for _, v := range vs {
			names = append(names, "{{"+v.Name+"}}")
		}
		o.WriteString("  " + strings.Repeat(" ", 20) + s.AI.Render(i18n.T("Variablen: ")+trunc(strings.Join(names, " "), w-40)) + s.Muted.Render(i18n.T(" · werden vor dem Start abgefragt")) + "\n")
	} else if b.src == 0 && len(steps) == 0 {
		o.WriteString("  " + strings.Repeat(" ", 20) + s.Muted.Render(i18n.T("Tipp: {{name}} wird vor dem Start abgefragt, {{host}} pro Host eingesetzt")) + "\n")
	}
	for _, d := range broadcast.Dangers(steps) {
		o.WriteString("  " + strings.Repeat(" ", 20) + s.Danger.Render("▲ "+strings.Join(d.Reasons, ", ")) + "\n")
	}
	if b.err != "" {
		o.WriteString("\n" + s.Danger.Render("× "+b.err) + "\n")
	}
	return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, s.Panel.Width(w).Render(o.String()))
}

func (a *App) viewBcConfirm() string {
	b, s := a.bc, a.s
	w := min(100, a.w-4)
	hosts := b.chosen()
	per := b.perHost(hosts)
	steps := b.stepList()
	if len(hosts) > 0 {
		steps = runbook.Fill(steps, b.vals, hosts[0]) // Vorschau mit den Werten des ersten Hosts
	}
	dangers := allDangers(per)
	var o strings.Builder
	o.WriteString(s.Accent.Render(fmt.Sprintf(i18n.T("» Auf %s ausführen?"), plural(len(hosts), "Host", "Hosts"))) + "\n\n")
	var names []string
	nets := 0
	for _, c := range hosts {
		names = append(names, c.Alias)
		if c.Device == domain.DeviceNetwork {
			nets++
		}
	}
	o.WriteString(lipgloss.NewStyle().Width(w-6).Render(s.Text.Render(strings.Join(names, ", "))) + "\n\n")
	for i, st := range steps {
		pre := s.Muted.Render(fmt.Sprintf("%d. ", i+1))
		if len(steps) == 1 {
			pre = s.Muted.Render("$ ")
		}
		o.WriteString(pre + s.Code.Render(trunc(st, w-10)) + "\n")
	}
	if len(hosts) > 1 && runbook.HasBuiltins(b.stepList()) {
		o.WriteString(s.Muted.Render(fmt.Sprintf(i18n.T("(Vorschau für %s; {{host}} usw. wird je Host eingesetzt)"), hosts[0].Alias)) + "\n")
	}
	if rl := a.reviewLines(hosts, w-6); len(rl) > 0 {
		o.WriteString("\n" + s.AI.Bold(true).Render(i18n.T("KI-Prüfung gegen das Systemprofil")) + "\n" + strings.Join(rl, "\n") + "\n")
	} else {
		o.WriteString("\n" + s.Muted.Render(i18n.T("k = KI prüft, ob die Schritte zum Systemprofil jedes Hosts passen")) + "\n")
	}
	elevated := false
	for _, p := range per {
		elevated = elevated || broadcast.Elevated(p)
	}
	if elevated {
		o.WriteString("\n" + s.Warn.Render(i18n.T("▲ läuft mit erhöhten Rechten (sudo)")) + "\n")
	}
	if nets > 0 {
		o.WriteString(s.Muted.Render(i18n.T("Netzwerkgeräte bekommen die Befehle in die Shell getippt (ohne Exit-Code).")) + "\n")
	}
	back := s.Muted.Render(" · ") + a.hints(s.Muted, [2]string{"Esc", i18n.T("zurück")})
	hint := a.hints(s.Muted, [2]string{"⏎", i18n.T("ausführen")}) + back
	if len(dangers) > 0 {
		var d strings.Builder
		d.WriteString(s.Danger.Render(i18n.T("GEFAHR")) + "\n")
		for _, x := range dangers {
			d.WriteString(s.Danger.Render(fmt.Sprintf(i18n.T("Schritt %d: %s"), x.Step, strings.Join(x.Reasons, ", "))) + s.Muted.Render("  "+trunc(x.Command, max(10, w-40))) + "\n")
		}
		d.WriteString(s.Text.Render(fmt.Sprintf(i18n.T("Das betrifft %s gleichzeitig."), plural(len(hosts), "Host", "Hosts"))))
		o.WriteString("\n" + s.DangerBox.Width(w-6).Render(d.String()) + "\n")
		hint = a.keyZone("⏎", s.Danger.Render(i18n.T("⏎ zweimal drücken zum Ausführen"))) + back
		if b.armed {
			hint = a.keyZone("⏎", s.Danger.Render(i18n.T("nochmal ⏎ = wirklich ausführen"))) + back
		}
	}
	o.WriteString("\n" + hint)
	box := s.Panel.Width(w)
	if len(dangers) > 0 {
		box = box.BorderForeground(s.T.Danger)
	}
	return lipgloss.Place(a.w, a.h-2, lipgloss.Center, lipgloss.Center, box.Render(o.String()))
}

func (a *App) bcStatus(r broadcast.Result, steps int) (icon string, text string, st lipgloss.Style) {
	s := a.s
	dur := ""
	if !r.End.IsZero() {
		dur = " · " + bcDur(r.End.Sub(r.Start))
	}
	switch r.State {
	case broadcast.Waiting:
		return "○", i18n.T("wartet"), s.Muted
	case broadcast.Connecting:
		return "◌", i18n.T("verbinde …"), s.Warn
	case broadcast.Running:
		if steps > 1 {
			return "◐", fmt.Sprintf(i18n.T("Schritt %d/%d …"), r.Step, steps), s.AI
		}
		return "◐", i18n.T("läuft …"), s.AI
	case broadcast.Canceled:
		return "■", i18n.T("abgebrochen") + dur, s.Muted
	case broadcast.Failed:
		return "×", r.Err + dur, s.Danger
	}
	if r.Net {
		return "✓", i18n.T("fertig (Netzwerkgerät)") + dur, s.OK
	}
	if r.Exit != 0 {
		t := fmt.Sprintf(i18n.T("Exit %d"), r.Exit)
		if r.Err != "" {
			t = r.Err
		}
		return "×", t + dur, s.Danger
	}
	return "✓", i18n.T("Exit 0") + dur, s.OK
}

func (a *App) viewBcRun() string {
	b, s := a.bc, a.s
	h := a.h - 2
	n := len(b.results)
	ok, failed, running := broadcast.Summary(b.results)
	cmdText := oneLine(strings.Join(b.steps, " ; "))
	if b.src > 0 && b.src <= len(b.books) {
		cmdText = "▶ " + b.books[b.src-1].Name
	}
	sum := s.OK.Render(fmt.Sprintf("%d ✓", ok)) + "  " + s.Danger.Render(fmt.Sprintf("%d ×", failed))
	if running > 0 {
		sum += "  " + s.AI.Render(fmt.Sprintf(i18n.T("%d …"), running))
	}
	top := " " + s.Accent.Render("» ") + s.Code.Render(trunc(cmdText, max(10, a.w-30))) + "   " + sum
	lines := []string{top, ""}

	cols := max(1, min(n, a.w/32))
	if b.zoom {
		cols = 1
	}
	if b.sel < b.first {
		b.first = b.sel
	}
	if b.sel >= b.first+cols {
		b.first = b.sel - cols + 1
	}
	b.first = clamp(b.first, 0, max(0, n-cols))
	colW := (a.w - (cols - 1)) / cols
	bodyH := max(1, h-len(lines)-3)

	// Ausgabe pro Spalte umbrechen, Blättern gilt für alle Spalten gemeinsam.
	wrapped := make([][]string, cols)
	maxLen := 0
	for c := 0; c < cols && b.first+c < n; c++ {
		r := b.results[b.first+c]
		out := strings.ReplaceAll(strings.ReplaceAll(r.Output, "\t", "    "), "\r", "")
		if r.State == broadcast.Failed && r.Err != "" {
			out = strings.TrimRight(out, "\n")
			if out != "" {
				out += "\n\n"
			}
			out += i18n.T("Fehler: ") + r.Err
		} else if out == "" && r.Finished() {
			out = i18n.T("(keine Ausgabe)")
		}
		wrapped[c] = strings.Split(ansi.Hardwrap(ansi.Strip(out), colW-1, true), "\n")
		maxLen = max(maxLen, len(wrapped[c]))
	}
	b.scroll = clamp(b.scroll, 0, max(0, maxLen-bodyH))

	var colViews []string
	for c := 0; c < cols && b.first+c < n; c++ {
		i := b.first + c
		r := b.results[i]
		nsteps := len(b.steps)
		if i < len(b.per) && len(b.per[i]) > 0 {
			nsteps = len(b.per[i])
		}
		icon, txt, st := a.bcStatus(r, nsteps)
		title := st.Render(icon) + " " + s.Text.Bold(true).Render(trunc(r.Alias, colW-4))
		if i == b.sel {
			title = s.Selected.Width(colW).Render(ansi.Strip(title))
		}
		var cb strings.Builder
		cb.WriteString(title + "\n" + st.Render(trunc(txt, colW-1)) + "\n")
		cb.WriteString(lipgloss.NewStyle().Foreground(s.T.Border).Render(strings.Repeat("─", colW)) + "\n")
		wl := wrapped[c]
		for j := b.scroll; j < len(wl) && j < b.scroll+bodyH; j++ {
			cb.WriteString(s.Text.Render(wl[j]) + "\n")
		}
		i2 := i
		view := lipgloss.NewStyle().Width(colW).Height(bodyH + 3).MaxHeight(bodyH + 3).Render(cb.String())
		view = a.z.mark(view, zoneAction{
			block: true,
			click: func(_ tea.Mouse, double bool) tea.Cmd {
				b.sel = i2
				if double {
					b.zoom = !b.zoom
				}
				return nil
			},
			wheel: func(up bool) tea.Cmd {
				if up {
					b.scroll = max(0, b.scroll-3)
				} else {
					b.scroll += 3
				}
				return nil
			},
		})
		colViews = append(colViews, view)
		if c < cols-1 && i < n-1 {
			colViews = append(colViews, lipgloss.NewStyle().Foreground(s.T.Border).Render(strings.Repeat("│\n", bodyH+2)+"│"))
		}
	}
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, colViews...))
	more := ""
	if n > cols {
		more = fmt.Sprintf(i18n.T("Host %d–%d von %d · ←/→ weitere"), b.first+1, min(n, b.first+cols), n)
	}
	if maxLen > bodyH {
		if more != "" {
			more += " · "
		}
		more += fmt.Sprintf(i18n.T("Zeile %d–%d von %d"), b.scroll+1, min(maxLen, b.scroll+bodyH), maxLen)
	}
	lines = append(lines, " "+s.Muted.Render(more))
	return strings.Join(lines, "\n")
}

func bcDur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return d.Round(100 * time.Millisecond).String()
}
