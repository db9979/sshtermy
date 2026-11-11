package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/relay"
	"github.com/db9979/sshtermy/internal/sessionlog"
	"github.com/db9979/sshtermy/internal/termemu"
	"github.com/db9979/sshtermy/internal/version"
)

// Sitzung live teilen (Palette „teilen“, sharedialog.go für die Auswahl)
//
// Gastgeber: Die Ausgabe des Tabs geht Ende-zu-Ende verschlüsselt an alle,
// die der Gastgeber zugelassen hat: im lokalen Netz direkt (eingebauter
// Vermittler, relay/lan.go) oder über einen Vermittler. Zuschauer kommen mit
// einem Code („7-taube-kompass-ofen“) oder Link, klopfen an, und erst „j“
// lässt sie herein. Standard: nur zusehen, 30 Minuten, eine Person, ab
// jetzt (nichts von vorher), Passwortabfragen ausgeblendet.
//
// Zuschauer: Code oder Link in die Palette. Es öffnet sich ein Tab so groß wie
// das Terminal des Gastgebers; Tasten gehen nirgendwo hin, außer der Gastgeber
// hat Mitschreiben erlaubt.

type shareKind int

const (
	sfData   shareKind = iota // Ausgabe an alle
	sfDrop                    // Ausgabe während der Passwortabfrage: geht nirgendwohin
	sfSize                    // neue Größe
	sfAdmit                   // Zuschauer zulassen + Bildschirm schicken
	sfInfo                    // Info an alle
	sfPause                   // Passwortabfrage beginnt
	sfResume                  // vorbei: Bildschirm neu
	sfFull                    // Bildschirm neu (nach verworfener Ausgabe)
)

type shareFrame struct {
	kind shareKind
	data []byte
	v    uint32
	info relay.ShareInfo
}

// shareOut: laufende Freigabe eines Tabs.
type shareOut struct {
	hs     *relay.HostSession
	lan    *relay.LAN // nil = über einen Vermittler
	via    string     // „im lokalen Netz“ / Name des Vermittlers
	term   *termemu.Term
	shadow *termemu.Term // nur, was die Zuschauer sehen dürfen
	hidePw bool
	prompt string // Zeile der Passwortabfrage (nur Lese-Goroutine)
	knock  bool
	until  time.Time
	codeAt time.Time

	ch      chan shareFrame
	stop    chan struct{}
	lost    atomic.Bool // Ausgabe verworfen (Leitung zu langsam): Bildschirm neu schicken
	stopped atomic.Bool
	paused  atomic.Bool

	// nur UI-Thread
	viewers int
	names   map[uint32]string
}

func (o *shareOut) push(f shareFrame) bool {
	select {
	case <-o.stop:
		return false
	default:
	}
	select {
	case o.ch <- f:
		return true
	case <-o.stop:
	default:
		o.lost.Store(true)
	}
	return false
}

// feed: aus outWriter (Lese-Goroutine der Sitzung, nach term.Write), blockiert nie.
// Steht der Cursor hinter einer Passwortabfrage, geht ab da nichts mehr
// hinaus, bis der Cursor diese Zeile verlassen hat (auch kein Echo).
func (o *shareOut) feed(p []byte) {
	d := append([]byte(nil), p...)
	if !o.hidePw {
		o.push(shareFrame{kind: sfData, data: d})
		return
	}
	line := strings.TrimRight(o.term.CursorLine(), " ")
	if o.paused.Load() {
		if o.prompt != "" && strings.HasPrefix(line, o.prompt) {
			return // auch nicht in den Schattenbildschirm: was hier erscheint, sieht nie jemand
		}
		o.paused.Store(false)
		o.push(shareFrame{kind: sfResume})
	}
	o.push(shareFrame{kind: sfData, data: d})
	if agent.LooksLikeSecretPrompt(line) {
		o.prompt = line
		o.paused.Store(true)
		o.push(shareFrame{kind: sfPause})
	}
}

func (o *shareOut) end() {
	if o.stopped.CompareAndSwap(false, true) {
		close(o.stop)
	}
}

// guestNames: zugelassene Zuschauer (UI-Thread).
func (o *shareOut) guestNames() []string {
	var out []string
	for _, g := range o.hs.Guests() {
		if g.Admitted {
			out = append(out, g.Hello.Name)
		}
	}
	return out
}

type shareStartedMsg struct {
	sid  int
	sh   *relay.Share2
	lan  *relay.LAN
	via  string
	opts shareOpts
	err  error
}

type shareEventMsg struct {
	sid int
	o   *shareOut
	e   relay.HostEvent
}

type shareEndedMsg struct {
	sid int
	o   *shareOut
}

// shareOpts: Auswahl im Teilen-Dialog.
type shareOpts struct {
	way     string // "lan", "relay:<id>", "public"
	minutes int
	max     int
	knock   bool
	hidePw  bool
	screen  bool // aktuellen Bildschirm mitschicken
}

func (o shareOpts) withDefaults() shareOpts {
	if o.minutes <= 0 {
		o.minutes = 30
	}
	if o.max <= 0 {
		o.max = 1
	}
	return o
}

// shareRelays: Vermittler, über die geteilt werden kann (eigene; Spark erst,
// wenn das Panel Teilen v2 kann – der Versuch sagt es dann).
func (a *App) shareRelays() []domain.Relay { return a.st.Relays() }

// startShare: „teilen“ öffnet immer den Dialog (sharedialog.go).
func (a *App) startShare(s *session) tea.Cmd {
	if s == nil || s.state != stConnected || s.term == nil {
		return nil
	}
	if s.conn.Protocol == domain.ProtoWatch {
		a.flash(i18n.T("Eine geteilte Sitzung lässt sich nicht weiterteilen"))
		return nil
	}
	if o := s.share.Load(); o != nil {
		return a.openShareManage(s, o)
	}
	return a.openShareForm(s)
}

// startShareWith legt die Freigabe an (im Hintergrund).
func (a *App) startShareWith(s *session, opt shareOpts) tea.Cmd {
	opt = opt.withDefaults()
	sid := s.id
	var cl *relay.Client
	via := ""
	switch {
	case opt.way == "lan":
		via = i18n.T("im lokalen Netz")
	case opt.way == "public":
		base, fp, err := parsePublicRelay(a.st.Settings().Share.Public)
		if err != nil {
			a.flashErr(err.Error())
			return nil
		}
		if cl, err = relay.NewClient(base, "", fp); err != nil {
			a.flashErr(err.Error())
			return nil
		}
		via = strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(opt.way, "relay:"):
		c, r, err := a.relayClient(strings.TrimPrefix(opt.way, "relay:"))
		if err != nil {
			a.flashErr(err.Error())
			return nil
		}
		cl, via = c, r.Name
	default:
		return nil
	}
	a.flash(i18n.T("Freigabe wird angelegt …"))
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var lan *relay.LAN
		if opt.way == "lan" {
			l, err := relay.StartLAN()
			if err != nil {
				return shareStartedMsg{sid: sid, err: err}
			}
			lan, cl = l, l.Local
		}
		sh, err := cl.StartShare2(ctx, opt.minutes)
		if err != nil {
			if lan != nil {
				lan.Close()
			}
			var se *relay.StatusError
			if errors.As(err, &se) && (se.Code == 404 || se.Code == 405) {
				err = fmt.Errorf(i18n.T("„%s“ kann noch nicht mit Code teilen (Vermittler aktualisieren)"), via)
			}
			return shareStartedMsg{sid: sid, err: err}
		}
		if lan != nil {
			sh.Base, sh.FP = lan.Base, lan.FP
		}
		return shareStartedMsg{sid: sid, sh: sh, lan: lan, via: via, opts: opt}
	}
}

// parsePublicRelay: „https://host[:port][#fp=SHA256]“.
func parsePublicRelay(v string) (string, string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", "", errors.New(i18n.T("Kein öffentlicher Vermittler eingetragen (Einstellungen › Vermittler)"))
	}
	base, frag, _ := strings.Cut(v, "#")
	fp := strings.ToLower(strings.TrimPrefix(frag, "fp="))
	b, err := relay.NormBase(base)
	if err != nil {
		return "", "", err
	}
	if fp != "" && len(fp) != 64 {
		return "", "", errors.New(i18n.T("Fingerabdruck des öffentlichen Vermittlers ungültig"))
	}
	return b, fp, nil
}

// localName: so heißt man beim Gastgeber.
func localName() (name, host string) {
	host, _ = os.Hostname()
	if u, err := user.Current(); err == nil {
		name = u.Name
		if name == "" {
			name = u.Username
		}
		if i := strings.LastIndexAny(name, `\/`); i >= 0 {
			name = name[i+1:]
		}
	}
	if name == "" {
		name = host
	}
	return trunc(strings.TrimSpace(name), 40), trunc(host, 60)
}

func (a *App) shareMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case shareStartedMsg:
		s := a.byID(m.sid)
		if m.err != nil {
			a.flashErr(i18n.T("Teilen: ") + m.err.Error())
			return nil, true
		}
		if s == nil || s.state != stConnected || s.term == nil {
			m.sh.Close()
			if m.lan != nil {
				m.lan.Close()
			}
			return nil, true
		}
		c, r := s.term.Size()
		o := &shareOut{hs: relay.NewHostSession(m.sh, m.opts.max), lan: m.lan, via: m.via, term: s.term,
			shadow: termemu.New(c, r, io.Discard), hidePw: m.opts.hidePw, knock: m.opts.knock,
			until: time.Now().Add(time.Duration(m.opts.minutes) * time.Minute), codeAt: time.Now(),
			ch: make(chan shareFrame, 512), stop: make(chan struct{}), names: map[uint32]string{}}
		if !m.sh.Until.IsZero() && m.sh.Until.Before(o.until) {
			o.until = m.sh.Until
		}
		if m.opts.screen {
			_, _ = o.shadow.Write([]byte(replayScreen(s.term)))
		}
		if m.lan != nil {
			m.lan.Announce(m.sh.Platz, true)
		}
		s.share.Store(o)
		go o.pump(s.id, a.events)
		a.shareLog(s, fmt.Sprintf(i18n.T("# Teilen gestartet (%s, %d min, höchstens %d Zuschauer)"), o.via, m.opts.minutes, m.opts.max))
		a.showInvite(s, o)
		return nil, true
	case shareEventMsg:
		s := a.byID(m.sid)
		if s == nil || s.share.Load() != m.o {
			return nil, true
		}
		return a.shareEvent(s, m.o, m.e), true
	case shareEndedMsg:
		s := a.byID(m.sid)
		if s != nil && s.share.CompareAndSwap(m.o, nil) {
			a.flash(s.tabName() + i18n.T(": Teilen beendet (Verbindung getrennt)"))
			a.shareLog(s, i18n.T("# Teilen beendet"))
		}
		return nil, true
	case watchSizeMsg:
		s := a.byID(m.sid)
		if s != nil {
			s.fixedCols, s.fixedRows = m.cols, m.rows
			a.resize(s)
		}
		return nil, true
	case watchCtlMsg:
		if s := a.byID(m.sid); s != nil {
			if m.write.IsZero() {
				a.pushToast(toastInfo, s.tabName()+i18n.T(": Mitschreiben beendet – nur zusehen"))
			} else {
				a.pushToast(toastWarn, fmt.Sprintf(i18n.T("%s: Du darfst mitschreiben bis %s – Tasten gehen jetzt an den Server"), s.tabName(), m.write.Format("15:04")))
			}
		}
		return nil, true
	}
	return nil, false
}

// shareLog: Eintrag im Sitzungsverlauf (wer wann zusah).
func (a *App) shareLog(s *session, line string) {
	if s.log != nil {
		s.log.Add(sessionlog.Entry{At: time.Now(), Command: line})
	}
}

func (a *App) shareEvent(s *session, o *shareOut, e relay.HostEvent) tea.Cmd {
	u, ok := o.hs.Handle(e)
	if !ok {
		return nil
	}
	tab := s.tabName()
	switch u.Kind {
	case "zuschauer":
		o.viewers = u.N
	case "klopft":
		g := u.G
		if !o.knock {
			a.shareAdmit(s, o, g)
			return nil
		}
		body := fmt.Sprintf(i18n.T("Rechner:  %s · sshTermy %s\nVon:      %s\n\nPrüfwörter – frag %s, ob bei ihr/ihm dieselben stehen:\n\n      %s\n\nPassen sie nicht, sitzt jemand dazwischen: ablehnen.\n\nj = zulassen (nur zusehen)   n = ablehnen"),
			g.Hello.Rechner, g.Hello.Version, g.IP, g.Hello.Name, strings.ToUpper(strings.Join(g.Check[:], " · ")))
		m := confirmModal(fmt.Sprintf(i18n.T("▸ %s möchte zusehen · %s"), g.Hello.Name, tab), body, func() tea.Cmd {
			if s.share.Load() == o {
				a.shareAdmit(s, o, g)
			}
			return nil
		})
		m.onCancel = func() tea.Cmd {
			o.hs.Deny(g.V)
			a.shareLog(s, fmt.Sprintf(i18n.T("# Zuschauer abgelehnt: %s (%s, %s)"), g.Hello.Name, g.Hello.Rechner, g.IP))
			return nil
		}
		a.showAsync(m)
	case "falsch":
		a.pushToast(toastErr, tab+i18n.T(": Jemand hat einen falschen Code versucht – Einladung gesperrt. Neuer Code: Palette › Teilen verwalten"))
		a.shareLog(s, fmt.Sprintf(i18n.T("# Falscher Code-Versuch von %s – Einladung gesperrt"), u.G.IP))
	case "weg":
		if _, ok := o.names[u.G.V]; ok {
			delete(o.names, u.G.V)
			a.pushToast(toastInfo, fmt.Sprintf(i18n.T("%s: %s sieht nicht mehr zu"), tab, u.G.Hello.Name))
			a.shareLog(s, fmt.Sprintf(i18n.T("# Zuschauer gegangen: %s"), u.G.Hello.Name))
		}
	case "hand":
		a.pushToast(toastWarn, fmt.Sprintf(i18n.T("%s: %s hebt die Hand (möchte mitschreiben) – Palette › Teilen verwalten"), tab, u.G.Hello.Name))
	case "tasten":
		return a.shareGuestKeys(s, o, u.G, u.Data)
	}
	return nil
}

func (a *App) shareInfo(s *session, o *shareOut) relay.ShareInfo {
	n, _ := localName()
	return relay.ShareInfo{Tab: s.tabName(), Von: n, Bis: o.until.Unix(), Pause: o.paused.Load()}
}

func (a *App) shareAdmit(s *session, o *shareOut, g *relay.GuestState) {
	o.names[g.V] = g.Hello.Name
	o.push(shareFrame{kind: sfAdmit, v: g.V, info: a.shareInfo(s, o)})
	a.pushToast(toastInfo, fmt.Sprintf(i18n.T("%s: %s sieht jetzt zu (nur zusehen)"), s.tabName(), g.Hello.Name))
	a.shareLog(s, fmt.Sprintf(i18n.T("# Zuschauer zugelassen: %s (%s, %s)"), g.Hello.Name, g.Hello.Rechner, g.IP))
}

// shareGuestKeys: ein Zuschauer mit Mitschreib-Recht tippt. ⏎ in einer
// produktiven Verbindung (Standard) braucht die Bestätigung des Gastgebers.
func (a *App) shareGuestKeys(s *session, o *shareOut, g *relay.GuestState, data []byte) tea.Cmd {
	if s.shell == nil {
		return nil
	}
	i := strings.IndexAny(string(data), "\r\n")
	if i < 0 || !isProduction(s.conn) {
		_, _ = s.shell.Write(data)
		if i >= 0 {
			a.shareLog(s, fmt.Sprintf(i18n.T("# ⏎ von %s (mitschreiben)"), g.Hello.Name))
		}
		return nil
	}
	if i > 0 {
		_, _ = s.shell.Write(data[:i])
	}
	line := strings.TrimSpace(s.term.CursorLine())
	name := g.Hello.Name
	m := confirmModal(fmt.Sprintf(i18n.T("⏎ von %s ausführen? · %s"), name, s.tabName()),
		fmt.Sprintf(i18n.T("%s darf mitschreiben und möchte ⏎ drücken. %s gilt als produktiv.\n\nZeile: %s\n\nj = ausführen   n = verwerfen (Zeile bleibt stehen)"), name, s.conn.Alias, trunc(line, 200)),
		func() tea.Cmd {
			if s.shell != nil {
				_, _ = s.shell.Write([]byte{'\r'})
				a.shareLog(s, fmt.Sprintf(i18n.T("# ⏎ von %s (mitschreiben, bestätigt)"), name))
			}
			return nil
		})
	a.showAsync(m)
	return nil
}

// pump schickt die Rahmen der Reihe nach und meldet, was der Vermittler sagt.
func (o *shareOut) pump(sid int, ev chan<- tea.Msg) {
	defer func() {
		o.end()
		_ = o.hs.Frame(0, relay.FrameEnd, nil)
		o.hs.Sh.Close()
		if o.lan != nil {
			o.lan.Close()
		}
		o.shadow.Close()
		ev <- shareEndedMsg{sid, o}
	}()
	for {
		select {
		case f := <-o.ch:
			o.do(f)
		case e, ok := <-o.hs.Sh.Events():
			if !ok {
				return
			}
			ev <- shareEventMsg{sid, o, e}
		case <-o.hs.Sh.Done():
			return
		case <-o.stop:
			return
		}
	}
}

func (o *shareOut) screen(v uint32) {
	c, r := o.shadow.Size()
	_ = o.hs.Frame(v, relay.FrameSize, []byte(fmt.Sprintf("%d %d", c, r)))
	_ = o.hs.Frame(v, relay.FrameData, []byte(replayScreen(o.shadow)))
}

func (o *shareOut) data(v uint32, p []byte) {
	const chunk = 256 << 10
	for len(p) > chunk {
		_ = o.hs.Frame(v, relay.FrameData, p[:chunk])
		p = p[chunk:]
	}
	_ = o.hs.Frame(v, relay.FrameData, p)
}

func (o *shareOut) do(f shareFrame) {
	switch f.kind {
	case sfData:
		_, _ = o.shadow.Write(f.data)
		o.data(0, f.data)
	case sfSize:
		var c, r int
		if _, err := fmt.Sscanf(string(f.data), "%d %d", &c, &r); err == nil {
			o.shadow.Resize(c, r)
		}
		o.screen(0)
	case sfAdmit:
		if o.hs.Admit(f.v, f.info) == nil {
			o.screen(f.v)
			if o.paused.Load() {
				_ = o.hs.Frame(f.v, relay.FramePause, nil)
			}
		}
	case sfInfo:
		b, _ := json.Marshal(f.info)
		_ = o.hs.Frame(0, relay.FrameInfo, b)
	case sfPause:
		_ = o.hs.Frame(0, relay.FramePause, nil)
	case sfResume:
		_ = o.hs.Frame(0, relay.FrameResume, nil)
		o.screen(0)
	case sfFull:
		o.screen(0)
	}
}

// shareTick (jede Sekunde): Bildschirm nach Verlust, Ablauf, Einladung.
func (a *App) shareTick(s *session) {
	o := s.share.Load()
	if o == nil {
		return
	}
	if o.lost.CompareAndSwap(true, false) {
		o.push(shareFrame{kind: sfFull})
	}
	now := time.Now()
	if now.After(o.until) {
		a.stopShareWhy(s, i18n.T(": Teilen beendet (Zeit abgelaufen)"))
		return
	}
	if !o.hs.Locked() && now.Sub(o.codeAt) > relay.CodeTTL {
		o.hs.LockCode()
		a.flash(s.tabName() + i18n.T(": Einladung abgelaufen (neuer Code: Palette › Teilen verwalten)"))
	}
	if o.lan != nil {
		o.lan.Announce(o.hs.Sh.Platz, !o.hs.Locked())
	}
}

// shareResized: neue Größe an die Zuschauer.
func (a *App) shareResized(s *session) {
	if o := s.share.Load(); o != nil && s.term != nil {
		c, r := s.term.Size()
		o.push(shareFrame{kind: sfSize, data: []byte(fmt.Sprintf("%d %d", c, r))})
	}
}

func (a *App) stopShare(s *session) { a.stopShareWhy(s, i18n.T(": Teilen beendet")) }

func (a *App) stopShareWhy(s *session, why string) {
	if o := s.share.Swap(nil); o != nil {
		o.end()
		a.flash(s.tabName() + why)
		a.shareLog(s, "#"+why)
	}
}

// shareChip: „● LIVE · Anna · 27 min“ für die Sitzungsleiste.
func (a *App) shareChip(s *session) string {
	o := s.share.Load()
	if o == nil {
		return ""
	}
	names := o.guestNames()
	who := i18n.T("wartet auf Zuschauer")
	if len(names) > 0 {
		who = "◉ " + strings.Join(names, ", ")
	}
	left := time.Until(o.until).Round(time.Minute)
	txt := fmt.Sprintf(i18n.T("● LIVE · %s · noch %d min"), who, int(left.Minutes()))
	if o.paused.Load() {
		txt += i18n.T(" · ▪ Passwort verborgen")
	}
	return a.s.ChipDanger.Bold(true).Render(txt)
}

// ---------------------------------------------------------------- Einladung

func (a *App) showInvite(s *session, o *shareOut) {
	inv := o.hs.Invite()
	link := inv.Link()
	go func() { _ = clipboard.WriteAll(link) }()
	st := a.s
	code := strings.ReplaceAll(inv.Code(), "-", " – ")
	lines := []string{
		st.Text.Bold(true).Render(i18n.T("Code zum Vorlesen oder Abtippen")),
		"",
		"      " + st.Chip.Background(st.T.Accent).Foreground(st.T.Bg).Bold(true).Render("  "+code+"  "),
		"",
		st.Muted.Render(i18n.T("Zuschauer: Strg+Leertaste › Code eintippen (oder Link einfügen).")),
	}
	if o.lan != nil {
		lines = append(lines, st.Muted.Render(fmt.Sprintf(i18n.T("Im selben Netz findet sshTermy dich über den Code (%s)."), strings.TrimPrefix(o.lan.Base, "https://"))))
		if !o.lan.Findable() {
			lines = append(lines, st.Warn.Render(i18n.T("▲ Suchen im Netz geht gerade nicht (Port belegt): bitte den Link schicken.")))
		}
	} else {
		lines = append(lines, st.Muted.Render(fmt.Sprintf(i18n.T("Über den Vermittler „%s“: der Code reicht, wenn der Zuschauer ihn kennt, sonst den Link schicken."), o.via)))
	}
	lines = append(lines, "",
		st.Text.Bold(true).Render(i18n.T("Oder Link"))+st.Muted.Render(i18n.T(" (in der Zwischenablage, c kopiert erneut)")),
		st.Accent.Render(link), "")
	if q := qrLines(link); len(q) > 0 {
		lines = append(lines, q...)
		lines = append(lines, st.Muted.Render(i18n.T("QR-Code: enthält den Link (zum Abfotografieren oder Scannen)")), "")
	}
	lines = append(lines,
		st.Muted.Render(i18n.T("Der Code ist kein Schlüssel: beide Seiten leiten den Schlüssel daraus ab (PAKE).")),
		st.Muted.Render(i18n.T("Wer falsch rät, hat einen Versuch – danach ist die Einladung gesperrt.")),
		"",
		st.Text.Bold(true).Render(i18n.T("Gilt  "))+st.OK.Render(i18n.T("einmal"))+fmt.Sprintf(i18n.T(" · %d Minuten zum Einlösen"), int(relay.CodeTTL.Minutes())),
		st.Text.Bold(true).Render(i18n.T("Dann  "))+a.shareSummary(o),
		"",
		st.Muted.Render(i18n.T("Verwalten, Mitschreiben erlauben, beenden: Palette › „Teilen verwalten“")),
	)
	m := textModal(i18n.T("Einladung · ")+s.tabName(), lines)
	m.styled, m.copy = true, link
	a.modal = m
}

func (a *App) shareSummary(o *shareOut) string {
	parts := []string{i18n.T("nur zusehen"), fmt.Sprintf(i18n.T("bis %s"), o.until.Format("15:04")), fmt.Sprintf(i18n.T("höchstens %d"), o.hs.Max)}
	if o.knock {
		parts = append(parts, i18n.T("anklopfen"))
	}
	if o.hidePw {
		parts = append(parts, i18n.T("Passwörter verborgen"))
	}
	return strings.Join(parts, " · ")
}

// ---------------------------------------------------------------- Verwalten

func (a *App) openShareManage(s *session, o *shareOut) tea.Cmd {
	items := []menuItem{
		{label: i18n.T("Einladung anzeigen (Code, Link, QR)"), hint: "⏎", run: func() tea.Cmd { a.showInvite(s, o); return nil }},
		{label: i18n.T("Neuer Code (der alte gilt nicht mehr)"), hint: i18n.T("10 min"), run: func() tea.Cmd {
			o.hs.NewCode()
			o.codeAt = time.Now()
			a.showInvite(s, o)
			return nil
		}},
	}
	if !o.hs.Locked() {
		items = append(items, menuItem{label: i18n.T("Einladung sperren (keine neuen Zuschauer)"), run: func() tea.Cmd {
			o.hs.LockCode()
			a.flash(i18n.T("Einladung gesperrt"))
			return nil
		}})
	}
	for _, g := range o.hs.Guests() {
		if !g.Admitted {
			continue
		}
		g := g
		if time.Now().Before(g.WriteBis) {
			items = append(items, menuItem{label: i18n.T("Mitschreiben entziehen: ") + g.Hello.Name, hint: g.WriteBis.Format("15:04"), run: func() tea.Cmd {
				_ = o.hs.SetWrite(g.V, time.Time{})
				a.shareLog(s, i18n.T("# Mitschreiben entzogen: ")+g.Hello.Name)
				return nil
			}})
		} else {
			items = append(items, menuItem{label: i18n.T("Mitschreiben erlauben (10 min): ") + g.Hello.Name, hint: "…", run: func() tea.Cmd {
				body := fmt.Sprintf(i18n.T("%s kann dann in %s tippen. Jedes ⏎ musst du bestätigen, solange die Verbindung als produktiv gilt.\n\nj = erlauben (10 Minuten)   n = abbrechen"), g.Hello.Name, s.conn.Alias)
				a.modal = confirmModal(i18n.T("Mitschreiben erlauben?"), body, func() tea.Cmd {
					_ = o.hs.SetWrite(g.V, time.Now().Add(10*time.Minute))
					a.shareLog(s, i18n.T("# Mitschreiben erlaubt (10 min): ")+g.Hello.Name)
					return nil
				})
				return nil
			}})
		}
		items = append(items, menuItem{label: i18n.T("Trennen: ") + g.Hello.Name, hint: g.Hello.Rechner, run: func() tea.Cmd {
			_ = o.hs.Sh.Kick(g.V)
			return nil
		}})
	}
	items = append(items, menuItem{label: i18n.T("Teilen beenden"), hint: i18n.T("alle trennen"), run: func() tea.Cmd { a.stopShare(s); return nil }})
	title := fmt.Sprintf(i18n.T("Teilen · %s · %s · noch %d min"), s.tabName(), o.via, int(time.Until(o.until).Minutes()))
	a.modal = &modal{kind: mMenu, title: title, items: items}
	return nil
}

// ---------------------------------------------------------------- Zuschauer

type watchSizeMsg struct {
	sid        int
	cols, rows int
}

// watchStream: Tab eines Zuschauers. Tasten werden verworfen.
type watchStream struct{ v *relay.ShareView }

func (w *watchStream) Write(p []byte) (int, error) { return len(p), nil }
func (w *watchStream) Resize(int, int) error       { return nil }
func (w *watchStream) Close() error                { return w.v.Close() }

// dialWatchV1: alter Link (https://…#teilen=…) von sshTermy vor Teilen v2.
func dialWatchV1(sid int, link string, out io.Writer, ev chan<- tea.Msg) (termStream, <-chan error, error) {
	l, err := relay.ParseShareLink(link)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	v, err := relay.WatchShare(ctx, l)
	if err != nil {
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() {
		for {
			typ, p, err := v.Next(context.Background())
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				done <- err
				return
			}
			switch typ {
			case relay.FrameData:
				_, _ = out.Write(p)
			case relay.FrameSize:
				var c, r int
				if _, err := fmt.Sscanf(string(p), "%d %d", &c, &r); err == nil && c >= 10 && r >= 3 && c <= 1000 && r <= 500 {
					ev <- watchSizeMsg{sid, c, r}
				}
			}
		}
	}()
	return &watchStream{v}, done, nil
}

// watchConn: Verbindung für einen Link zum Zusehen (nicht gespeichert).
func watchConn(link string) domain.Connection {
	return domain.Connection{Alias: i18n.T("Zusehen"), Protocol: domain.ProtoWatch, Device: domain.DeviceServer, WatchLink: strings.TrimSpace(link)}
}

func (a *App) askWatch() tea.Cmd {
	m := inputModal(i18n.T("Geteilte Sitzung ansehen"), i18n.T("Code (z. B. 7-taube-kompass-ofen) oder Link einfügen"), "",
		func(v string) tea.Cmd { return a.watchLink(v) })
	a.modal = m
	return m.in.Focus()
}

func (a *App) watchLink(v string) tea.Cmd {
	if relay.IsShareLink(v) {
		if _, err := relay.ParseShareLink(v); err != nil {
			a.flashErr(err.Error())
			return nil
		}
		return a.connect(watchConn(v))
	}
	if _, err := relay.ParseInvite(v); err != nil {
		a.flashErr(err.Error())
		return nil
	}
	return a.connect(watchConn(v))
}

// cropView: Bildschirm des Gastgebers ist größer als der Platz hier.
func cropView(body string, cols, rows int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for i, l := range lines {
		if ansi.StringWidth(l) > cols {
			lines[i] = ansi.Truncate(l, cols, "")
		}
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- Zuschauer v2

type watchCtlMsg struct {
	sid   int
	write time.Time
}

// watch2Stream: Tab eines Zuschauers mit Code. Tasten gehen nur weiter,
// solange der Gastgeber Mitschreiben erlaubt hat.
type watch2Stream struct {
	w       atomic.Pointer[relay.Watch2]
	writeTo atomic.Int64 // Unix-Zeit, bis wann mitschreiben erlaubt ist
	info    atomic.Pointer[relay.ShareInfo]
	check   atomic.Pointer[[3]string]
	paused  atomic.Bool
	cancel  context.CancelFunc
}

func (w *watch2Stream) Write(p []byte) (int, error) {
	if x := w.w.Load(); x != nil && time.Now().Unix() < w.writeTo.Load() {
		_ = x.SendCtl(relay.Ctl{T: "tasten", Data: append([]byte(nil), p...)})
	}
	return len(p), nil
}
func (w *watch2Stream) Resize(int, int) error { return nil }
func (w *watch2Stream) Close() error {
	w.cancel()
	if x := w.w.Load(); x != nil {
		return x.Close()
	}
	return nil
}

// hand: um Mitschreiben bitten.
func (w *watch2Stream) hand() error {
	x := w.w.Load()
	if x == nil {
		return errors.New(i18n.T("noch nicht zugelassen"))
	}
	return x.SendCtl(relay.Ctl{T: "hand"})
}

// shareKnown: wo ein Code ohne Ort gesucht wird (nach dem lokalen Netz).
func (a *App) shareKnown() []relay.Found {
	var out []relay.Found
	for _, r := range a.st.Relays() {
		out = append(out, relay.Found{Base: r.Base, FP: r.FP})
	}
	if b, fp, err := parsePublicRelay(a.st.Settings().Share.Public); err == nil {
		out = append(out, relay.Found{Base: b, FP: fp})
	}
	return out
}

// dialWatch verbindet einen Zuschauer-Tab (aus connectRawCmd): alter Link
// oder Code/Link v2. Bei v2 steht der Tab sofort; Prüfwörter und Warten auf
// Zulassung erscheinen als Text darin.
func (a *App) dialWatch(sid int, link string, out io.Writer, ev chan<- tea.Msg) (termStream, <-chan error, error) {
	if relay.IsShareLink(link) {
		return dialWatchV1(sid, link, out, ev)
	}
	inv, err := relay.ParseInvite(link)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	g, base, err := relay.Locate(ctx, inv, a.shareKnown())
	cancel()
	if err != nil {
		return nil, nil, err
	}
	st := &watch2Stream{}
	jctx, jcancel := context.WithCancel(context.Background())
	st.cancel = jcancel
	say := func(s string) { _, _ = out.Write([]byte(strings.ReplaceAll(s, "\n", "\r\n"))) }
	say(fmt.Sprintf("\x1b[2m%s\x1b[0m\n", fmt.Sprintf(i18n.T("sshTermy: Freigabe %d gefunden (%s). Prüfe den Code …"), inv.Platz, strings.TrimPrefix(base, "https://"))))
	done := make(chan error, 1)
	go func() {
		name, host := localName()
		wctx, wcancel := context.WithTimeout(jctx, 4*time.Minute)
		w, err := relay.Join(wctx, g, inv, relay.Hello{Name: name, Rechner: host, Version: version.Version}, func(chk [3]string) {
			st.check.Store(&chk)
			say("\x1b[32m" + i18n.T("✓ Code stimmt.") + "\x1b[0m\n\n")
			say(i18n.T("Prüfwörter – vergleiche sie mit dem Gastgeber:") + "\n\n")
			say("      \x1b[1;35m" + strings.ToUpper(strings.Join(chk[:], " · ")) + "\x1b[0m\n\n")
			say("\x1b[2m" + i18n.T("Stimmen sie nicht, sitzt jemand dazwischen: Tab schließen.") + "\x1b[0m\n")
			say(i18n.T("Warte, bis der Gastgeber dich zulässt …") + "\n")
		})
		wcancel()
		if err != nil {
			say("\n\x1b[31m× " + err.Error() + "\x1b[0m\n")
			done <- err
			return
		}
		st.w.Store(w)
		info := w.Info
		st.info.Store(&info)
		say("\x1b[0m\x1b[2J\x1b[H")
		for {
			typ, p, err := w.Next(jctx)
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				done <- err
				return
			}
			switch typ {
			case relay.FrameData:
				_, _ = out.Write(p)
			case relay.FrameSize:
				var c, r int
				if _, err := fmt.Sscanf(string(p), "%d %d", &c, &r); err == nil && c >= 10 && r >= 3 && c <= 1000 && r <= 500 {
					ev <- watchSizeMsg{sid, c, r}
				}
			case relay.FramePause:
				st.paused.Store(true)
				who := w.Info.Von
				if who == "" {
					who = i18n.T("Der Gastgeber")
				}
				say("\n\x1b[2m··· " + fmt.Sprintf(i18n.T("%s gibt gerade ein Passwort ein – ausgeblendet"), who) + " ···\x1b[0m")
			case relay.FrameResume:
				st.paused.Store(false)
			case relay.FrameInfo:
				info := w.Info
				st.info.Store(&info)
			case 'c':
				var c relay.Ctl
				if json.Unmarshal(p, &c) == nil && c.T == "schreiben" {
					var until time.Time
					if c.Bis > 0 {
						until = time.Unix(c.Bis, 0)
					}
					st.writeTo.Store(c.Bis)
					ev <- watchCtlMsg{sid, until}
				}
			}
		}
	}()
	return st, done, nil
}

// watchBadge: „◉ nur Zusehen · web02 bei Dominik · noch 27 min“.
func (a *App) watchBadge(s *session) string {
	st := a.s
	w, ok := s.shell.(*watch2Stream)
	if !ok {
		return st.Badge.Foreground(st.T.AI).Render("◉ " + i18n.T("nur Zusehen"))
	}
	txt := "◉ " + i18n.T("nur Zusehen")
	if bis := w.writeTo.Load(); time.Now().Unix() < bis {
		txt = "» " + fmt.Sprintf(i18n.T("darf mitschreiben bis %s"), time.Unix(bis, 0).Format("15:04"))
	}
	if inf := w.info.Load(); inf != nil {
		if inf.Tab != "" {
			txt += " · " + fmt.Sprintf(i18n.T("%s bei %s"), inf.Tab, inf.Von)
		}
		if inf.Bis > 0 {
			txt += fmt.Sprintf(i18n.T(" · noch %d min"), int(time.Until(time.Unix(inf.Bis, 0)).Minutes()))
		}
	} else if w.w.Load() == nil {
		txt += " · " + i18n.T("wartet auf Zulassung")
	}
	if c := w.check.Load(); c != nil {
		txt += " · " + strings.Join(c[:], "·")
	}
	return st.Badge.Foreground(st.T.AI).Render(txt)
}

// watchHand: Palette „Hand heben“ im Zuschauer-Tab.
func (a *App) watchHand(s *session) tea.Cmd {
	w, ok := s.shell.(*watch2Stream)
	if !ok {
		return nil
	}
	if err := w.hand(); err != nil {
		a.flashErr(err.Error())
	} else {
		a.flash(i18n.T("Hand gehoben – der Gastgeber sieht die Bitte ums Mitschreiben"))
	}
	return nil
}
