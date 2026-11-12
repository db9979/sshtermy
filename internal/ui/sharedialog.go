package ui

import (
	"errors"
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"rsc.io/qr"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Teilen-Dialog: statt einer Fehlermeldung („erst Vermittler einrichten“)
// wählt man hier den Weg – im selben Netz direkt geht immer – und die
// Freigabe. Die Vorgaben sind die sicheren: nur zusehen, 30 Minuten, eine
// Person, anklopfen, Passwörter verborgen, ab jetzt.

type shareFormState struct {
	sid  int
	ways map[string]string // Beschriftung → "lan" | "relay:<id>" | "public" | "setup"
}

var (
	shareMinutes = []int{15, 30, 60, 120, 240}
	shareMax     = []int{1, 2, 5, 10}
)

func minutesLabel(n int) string { return fmt.Sprintf(i18n.T("%d Minuten"), n) }

func maxLabel(n int) string {
	if n == 1 {
		return i18n.T("1 Person")
	}
	return fmt.Sprintf(i18n.T("%d Personen"), n)
}

func (a *App) openShareForm(s *session) tea.Cmd {
	cfg := a.st.Settings().Share
	ways := map[string]string{}
	var labels []string
	add := func(label, id string) {
		ways[label] = id
		labels = append(labels, label)
	}
	add(i18n.T("Im selben Netz – direkt, ohne Vermittler"), "lan")
	for _, r := range a.st.Relays() {
		add(fmt.Sprintf(i18n.T("Über Vermittler „%s“"), r.Name), "relay:"+r.ID)
	}
	if b, _, err := parsePublicRelay(cfg.Public); err == nil {
		add(fmt.Sprintf(i18n.T("Über öffentlichen Vermittler (%s)"), strings.TrimPrefix(b, "https://")), "public")
	}
	add(i18n.T("Eigenen Vermittler einrichten … (Assistent)"), "setup")
	cur := labels[0]
	for l, id := range ways {
		if id == cfg.Way && id != "setup" {
			cur = l
		}
	}
	a.shareForm = shareFormState{sid: s.id, ways: ways}

	f := &form{title: i18n.T("Sitzung teilen: ") + s.tabName()}
	f.choice("way", i18n.T("Wie zusehen?"), labels, cur, i18n.T("selbes Netz: sofort, ohne Einrichtung · übers Internet: Vermittler (Ende-zu-Ende verschlüsselt)"))
	f.section("sec", i18n.T("Freigabe"), nil)
	setup := func(f *form) bool { return ways[f.sel("way")] == "setup" }
	minOpts := make([]string, len(shareMinutes))
	for i, n := range shareMinutes {
		minOpts[i] = minutesLabel(n)
	}
	mins := cfg.Minutes
	if mins == 0 {
		mins = 30
	}
	f.choice("min", i18n.T("Gültig"), minOpts, minutesLabel(mins), i18n.T("danach endet das Teilen von selbst")).hidden = setup
	maxOpts := make([]string, len(shareMax))
	for i, n := range shareMax {
		maxOpts[i] = maxLabel(n)
	}
	mx := cfg.Max
	if mx == 0 {
		mx = 1
	}
	f.choice("max", i18n.T("Wie viele"), maxOpts, maxLabel(mx), i18n.T("danach ist der Code verbraucht")).hidden = setup
	f.toggle("knock", i18n.T("Anklopfen lassen"), !cfg.NoKnock, i18n.T("jeder Zuschauer braucht dein „j“; du siehst Name, Rechner und Prüfwörter")).hidden = setup
	f.toggle("hidepw", i18n.T("Passwörter verbergen"), !cfg.ShowPw, i18n.T("Bild pausiert bei Passwortabfragen (sudo, ssh, su …)")).hidden = setup
	f.toggle("screen", i18n.T("Aktuellen Bildschirm zeigen"), cfg.Screen, i18n.T("aus = Zuschauer sehen nur, was ab jetzt kommt")).hidden = setup
	if isProduction(s.conn) {
		f.section("prod", fmt.Sprintf(i18n.T("▲ %s gilt als produktiv: Zuschauer sehen alles, was ab dem Start in diesem Tab erscheint"), s.conn.Alias),
			func(t Theme) color.Color { return t.Danger }).hidden = setup
	}
	a.returnToTerminal = a.scr == scrTerminal
	a.form, a.formID, a.scr = f, "", scrShareForm
	return f.setFocus(0)
}

func pickInt(label string, opts []int, name func(int) string, def int) int {
	for _, n := range opts {
		if name(n) == label {
			return n
		}
	}
	return def
}

// submitShareForm: Strg+S im Teilen-Dialog.
func (a *App) submitShareForm() (tea.Cmd, error) {
	f := a.form
	s := a.byID(a.shareForm.sid)
	if s == nil || s.state != stConnected {
		return nil, errors.New(i18n.T("Der Tab ist nicht mehr verbunden"))
	}
	way := a.shareForm.ways[f.sel("way")]
	if way == "setup" {
		return a.openRelaySetup(s), nil
	}
	opt := shareOpts{way: way, minutes: pickInt(f.sel("min"), shareMinutes, minutesLabel, 30), max: pickInt(f.sel("max"), shareMax, maxLabel, 1),
		knock: f.flag("knock"), hidePw: f.flag("hidepw"), screen: f.flag("screen")}
	_ = a.st.UpdateSettings(func(st *domain.Settings) {
		pub := st.Share.Public
		st.Share = domain.ShareConfig{Minutes: opt.minutes, Max: opt.max, NoKnock: !opt.knock, ShowPw: !opt.hidePw, Screen: opt.screen, Way: way, Public: pub}
	})
	a.returnToTerminal = true
	return a.startShareWith(s, opt), nil
}

// askPublicRelay: Einstellungen › Vermittler › öffentlicher Vermittler fürs Teilen.
func (a *App) askPublicRelay() tea.Cmd {
	cur := a.st.Settings().Share.Public
	m := inputModal(i18n.T("Öffentlicher Vermittler fürs Teilen"),
		i18n.T("Adresse eines Vermittlers mit „--offen“ (https://host:8022#fp=…). Leer = keiner. Inhalte bleiben Ende-zu-Ende verschlüsselt."), cur,
		func(v string) tea.Cmd {
			v = strings.TrimSpace(v)
			if v != "" {
				if _, _, err := parsePublicRelay(v); err != nil {
					a.flashErr(err.Error())
					return nil
				}
			}
			if err := a.st.UpdateSettings(func(s *domain.Settings) { s.Share.Public = v }); err != nil {
				a.flashErr(err.Error())
				return nil
			}
			if v == "" {
				a.flash(i18n.T("Kein öffentlicher Vermittler"))
			} else {
				a.flash(i18n.T("Öffentlicher Vermittler gespeichert"))
			}
			return nil
		})
	a.modal = m
	return m.in.Focus()
}

// qrLines: QR-Code aus Halbblöcken (hell auf dunkel fest eingefärbt, damit er
// in jedem Theme scannbar bleibt). Leer, wenn der Text zu lang ist.
func qrLines(text string) []string {
	c, err := qr.Encode(text, qr.L)
	if err != nil || c.Size > 61 {
		return nil
	}
	const quiet = 2
	light := func(x, y int) bool {
		if x < 0 || y < 0 || x >= c.Size || y >= c.Size {
			return true
		}
		return !c.Black(x, y)
	}
	var out []string
	for y := -quiet; y < c.Size+quiet; y += 2 {
		var b strings.Builder
		b.WriteString("      \x1b[97;40m")
		for x := -quiet; x < c.Size+quiet; x++ {
			t, u := light(x, y), light(x, y+1)
			switch {
			case t && u:
				b.WriteString("█")
			case t:
				b.WriteString("▀")
			case u:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\x1b[0m")
		out = append(out, b.String())
	}
	return out
}
