package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/relay"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Vermittler (internal/relay): Server hinter NAT/Firewall halten selbst eine
// Leitung zu einem Vermittler (eigener `sshtermy vermittler` oder Speech on
// DGX Spark), sshTermy erreicht ihr sshd darüber. SSH läuft Ende-zu-Ende, der
// Vermittler reicht nur Bytes durch. Alles hier ist freiwillig: ohne
// gekoppelten Vermittler sieht man davon nur den Eintrag in der Palette.

type relayList struct {
	l   relay.SystemList
	err error
	at  time.Time
}

// relayCache: letzte Systemliste je Vermittler (für Online-Punkt und Auswahl).
type relayCache struct {
	mu    sync.Mutex
	lists map[string]relayList
}

func (c *relayCache) put(id string, l relay.SystemList, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lists == nil {
		c.lists = map[string]relayList{}
	}
	c.lists[id] = relayList{l, err, time.Now()}
}

func (c *relayCache) get(id string) (relayList, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.lists[id]
	return r, ok
}

// relayMsg: Ergebnis einer Abfrage, angewendet im UI-Thread.
type relayMsg struct{ apply func(a *App) tea.Cmd }

func (a *App) relayClient(id string) (*relay.Client, domain.Relay, error) {
	r, ok := a.st.Relay(id)
	if !ok {
		return nil, r, errors.New(i18n.T("Vermittler auf diesem Rechner nicht gekoppelt (Palette → „Vermittler koppeln“)"))
	}
	cl, err := relay.NewClient(r.Base, r.Key.Reveal(), r.FP)
	return cl, r, err
}

// fetchList fragt einen Vermittler nach seinen Systemen und merkt das Ergebnis.
func (a *App) fetchList(ctx context.Context, id string) (relay.SystemList, domain.Relay, error) {
	cl, r, err := a.relayClient(id)
	if err != nil {
		return relay.SystemList{}, r, err
	}
	l, err := cl.Systems(ctx)
	if err == nil && l.Locked {
		err = fmt.Errorf(i18n.T("Vermittler „%s“ ist gesperrt: Palette → „Vermittler verwalten“ → Entsperren"), r.Name)
	}
	a.relays.put(id, l, err)
	return l, r, err
}

// relayDialer öffnet den Kanal für sshx.Dial.
func (a *App) relayDialer() sshx.RelayDialer {
	return func(ctx context.Context, c domain.Connection) (net.Conn, string, error) {
		l, r, err := a.fetchList(ctx, c.RelayID)
		if err != nil {
			return nil, "", err
		}
		var sys *relay.System
		for i := range l.Systems {
			if l.Systems[i].ID == c.RelaySystem {
				sys = &l.Systems[i]
			}
		}
		if sys == nil {
			return nil, "", fmt.Errorf(i18n.T("„%s“ gibt es beim Vermittler „%s“ nicht mehr"), c.Alias, r.Name)
		}
		if !sys.Online {
			return nil, "", fmt.Errorf(i18n.T("„%s“ ist gerade nicht mit dem Vermittler „%s“ verbunden (Anker läuft nicht?)"), sys.Name, r.Name)
		}
		cl, _, err := a.relayClient(c.RelayID)
		if err != nil {
			return nil, "", err
		}
		conn, err := cl.Dial(ctx, sys.ID, c.Addr())
		if err != nil {
			return nil, "", fmt.Errorf(i18n.T("Vermittler „%s“: %w"), r.Name, err)
		}
		return conn, sys.HostKey, nil
	}
}

func busy(title, body string) *modal { return &modal{kind: mBusy, title: title, body: body} }

// ---------------------------------------------------------------- Koppeln

func (a *App) openRelayForm() tea.Cmd {
	f := &form{title: i18n.T("Vermittler koppeln (eigener sshtermy vermittler oder Speech on DGX Spark)")}
	f.text("line", i18n.T("Kopplungszeile"), "", i18n.T("https://…#termy=CODE&fp=… · eigener: „sshtermy vermittler code“ · Spark: Fernzugang → sshTermy koppeln"))
	h, _ := os.Hostname()
	f.text("device", i18n.T("Dieser Rechner"), trunc(h, 40), i18n.T("so heißt dieser Rechner beim Vermittler"))
	a.form, a.formID, a.scr = f, "", scrRelayForm
	return f.setFocus(0)
}

func (a *App) startRelayPair() (tea.Cmd, error) {
	f := a.form
	pl, err := relay.ParseLine(f.raw("line"))
	if err != nil {
		return nil, err
	}
	device := f.str("device")
	if device == "" {
		device = "sshTermy"
	}
	a.modal = busy(i18n.T("Vermittler"), i18n.T("Kopple mit ")+pl.Base+" …")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cl, err := relay.NewClient(pl.Base, "", pl.FP)
		var pr relay.PairResult
		if err == nil {
			pr, err = cl.Pair(ctx, pl.Code, device)
		}
		return relayMsg{func(a *App) tea.Cmd {
			a.modal = nil
			if err != nil {
				a.flashErr(i18n.T("Koppeln fehlgeschlagen: ") + err.Error())
				return nil
			}
			kind := pr.Kind
			if kind == "" {
				kind = relay.KindSSHTermy
			}
			r, err := a.st.UpsertRelay(domain.Relay{Name: orDefault(pr.Name, cl.Base), Kind: kind, Base: cl.Base, FP: pl.FP,
				Key: domain.Secret(pr.Key), Device: device})
			if err != nil {
				a.flashErr(err.Error())
				return nil
			}
			a.flash(i18n.T("Gekoppelt mit Vermittler „") + r.Name + "“")
			return a.openRelaySystems()
		}}
	}, nil
}

// ---------------------------------------------------------------- Systeme

// fetchAll fragt alle gekoppelten Vermittler gleichzeitig.
func (a *App) fetchAll(rs []domain.Relay) {
	var wg sync.WaitGroup
	for _, r := range rs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _, _ = a.fetchList(ctx, id)
		}(r.ID)
	}
	wg.Wait()
}

// openRelaySystems: alle Systeme aller Vermittler; ⏎ verbindet (gespeichert)
// oder legt die Verbindung an (Benutzer und Anmeldung im Formular).
func (a *App) openRelaySystems() tea.Cmd {
	rs := a.st.Relays()
	if len(rs) == 0 {
		a.flash(i18n.T("Noch kein Vermittler gekoppelt"))
		return a.openRelayForm()
	}
	a.modal = busy(i18n.T("Vermittler"), i18n.T("Frage die Vermittler nach ihren Systemen …"))
	return func() tea.Msg {
		a.fetchAll(rs)
		return relayMsg{func(a *App) tea.Cmd {
			a.modal = nil
			return a.openPicker(a.relayPicker("\x00", func(it pickItem) tea.Cmd { return a.relayPicked(it.value) }))
		}}
	}
}

// relayValue: Wert eines Systems in Auswahllisten und im Formularfeld.
func relayValue(relayID, sys string) string { return relayID + "/" + sys }

func splitRelayValue(v string) (string, string) {
	r, s, _ := strings.Cut(v, "/")
	return r, s
}

// savedVia: gespeicherte Verbindung zu diesem System, falls es eine gibt.
func (a *App) savedVia(relayID, sys string) (domain.Connection, bool) {
	for _, c := range a.st.Connections() {
		if c.RelayID == relayID && c.RelaySystem == sys {
			return c, true
		}
	}
	return domain.Connection{}, false
}

func (a *App) relayPicker(current string, onPick func(pickItem) tea.Cmd) *picker {
	var items []pickItem
	if current != "\x00" {
		items = append(items, pickItem{value: "", label: i18n.T("— direkt (ohne Vermittler)")})
	}
	for _, r := range a.st.Relays() {
		rl, _ := a.relays.get(r.ID)
		head := pickItem{value: "relay:" + r.ID, label: "▾ " + r.Name, detail: relayKindLabel(r.Kind)}
		if rl.err != nil {
			head.disabled = "✗ " + trunc(rl.err.Error(), 60)
		} else {
			on := 0
			for _, s := range rl.l.Systems {
				if s.Online {
					on++
				}
			}
			head.disabled = fmt.Sprintf(i18n.T("%d Systeme, %d online"), len(rl.l.Systems), on)
		}
		items = append(items, head)
		syss := append([]relay.System(nil), rl.l.Systems...)
		sort.SliceStable(syss, func(i, j int) bool {
			if syss[i].Folder != syss[j].Folder {
				return syss[i].Folder < syss[j].Folder
			}
			return strings.ToLower(syss[i].Name) < strings.ToLower(syss[j].Name)
		})
		for _, s := range syss {
			it := pickItem{value: relayValue(r.ID, s.ID), label: s.Name, indent: 1, search: r.Name + " " + s.Folder + " " + s.OS}
			it.detail = strings.Trim(strings.Join([]string{s.OS, s.Folder}, " · "), " ·")
			hint := []string{}
			if s.Online {
				hint = append(hint, i18n.T("● online"))
			} else {
				hint = append(hint, i18n.T("○ offline"))
			}
			if c, ok := a.savedVia(r.ID, s.ID); ok {
				hint = append(hint, i18n.T("gespeichert als „")+trunc(c.Alias, 20)+"“")
			}
			it.hint = strings.Join(hint, " · ")
			items = append(items, it)
		}
	}
	cur := current
	if cur == "\x00" {
		cur = ""
	}
	p := newPicker(i18n.T("Systeme über Vermittler"), i18n.T("Name, Vermittler, Ordner oder System"), items, cur, onPick)
	p.help = i18n.T("Vermittler koppeln/verwalten: Strg+Leertaste")
	return p
}

func relayKindLabel(k string) string {
	if k == relay.KindSpark {
		return "Speech on DGX Spark"
	}
	return "sshtermy vermittler"
}

// relayPicked (aus dem Manager/der Palette): verbinden oder neu anlegen.
func (a *App) relayPicked(v string) tea.Cmd {
	rid, sid := splitRelayValue(v)
	if rid == "" || sid == "" {
		return nil
	}
	if c, ok := a.savedVia(rid, sid); ok {
		return a.openOrConnect(c)
	}
	r, _ := a.st.Relay(rid)
	rl, _ := a.relays.get(rid)
	n := newConnDefaults()
	n.RelayID, n.RelaySystem = rid, sid
	n.Alias, n.Host = sid, sid
	for _, s := range rl.l.Systems {
		if s.ID == sid {
			n.Alias, n.Host, n.Username = s.Name, s.Name, s.User
			n.Group = domain.NormPath(i18n.T("Vermittler") + " / " + r.Name)
			if s.Folder != "" {
				n.Group = domain.NormPath(n.Group + " / " + s.Folder)
			}
		}
	}
	return a.openConnForm(n)
}

// ---------------------------------------------------------------- Formularfeld

// addRelayField: im Verbindungsformular „Über Vermittler“ (nur wenn einer
// gekoppelt ist oder die Verbindung schon einen nutzt). Gewählt, sind Host,
// Port und Jump-Host ausgeblendet.
func (a *App) addRelayField(f *form, c domain.Connection) {
	if len(a.st.Relays()) == 0 && !c.ViaRelay() {
		return
	}
	val := ""
	if c.ViaRelay() {
		val = relayValue(c.RelayID, c.RelaySystem)
	}
	fl := &field{key: "relay", label: i18n.T("Über Vermittler"), kind: fPick, value: val,
		help:    i18n.T("Server hinter NAT/Firewall mit Anker · ⏎ öffnet die Systeme aller Vermittler"),
		display: func(v string) string { return a.relayFieldText(v) },
		open: func() tea.Cmd {
			fl := f.get("relay")
			rs := a.st.Relays()
			return func() tea.Msg {
				a.fetchAll(rs)
				return relayMsg{func(a *App) tea.Cmd {
					return a.openPicker(a.relayPicker(fl.value, func(it pickItem) tea.Cmd {
						fl.value = it.value
						return nil
					}))
				}}
			}
		}}
	via := func(f *form) bool { return f.val("relay") != "" }
	pos := len(f.fields)
	for i, x := range f.fields {
		switch x.key {
		case "host", "port", "jump":
			old := x.hidden
			x.hidden = func(f *form) bool { return via(f) || (old != nil && old(f)) }
			if x.key == "host" && i < pos {
				pos = i
			}
		}
	}
	proto := f.get("proto")
	fl.hidden = func(f *form) bool { return proto.options != nil && protoFromLabel(f.sel("proto")).Raw() }
	f.fields = append(f.fields[:pos], append([]*field{fl}, f.fields[pos:]...)...)
}

func (a *App) relayFieldText(v string) string {
	s := a.s
	if v == "" {
		return s.Muted.Render(i18n.T("— direkt (ohne Vermittler)"))
	}
	rid, sid := splitRelayValue(v)
	r, ok := a.st.Relay(rid)
	if !ok {
		return s.Danger.Render(i18n.T("⚠ Vermittler hier nicht gekoppelt"))
	}
	name := sid
	state := ""
	if rl, ok := a.relays.get(rid); ok {
		for _, x := range rl.l.Systems {
			if x.ID == sid {
				name = x.Name
				state = i18n.T(" · ○ offline")
				if x.Online {
					state = i18n.T(" · ● online")
				}
			}
		}
	}
	return name + s.Muted.Render(" · "+r.Name+state)
}

// applyRelayField übernimmt das Feld in die Verbindung (saveConn).
func (a *App) applyRelayField(f *form, c *domain.Connection) {
	c.RelayID, c.RelaySystem = "", ""
	v := f.val("relay")
	if v == "" || c.Protocol.Raw() {
		return
	}
	c.RelayID, c.RelaySystem = splitRelayValue(v)
	c.JumpHostID, c.Port = "", 22
	if rl, ok := a.relays.get(c.RelayID); ok {
		for _, x := range rl.l.Systems {
			if x.ID == c.RelaySystem {
				c.Host = x.Name
			}
		}
	}
	if c.Host == "" {
		c.Host = c.RelaySystem
	}
	if c.Alias == "" {
		c.Alias = c.Host
	}
}

// ---------------------------------------------------------------- Verwalten

func (a *App) openRelayManage() tea.Cmd {
	items := []menuItem{{label: i18n.T("Neuen Vermittler koppeln"), hint: "+", run: func() tea.Cmd { return a.openRelayForm() }}}
	for _, r := range a.st.Relays() {
		r := r
		items = append(items,
			menuItem{label: i18n.T("Testen: ") + r.Name, hint: relayKindLabel(r.Kind), run: func() tea.Cmd { return a.testRelay(r) }},
			menuItem{label: i18n.T("Entsperren: ") + r.Name, hint: i18n.T("Code"), run: func() tea.Cmd { return a.unlockRelay(r) }},
			menuItem{label: i18n.T("Entfernen: ") + r.Name, hint: i18n.T("nur hier"), run: func() tea.Cmd {
				a.modal = confirmModal(i18n.T("Vermittler entfernen?"),
					i18n.T("„")+r.Name+i18n.T("“ wird auf diesem Rechner vergessen. Verbindungen darüber bleiben gespeichert, gehen aber erst nach neuem Koppeln wieder. Beim Vermittler selbst bleibt der Eintrag, bis du ihn dort entfernst."),
					func() tea.Cmd {
						if err := a.st.DeleteRelay(r.ID); err != nil {
							a.flashErr(err.Error())
						} else {
							a.flash(i18n.T("Vermittler entfernt: ") + r.Name)
						}
						return nil
					})
				return nil
			}})
	}
	items = append(items, menuItem{label: i18n.T("Systeme über Vermittler"), hint: "⏎", run: func() tea.Cmd { return a.openRelaySystems() }})
	a.modal = &modal{kind: mMenu, title: i18n.T("Vermittler"), items: items}
	return nil
}

// testRelay: erreichbar? Zertifikat? Schlüssel? Systeme?
func (a *App) testRelay(r domain.Relay) tea.Cmd {
	a.modal = busy(i18n.T("Vermittler testen"), r.Base+" …")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		t0 := time.Now()
		l, _, err := a.fetchList(ctx, r.ID)
		ms := time.Since(t0).Milliseconds()
		return relayMsg{func(a *App) tea.Cmd {
			pin := i18n.T("Prüfung gegen die Zertifizierungsstellen des Systems")
			if r.FP != "" {
				pin = i18n.T("gepinnt, SHA256 ") + r.FP[:16] + "…"
			}
			lines := []string{
				i18n.T("Adresse:      ") + r.Base,
				i18n.T("Art:          ") + relayKindLabel(r.Kind),
				i18n.T("Zertifikat:   ") + pin,
				i18n.T("Dieser PC:    ") + r.Device,
				"",
			}
			if err != nil {
				lines = append(lines, "✗ "+err.Error())
			} else {
				on := 0
				for _, s := range l.Systems {
					if s.Online {
						on++
					}
				}
				lines = append(lines, i18n.T("✓ erreichbar, Zertifikat und Schlüssel gültig")+fmt.Sprintf(" (%d ms)", ms),
					fmt.Sprintf(i18n.T("✓ %d Systeme, davon %d online"), len(l.Systems), on))
			}
			a.modal = &modal{kind: mInfo, title: i18n.T("Vermittler „") + r.Name + "“", body: strings.Join(lines, "\n")}
			return nil
		}}
	}
}

// unlockRelay: Vermittler mit Sperre (Spark) mit einem frischen Code öffnen.
func (a *App) unlockRelay(r domain.Relay) tea.Cmd {
	a.modal = inputModal(i18n.T("Entsperren: ")+r.Name, i18n.T("Code aus der App (zweiter Schritt)"), "", func(code string) tea.Cmd {
		code = strings.TrimSpace(code)
		if code == "" {
			return nil
		}
		a.modal = busy(i18n.T("Vermittler"), i18n.T("Entsperre ")+r.Name+" …")
		return func() tea.Msg {
			cl, _, err := a.relayClient(r.ID)
			var until time.Time
			if err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				until, err = cl.Unlock(ctx, code)
			}
			return relayMsg{func(a *App) tea.Cmd {
				a.modal = nil
				var se *relay.StatusError
				switch {
				case errors.As(err, &se) && se.Code == 404:
					a.flash(i18n.T("Dieser Vermittler braucht kein Entsperren"))
				case err != nil:
					a.flashErr(i18n.T("Entsperren: ") + err.Error())
				default:
					a.flash(i18n.T("Entsperrt bis ") + until.Local().Format("15:04"))
				}
				return nil
			}}
		}
	})
	return nil
}

// ---------------------------------------------------------------- Online-Punkt

// relayReach: Online-Punkt für Verbindungen über Vermittler aus der
// Systemliste (statt TCP-Prüfung, die Server sind ja nicht direkt erreichbar).
func (a *App) relayReach(conns []domain.Connection) map[string]reachResult {
	ids := map[string]bool{}
	var rs []domain.Relay
	for _, c := range conns {
		if !ids[c.RelayID] {
			ids[c.RelayID] = true
			if r, ok := a.st.Relay(c.RelayID); ok {
				rs = append(rs, r)
			}
		}
	}
	a.fetchAll(rs)
	out := map[string]reachResult{}
	for _, c := range conns {
		r := reachResult{at: time.Now(), err: "vermittler"}
		if rl, ok := a.relays.get(c.RelayID); ok && rl.err == nil {
			r.err = "offline"
			for _, s := range rl.l.Systems {
				if s.ID == c.RelaySystem && s.Online {
					r.ok, r.err = true, ""
				}
			}
		}
		out[c.ID] = r
	}
	return out
}
