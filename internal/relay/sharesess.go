package relay

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Gastgeber-Seite einer Freigabe v2 (Ablauf: siehe internal/sharewire/guest.go).

// ---------------------------------------------------------------- Gastgeber

// GuestState: ein Zuschauer aus Sicht des Gastgebers.
type GuestState struct {
	V        uint32
	IP       string
	Hello    Hello
	Check    [3]string
	Since    time.Time
	Admitted bool
	WriteBis time.Time // darf mitschreiben bis (Null = nein)
	keys     PakeKeys
	ready    bool // PAKE fertig und bestätigt
}

// HostUI: was die Oberfläche erfährt.
type HostUI struct {
	Kind string // klopft, falsch, weg, hand, tasten, zuschauer, ende
	G    *GuestState
	Data []byte
	N    int
}

// HostSession: Gastgeber-Seite einer Freigabe v2.
type HostSession struct {
	Sh    *Share2
	Words [3]string
	Key   []byte // Freigabe-Schlüssel (Rahmen an alle)
	Max   int    // höchstens so viele Zuschauer (0 = 10)

	mu     sync.Mutex
	guests map[uint32]*GuestState
	used   int // erfolgreiche Anmeldungen mit diesem Code
	locked bool
}

// NewHostSession: frischer Freigabe-Schlüssel und Code.
func NewHostSession(sh *Share2, max int) *HostSession {
	key := make([]byte, chacha20poly1305.KeySize)
	_, _ = rand.Read(key)
	return &HostSession{Sh: sh, Words: NewWords(), Key: key, Max: max, guests: map[uint32]*GuestState{}}
}

// Invite: Code (und Ort) zum Weitergeben.
func (h *HostSession) Invite() Invite {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Invite{Platz: h.Sh.Platz, Words: h.Words, Base: h.Sh.Base, FP: h.Sh.FP}
}

// Locked: Code verbraucht oder gesperrt.
func (h *HostSession) Locked() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.locked }

// NewCode: neue Wörter, Einladung wieder offen (alter Code gilt nicht mehr).
func (h *HostSession) NewCode() Invite {
	h.mu.Lock()
	h.Words, h.used, h.locked = NewWords(), 0, false
	h.mu.Unlock()
	_ = h.Sh.Unlock()
	return h.Invite()
}

// LockCode: keine neuen Zuschauer mehr.
func (h *HostSession) LockCode() {
	h.mu.Lock()
	h.locked = true
	h.mu.Unlock()
	_ = h.Sh.Lock()
}

// Guests: alle angemeldeten Zuschauer (Kopien, nach Ankunft sortiert).
func (h *HostSession) Guests() []GuestState {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []GuestState
	for _, g := range h.guests {
		if g.ready {
			out = append(out, *g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].V < out[j].V })
	return out
}

func (h *HostSession) max() int {
	if h.Max <= 0 || h.Max > maxViewers {
		return maxViewers
	}
	return h.Max
}

// Handle verarbeitet eine Meldung des Vermittlers (aus Share2.Events).
func (h *HostSession) Handle(e HostEvent) (HostUI, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch e.Kind {
	case "neu":
		h.guests[e.V] = &GuestState{V: e.V, IP: e.IP, Since: time.Now()}
	case "weg":
		g := h.guests[e.V]
		delete(h.guests, e.V)
		if g != nil && g.ready {
			return HostUI{Kind: "weg", G: g}, true
		}
		if g != nil && g.keys.Key != nil && !h.locked {
			// Code ausprobiert und dann gegangen: falsch geraten. Ein Versuch.
			h.burn()
			return HostUI{Kind: "falsch", G: g}, true
		}
	case "zuschauer":
		return HostUI{Kind: "zuschauer", N: e.N}, true
	case "msg":
		g := h.guests[e.V]
		if g == nil || len(e.Data) == 0 {
			return HostUI{}, false
		}
		return h.guestMsg(g, e.Data)
	}
	return HostUI{}, false
}

func (h *HostSession) burn() {
	h.locked = true
	go func() { _ = h.Sh.Lock() }()
}

// guestMsg (h.mu gehalten).
func (h *HostSession) guestMsg(g *GuestState, b []byte) (HostUI, bool) {
	switch b[0] {
	case MsgPakeA:
		if g.ready || g.keys.Key != nil || h.locked {
			return HostUI{}, false
		}
		msgB, k, err := HostPake(h.Words, h.Sh.Platz, b[1:])
		if err != nil {
			return HostUI{}, false
		}
		g.keys = k
		v, out := g.V, PakeB(msgB, k.Host)
		go func() { _ = h.Sh.To(v, out) }()
	case MsgHello:
		if g.ready || g.keys.Key == nil || len(b) < 33 {
			return HostUI{}, false
		}
		if !hmac.Equal(b[1:33], g.keys.Guest) {
			// Falscher Code: Einladung sofort sperren (ein Rateversuch).
			h.burn()
			v := g.V
			go func() {
				_ = h.Sh.To(v, append([]byte{MsgNo}, "Code passt nicht"...))
				_ = h.Sh.Kick(v)
			}()
			return HostUI{Kind: "falsch", G: g}, true
		}
		p, err := g.keys.OpenCtl(b[33:])
		if err != nil || json.Unmarshal(p, &g.Hello) != nil {
			return HostUI{}, false
		}
		g.Hello.Name = clean(g.Hello.Name, 40)
		g.Hello.Rechner = clean(g.Hello.Rechner, 60)
		g.Hello.Version = clean(g.Hello.Version, 30)
		if g.Hello.Name == "" {
			g.Hello.Name = "?"
		}
		g.ready, g.Check = true, g.keys.Check
		h.used++
		if h.used >= h.max() {
			h.burn()
		}
		return HostUI{Kind: "klopft", G: g}, true
	case MsgCtl:
		if !g.ready || !g.Admitted {
			return HostUI{}, false
		}
		p, err := g.keys.OpenCtl(b[1:])
		if err != nil {
			return HostUI{}, false
		}
		var c Ctl
		if json.Unmarshal(p, &c) != nil {
			return HostUI{}, false
		}
		switch c.T {
		case "hand":
			return HostUI{Kind: "hand", G: g}, true
		case "tasten":
			if time.Now().Before(g.WriteBis) && len(c.Data) > 0 {
				return HostUI{Kind: "tasten", G: g, Data: c.Data}, true
			}
		}
	}
	return HostUI{}, false
}

// SendCtl: Steuernachricht an einen Zuschauer.
func (h *HostSession) SendCtl(v uint32, c Ctl) error {
	h.mu.Lock()
	g := h.guests[v]
	h.mu.Unlock()
	if g == nil || !g.ready {
		return errors.New("unbekannt")
	}
	b, _ := json.Marshal(c)
	f, err := g.keys.SealCtl(b)
	if err != nil {
		return err
	}
	return h.Sh.To(v, append([]byte{MsgCtl}, f...))
}

// Admit: Zuschauer zulassen (bekommt Schlüssel, danach die Rahmen an alle).
func (h *HostSession) Admit(v uint32, info ShareInfo) error {
	if err := h.SendCtl(v, Ctl{T: "ok", Key: h.Key, ID: h.Sh.ID, Info: &info}); err != nil {
		return err
	}
	h.mu.Lock()
	if g := h.guests[v]; g != nil {
		g.Admitted = true
	}
	h.mu.Unlock()
	return h.Sh.Admit(v)
}

// Deny: ablehnen und trennen.
func (h *HostSession) Deny(v uint32) {
	_ = h.Sh.To(v, append([]byte{MsgNo}, "abgelehnt"...))
	_ = h.Sh.Kick(v)
}

// SetWrite: Mitschreiben erlauben (bis) oder entziehen (Null).
func (h *HostSession) SetWrite(v uint32, until time.Time) error {
	h.mu.Lock()
	if g := h.guests[v]; g != nil {
		g.WriteBis = until
	}
	h.mu.Unlock()
	var bis int64
	if !until.IsZero() {
		bis = until.Unix()
	}
	return h.SendCtl(v, Ctl{T: "schreiben", Bis: bis})
}

// Frame: Rahmen an alle Zugelassenen (v=0) oder an einen.
func (h *HostSession) Frame(v uint32, typ byte, p []byte) error {
	f, err := SealFrame(h.Key, h.Sh.ID, typ, p)
	if err != nil {
		return err
	}
	if v == 0 {
		return h.Sh.All(f)
	}
	return h.Sh.To(v, f)
}

// ---------------------------------------------------------------- Finden

// Locate sucht die Freigabe zum Code: Ort aus dem Link, sonst im lokalen
// Netz, sonst bei den bekannten Vermittlern (gekoppelte, öffentlicher).
func Locate(ctx context.Context, inv Invite, known []Found) (*Guest2, string, error) {
	try := func(f Found) (*Guest2, error) {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return JoinShare(c, f.Base, f.FP, inv.Platz)
	}
	if inv.Base != "" {
		g, err := try(Found{inv.Base, inv.FP})
		return g, inv.Base, err
	}
	var cands []Found
	if l, err := FindLAN(ctx, inv.Platz, 1200*time.Millisecond); err == nil {
		cands = append(cands, l...)
	}
	cands = append(cands, known...)
	var last error
	for _, f := range cands {
		g, err := try(f)
		if err == nil {
			return g, f.Base, nil
		}
		last = err
	}
	if last != nil {
		var se *StatusError
		if errors.As(last, &se) && se.Code != http.StatusNotFound {
			return nil, "", last
		}
	}
	return nil, "", i18n.NewError("Freigabe nicht gefunden: nicht im selben Netz? Dann den Link (sshtermy://zusehen/…) statt des Codes benutzen")
}
