package ui

// Befehlssätze (Einstellungen › Befehlssätze)
//
// Liste aller Befehlssätze: eingebaute, eigene (Konfigordner/befehlssaetze)
// und Team-Sätze (…/befehlssaetze/team, erst nach Freigabe aktiv). Eigene
// Sätze legt man über das Formular an (n, meist „basiert auf“ einem
// eingebauten), bearbeitet sie dort oder in der Datei (o), testet sie am
// offenen Gerät (t), lässt die KI einen Entwurf schreiben (k) oder übernimmt
// die Korrekturen eines Geräts (l). Export/Import als .yaml (x/i).

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/cmdsetcheck"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sysprof"
	"github.com/db9979/sshtermy/internal/team"
)

type cmdsetState struct {
	sel  int
	err  string
	back screen
	edit *cmdset.Set // im Formular: Ausgangssatz (eigener Satz oder Grundlage)
	own  bool        // edit ist ein eigener Satz (überschreiben statt neu)
}

type cmdsetTestMsg struct {
	sid   int
	title string
	lines []string
}

type cmdsetDraftMsg struct {
	sid  int
	yaml string
	err  error
}

func (a *App) cmdsetDir() string { return filepath.Join(a.st.Dir, cmdset.DirName) }

// loadCmdSets liest eigene und Team-Sätze und nimmt sie in Betrieb.
func (a *App) loadCmdSets() {
	cmdset.Install(cmdset.ReadDir(a.cmdsetDir()), a.st.Settings().CmdSetApproved, nil)
	cmdset.RunSelfTests(cmdsetcheck.SelfTest)
}

// cmdsetSource: Herkunft für die Anzeige.
func cmdsetSource(s *cmdset.Set) string {
	switch s.Source {
	case cmdset.Own:
		return i18n.T("eigen")
	case cmdset.Synced:
		return i18n.T("vom Sync")
	case cmdset.Team:
		if s.From != "" {
			return i18n.T("Team: ") + s.From
		}
		return i18n.T("Team")
	}
	return i18n.T("eingebaut")
}

func (a *App) cmdsetHint() string {
	own, pend := 0, 0
	for _, st := range cmdset.Statuses() {
		if st.Set.Source != cmdset.Builtin && st.Set.Source != cmdset.Synced {
			own++
		}
		if st.Pending {
			pend++
		}
	}
	h := fmt.Sprintf(i18n.T("%d eingebaut"), len(cmdset.Builtins()))
	if own > 0 {
		h += fmt.Sprintf(i18n.T(" · %d eigene"), own)
	}
	if pend > 0 {
		h += fmt.Sprintf(i18n.T(" · %d warten"), pend)
	}
	return h
}

func (a *App) openCmdSets() tea.Cmd {
	a.loadCmdSets()
	c := &a.cmds
	c.err = ""
	if a.scr != scrCmdSets {
		c.back = a.scr
	}
	a.scr = scrCmdSets
	return nil
}

// cmdsetRows: eigene und Team-Sätze zuerst, dann die eingebauten.
func cmdsetRows() []cmdset.Status {
	all := cmdset.Statuses()
	sort.SliceStable(all, func(i, j int) bool {
		bi, bj := all[i].Set.Source == cmdset.Builtin, all[j].Set.Source == cmdset.Builtin
		if bi != bj {
			return !bi
		}
		if bi {
			return false
		}
		return all[i].Set.ID < all[j].Set.ID
	})
	return all
}

func (a *App) cmdsetSelected() (cmdset.Status, bool) {
	rows := cmdsetRows()
	if a.cmds.sel < 0 || a.cmds.sel >= len(rows) {
		return cmdset.Status{}, false
	}
	return rows[a.cmds.sel], true
}

func (a *App) cmdSetsKeys() [][2]string {
	k := [][2]string{{"⏎", i18n.T("Details")}, {"n", i18n.T("neu")}}
	if st, ok := a.cmdsetSelected(); ok && st.Set.Source == cmdset.Synced {
		if st.Pending {
			k = append(k, [2]string{"f", i18n.T("übernehmen")})
		}
		k = append(k, [2]string{"Entf", i18n.T("verwerfen")})
	} else if ok && st.Set.Source != cmdset.Builtin {
		k = append(k, [2]string{"e", i18n.T("bearbeiten")}, [2]string{"o", i18n.T("Datei")}, [2]string{"Entf", i18n.T("löschen")})
		if st.Pending {
			k = append(k, [2]string{"f", i18n.T("freigeben")})
		}
	}
	return append(k, [2]string{"t", i18n.T("am Gerät testen")}, [2]string{"k", i18n.T("KI-Entwurf")}, [2]string{"l", i18n.T("Korrekturen")},
		[2]string{"x", i18n.T("export")}, [2]string{"i", i18n.T("import")}, [2]string{"Esc", i18n.T("zurück")})
}

func (a *App) cmdsetBack() {
	a.scr = a.cmds.back
	if a.scr == scrCmdSets || a.scr == scrCmdSetForm {
		a.scr = scrManager
	}
}

func (a *App) updateCmdSets(msg tea.Msg) tea.Cmd {
	c := &a.cmds
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	rows := cmdsetRows()
	c.err = ""
	st, have := a.cmdsetSelected()
	switch k.String() {
	case "esc", "q":
		a.cmdsetBack()
	case "up", "k":
		if k.String() == "k" {
			return a.cmdsetDraft()
		}
		c.sel = max(0, c.sel-1)
	case "down", "j":
		c.sel = min(len(rows)-1, c.sel+1)
	case "pgup":
		c.sel = max(0, c.sel-10)
	case "pgdown":
		c.sel = min(len(rows)-1, c.sel+10)
	case "home":
		c.sel = 0
	case "end":
		c.sel = len(rows) - 1
	case "enter":
		if have {
			a.modal = textModal(st.Set.Name, a.cmdsetDetails(st))
		}
	case "n":
		if have {
			return a.openCmdSetForm(st.Set, false)
		}
	case "e":
		if have && st.Set.Source != cmdset.Builtin {
			return a.openCmdSetForm(st.Set, true)
		}
		if have {
			return a.openCmdSetForm(st.Set, false) // eingebaut: Kopie mit Grundlage
		}
	case "o":
		if have && st.Set.File != "" {
			return a.launchEditor(st.Set.File)
		}
	case "f":
		switch {
		case have && st.Pending && st.Set.Source == cmdset.Synced:
			a.askAdoptSyncCmdSet(st.Set)
		case have && st.Pending:
			a.askApproveCmdSet(st.Set)
		}
	case "delete", "backspace", "ctrl+d":
		switch {
		case have && st.Set.Source == cmdset.Synced && st.Set.File != "":
			a.rejectSyncCmdSet(st.Set)
		case have && st.Set.File != "":
			a.askDeleteCmdSet(st.Set)
		}
	case "t":
		if have {
			return a.cmdsetTest(st.Set)
		}
	case "l":
		return a.cmdsetFromLearned()
	case "x":
		if have {
			a.exportCmdSet(st.Set)
		}
	case "i":
		a.modal = inputModal(i18n.T("Befehlssatz importieren"), i18n.T("Pfad zur .yaml-Datei:"), filepath.Join(exportDir(), ""), func(p string) tea.Cmd {
			if err := a.importCmdSet(expandHome(strings.TrimSpace(p))); err != nil {
				a.cmds.err = err.Error()
			}
			return nil
		})
	case "r":
		a.cmdsetsEdited() // auch von Hand geänderte Dateien abgleichen
		a.flash(i18n.T("Befehlssätze neu geladen"))
	}
	return nil
}

func (a *App) viewCmdSets() string {
	s := a.s
	c := &a.cmds
	w := min(120, a.w-2)
	var b strings.Builder
	b.WriteString(s.Accent.Render(i18n.T(" ▤ Befehlssätze")) + s.Muted.Render(i18n.T(" · was sshTermy je Hersteller tippt, liest und der KI sagt")) + "\n\n")
	b.WriteString(s.Muted.Render(" "+trunc(i18n.T("Eigene Sätze liegen in ")+a.cmdsetDir()+".", w-1)) + "\n")
	b.WriteString(s.Muted.Render(wrap(" "+strings.TrimPrefix(i18n.T(". Sie können eingebaute Gefahrenregeln nur ergänzen, tippen nur Lesebefehle und erkennen ein Gerät erst, wenn kein eingebauter Satz passt."), ". "), w)) + "\n\n")
	if c.err != "" {
		b.WriteString(" " + s.Danger.Render(wrap(c.err, w-2)) + "\n\n")
	}
	rows := cmdsetRows()
	errs := cmdset.Errors()
	head := lipgloss.Height(b.String())
	avail := max(3, a.h-2-head-3-min(len(errs), 4))
	start := 0
	if c.sel >= avail {
		start = c.sel - avail + 1
	}
	for i := start; i < len(rows) && i < start+avail; i++ {
		st := rows[i]
		mark := "  "
		name := fmt.Sprintf("%-34s", trunc(st.Set.DisplayName(), 34))
		id := fmt.Sprintf("%-16s", trunc(st.Set.ID, 16))
		src := fmt.Sprintf("%-16s", trunc(cmdsetSource(st.Set), 16))
		state := s.OK.Render(i18n.T("aktiv"))
		switch {
		case st.Pending:
			state = s.Warn.Render(st.Problem)
		case !st.Active:
			state = s.Danger.Render(trunc(firstLine(st.Problem), max(10, w-74)))
		case st.Problem != "":
			state = s.Warn.Render(trunc(firstLine(st.Problem), max(10, w-74)))
		}
		var line string
		if i == c.sel {
			mark = s.Accent.Render("▸ ")
			line = s.Selected.Render(name) + " " + s.Muted.Render(id) + " " + s.Muted.Render(src) + " "
		} else {
			line = s.Text.Render(name) + " " + s.Muted.Render(id) + " " + s.Muted.Render(src) + " "
		}
		i := i
		b.WriteString(a.z.click(" "+mark+line+state, func(_ tea.Mouse, double bool) tea.Cmd {
			c.sel = i
			if double {
				a.modal = textModal(rows[i].Set.Name, a.cmdsetDetails(rows[i]))
			}
			return nil
		}) + "\n")
	}
	for i, e := range errs {
		if i >= 4 {
			break
		}
		b.WriteString(" " + s.Danger.Render(trunc("× "+filepath.Base(e.File)+": "+firstLine(e.Err), w-2)) + "\n")
	}
	return b.String()
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// cmdsetDetails: alles Wichtige eines Satzes zum Lesen.
func (a *App) cmdsetDetails(st cmdset.Status) []string {
	s := st.Set
	eff := cmdset.Resolve(s.ID, "", "")
	if eff == nil {
		eff = s
	}
	var out []string
	add := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			out = append(out, fmt.Sprintf("%-14s %s", k, v))
		}
	}
	add(i18n.T("Kennung"), s.ID)
	add(i18n.T("Herkunft"), cmdsetSource(s))
	add(i18n.T("Familie"), eff.EffFamily())
	add(i18n.T("Grundlage"), s.Base)
	if s.File != "" {
		add(i18n.T("Datei"), s.File)
	}
	if st.Problem != "" {
		out = append(out, "", a.s.Warn.Render("! "+st.Problem))
	}
	out = append(out, "", a.s.Accent.Render(i18n.T("Erkennung")))
	add(i18n.T("SSH-Kennung"), strings.Join(s.Detect.Banner, "  ||  "))
	add(i18n.T("Bildschirm"), strings.Join(s.Detect.Screen, "  ||  "))
	add(i18n.T("Version"), strings.Join(s.Detect.Version, "  ||  "))
	add(i18n.T("Datei da"), strings.Join(s.Detect.Probe, "  ||  "))
	out = append(out, "", a.s.Accent.Render(i18n.T("Befehle (nur lesend)")))
	cm := eff.Commands
	add(i18n.T("Blättern aus"), cm.PagingOff)
	add(i18n.T("Version"), cm.Version)
	add(i18n.T("System"), cm.System)
	add(i18n.T("Speicher"), cm.Memory)
	add("Ports", cm.Ports)
	add("VLANs", cm.VLANs)
	add(i18n.T("Konfiguration"), cm.Config)
	for _, e := range cm.Extra {
		add(hostinfo.ExtraLabel(e), e.Cmd)
	}
	add(i18n.T("Logs"), strings.Join(eff.Logs.Commands, "  ||  "))
	out = append(out, "", a.s.Accent.Render(i18n.T("Syslog einrichten (ändert das Gerät)")))
	for _, x := range eff.Syslog.Steps {
		out = append(out, "  "+x)
	}
	if eff.Syslog.Note != "" {
		out = append(out, "  "+eff.Syslog.Note)
	}
	add(i18n.T("Speichern"), eff.Syslog.Save)
	if len(s.Variants) > 0 {
		out = append(out, "", a.s.Accent.Render(i18n.T("Varianten")))
		for _, v := range s.Variants {
			cond := strings.TrimSpace(strings.Join([]string{v.Model, v.Firmware}, " "))
			out = append(out, "  "+v.Name+a.s.Muted.Render("  ("+cond+")"))
		}
	}
	if len(eff.Danger) > 0 {
		out = append(out, "", a.s.Accent.Render(i18n.T("Zusätzliche Gefahrenregeln (gelten für alle Geräte)")))
		for _, d := range eff.Danger {
			out = append(out, "  "+d.Reason+a.s.Muted.Render("  "+d.Rx))
		}
	}
	if eff.Hint != "" {
		out = append(out, "", a.s.Accent.Render(i18n.T("Hinweis für die KI")), wrap(eff.Hint, min(100, a.w-8)))
	}
	if len(s.Samples) > 0 {
		out = append(out, "", a.s.Muted.Render(fmt.Sprintf(i18n.T("%d Beispiele für den Selbsttest"), len(s.Samples))))
	}
	return out
}

// ---------------------------------------------------------------- Formular

const cmdsetSep = "||"

func joinList(l []string) string { return strings.Join(l, " "+cmdsetSep+" ") }

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, cmdsetSep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// openCmdSetForm: own = vorhandenen eigenen Satz bearbeiten, sonst neuer
// Satz, der auf from aufbaut (eingebaut) bzw. eine Kopie davon ist.
func (a *App) openCmdSetForm(from *cmdset.Set, own bool) tea.Cmd {
	c := &a.cmds
	c.edit, c.own = from, own
	f := &form{title: i18n.T("Befehlssatz")}
	var s cmdset.Set
	if own {
		s = *from
	} else {
		s = cmdset.Set{ID: from.ID + "-eigen", Name: from.Name + i18n.T(" (eigen)"), Base: from.ID}
		if from.Source != cmdset.Builtin {
			s = *from
			s.ID, s.Name, s.File = from.ID+"-kopie", from.Name+i18n.T(" (Kopie)"), ""
		}
	}
	accent := func(t Theme) color.Color { return t.Accent }
	f.section("s_main", i18n.T("Befehlssatz"), accent)
	f.text("id", i18n.T("Kennung"), s.ID, i18n.T("a–z, 0–9 und -, eindeutig; eingebaute Kennungen sind vergeben"))
	f.text("name", i18n.T("Name"), s.Name, i18n.T("so erscheint er in der Auswahl „Befehlssatz“ der Verbindung"))
	bases := []string{"—"}
	for _, x := range cmdset.All() {
		if x.ID != s.ID {
			bases = append(bases, x.ID)
		}
	}
	cur := s.Base
	if cur == "" {
		cur = "—"
	}
	f.choice("base", i18n.T("Basiert auf"), bases, cur, i18n.T("leer gelassene Felder kommen von dort (Befehle, Leser, Hinweis); Erkennung nicht"))
	fam := s.Family
	if fam == "" && s.Base != "" {
		if b := cmdset.Resolve(s.Base, "", ""); b != nil {
			fam = b.EffFamily() // erbt die Familie der Grundlage
		}
	}
	if fam == "" {
		fam = cmdset.FamilyNetwork
	}
	f.choice("family", i18n.T("Familie"), []string{cmdset.FamilyNetwork, cmdset.FamilyLinux, cmdset.FamilyBSD, cmdset.FamilyWindows, cmdset.FamilyMacOS}, fam,
		i18n.T("network = Switch/Router/Firewall (Befehle sichtbar am Prompt), sonst Server"))
	f.text("hint", i18n.T("Hinweis für die KI"), s.Hint, i18n.T("welche CLI, typische Befehle, wie man speichert; kommt zum Hinweis der Grundlage dazu"))
	f.section("s_detect", i18n.T("Erkennung (reguläre Ausdrücke, mehrere mit ||)"), func(t Theme) color.Color { return t.OK })
	f.text("d_screen", i18n.T("Bildschirm"), joinList(s.Detect.Screen), i18n.T("Login-Banner oder Prompt, z. B. (?i)acme switch"))
	f.text("d_version", i18n.T("Versionsausgabe"), joinList(s.Detect.Version), i18n.T("Text in der Ausgabe des Versionsbefehls"))
	f.text("d_banner", i18n.T("SSH-Kennung"), joinList(s.Detect.Banner), i18n.T("z. B. (?i)acmessh"))
	f.section("s_cmds", i18n.T("Befehle, die sshTermy tippt (nur lesend)"), func(t Theme) color.Color { return t.Warn })
	f.text("c_paging", i18n.T("Blättern aus"), s.Commands.PagingOff, i18n.T("nur bekannte: terminal length 0, no page, screen-length disable, terminal pager 0, …"))
	f.text("c_version", i18n.T("Version"), s.Commands.Version, "show version")
	f.text("c_system", i18n.T("System (CPU)"), s.Commands.System, "")
	f.text("c_memory", i18n.T("Speicher"), s.Commands.Memory, "")
	f.text("c_ports", "Ports", s.Commands.Ports, "")
	f.text("c_vlans", "VLANs", s.Commands.VLANs, "")
	f.text("c_config", i18n.T("Konfiguration"), s.Commands.Config, i18n.T("für die Sicherung (F6 b)"))
	f.text("l_cmds", i18n.T("Logs"), joinList(s.Logs.Commands), i18n.T("für den Log-Blick (Alt+E), mehrere mit ||"))
	f.section("s_read", i18n.T("Lesen (reguläre Ausdrücke, eine Klammer = Wert)"), accent)
	f.text("p_firmware", "Firmware", s.Parse.Firmware, "")
	f.text("p_model", i18n.T("Modell"), s.Parse.Model, "")
	f.text("p_name", "Name", s.Parse.Name, "")
	f.text("p_cpu", "CPU", s.Parse.CPU, i18n.T("Klammer = belegt in %, (?P<idle>…) = frei"))
	f.text("p_mem", "RAM", s.Parse.Mem, i18n.T("Klammer = belegt in %, oder (?P<used>…)/(?P<free>…) mit (?P<total>…)"))
	f.section("s_syslog", i18n.T("Syslog einrichten (ändert das Gerät, wird jedes Mal gezeigt)"), func(t Theme) color.Color { return t.Danger })
	f.text("s_steps", i18n.T("Schritte"), joinList(s.Syslog.Steps), i18n.T("mit || getrennt, Platzhalter {{ip}} und {{port}}; jede Zeile wird gegen die Gefahrenregeln geprüft"))
	f.text("s_save", i18n.T("Speichern mit"), s.Syslog.Save, "write memory")
	f.section("s_danger", i18n.T("Zusätzliche Gefahrenregel (nur ergänzen)"), func(t Theme) color.Color { return t.Danger })
	dRx, dReason := "", ""
	if len(s.Danger) > 0 {
		dRx, dReason = s.Danger[0].Rx, s.Danger[0].Reason
	}
	f.text("g_rx", i18n.T("Muster"), dRx, i18n.T("z. B. ^\\s*factory-reset\\b · weitere Regeln, Varianten und Beispiele in der Datei (o)"))
	f.text("g_reason", i18n.T("Grund"), dReason, i18n.T("wird bei einem Treffer angezeigt"))
	a.form, a.scr = f, scrCmdSetForm
	return f.setFocus(1)
}

// saveCmdSetForm schreibt den Satz als Datei und lädt neu.
func (a *App) saveCmdSetForm() error {
	f := a.form
	c := &a.cmds
	var s cmdset.Set
	if c.own && c.edit != nil {
		s = *c.edit // Felder, die das Formular nicht zeigt, bleiben
		if len(s.Danger) > 1 {
			s.Danger = s.Danger[1:]
		} else {
			s.Danger = nil
		}
	} else if c.edit != nil && c.edit.Source != cmdset.Builtin {
		s = *c.edit
		s.Danger = nil
	}
	s.Source = cmdset.Own
	s.ID, s.Name, s.Hint = f.str("id"), f.str("name"), f.str("hint")
	s.Base = f.sel("base")
	if s.Base == "—" {
		s.Base = ""
	}
	s.Family = f.sel("family")
	inherit := cmdset.FamilyNetwork
	if s.Base != "" {
		if b := cmdset.Resolve(s.Base, "", ""); b != nil {
			inherit = b.EffFamily()
		}
	}
	if s.Family == cmdset.FamilyNetwork && inherit == cmdset.FamilyNetwork {
		s.Family = "" // Standard
	}
	s.Detect.Screen, s.Detect.Version, s.Detect.Banner = splitList(f.str("d_screen")), splitList(f.str("d_version")), splitList(f.str("d_banner"))
	s.Commands.PagingOff, s.Commands.Version, s.Commands.System = f.str("c_paging"), f.str("c_version"), f.str("c_system")
	s.Commands.Memory, s.Commands.Ports, s.Commands.VLANs, s.Commands.Config = f.str("c_memory"), f.str("c_ports"), f.str("c_vlans"), f.str("c_config")
	s.Logs.Commands = splitList(f.str("l_cmds"))
	s.Parse.Firmware, s.Parse.Model, s.Parse.Name, s.Parse.CPU, s.Parse.Mem = f.str("p_firmware"), f.str("p_model"), f.str("p_name"), f.str("p_cpu"), f.str("p_mem")
	s.Syslog.Steps, s.Syslog.Save = splitList(f.str("s_steps")), f.str("s_save")
	if rx := f.str("g_rx"); rx != "" {
		s.Danger = append([]cmdset.Danger{{Rx: rx, Reason: f.str("g_reason")}}, s.Danger...)
	}
	if cmdset.IsBuiltin(s.ID) {
		return fmt.Errorf(i18n.T("Kennung „%s“ gehört zu einem eingebauten Satz; bitte eine eigene wählen (z. B. %s-eigen)"), s.ID, s.ID)
	}
	if !(c.own && c.edit != nil && c.edit.ID == s.ID) {
		if _, taken := cmdset.Lookup(s.ID); taken {
			return fmt.Errorf(i18n.T("Kennung „%s“ ist schon vergeben"), s.ID)
		}
	}
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := cmdset.Marshal(&s)
	if err != nil {
		return err
	}
	path := filepath.Join(a.cmdsetDir(), s.ID+cmdset.Ext)
	if err := writeCmdSet(path, raw); err != nil {
		return err
	}
	if c.own && c.edit != nil && c.edit.File != "" && c.edit.File != path {
		_ = os.Remove(c.edit.File) // umbenannt
		a.cmdsetTombstone(c.edit.ID)
	}
	a.cmdsetsEdited()
	a.selectCmdSet(s.ID)
	a.flashOK(fmt.Sprintf(i18n.T("Befehlssatz „%s“ gespeichert"), s.Name))
	return nil
}

func writeCmdSet(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func (a *App) selectCmdSet(id string) {
	for i, st := range cmdsetRows() {
		if st.Set.ID == id {
			a.cmds.sel = i
			return
		}
	}
}

// ---------------------------------------------------------------- Löschen, Freigabe

func (a *App) askDeleteCmdSet(s *cmdset.Set) {
	path := s.File
	a.modal = confirmModal(i18n.T("Befehlssatz löschen"), fmt.Sprintf(i18n.T("„%s“ (%s) löschen?\n\n%s\n\nVerbindungen, die ihn nutzen, fallen auf die Erkennung zurück."), s.Name, s.ID, path), func() tea.Cmd {
		if err := os.Remove(path); err != nil {
			a.cmds.err = err.Error()
			return nil
		}
		if s.Source == cmdset.Own {
			a.cmdsetTombstone(s.ID) // auch auf den anderen Rechnern weg
		}
		a.loadCmdSets()
		a.cmds.sel = max(0, a.cmds.sel-1)
		return nil
	})
}

// diffLines: Unterschiede zweier Dateien, eingefärbt.
func (a *App) diffLines(old, cur []byte) []string {
	diff, add, del := hostinfo.Diff(strings.Split(string(old), "\n"), strings.Split(string(cur), "\n"), 2)
	lines := []string{fmt.Sprintf("+%d −%d", add, del)}
	for _, l := range diff {
		switch l.Op {
		case hostinfo.DiffAdd:
			lines = append(lines, a.s.OK.Render("+ "+l.Text))
		case hostinfo.DiffDel:
			lines = append(lines, a.s.Danger.Render("- "+l.Text))
		case hostinfo.DiffHunk:
			lines = append(lines, a.s.AI.Render(l.Text))
		default:
			lines = append(lines, a.s.Muted.Render("  "+l.Text))
		}
	}
	return lines
}

func approvedCopy(dir, id string) string {
	return filepath.Join(dir, cmdset.TeamDir, ".freigegeben", id+cmdset.Ext)
}

// askApproveCmdSet zeigt Inhalt bzw. Änderungen eines Team-Satzes und gibt
// ihn nach Bestätigung frei (Prüfsumme in den verschlüsselten Einstellungen).
func (a *App) askApproveCmdSet(s *cmdset.Set) {
	if err := s.Validate(); err != nil {
		a.cmds.err = err.Error()
		return
	}
	var lines []string
	old, _ := os.ReadFile(approvedCopy(a.cmdsetDir(), s.ID))
	if len(old) > 0 {
		lines = append(lines, i18n.T("Änderungen seit der letzten Freigabe:"), "")
		lines = append(lines, a.diffLines(old, s.Raw)...)
	} else {
		lines = append(lines, i18n.T("Neuer Befehlssatz aus dem Team:"), "")
		lines = append(lines, strings.Split(strings.TrimRight(string(s.Raw), "\n"), "\n")...)
	}
	body := strings.Join(lines, "\n") + "\n\n" +
		i18n.T("Freigeben? Danach gelten Erkennung, Befehle, KI-Hinweis und zusätzliche Gefahrenregeln dieses Satzes. Lesebefehle prüft sshTermy trotzdem, Syslog-Schritte zeigt es jedes Mal.")
	id, sum, raw := s.ID, s.Sum, s.Raw
	a.modal = confirmModal(fmt.Sprintf(i18n.T("Team-Befehlssatz „%s“ freigeben"), s.Name), body, func() tea.Cmd {
		err := a.st.UpdateSettings(func(x *domain.Settings) {
			m := map[string]string{}
			for k, v := range x.CmdSetApproved {
				m[k] = v
			}
			m[id] = sum
			x.CmdSetApproved = m
		})
		if err == nil {
			_ = writeCmdSet(approvedCopy(a.cmdsetDir(), id), raw)
		} else {
			a.cmds.err = err.Error()
		}
		a.loadCmdSets()
		return nil
	})
}

// ---------------------------------------------------------------- Export / Import

func (a *App) exportCmdSet(s *cmdset.Set) {
	raw := s.Raw
	if len(raw) == 0 {
		var err error
		if raw, err = cmdset.Marshal(s); err != nil {
			a.cmds.err = err.Error()
			return
		}
	}
	path := filepath.Join(exportDir(), "sshtermy-befehlssatz-"+unsafeName.ReplaceAllString(s.ID, "_")+cmdset.Ext)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		a.cmds.err = err.Error()
		return
	}
	a.flashOK(i18n.T("Exportiert: ") + path)
}

// importCmdSet übernimmt eine Datei als eigenen Satz (nach Prüfung).
func (a *App) importCmdSet(path string) error {
	if path == "" {
		return errors.New(i18n.T("Datei fehlt"))
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > cmdset.MaxFileSize {
		return fmt.Errorf(i18n.T("Datei zu groß (höchstens %d KB)"), cmdset.MaxFileSize>>10)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s, err := cmdset.ParseOwn(raw)
	if err != nil {
		return err
	}
	if _, taken := cmdset.Lookup(s.ID); taken {
		return fmt.Errorf(i18n.T("Kennung „%s“ ist schon vergeben"), s.ID)
	}
	if err := writeCmdSet(filepath.Join(a.cmdsetDir(), s.ID+cmdset.Ext), raw); err != nil {
		return err
	}
	a.cmdsetsEdited()
	a.selectCmdSet(s.ID)
	a.flashOK(fmt.Sprintf(i18n.T("Befehlssatz „%s“ importiert"), s.Name))
	return nil
}

// ---------------------------------------------------------------- Team

// storeTeamCmdSets legt Sätze aus einem Team-Paket als Team-Dateien ab.
// Aktiv werden sie erst nach Freigabe in der Liste (f).
func (a *App) storeTeamCmdSets(from string, files []cmdsetFile) int {
	n := 0
	for _, f := range files {
		if len(f.YAML) > cmdset.MaxFileSize {
			continue
		}
		s, err := cmdset.ParseFile([]byte(f.YAML))
		if err != nil || cmdset.IsBuiltin(s.ID) || s.ID != f.ID {
			continue
		}
		path := filepath.Join(a.cmdsetDir(), cmdset.TeamDir, s.ID+cmdset.Ext)
		head := ""
		if from = strings.TrimSpace(strings.ReplaceAll(from, "\n", " ")); from != "" {
			head = "# von: " + from + "\n"
		}
		raw := []byte(head + strings.TrimPrefix(f.YAML, firstLineIf(f.YAML, "# von:")))
		if old, err := os.ReadFile(path); err == nil && string(old) == string(raw) {
			continue
		}
		if writeCmdSet(path, raw) == nil {
			n++
		}
	}
	if n > 0 {
		a.loadCmdSets()
		a.pushToast(toastWarn, fmt.Sprintf(i18n.T("%d Befehlssätze vom Team warten auf Freigabe (Einstellungen › Befehlssätze, f)"), n))
	}
	return n
}

type cmdsetFile = team.CmdSet

func firstLineIf(s, prefix string) string {
	l, _, _ := strings.Cut(s, "\n")
	if strings.HasPrefix(strings.TrimSpace(l), prefix) {
		return l + "\n"
	}
	return ""
}

// ---------------------------------------------------------------- am Gerät testen

// cmdsetTest führt die Lesebefehle eines Satzes im aktuellen Tab aus und
// zeigt, was dabei herauskommt.
func (a *App) cmdsetTest(set *cmdset.Set) tea.Cmd {
	s := a.current()
	if s == nil || s.state != stConnected {
		a.cmds.err = i18n.T("Zum Testen erst eine Verbindung zu dem Gerät öffnen (der aktuelle Tab wird benutzt).")
		return nil
	}
	eff := cmdset.Resolve(set.ID, s.prof.Model, s.prof.Version)
	if eff == nil {
		a.cmds.err = i18n.T("Der Satz ist nicht aktiv (Freigabe oder Fehler?).")
		return nil
	}
	sid := s.id
	title := fmt.Sprintf(i18n.T("Test „%s“ an %s"), set.Name, s.conn.Alias)
	if eff.EffFamily() != cmdset.FamilyNetwork {
		if s.client == nil || s.netDevice() {
			a.cmds.err = i18n.T("Server-Sätze testet sshTermy über eine eigene SSH-Sitzung; der aktuelle Tab ist keine.")
			return nil
		}
		cl := s.client
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			var lines []string
			out, _ := cl.RunScript(ctx, sysprof.Probe())
			p := sysprof.FromProbe(out)
			lines = append(lines, fmt.Sprintf(i18n.T("Erkannt: %s (%s)"), orDefault(p.Dialect, "?"), orDefault(p.OS, "?")))
			for _, e := range eff.Commands.Extra {
				eo, err := cl.RunScript(ctx, e.Cmd+" 2>/dev/null\n")
				v := hostinfo.ExtraValue(e, eo)
				if err != nil {
					v = err.Error()
				}
				lines = append(lines, fmt.Sprintf("%-12s %s   %s", hostinfo.ExtraLabel(e), orDefault(v, "—"), e.Cmd))
			}
			return cmdsetTestMsg{sid: sid, title: title, lines: lines}
		}
	}
	if s.shell == nil || s.term == nil {
		return nil
	}
	prompt := strings.TrimSpace(s.term.CursorLine())
	if s.term.AltScreen() || !hostinfo.IsPrompt(prompt) || s.host.busy {
		a.cmds.err = i18n.T("Abfrage nur an einem leeren Prompt (z. B. switch#). Bitte erst Eingabe beenden oder ⏎ drücken.")
		return nil
	}
	if configPrompt(prompt) {
		a.cmds.err = i18n.T("Der Switch ist im Konfigurationsmodus. Erst mit end bzw. exit zurück an den normalen Prompt.")
		return nil
	}
	s.host.busy, s.host.step = true, i18n.T("Befehlssatz testen …")
	a.scr = scrTerminal
	s.cap.hold() // bis alles getippt ist keine Tasten an diese Shell (shelllock.go)
	return func() tea.Msg {
		defer s.cap.release()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		var lines []string
		run := func(cmd string) (string, bool) {
			if cmd == "" {
				return "", false
			}
			out, err := shellRun(ctx, s, prompt, cmd, 30*time.Second)
			txt := strings.Join(out, "\n")
			state := "✓"
			switch {
			case err != nil:
				state = "× " + err.Error()
			case sysprof.Rejection(txt) != "":
				state = "× " + sysprof.Rejection(txt)
			}
			lines = append(lines, fmt.Sprintf("%-40s %s", trunc(cmd, 40), state))
			return txt, err == nil
		}
		c := eff.Commands
		ver, _ := run(orDefault(c.Version, "show version"))
		if d := hostinfo.DetectVendor(ver); d != hostinfo.VendorUnknown && string(d) != set.ID {
			lines = append(lines, fmt.Sprintf(i18n.T("! Die Versionsausgabe erkennt der Satz „%s“ (Vorrang vor eigenen Sätzen)."), d))
		}
		run(c.PagingOff)
		sys, _ := run(c.System)
		mem, _ := run(c.Memory)
		ports, _ := run(c.Ports)
		vlans, _ := run(c.VLANs)
		sw := hostinfo.ParseWith(eff, ver, sys+"\n"+mem, ports, vlans)
		if v := cmdset.Resolve(set.ID, sw.Model, sw.Firmware); v != nil {
			eff = v
			sw = hostinfo.ParseWith(v, ver, sys+"\n"+mem, ports, vlans)
		}
		for _, e := range eff.Commands.Extra {
			if out, ok := run(e.Cmd); ok {
				sw.Extras = append(sw.Extras, hostinfo.Extra{Label: hostinfo.ExtraLabel(e), Value: hostinfo.ExtraValue(e, out)})
			}
		}
		for _, lc := range eff.Logs.Commands {
			run(lc)
		}
		res := []string{
			fmt.Sprintf("%-12s %s", i18n.T("Modell"), orDefault(sw.Model, "—")),
			fmt.Sprintf("%-12s %s", "Firmware", orDefault(sw.Firmware, "—")),
			fmt.Sprintf("%-12s %s", "Name", orDefault(sw.Name, "—")),
			fmt.Sprintf("%-12s %s", i18n.T("Laufzeit"), orDefault(sw.Uptime, "—")),
			fmt.Sprintf("%-12s %s", "CPU", pctText(sw.CPUPct)),
			fmt.Sprintf("%-12s %s", "RAM", pctText(sw.MemPct)),
			fmt.Sprintf("%-12s %d / %d", "Ports", sw.PortsUp, sw.PortsUp+sw.PortsDown),
			fmt.Sprintf("%-12s %d", "VLANs", len(sw.VLANs)),
		}
		if vn := cmdset.VariantName(set.ID, sw.Model, sw.Firmware); vn != "" {
			res = append(res, fmt.Sprintf("%-12s %s", i18n.T("Variante"), vn))
		}
		for _, e := range sw.Extras {
			res = append(res, fmt.Sprintf("%-12s %s", e.Label, orDefault(e.Value, "—")))
		}
		res = append(res, "", i18n.T("Befehle:"))
		return cmdsetTestMsg{sid: sid, title: title, lines: append(res, lines...)}
	}
}

func pctText(p int) string {
	if p < 0 {
		return "—"
	}
	return fmt.Sprintf("%d %%", p)
}

// ---------------------------------------------------------------- KI-Entwurf

// cmdsetDraft: die KI schreibt aus dem aktuellen Bildschirm einen Entwurf.
func (a *App) cmdsetDraft() tea.Cmd {
	s := a.current()
	if s == nil || s.term == nil {
		a.cmds.err = i18n.T("Für einen KI-Entwurf erst am Gerät anmelden und z. B. show version eingeben; die KI liest den aktuellen Tab.")
		return nil
	}
	ag, err := a.agentClientFor(s.conn)
	if err != nil {
		a.cmds.err = "KI: " + err.Error()
		return nil
	}
	if !a.st.LLM().SendScreen {
		a.cmds.err = i18n.T("Der KI-Entwurf braucht den Terminal-Inhalt; in den KI-Einstellungen ist „Bildschirm mitschicken“ aus.")
		return nil
	}
	c := a.agentContext(s, false)
	screen := s.term.PlainLines(200)
	sid := s.id
	a.flash(i18n.T("KI schreibt einen Entwurf …"))
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		y, err := ag.DraftCmdSet(ctx, screen, c)
		return cmdsetDraftMsg{sid: sid, yaml: y, err: err}
	}
}

func (a *App) onCmdSetDraft(m cmdsetDraftMsg) {
	if m.err != nil {
		a.cmds.err = i18n.T("KI-Entwurf: ") + m.err.Error()
		return
	}
	s, err := cmdset.ParseOwn([]byte(m.yaml))
	lines := strings.Split(strings.TrimRight(m.yaml, "\n"), "\n")
	if err != nil {
		lines = append([]string{a.s.Danger.Render(i18n.T("Der Entwurf besteht die Prüfung nicht:")), a.s.Danger.Render(err.Error()), ""}, lines...)
		path := filepath.Join(exportDir(), "sshtermy-befehlssatz-entwurf"+cmdset.Ext)
		if os.WriteFile(path, []byte(m.yaml), 0o600) == nil {
			lines = append(lines, "", i18n.T("Gespeichert zum Nacharbeiten: ")+path+i18n.T(" (danach mit i importieren)"))
		}
		a.modal = textModal(i18n.T("KI-Entwurf"), lines)
		return
	}
	if _, taken := cmdset.Lookup(s.ID); taken {
		s.ID += "-ki"
		raw, _ := cmdset.Marshal(s)
		m.yaml = string(raw)
	}
	body := strings.Join(lines, "\n") + "\n\n" + i18n.T("Als eigenen Befehlssatz speichern? Danach am Gerät testen (t) und bei Bedarf bearbeiten (e).")
	id, raw := s.ID, []byte(m.yaml)
	a.modal = confirmModal(i18n.T("KI-Entwurf"), body, func() tea.Cmd {
		if err := writeCmdSet(filepath.Join(a.cmdsetDir(), id+cmdset.Ext), raw); err != nil {
			a.cmds.err = err.Error()
			return nil
		}
		a.cmdsetsEdited()
		a.selectCmdSet(id)
		return nil
	})
}

// ---------------------------------------------------------------- Korrekturen übernehmen

// cmdsetFromLearned: Was der Admin an KI-Vorschlägen auf diesem Gerät
// korrigiert hat, kommt in den Hinweis eines eigenen Satzes. Ist der Satz
// eingebaut, entsteht ein eigener, der darauf aufbaut, und die Verbindung
// nutzt ihn.
func (a *App) cmdsetFromLearned() tea.Cmd {
	s := a.current()
	if s == nil || len(s.prof.Learned) == 0 {
		a.cmds.err = i18n.T("Im aktuellen Tab gibt es keine gemerkten Korrekturen (entstehen, wenn du einen KI-Vorschlag vor dem Ausführen änderst).")
		return nil
	}
	base := s.prof.Dialect
	if base == "" {
		a.cmds.err = i18n.T("Für dieses Gerät ist noch kein Befehlssatz erkannt.")
		return nil
	}
	var add []string
	for _, c := range s.prof.Learned {
		add = append(add, fmt.Sprintf("statt „%s“: „%s“", c.AI, c.Fixed))
	}
	note := "Auf diesen Geräten so: " + strings.Join(add, "; ") + "."
	cur, _ := cmdset.Lookup(base)
	var target cmdset.Set
	if cur != nil && cur.Source == cmdset.Own {
		target = *cur
		if !strings.Contains(target.Hint, note) {
			target.Hint = strings.TrimSpace(target.Hint + " " + note)
		}
	} else {
		id := base + "-eigen"
		if _, taken := cmdset.Lookup(id); taken {
			if o, _ := cmdset.Lookup(id); o.Source == cmdset.Own {
				target = *o
				target.Hint = strings.TrimSpace(target.Hint + " " + note)
			} else {
				a.cmds.err = fmt.Sprintf(i18n.T("Kennung „%s“ ist schon vergeben"), id)
				return nil
			}
		} else {
			name := base
			if cur != nil {
				name = cur.Name
			}
			target = cmdset.Set{ID: id, Name: trunc(name+i18n.T(" (eigen)"), 60), Base: base, Hint: note}
		}
	}
	target.Source = cmdset.Own
	if len([]rune(target.Hint)) > 2000 {
		a.cmds.err = i18n.T("Der KI-Hinweis wäre zu lang; bitte den Satz zuerst bearbeiten (e).")
		return nil
	}
	if err := target.Validate(); err != nil {
		a.cmds.err = err.Error()
		return nil
	}
	raw, err := cmdset.Marshal(&target)
	if err != nil {
		a.cmds.err = err.Error()
		return nil
	}
	body := fmt.Sprintf(i18n.T("Befehlssatz „%s“ (%s) bekommt diesen Hinweis für die KI:\n\n%s\n\nDie Verbindung %s nutzt danach diesen Satz."), target.Name, target.ID, note, s.conn.Alias)
	sid, id := s.id, target.ID
	a.modal = confirmModal(i18n.T("Korrekturen übernehmen"), body, func() tea.Cmd {
		if err := writeCmdSet(filepath.Join(a.cmdsetDir(), id+cmdset.Ext), raw); err != nil {
			a.cmds.err = err.Error()
			return nil
		}
		a.cmdsetsEdited()
		if x := a.byID(sid); x != nil {
			x.prof.Dialect, x.prof.Manual = id, true
			a.saveProfile(x)
		}
		a.selectCmdSet(id)
		a.flashOK(fmt.Sprintf(i18n.T("Korrekturen in „%s“ übernommen"), id))
		return nil
	})
	return nil
}

// cmdsetMsg verarbeitet Ergebnisse (Test, KI-Entwurf).
func (a *App) cmdsetMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case cmdsetTestMsg:
		if s := a.byID(m.sid); s != nil {
			s.host.busy, s.host.step = false, ""
		}
		a.modal = textModal(m.title, m.lines)
		return nil, true
	case cmdsetDraftMsg:
		a.onCmdSetDraft(m)
		return nil, true
	}
	return nil, false
}
