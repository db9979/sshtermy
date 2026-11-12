package relay

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Teilen v2: Einladung mit Code, Zulassung durch den Gastgeber
//
// Der Vermittler kennt je Freigabe einen Platz (kleine Zahl, steht vorn im
// Code). Zuschauer melden sich mit dem Platz an und sind erst „wartend“: Ihre
// Nachrichten gehen nur an den Gastgeber, sie bekommen nur, was der Gastgeber
// ihnen persönlich schickt (PAKE-Antwort). Erst wenn der Gastgeber sie
// zulässt, bekommen sie auch die Rahmen an alle. Der Vermittler sieht nur
// verschlüsselte Bytes; Schlüssel und Code kennt er nie.
//
// Gastgeber → Vermittler: Binär 0x00+Daten = an alle Zugelassenen,
// 0x01+ID(4)+Daten = an einen Zuschauer; Text {"typ":"zulassen"|"trennen","v"},
// {"typ":"zu"|"auf"} (keine neuen Zuschauer / wieder offen).
// Vermittler → Gastgeber: Binär ID(4)+Daten (vom Zuschauer); Text
// {"typ":"neu","v","ip"}, {"typ":"weg","v"}, {"typ":"zuschauer","n"}.

const (
	maxPending   = 3               // gleichzeitig wartende Zuschauer je Freigabe
	pendingWait  = 3 * time.Minute // so lange darf ein Zuschauer warten
	maxShareLife = 12 * time.Hour  // längste Freigabe (gekoppelte Geräte)
	openLife     = 4 * time.Hour   // längste Freigabe ohne Kopplung (offener Vermittler)
	guestMsgMax  = 16 * 1024       // Nachricht eines Zuschauers (Tasten, Hallo)
	platzMax     = 999             // Plätze 1..999
	lanPlatzMin  = 100             // im lokalen Netz dreistellig (weniger Zusammenstöße)
	openPerHour  = 10              // Freigaben je Adresse und Stunde ohne Kopplung
	openShareMax = 32              // Freigaben ohne Kopplung insgesamt
	routeAll     = 0x00
	routeOne     = 0x01
)

type guest struct {
	ws       *websocket.Conn
	admitted bool
	ip       string
	since    time.Time
	cancel   context.CancelFunc
}

type room2 struct {
	id      string
	platz   int
	owner   string
	open    bool // ohne Kopplung angelegt
	host    *websocket.Conn
	guests  map[uint32]*guest
	next    uint32
	created time.Time
	until   time.Time
	closed  bool // „zu“: keine neuen Zuschauer
}

func (s *Server) rooms2() map[string]*room2 {
	if s.shares2 == nil {
		s.shares2 = map[string]*room2{}
	}
	return s.shares2
}

// shareOwner: wer darf eine Freigabe anlegen? Gekoppeltes Gerät, der
// eingebaute Vermittler im lokalen Netz (LocalKey) oder – bei einem offenen
// Vermittler – jeder, gebremst je Adresse.
func (s *Server) shareOwner(w http.ResponseWriter, r *http.Request, create bool) (string, bool, bool) {
	key := bearer(r)
	if s.LocalKey != "" {
		if key == s.LocalKey {
			return "lokal", false, true
		}
		fail(w, http.StatusUnauthorized, "unbekannter Schlüssel")
		return "", false, false
	}
	if key != "" || !s.Open {
		id, ok := s.peer(w, r, false)
		return id, false, ok
	}
	ip := s.ip(r)
	if create && !s.limit.allow("open:"+ip, openPerHour, time.Hour) {
		fail(w, http.StatusTooManyRequests, "zu viele Freigaben")
		return "", false, false
	}
	return "ip:" + ip, true, true
}

// create2: neue Freigabe v2 mit Platz.
func (s *Server) create2(w http.ResponseWriter, owner string, open bool, minutes int) {
	life := maxShareLife
	if open {
		life = openLife
	}
	if d := time.Duration(minutes) * time.Minute; d > 0 && d < life {
		life = d + time.Minute // etwas Luft: der Gastgeber beendet selbst
	}
	s.mu.Lock()
	rooms := s.rooms2()
	mine, opens := 0, 0
	used := map[int]bool{}
	for id, x := range rooms {
		if x.host == nil && time.Since(x.created) > shareHostWait {
			delete(rooms, id)
			continue
		}
		if x.owner == owner {
			mine++
		}
		if x.open {
			opens++
		}
		used[x.platz] = true
	}
	if len(rooms)+len(s.shareRooms()) >= maxShares || (owner != "lokal" && mine >= maxSharesPer) || (open && opens >= openShareMax) {
		s.mu.Unlock()
		fail(w, http.StatusTooManyRequests, "zu viele Freigaben")
		return
	}
	lo, hi := 1, 99
	if s.LocalKey != "" {
		lo, hi = lanPlatzMin, platzMax
	} else if len(used) > 60 {
		hi = platzMax
	}
	platz := 0
	for i := 0; i < 200 && platz == 0; i++ {
		if p := lo + randN(hi-lo+1); !used[p] {
			platz = p
		}
	}
	if platz == 0 {
		s.mu.Unlock()
		fail(w, http.StatusTooManyRequests, "zu viele Freigaben")
		return
	}
	id := randID()
	rooms[id] = &room2{id: id, platz: platz, owner: owner, open: open, guests: map[uint32]*guest{},
		created: time.Now(), until: time.Now().Add(life)}
	s.mu.Unlock()
	s.log("teilen_neu", 2)
	reply(w, map[string]any{"id": id, "platz": platz, "bis": time.Now().Add(life).Unix()})
}

func hostText(ws *websocket.Conn, v any) {
	if ws == nil {
		return
	}
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), shareWriteWait)
	defer cancel()
	_ = ws.Write(ctx, websocket.MessageText, b)
}

func (rm *room2) admittedN() int {
	n := 0
	for _, g := range rm.guests {
		if g.admitted {
			n++
		}
	}
	return n
}

// send2: Leitung des Gastgebers.
func (s *Server) send2(w http.ResponseWriter, r *http.Request, rm *room2, owner string) {
	s.mu.Lock()
	bad := rm.owner != owner || rm.host != nil
	s.mu.Unlock()
	if bad {
		fail(w, http.StatusNotFound, "Freigabe unbekannt")
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(shareFrameMax + 1024)
	ctx, cancel := context.WithDeadline(context.Background(), rm.until)
	defer cancel()
	s.mu.Lock()
	rm.host = ws
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.rooms2(), rm.id)
		gs := rm.guests
		rm.guests = map[uint32]*guest{}
		s.mu.Unlock()
		for _, g := range gs {
			g.ws.Close(websocket.StatusNormalClosure, "Ende")
		}
		ws.Close(websocket.StatusNormalClosure, "")
		s.log("teilen_ende", 2)
	}()
	for {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageText {
			s.hostCtl(rm, b)
			continue
		}
		if len(b) < 1 {
			continue
		}
		var targets []*guest
		var payload []byte
		s.mu.Lock()
		switch {
		case b[0] == routeAll:
			payload = b[1:]
			for _, g := range rm.guests {
				if g.admitted {
					targets = append(targets, g)
				}
			}
		case b[0] == routeOne && len(b) >= 5:
			payload = b[5:]
			if g := rm.guests[binary.BigEndian.Uint32(b[1:5])]; g != nil {
				targets = append(targets, g)
			}
		}
		s.mu.Unlock()
		for _, g := range targets {
			wctx, c := context.WithTimeout(context.Background(), shareWriteWait)
			err := g.ws.Write(wctx, websocket.MessageBinary, payload)
			c()
			if err != nil { // zu langsam oder weg
				g.cancel()
			}
		}
	}
}

func (s *Server) hostCtl(rm *room2, b []byte) {
	var m struct {
		Typ string `json:"typ"`
		V   uint32 `json:"v"`
	}
	if json.Unmarshal(b, &m) != nil {
		return
	}
	s.mu.Lock()
	g := rm.guests[m.V]
	var notify bool
	switch m.Typ {
	case "zulassen":
		if g != nil && !g.admitted && rm.admittedN() < maxViewers {
			g.admitted, notify = true, true
		}
	case "trennen":
		if g != nil {
			g.cancel()
		}
	case "zu":
		rm.closed = true
	case "auf":
		rm.closed = false
	}
	n, host := rm.admittedN(), rm.host
	s.mu.Unlock()
	if notify {
		hostText(host, map[string]any{"typ": "zuschauer", "n": n})
		s.log("zuschauer", n)
	}
}

// watch2: Zuschauer mit Platz (aus dem Code).
func (s *Server) watch2(w http.ResponseWriter, r *http.Request) {
	ip := s.ip(r)
	if !s.limit.allow("watch:"+ip, 30, time.Minute) {
		fail(w, http.StatusTooManyRequests, "zu viele Anfragen")
		return
	}
	platz, _ := strconv.Atoi(r.URL.Query().Get("platz"))
	find := func() *room2 {
		for _, x := range s.rooms2() {
			if x.platz == platz {
				return x
			}
		}
		return nil
	}
	for i := 0; i < 20; i++ {
		s.mu.Lock()
		rm := find()
		ready := rm == nil || rm.host != nil
		s.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	rm := find()
	code, msg := 0, ""
	switch {
	case rm == nil || rm.host == nil:
		code, msg = http.StatusNotFound, "Freigabe beendet oder unbekannt"
	case rm.closed:
		code, msg = http.StatusGone, "Einladung schon benutzt oder gesperrt"
	default:
		pend := 0
		for _, g := range rm.guests {
			if !g.admitted {
				pend++
			}
		}
		if pend >= maxPending || len(rm.guests) >= maxViewers+maxPending {
			code, msg = http.StatusTooManyRequests, "schon zu viele Zuschauer"
		}
	}
	s.mu.Unlock()
	if code != 0 {
		fail(w, code, msg)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(guestMsgMax)
	ctx, cancel := context.WithDeadline(context.Background(), rm.until)
	defer cancel()
	s.mu.Lock()
	if s.rooms2()[rm.id] != rm {
		s.mu.Unlock()
		ws.Close(websocket.StatusNormalClosure, "Ende")
		return
	}
	rm.next++
	id := rm.next
	g := &guest{ws: ws, ip: ip, since: time.Now(), cancel: cancel}
	rm.guests[id] = g
	host := rm.host
	s.mu.Unlock()
	hostText(host, map[string]any{"typ": "neu", "v": id, "ip": ip})
	// Wer nicht zugelassen wird, fliegt nach pendingWait raus.
	go func() {
		t := time.NewTimer(pendingWait)
		defer t.Stop()
		select {
		case <-t.C:
			s.mu.Lock()
			adm := g.admitted
			s.mu.Unlock()
			if !adm {
				cancel()
			}
		case <-ctx.Done():
		}
	}()
	pre := make([]byte, 4)
	binary.BigEndian.PutUint32(pre, id)
	for {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			break
		}
		if typ != websocket.MessageBinary {
			continue
		}
		wctx, c := context.WithTimeout(context.Background(), shareWriteWait)
		err = host.Write(wctx, websocket.MessageBinary, append(pre[:4:4], b...))
		c()
		if err != nil {
			break
		}
	}
	s.mu.Lock()
	_, had := rm.guests[id]
	delete(rm.guests, id)
	n := rm.admittedN()
	s.mu.Unlock()
	ws.Close(websocket.StatusNormalClosure, "Ende")
	if had {
		hostText(host, map[string]any{"typ": "weg", "v": id})
		if g.admitted {
			hostText(host, map[string]any{"typ": "zuschauer", "n": n})
		}
	}
}

// ---------------------------------------------------------------- Gastgeber (Client)

// HostEvent: was der Vermittler dem Gastgeber meldet.
type HostEvent struct {
	Kind string // "neu", "weg", "zuschauer", "msg"
	V    uint32
	IP   string
	N    int
	Data []byte
}

// Share2: offene Freigabe v2 auf Seiten des Gastgebers.
type Share2 struct {
	ID    string
	Platz int
	Base  string
	FP    string
	Until time.Time
	ws    *websocket.Conn
	ev    chan HostEvent
	done  chan struct{}
	once  sync.Once
}

// StartShare2 legt eine Freigabe mit Platz an und verbindet sich als Gastgeber.
// minutes begrenzt die Dauer auch beim Vermittler.
func (c *Client) StartShare2(ctx context.Context, minutes int) (*Share2, error) {
	var r struct {
		ID    string `json:"id"`
		Platz int    `json:"platz"`
		Bis   int64  `json:"bis"`
	}
	if err := c.do(ctx, http.MethodPost, PathShare, map[string]int{"v": 2, "min": minutes}, &r); err != nil {
		return nil, err
	}
	if !shareIDRE.MatchString(r.ID) || r.Platz < 1 || r.Platz > platzMax {
		return nil, i18n.NewError("Vermittler kann noch nicht mit Code teilen (zu alt?)")
	}
	h := http.Header{}
	c.auth(h)
	ws, err := c.dialWS(ctx, PathShareSend+"?id="+r.ID, h)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(guestMsgMax + 64)
	sh := &Share2{ID: r.ID, Platz: r.Platz, Base: c.Base, FP: c.FP, Until: time.Unix(r.Bis, 0), ws: ws,
		ev: make(chan HostEvent, 64), done: make(chan struct{})}
	go sh.read()
	return sh, nil
}

func (h *Share2) read() {
	defer h.once.Do(func() { close(h.done) })
	for {
		typ, b, err := h.ws.Read(context.Background())
		if err != nil {
			return
		}
		var e HostEvent
		if typ == websocket.MessageBinary {
			if len(b) < 4 {
				continue
			}
			e = HostEvent{Kind: "msg", V: binary.BigEndian.Uint32(b[:4]), Data: b[4:]}
		} else {
			var m struct {
				Typ string `json:"typ"`
				V   uint32 `json:"v"`
				IP  string `json:"ip"`
				N   int    `json:"n"`
			}
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			e = HostEvent{Kind: m.Typ, V: m.V, IP: m.IP, N: m.N}
		}
		select {
		case h.ev <- e:
		case <-h.done:
			return
		}
	}
}

// Events: Meldungen des Vermittlers (in Reihenfolge).
func (h *Share2) Events() <-chan HostEvent { return h.ev }

// Done: Leitung zu Ende.
func (h *Share2) Done() <-chan struct{} { return h.done }

func (h *Share2) write(typ websocket.MessageType, b []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.ws.Write(ctx, typ, b)
}

// All: an alle Zugelassenen.
func (h *Share2) All(p []byte) error {
	return h.write(websocket.MessageBinary, append([]byte{routeAll}, p...))
}

// To: an einen Zuschauer (auch wartend).
func (h *Share2) To(v uint32, p []byte) error {
	b := make([]byte, 5, 5+len(p))
	b[0] = routeOne
	binary.BigEndian.PutUint32(b[1:], v)
	return h.write(websocket.MessageBinary, append(b, p...))
}

func (h *Share2) ctl(typ string, v uint32) error {
	b, _ := json.Marshal(map[string]any{"typ": typ, "v": v})
	return h.write(websocket.MessageText, b)
}

// Admit lässt einen Zuschauer zu (bekommt ab jetzt die Rahmen an alle).
func (h *Share2) Admit(v uint32) error { return h.ctl("zulassen", v) }

// Kick trennt einen Zuschauer.
func (h *Share2) Kick(v uint32) error { return h.ctl("trennen", v) }

// Lock: keine neuen Zuschauer (Code verbraucht). Unlock: wieder offen.
func (h *Share2) Lock() error   { return h.ctl("zu", 0) }
func (h *Share2) Unlock() error { return h.ctl("auf", 0) }

// Close beendet die Freigabe.
func (h *Share2) Close() {
	h.ws.Close(websocket.StatusNormalClosure, "")
	h.once.Do(func() { close(h.done) })
}

// ---------------------------------------------------------------- Zuschauer (Client)

// Guest2: Leitung eines Zuschauers (v2).
type Guest2 struct {
	ws *websocket.Conn
}

// JoinShare verbindet sich mit dem Platz einer Freigabe.
func JoinShare(ctx context.Context, base, fp string, platz int) (*Guest2, error) {
	c, err := NewClient(base, "", fp)
	if err != nil {
		return nil, err
	}
	ws, err := c.dialWS(ctx, PathShareWatch+"?platz="+strconv.Itoa(platz), nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(shareFrameMax + 1024)
	return &Guest2{ws: ws}, nil
}

// Send: Nachricht an den Gastgeber.
func (g *Guest2) Send(p []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return g.ws.Write(ctx, websocket.MessageBinary, p)
}

// Recv: nächste Nachricht vom Gastgeber (io.EOF-artig: Fehler bei Ende).
func (g *Guest2) Recv(ctx context.Context) ([]byte, error) {
	for {
		typ, b, err := g.ws.Read(ctx)
		if err != nil {
			return nil, err
		}
		if typ == websocket.MessageBinary {
			return b, nil
		}
	}
}

// Close trennt.
func (g *Guest2) Close() error { return g.ws.Close(websocket.StatusNormalClosure, "") }
