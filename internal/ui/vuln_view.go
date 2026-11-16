package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vuln"
)

func (a *App) vulnKeys() [][2]string {
	v := &a.vscan
	if v.busy {
		return [][2]string{{"x", i18n.T("abbrechen")}, {"Esc", i18n.T("zurück")}}
	}
	var k [][2]string
	if len(a.vulnVisible()) > 0 {
		k = append(k, [2]string{"↑↓", i18n.T("wählen")}, [2]string{"⏎", i18n.T("alle Lücken")})
		if v.hidden {
			k = append(k, [2]string{"x", i18n.T("zurückholen")})
		} else {
			k = append(k, [2]string{"k", i18n.T("KI erklärt")}, [2]string{"u", i18n.T("Update einfügen")}, [2]string{"x", i18n.T("betrifft uns nicht")})
		}
	}
	fix := i18n.T("nur mit Update")
	if v.onlyFix {
		fix = i18n.T("alle")
	}
	hid := i18n.T("Ausgeblendete")
	if v.hidden {
		hid = i18n.T("Liste")
	}
	return append(k, [2]string{"f", fix}, [2]string{"n", hid}, [2]string{"o", i18n.T("Ordner prüfen")},
		[2]string{"r", i18n.T("neu prüfen")}, [2]string{"c", i18n.T("kopieren")}, [2]string{"Esc", i18n.T("zurück")})
}

func (a *App) vulnStyle(f vuln.Finding) lipgloss.Style {
	st := a.s
	if f.KEV {
		return st.Danger
	}
	if f.Minor {
		return st.Muted
	}
	switch f.Level() {
	case vuln.LevelCritical, vuln.LevelHigh:
		return st.Danger
	case vuln.LevelMedium:
		return st.Warn
	}
	return st.Muted
}

type vulnCols struct{ pkg, inst, fix, ctr int }

func vulnColumns(w int, containers bool) vulnCols {
	c := vulnCols{pkg: 22, inst: 24, fix: 24}
	if containers {
		c.ctr = 22
	}
	// fest: Stufe 9 + CVEs 5 + läuft 16 + Abstände
	rest := w - 9 - 6 - 18 - 8 - c.pkg - c.inst - c.fix - c.ctr
	if rest < 0 {
		shrink := -rest
		for _, p := range []*int{&c.ctr, &c.inst, &c.fix, &c.pkg} {
			cut := min(shrink, max(0, *p-12))
			*p -= cut
			shrink -= cut
		}
	} else {
		c.pkg += min(rest, 12)
	}
	return c
}

func (c vulnCols) header() string {
	h := padRight(i18n.T("Stufe"), 9) + " " + padRight(i18n.T("Paket"), c.pkg) + " " + padRight(i18n.T("installiert"), c.inst) + " " +
		padRight(i18n.T("behoben in"), c.fix) + " " + fmt.Sprintf("%5s", i18n.T("Lücken")) + "  " + padRight(i18n.T("läuft"), 18)
	if c.ctr > 0 {
		h += " " + i18n.T("Container")
	}
	return h
}

func (a *App) vulnRow(f vuln.Finding, c vulnCols, sel, fresh bool) string {
	st := a.s
	lvl := vulnLevelLabel(f.Level())
	if f.KEV {
		lvl += " KEV"
	} else if f.Minor {
		lvl = i18n.T("unwichtig")
	}
	fix := orDefault(f.Fixed, i18n.T("– kein Fix"))
	if f.Fixed != "" && f.NoFix > 0 {
		fix += "*"
	}
	run := "–"
	if len(f.Running) > 0 {
		run = strings.Join(f.Running, ", ")
	}
	pkg := f.Package
	if fresh {
		pkg = "● " + pkg
	}
	plain := padRight(lvl, 9) + " " + padRight(trunc(pkg, c.pkg), c.pkg) + " " + padRight(trunc(f.Installed, c.inst), c.inst) + " " +
		padRight(trunc(fix, c.fix), c.fix) + " " + fmt.Sprintf("%5d", len(f.CVEs)) + "  " + padRight(trunc(run, 18), 18)
	if c.ctr > 0 {
		plain += " " + trunc(f.Container, c.ctr)
	}
	if sel {
		return st.Selected.Render(plain)
	}
	ss := a.vulnStyle(f)
	fixS := st.OK
	if f.Fixed == "" {
		fixS = st.Muted
	}
	runS := st.Muted
	if len(f.Running) > 0 {
		runS = st.Warn
	}
	pkgS := st.Text
	if fresh {
		pkgS = st.Accent
	}
	out := ss.Render(padRight(lvl, 9)) + " " + pkgS.Render(padRight(trunc(pkg, c.pkg), c.pkg)) + " " + st.Muted.Render(padRight(trunc(f.Installed, c.inst), c.inst)) + " " +
		fixS.Render(padRight(trunc(fix, c.fix), c.fix)) + " " + st.Text.Render(fmt.Sprintf("%5d", len(f.CVEs))) + "  " + runS.Render(padRight(trunc(run, 18), 18))
	if c.ctr > 0 {
		out += " " + st.Muted.Render(trunc(f.Container, c.ctr))
	}
	return out
}

// vulnDetail: Kasten unter der Liste zum gewählten Paket.
func (a *App) vulnDetail(f vuln.Finding, w int) []string {
	st := a.s
	out := []string{st.Muted.Render(strings.Repeat("─", w))}
	for i, c := range f.CVEs {
		if i >= 3 {
			out = append(out, st.Muted.Render(fmt.Sprintf(i18n.T("  … %d weitere Lücken (⏎ zeigt alle)"), len(f.CVEs)-3)))
			break
		}
		head := c.ID
		if c.Score > 0 {
			head += fmt.Sprintf(" · CVSS %.1f", c.Score)
		}
		style := st.Text
		if c.KEV {
			head += i18n.T(" · aktiv ausgenutzt (CISA)")
			style = st.Danger
		}
		line := style.Render(head)
		if c.Text != "" {
			line += st.Muted.Render(" · " + c.Text)
		}
		out = append(out, ansi.Truncate(line, w, "…"))
	}
	if f.Container != "" {
		out = append(out, st.Muted.Render(trunc(i18n.T("Container: Image aktualisieren und neu starten, nicht im Container selbst."), w)))
	} else if cmd := vuln.UpdateCommand(f); cmd != "" && f.Fixed != "" {
		out = append(out, st.Muted.Render(i18n.T("u fügt ein: "))+st.AI.Render(trunc(cmd, w-14)))
	}
	return out
}

func (a *App) viewVuln() string {
	st := a.s
	v := &a.vscan
	w := max(40, a.w-2)
	h := max(8, a.h-4)
	var b strings.Builder
	r := v.rep
	title := i18n.T("Sicherheitscheck · ") + v.conn.Alias
	if r != nil && r.System != "" {
		title += " · " + r.System
	}
	head := st.Accent.Render(title)
	if r != nil {
		c := r.Count(a.vulnSkip(v.conn))
		sum := vulnSumText(c)
		style := st.OK
		switch {
		case c.KEV > 0 || c.Critical > 0:
			style = st.Danger
		case c.High > 0 || c.Medium > 0:
			style = st.Warn
		}
		head += st.Muted.Render(" · ") + style.Render(sum)
		head += st.Muted.Render(fmt.Sprintf(i18n.T(" · %d Pakete · OSV %s · %s"), r.Packages, r.Mode, ago(r.At)))
	}
	b.WriteString(ansi.Truncate(head, w, "…") + "\n")
	switch {
	case v.busy:
		b.WriteString(st.AI.Render(spinGlyph(time.Now())+" "+v.step) + "\n")
	case r != nil && r.Containers > 0:
		b.WriteString(st.Muted.Render(trunc(fmt.Sprintf(i18n.T("Host und %d laufende Container geprüft (nur lesend) · in %s"), r.Containers, durText(r.Took)), w)) + "\n")
	case r != nil:
		b.WriteString(st.Muted.Render(trunc(fmt.Sprintf(i18n.T("nur lesend geprüft · in %s"), durText(r.Took)), w)) + "\n")
	default:
		b.WriteString("\n")
	}
	if v.err != "" {
		b.WriteString(lipgloss.NewStyle().Width(w).Render(st.Danger.Render("× "+v.err)) + "\n")
	}
	if r == nil {
		return b.String()
	}
	for _, n := range r.Notes {
		b.WriteString(st.Warn.Render(trunc("▲ "+n, w)) + "\n")
	}
	fresh := vulnNew(v.prev, r)
	if v.prev != nil && len(fresh) > 0 {
		b.WriteString(st.Accent.Render(fmt.Sprintf(i18n.T("● %d Pakete neu betroffen seit %s"), len(fresh), v.prev.At.Local().Format("02.01. 15:04"))) + "\n")
	}
	switch {
	case v.hidden:
		b.WriteString(st.Muted.Render(i18n.T("Ausgeblendet („betrifft uns nicht“) · x holt zurück · n zurück zur Liste")) + "\n")
	case v.onlyFix:
		b.WriteString(st.Muted.Render(i18n.T("Nur Pakete, für die es ein Update gibt · f zeigt alle")) + "\n")
	}
	b.WriteString("\n")
	vis := a.vulnVisible()
	if len(vis) == 0 {
		switch {
		case v.hidden:
			b.WriteString(st.Muted.Render(i18n.T("Nichts ausgeblendet.")) + "\n")
		case len(r.Findings) == 0 && len(r.Ecosystems) > 0:
			b.WriteString(st.OK.Render(i18n.T("✓ Keine bekannten Lücken in den installierten Paketen.")) + "\n")
		case len(r.Findings) == 0:
			b.WriteString(st.Muted.Render(i18n.T("Nichts geprüft (siehe Hinweis oben).")) + "\n")
		default:
			b.WriteString(st.OK.Render(i18n.T("✓ Nichts mehr offen in dieser Ansicht.")) + "\n")
		}
		return b.String()
	}
	containers := false
	for _, f := range vis {
		if f.Container != "" {
			containers = true
			break
		}
	}
	cols := vulnColumns(w, containers)
	b.WriteString(st.Muted.Render(cols.header()) + "\n")
	used := strings.Count(b.String(), "\n")
	v.sel = clamp(v.sel, 0, len(vis)-1)
	detail := a.vulnDetail(vis[v.sel], w)
	room := max(3, h-used-1-len(detail))
	if v.sel < v.top {
		v.top = v.sel
	}
	if v.sel >= v.top+room {
		v.top = v.sel - room + 1
	}
	v.top = clamp(v.top, 0, max(0, len(vis)-room))
	for i := v.top; i < min(len(vis), v.top+room); i++ {
		i := i
		row := a.vulnRow(vis[i], cols, i == v.sel, fresh[vis[i].Key()] && v.prev != nil)
		b.WriteString(a.z.click(row, func(_ tea.Mouse, double bool) tea.Cmd {
			v.sel = i
			if double {
				a.showVulnDetail(vis[i])
			}
			return nil
		}) + "\n")
	}
	if rest := len(vis) - (v.top + room); rest > 0 {
		b.WriteString(st.Muted.Render(fmt.Sprintf(i18n.T("  … %d weitere (↓)"), rest)) + "\n")
	}
	b.WriteString(strings.Join(detail, "\n"))
	return b.String()
}

// viewVulnRow: Zeile „Sicherheit“ in F6 (letzte Prüfung, s prüft).
func (a *App) viewVulnRow(b *strings.Builder, s *session, w int) {
	st := a.s
	sum := a.vulnSummary(s.conn)
	label := st.Muted.Render(fmt.Sprintf("%-9s ", i18n.T("Sicherheit")))
	if a.vscan.sid == s.id && a.vscan.busy {
		b.WriteString(label + st.AI.Render(trunc(a.vscan.step, w-10)) + "\n")
		return
	}
	if !sum.ok {
		b.WriteString(label + st.Muted.Render(trunc(i18n.T("noch nicht geprüft · s prüft gegen OSV"), w-10)) + "\n")
		return
	}
	style := st.OK
	switch {
	case sum.c.KEV > 0 || sum.c.Critical > 0:
		style = st.Danger
	case sum.c.High > 0 || sum.c.Medium > 0:
		style = st.Warn
	}
	line := style.Render(vulnSumText(sum.c))
	b.WriteString(label + lipgloss.NewStyle().Width(w-10).Render(line+st.Muted.Render(" · "+ago(sum.at))) + "\n")
	if sum.fixes > 0 {
		b.WriteString(strings.Repeat(" ", 10) + st.Muted.Render(trunc(fmt.Sprintf(i18n.T("%d Pakete mit Update · s zeigt sie"), sum.fixes), w-10)) + "\n")
	}
}
