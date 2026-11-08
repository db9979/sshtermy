package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/relay"
)

// Sitzung live teilen (Palette „teilen“)
//
// Gastgeber: Die Ausgabe des Tabs geht verschlüsselt über den eigenen
// Vermittler an alle, die den Link haben. Kommt ein Zuschauer dazu, schickt
// sshTermy erst den aktuellen Bildschirm. Zuschauer sehen nur zu.
//
// Zuschauer: Link in die Palette einfügen. Es öffnet sich ein Tab, der
// genau so groß ist wie das Terminal des Gastgebers; Tasten gehen nirgendwo
// hin.

type shareFrame struct {
	typ  byte
	data []byte
}

// shareOut: laufende Freigabe eines Tabs.
type shareOut struct {
	h       *relay.ShareHost
	ch      chan shareFrame
	stop    chan struct{}
	link    string
	relay   string
	viewers int
	lost    atomic.Bool // Ausgabe verworfen (Leitung zu langsam): Bildschirm neu schicken
	stopped atomic.Bool
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

// feed: aus outWriter (Lese-Goroutine der Sitzung), blockiert nie.
func (o *shareOut) feed(p []byte) { o.push(shareFrame{relay.FrameData, append([]byte(nil), p...)}) }

func (o *shareOut) end() {
	if o.stopped.CompareAndSwap(false, true) {
		close(o.stop)
	}
}

type shareStartedMsg struct {
	sid   int
	h     *relay.ShareHost
	relay string
	err   error
}

type shareViewersMsg struct {
	sid int
	o   *shareOut
	n   int
}

type shareEndedMsg struct {
	sid int
	o   *shareOut
}

// shareRelays: eigene Vermittler (nur „sshtermy vermittler“ kann teilen).
func (a *App) shareRelays() []domain.Relay {
	var out []domain.Relay
	for _, r := range a.st.Relays() {
		if r.Kind == "" || r.Kind == relay.KindSSHTermy {
			out = append(out, r)
		}
	}
	return out
}

func (a *App) startShare(s *session) tea.Cmd {
	if s == nil || s.state != stConnected || s.term == nil {
		return nil
	}
	if s.conn.Protocol == domain.ProtoWatch {
		a.flash(i18n.T("Eine geteilte Sitzung lässt sich nicht weiterteilen"))
		return nil
	}
	if o := s.share.Load(); o != nil {
		a.showShareLink(s, o)
		return nil
	}
	rs := a.shareRelays()
	if len(rs) == 0 {
		a.flashErr(i18n.T("Teilen geht über einen eigenen Vermittler: erst „sshtermy vermittler“ einrichten und koppeln (Palette → „Vermittler koppeln“)"))
		return nil
	}
	if len(rs) == 1 {
		return a.startShareVia(s, rs[0].ID)
	}
	var items []pickItem
	for _, r := range rs {
		items = append(items, pickItem{value: r.ID, label: r.Name, detail: r.Base})
	}
	return a.openPicker(newPicker(i18n.T("Über welchen Vermittler teilen?"), i18n.T("Vermittler"), items, "", func(it pickItem) tea.Cmd {
		return a.startShareVia(s, it.value)
	}))
}

func (a *App) startShareVia(s *session, relayID string) tea.Cmd {
	cl, r, err := a.relayClient(relayID)
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.flash(i18n.T("Freigabe wird angelegt …"))
	sid, name := s.id, r.Name
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		h, err := cl.StartShare(ctx)
		return shareStartedMsg{sid: sid, h: h, relay: name, err: err}
	}
}

func (a *App) shareMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case shareStartedMsg:
		s := a.byID(m.sid)
		if m.err != nil {
			a.flashErr(i18n.T("Teilen: ") + m.err.Error())
			return nil, true
		}
		if s == nil || s.state != stConnected {
			m.h.Close()
			return nil, true
		}
		o := &shareOut{h: m.h, ch: make(chan shareFrame, 512), stop: make(chan struct{}), link: m.h.Link.String(), relay: m.relay}
		s.share.Store(o)
		a.shareScreen(s, o)
		go o.pump(s.id, a.events)
		go func() { _ = clipboard.WriteAll(o.link) }()
		a.showShareLink(s, o)
		return nil, true
	case shareViewersMsg:
		s := a.byID(m.sid)
		if s == nil || s.share.Load() != m.o {
			return nil, true
		}
		if m.n > m.o.viewers {
			a.shareScreen(s, m.o) // Neuer sieht sofort den Bildschirm
			a.pushToast(toastInfo, fmt.Sprintf(i18n.T("%s: Zuschauer dazugekommen (%d)"), s.tabName(), m.n))
		} else if m.n < m.o.viewers {
			a.flash(fmt.Sprintf(i18n.T("%s: %d Zuschauer"), s.tabName(), m.n))
		}
		m.o.viewers = m.n
		return nil, true
	case shareEndedMsg:
		s := a.byID(m.sid)
		if s != nil && s.share.CompareAndSwap(m.o, nil) {
			a.flash(s.tabName() + i18n.T(": Teilen beendet (Vermittler getrennt)"))
		}
		return nil, true
	case watchSizeMsg:
		s := a.byID(m.sid)
		if s != nil {
			s.fixedCols, s.fixedRows = m.cols, m.rows
			a.resize(s)
		}
		return nil, true
	}
	return nil, false
}

// pump schickt die Rahmen der Reihe nach und meldet Zuschauer.
func (o *shareOut) pump(sid int, ev chan<- tea.Msg) {
	defer func() {
		o.end()
		o.h.Close()
		ev <- shareEndedMsg{sid, o}
	}()
	for {
		select {
		case f := <-o.ch:
			if err := o.h.Send(f.typ, f.data); err != nil {
				return
			}
		case n := <-o.h.Viewers():
			ev <- shareViewersMsg{sid, o, n}
		case <-o.h.Done():
			return
		case <-o.stop:
			return
		}
	}
}

// shareScreen: Größe und aktueller Bildschirm an alle.
func (a *App) shareScreen(s *session, o *shareOut) {
	if s.term == nil {
		return
	}
	c, r := s.term.Size()
	o.lost.Store(false)
	o.push(shareFrame{relay.FrameSize, []byte(fmt.Sprintf("%d %d", c, r))})
	o.push(shareFrame{relay.FrameData, []byte(replayScreen(s.term))})
}

// shareTick (jede Sekunde): nach verworfener Ausgabe den Bildschirm neu schicken.
func (a *App) shareTick(s *session) {
	if o := s.share.Load(); o != nil && o.lost.Load() {
		a.shareScreen(s, o)
	}
}

// shareResized: neue Größe an die Zuschauer.
func (a *App) shareResized(s *session) {
	if o := s.share.Load(); o != nil {
		a.shareScreen(s, o)
	}
}

func (a *App) stopShare(s *session) {
	if o := s.share.Swap(nil); o != nil {
		o.end()
		a.flash(s.tabName() + i18n.T(": Teilen beendet"))
	}
}

func (a *App) showShareLink(s *session, o *shareOut) {
	lines := []string{
		i18n.T("Link zum Zusehen (in der Zwischenablage):"),
		"",
		o.link,
		"",
		i18n.T("Wer den Link hat, sieht diesen Tab live in sshTermy (Link in die Palette einfügen)."),
		i18n.T("Zuschauer können nichts tippen. Der Vermittler sieht nur verschlüsselte Daten, der Schlüssel steht allein im Link."),
		fmt.Sprintf(i18n.T("Über: %s · Zuschauer gerade: %d"), o.relay, o.viewers),
		"",
		i18n.T("Beenden: Palette → „Teilen beenden“ oder Tab schließen."),
	}
	a.modal = textModal(i18n.T("Sitzung live teilen: ")+s.tabName(), lines)
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

// dialWatch verbindet einen Zuschauer-Tab (aus connectRawCmd).
func dialWatch(sid int, link string, out io.Writer, ev chan<- tea.Msg) (termStream, <-chan error, error) {
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
	m := inputModal(i18n.T("Geteilte Sitzung ansehen"), i18n.T("Link zum Zusehen einfügen (https://…#teilen=…)"), "",
		func(v string) tea.Cmd { return a.watchLink(v) })
	a.modal = m
	return m.in.Focus()
}

func (a *App) watchLink(v string) tea.Cmd {
	if _, err := relay.ParseShareLink(v); err != nil {
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
