package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/vuln"
)

// Firmware-Hinweise für Switches, Firewalls und Router (F6 s am
// Netzwerkgerät, Palette „Firmware-Hinweise“). OSV kennt keine
// Hersteller-Firmware; sshTermy fragt deshalb die NVD nach allen CVEs des
// Produkts (CPE aus dem Befehlssatz, advisory.cpe) und prüft die Version
// selbst. Nach außen geht nur der Produktname. Für einen Ordner reicht der
// gespeicherte Firmware-Stand aus dem Systemprofil, es wird nicht verbunden.

type fwTarget struct {
	conn     domain.Connection
	vendor   string // Befehlssatz
	model    string
	firmware string
}

type fwResult struct {
	t   fwTarget
	rep *vuln.FirmwareReport
	err string
	url string // Seite des Herstellers
}

type fwState struct {
	seq    int
	busy   bool
	step   string
	title  string
	single bool
	res    []fwResult
	cancel context.CancelFunc
}

type fwStepMsg struct {
	seq  int
	step string
}

type fwDoneMsg struct {
	seq int
	res []fwResult
}

// fwProduct: CVEs zu einem Produkt (in Tests austauschbar).
var fwProduct = func(a *App) func(ctx context.Context, product string, step func(string)) ([]vuln.NVDCVE, bool, error) {
	cfg := a.st.Settings().Vuln
	n := &vuln.NVD{HTTP: vuln.HTTP{Cache: vulnCacheDir()}, URL: strings.TrimSpace(cfg.NVDURL), Key: cfg.NVDKey.Reveal()}
	return n.Product
}

// fwKEV: Liste aktiv ausgenutzter Lücken (in Tests austauschbar).
var fwKEV = func(ctx context.Context) vuln.KEV {
	k, _ := vuln.LoadKEV(ctx, &vuln.HTTP{Cache: vulnCacheDir()})
	return k
}

// fwAdvisory: CPE-Angaben des Befehlssatzes.
func fwAdvisory(vendor, model, firmware string) cmdset.Advisory {
	return hostinfo.SetOf(hostinfo.Vendor(vendor), model, firmware).Advisory
}

// fwTargetOf: Gerät aus einer Sitzung (F6) bzw. einer Verbindung (Profil).
func fwTargetOf(s *session) (fwTarget, bool) {
	t := fwTarget{conn: s.conn, vendor: s.prof.Dialect, model: s.prof.Model, firmware: s.prof.Version}
	if s.host.vendor != hostinfo.VendorUnknown {
		t.vendor = string(s.host.vendor)
	}
	if sw := s.host.sw; sw != nil {
		if sw.Vendor != hostinfo.VendorUnknown {
			t.vendor = string(sw.Vendor)
		}
		t.model, t.firmware = orDefault(sw.Model, t.model), orDefault(sw.Firmware, t.firmware)
	}
	return t, t.vendor != "" && t.vendor != "network"
}

func fwTargetConn(c domain.Connection) (fwTarget, bool) {
	if c.Profile == nil || c.Profile.Family != domain.FamilyNetwork {
		return fwTarget{conn: c}, false
	}
	p := *c.Profile
	return fwTarget{conn: c, vendor: string(hostVendor(p)), model: p.Model, firmware: p.Version}, true
}

// openFirmware: F6 s am Netzwerkgerät.
func (a *App) openFirmware(s *session) tea.Cmd {
	t, ok := fwTargetOf(s)
	if !ok {
		s.host.err = i18n.T("Hersteller unbekannt: erst r drücken (liest Modell und Firmware)")
		return nil
	}
	if t.firmware == "" {
		s.host.err = i18n.T("Firmware unbekannt: erst r drücken (liest Modell und Firmware)")
		return nil
	}
	return a.runFirmware(s.conn.Alias, []fwTarget{t}, true)
}

// netConnsIn: Netzwerkgeräte im Ordner.
func (a *App) netConnsIn(folder string) []domain.Connection {
	var out []domain.Connection
	for _, c := range a.st.Connections() {
		if !domain.InFolder(c.Group, folder) {
			continue
		}
		if c.Device == domain.DeviceNetwork || (c.Profile != nil && c.Profile.Family == domain.FamilyNetwork) {
			out = append(out, c)
		}
	}
	return out
}

// openFirmwareFolder: Palette „Firmware-Hinweise: Netzwerkgeräte im Ordner“.
func (a *App) openFirmwareFolder() tea.Cmd {
	count := map[string]int{}
	for _, c := range a.netConnsIn("") {
		count[""]++
		p := c.Group
		for p != "" {
			count[p]++
			i := strings.LastIndex(p, domain.PathSep)
			if i < 0 {
				break
			}
			p = p[:i]
		}
	}
	if count[""] == 0 {
		a.flash(i18n.T("Keine Switches oder Firewalls gespeichert"))
		return nil
	}
	var paths []string
	for p := range count {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	items := []pickItem{{value: "\x00all", label: i18n.T("Alle Verbindungen"), hint: fmt.Sprint(count[""])}}
	for _, p := range paths {
		items = append(items, pickItem{value: p, label: p, hint: fmt.Sprint(count[p]), indent: strings.Count(p, domain.PathSep)})
	}
	cur := ""
	if a.scr == scrManager {
		if r, ok := a.mgrCur(); ok {
			if r.isFolder() {
				cur = r.node.Path
			} else {
				cur = r.conn.Group
			}
		}
	}
	a.pick = newPicker(i18n.T("Firmware-Hinweise: Netzwerkgeräte im Ordner"), i18n.T("Ordner suchen …"), items, cur, func(it pickItem) tea.Cmd {
		f := it.value
		if f == "\x00all" {
			f = ""
		}
		var ts []fwTarget
		for _, c := range a.netConnsIn(f) {
			t, _ := fwTargetConn(c)
			ts = append(ts, t)
		}
		where := f
		if where == "" {
			where = i18n.T("alle Verbindungen")
		}
		return a.runFirmware(where, ts, false)
	})
	a.pick.help = i18n.T("Verbindet sich nicht: nutzt Hersteller und Firmware, die F6 zuletzt gelesen hat. An die NVD geht nur der Produktname (z. B. fortios), nie Version oder Gerät.")
	return nil
}

func (a *App) runFirmware(title string, ts []fwTarget, single bool) tea.Cmd {
	f := &a.fwchk
	if f.cancel != nil {
		f.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	*f = fwState{seq: f.seq + 1, busy: true, title: title, single: single, cancel: cancel, step: i18n.T("frage die NVD …")}
	seq, ev := f.seq, a.events
	get := fwProduct(a)
	go func() {
		defer cancel()
		ev <- fwDoneMsg{seq: seq, res: firmwareCheck(ctx, ts, get, fwKEV, func(s string) { ev <- fwStepMsg{seq, s} })}
	}()
	a.showFirmware()
	return nil
}

// firmwareCheck: je Produkt einmal fragen, dann jedes Gerät prüfen.
func firmwareCheck(ctx context.Context, ts []fwTarget, get func(context.Context, string, func(string)) ([]vuln.NVDCVE, bool, error),
	kevf func(context.Context) vuln.KEV, step func(string)) []fwResult {
	type prod struct {
		cves  []vuln.NVDCVE
		stale bool
		err   error
	}
	cache := map[string]*prod{}
	var kev vuln.KEV
	res := make([]fwResult, len(ts))
	for i, t := range ts {
		r := fwResult{t: t}
		adv := fwAdvisory(t.vendor, t.model, t.firmware)
		r.url = adv.URL
		switch {
		case t.vendor == "" || t.vendor == "network":
			r.err = i18n.T("Hersteller unbekannt (einmal F6 öffnen)")
		case t.firmware == "":
			r.err = i18n.T("Firmware unbekannt (einmal F6 öffnen)")
		case len(adv.CPE) == 0:
			r.err = i18n.T("keine NVD-Zuordnung für diesen Hersteller")
		}
		if r.err != "" {
			res[i] = r
			continue
		}
		if kev == nil {
			kev = kevf(ctx)
			if kev == nil {
				kev = vuln.KEV{}
			}
		}
		var hits []vuln.FirmwareHit
		var total int
		var stale bool
		var rep *vuln.FirmwareReport
		for _, cpe := range adv.CPE {
			p := cache[cpe]
			if p == nil {
				step(fmt.Sprintf(i18n.T("frage die NVD nach %s …"), strings.TrimPrefix(cpe, "cpe:2.3:")))
				cv, st, err := get(ctx, cpe, step)
				p = &prod{cv, st, err}
				cache[cpe] = p
			}
			if p.err != nil {
				r.err = vulnErrText(p.err)
				continue
			}
			x := vuln.FirmwareReportFor(cpe, t.firmware, adv.Version, p.cves, kev)
			hits, total, stale = append(hits, x.Hits...), total+x.Total, stale || p.stale
			if rep == nil {
				rep = x
			}
		}
		if rep != nil {
			r.err = ""
			rep.Hits, rep.Total, rep.Stale, rep.URL = hits, total, stale, adv.URL
			rep.Product = strings.Join(adv.CPE, ", ")
			rep.MinFixed = ""
			for _, h := range hits {
				if h.FixedIn != "" && (rep.MinFixed == "" || vuln.CompareRPM(h.FixedIn, rep.MinFixed) > 0) {
					rep.MinFixed = h.FixedIn
				}
			}
			r.rep = rep
		}
		res[i] = r
	}
	return res
}

func vulnErrText(err error) string {
	if err == nil {
		return ""
	}
	if vuln.IsNotFound(err) {
		return i18n.T("NVD: Produkt nicht gefunden")
	}
	return trunc(err.Error(), 80)
}

func (a *App) firmwareMsgs(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case fwStepMsg:
		if m.seq == a.fwchk.seq {
			a.fwchk.step = m.step
			if a.modal != nil && a.modal.fw {
				a.showFirmware()
			}
		}
		return a.listen(), true
	case fwDoneMsg:
		f := &a.fwchk
		if m.seq != f.seq {
			return a.listen(), true
		}
		f.busy, f.res = false, m.res
		for _, r := range m.res {
			if r.rep != nil {
				if raw, err := json.Marshal(r.rep); err == nil {
					_ = a.st.SetFirmwareReport(r.t.conn.ID, raw)
				}
			}
		}
		if a.modal != nil && a.modal.fw {
			a.showFirmware()
		}
		return a.listen(), true
	}
	return nil, false
}

// storedFirmware: letzte Prüfung eines Geräts.
func (a *App) storedFirmware(connID string) *vuln.FirmwareReport {
	raw := a.st.FirmwareReport(connID)
	if len(raw) == 0 {
		return nil
	}
	var r vuln.FirmwareReport
	if json.Unmarshal(raw, &r) != nil {
		return nil
	}
	return &r
}

func fwCountText(c vuln.Counts) string {
	if c.Total == 0 {
		return i18n.T("keine bekannten Lücken")
	}
	var p []string
	if c.KEV > 0 {
		p = append(p, fmt.Sprintf(i18n.T("%d aktiv ausgenutzt"), c.KEV))
	}
	if c.Critical > 0 {
		p = append(p, fmt.Sprintf(i18n.T("%d kritisch"), c.Critical))
	}
	if c.High > 0 {
		p = append(p, fmt.Sprintf(i18n.T("%d hoch"), c.High))
	}
	if c.Medium > 0 {
		p = append(p, fmt.Sprintf(i18n.T("%d mittel"), c.Medium))
	}
	if n := c.Low + c.Unknown; n > 0 {
		p = append(p, fmt.Sprintf(i18n.T("%d gering"), n))
	}
	return strings.Join(p, " · ")
}

// showFirmware: Ergebnis als Textfenster (c kopiert Markdown).
func (a *App) showFirmware() {
	f := &a.fwchk
	var lines []string
	var md strings.Builder
	title := i18n.T("Firmware-Hinweise: ") + f.title
	fmt.Fprintf(&md, "# %s\n\n", title)
	if f.busy {
		lines = append(lines, spinGlyph(time.Now())+" "+f.step, "")
	}
	if f.single && len(f.res) == 1 {
		r := f.res[0]
		lines = append(lines, a.fwDetailLines(r, &md)...)
	} else if len(f.res) > 0 {
		lines = append(lines, fmt.Sprintf("%-22s %-14s %-16s %4s %5s %5s  %s", i18n.T("Gerät"), i18n.T("Hersteller"), "Firmware", "KEV", i18n.T("krit"), i18n.T("hoch"), i18n.T("behoben ab")))
		md.WriteString(i18n.T("| Gerät | Hersteller | Firmware | KEV | kritisch | hoch | behoben ab | Hinweis |\n|---|---|---|---|---|---|---|---|\n"))
		rows := append([]fwResult(nil), f.res...)
		sort.SliceStable(rows, func(i, j int) bool { return fwRank(rows[i]) > fwRank(rows[j]) })
		for _, r := range rows {
			name := trunc(r.t.conn.Alias, 22)
			vend := trunc(hostinfo.Vendor(r.t.vendor).Name(), 14)
			if r.rep == nil {
				lines = append(lines, fmt.Sprintf("%-22s %-14s %-16s  %s", name, vend, trunc(orDefault(r.t.firmware, "?"), 16), r.err))
				fmt.Fprintf(&md, "| %s | %s | %s | | | | | %s |\n", r.t.conn.Alias, vend, r.t.firmware, r.err)
				continue
			}
			c := r.rep.Counts()
			note := ""
			if r.rep.Total == 0 {
				note = i18n.T("NVD kennt das Produkt nicht – CPE im Befehlssatz prüfen")
			} else if r.rep.Stale {
				note = i18n.T("NVD nicht erreichbar, älterer Stand")
			}
			lines = append(lines, fmt.Sprintf("%-22s %-14s %-16s %4d %5d %5d  %s %s", name, vend, trunc(r.rep.Raw, 16), c.KEV, c.Critical, c.High, orDefault(r.rep.MinFixed, "–"), note))
			fmt.Fprintf(&md, "| %s | %s | %s | %d | %d | %d | %s | %s |\n", r.t.conn.Alias, vend, r.rep.Raw, c.KEV, c.Critical, c.High, r.rep.MinFixed, note)
		}
		lines = append(lines, "", i18n.T("Einzelheiten: Tab des Geräts öffnen, F6 › s. Firmware-Stand aus der letzten Abfrage in F6."))
	}
	lines = append(lines, "", i18n.T("Quelle: NVD (NIST) und CISA-Liste aktiv ausgenutzter Lücken. Die NVD ordnet Versionen nicht immer genau zu: Hersteller-Meldung prüfen."))
	scroll := 0
	if a.modal != nil && a.modal.kind == mText && a.modal.fw {
		scroll = a.modal.scroll
	}
	a.modal = textModal(title, lines)
	a.modal.copy, a.modal.scroll, a.modal.fw = md.String(), scroll, true
}

func fwRank(r fwResult) int {
	if r.rep == nil {
		return -1
	}
	c := r.rep.Counts()
	return c.KEV*10000 + c.Critical*100 + c.High
}

func (a *App) fwDetailLines(r fwResult, md *strings.Builder) []string {
	var lines []string
	vend := hostinfo.Vendor(r.t.vendor).Name()
	head := fmt.Sprintf("%s %s", vend, orDefault(r.t.firmware, "?"))
	if r.t.model != "" {
		head += " · " + r.t.model
	}
	lines = append(lines, head)
	fmt.Fprintf(md, "%s\n\n", head)
	if r.rep == nil {
		if r.err != "" {
			lines = append(lines, "× "+r.err)
			fmt.Fprintf(md, "× %s\n", r.err)
		}
		if r.url != "" {
			lines = append(lines, i18n.T("Sicherheitsmeldungen des Herstellers: ")+r.url)
			fmt.Fprintf(md, "%s%s\n", i18n.T("Sicherheitsmeldungen des Herstellers: "), r.url)
		}
		return lines
	}
	rep := r.rep
	c := rep.Counts()
	sum := fwCountText(c)
	if rep.MinFixed != "" {
		sum += fmt.Sprintf(i18n.T(" · behoben ab %s"), rep.MinFixed)
	}
	lines = append(lines, sum)
	fmt.Fprintf(md, "%s\n\n", sum)
	if rep.Total == 0 {
		lines = append(lines, i18n.T("Die NVD kennt keine CVEs zu diesem Produkt. Vermutlich stimmt die Zuordnung (advisory.cpe im Befehlssatz) nicht: das heißt nicht, dass die Firmware sicher ist."))
	}
	if rep.Stale {
		lines = append(lines, i18n.T("NVD nicht erreichbar, älterer Stand aus dem Zwischenspeicher."))
	}
	lines = append(lines, "")
	for _, h := range rep.Hits {
		lv := vulnLevelLabel(h.Level())
		flag := ""
		if h.KEV {
			flag = " KEV"
		}
		fix := ""
		if h.FixedIn != "" {
			fix = fmt.Sprintf(i18n.T(" · behoben ab %s"), h.FixedIn)
		}
		hw := ""
		if h.HW {
			hw = i18n.T(" · nur bestimmte Modelle")
		}
		lines = append(lines, fmt.Sprintf("%-5s %4.1f %-16s%s%s%s", lv, h.Score, h.ID, flag, fix, hw))
		if h.Summary != "" {
			lines = append(lines, "      "+trunc(h.Summary, 160))
		}
		fmt.Fprintf(md, "- **%s** %s %.1f%s%s%s: %s\n", h.ID, lv, h.Score, flag, fix, hw, h.Summary)
	}
	lines = append(lines, "", fmt.Sprintf(i18n.T("Geprüft: %s (%d CVEs zum Produkt in der NVD) · %s"), rep.Product, rep.Total, rep.At.Format("02.01.2006 15:04")))
	if rep.URL != "" {
		lines = append(lines, i18n.T("Sicherheitsmeldungen des Herstellers: ")+rep.URL)
		fmt.Fprintf(md, "\n%s%s\n", i18n.T("Sicherheitsmeldungen des Herstellers: "), rep.URL)
	}
	return lines
}

// viewFirmwareRow: Zeile „Sicherheit“ in F6 bei Netzwerkgeräten.
func (a *App) viewFirmwareRow(b *strings.Builder, s *session, w int) {
	st := a.s
	label := st.Muted.Render(fmt.Sprintf("%-9s ", i18n.T("Sicherheit")))
	if a.fwchk.busy && a.fwchk.single && len(a.fwchk.res) == 0 && a.fwchk.title == s.conn.Alias {
		b.WriteString(label + st.AI.Render(trunc(a.fwchk.step, w-10)) + "\n")
		return
	}
	rep := a.storedFirmware(s.conn.ID)
	if rep == nil {
		b.WriteString(label + st.Muted.Render(trunc(i18n.T("Firmware nicht geprüft · s fragt die NVD"), w-10)) + "\n")
		return
	}
	c := rep.Counts()
	style := st.OK
	switch {
	case c.KEV > 0 || c.Critical > 0:
		style = st.Danger
	case c.High > 0 || c.Medium > 0:
		style = st.Warn
	}
	txt := fwCountText(c)
	if rep.Total == 0 {
		style, txt = st.Muted, i18n.T("NVD kennt das Produkt nicht")
	}
	b.WriteString(label + style.Render(trunc(txt, w-10)) + "\n")
	if rep.MinFixed != "" {
		b.WriteString(strings.Repeat(" ", 10) + st.Muted.Render(trunc(fmt.Sprintf(i18n.T("behoben ab %s · s zeigt mehr"), rep.MinFixed), w-10)) + "\n")
	}
}

// askNVDKey: Einstellungen › Sicherheitscheck › NVD-API-Schlüssel.
func (a *App) askNVDKey() tea.Cmd {
	m := inputModal(i18n.T("Firmware-Hinweise: NVD-API-Schlüssel"),
		i18n.T("Kostenlos unter nvd.nist.gov/developers/request-an-api-key. Mit Schlüssel geht die Abfrage etwa zehnmal schneller. Leer = ohne Schlüssel."),
		"", func(v string) tea.Cmd {
			v = strings.TrimSpace(v)
			_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Vuln.NVDKey = domain.Secret(v) })
			if v == "" {
				a.flash(i18n.T("NVD ohne Schlüssel"))
			} else {
				a.flashOK(i18n.T("NVD-Schlüssel gespeichert"))
			}
			return nil
		})
	m.in.EchoMode = textinput.EchoPassword
	a.modal = m
	return m.in.Focus()
}

func nvdKeyLabel(s domain.Settings) string {
	if s.Vuln.NVDKey.IsSet() {
		return i18n.T("gesetzt")
	}
	return i18n.T("keiner")
}
