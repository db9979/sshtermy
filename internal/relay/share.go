package relay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/chacha20poly1305"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Sitzung live teilen
//
// Wer einen Vermittler gekoppelt hat, kann die Ausgabe eines Tabs live
// zeigen. Der Vermittler verteilt nur Bytes: sshTermy verschlüsselt jedes
// Stück mit einem Schlüssel, der allein im Link steht (hinter „#“, geht
// nie an den Vermittler). Zuschauer brauchen den Link und sshTermy, aber
// keine Kopplung. Sie sehen nur zu; Eingaben gehen nirgendwo hin.
//
//	Gastgeber ──WSS /v1/teilen/senden──► Vermittler ──WSS /v1/teilen/ansehen──► Zuschauer
//
// Link: https://vermittler#teilen=ID&k=SCHLÜSSEL[&fp=SHA256]

const (
	PathShare      = "/v1/teilen"         // POST → {id}
	PathShareSend  = "/v1/teilen/senden"  // WebSocket ?id=: Gastgeber
	PathShareWatch = "/v1/teilen/ansehen" // WebSocket ?id=: Zuschauer (ohne Schlüssel)

	maxViewers     = 10
	maxShares      = 64
	maxSharesPer   = 4
	shareHostWait  = time.Minute
	shareFrameMax  = 1 << 20
	shareWriteWait = 5 * time.Second
)

// Rahmenarten (erstes Byte des entschlüsselten Rahmens).
const (
	FrameData = 'd' // Ausgabe des Terminals
	FrameSize = 's' // „Spalten Zeilen“
	FrameEnd  = 'e' // Gastgeber hat beendet
)

// ---------------------------------------------------------------- Link

// ShareLink: alles, was ein Zuschauer braucht.
type ShareLink struct {
	Base string
	ID   string
	Key  []byte // 32 Byte
	FP   string
}

var shareIDRE = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (l ShareLink) String() string {
	s := l.Base + "#teilen=" + l.ID + "&k=" + base64.RawURLEncoding.EncodeToString(l.Key)
	if l.FP != "" {
		s += "&fp=" + l.FP
	}
	return s
}

// IsShareLink: sieht s wie ein Link zum Zusehen aus?
func IsShareLink(s string) bool {
	return strings.Contains(s, "#teilen=") && strings.Contains(s, "https://")
}

// ParseShareLink liest einen Link zum Zusehen.
func ParseShareLink(s string) (ShareLink, error) {
	s = strings.Trim(strings.TrimSpace(s), "'\"<> ")
	if i := strings.Index(s, "https://"); i > 0 {
		s = s[i:]
	}
	base, frag, ok := strings.Cut(s, "#")
	if !ok {
		return ShareLink{}, i18n.NewError("Link zum Zusehen ohne „#teilen=…“")
	}
	v, err := url.ParseQuery(frag)
	if err != nil {
		return ShareLink{}, err
	}
	l := ShareLink{ID: v.Get("teilen"), FP: strings.ToLower(v.Get("fp"))}
	if !shareIDRE.MatchString(l.ID) {
		return ShareLink{}, i18n.NewError("Link zum Zusehen: Kennung ungültig")
	}
	if l.Key, err = base64.RawURLEncoding.DecodeString(v.Get("k")); err != nil || len(l.Key) != chacha20poly1305.KeySize {
		return ShareLink{}, i18n.NewError("Link zum Zusehen: Schlüssel fehlt oder ist ungültig")
	}
	if l.FP != "" && !fpRE.MatchString(l.FP) {
		return ShareLink{}, i18n.NewError("Kopplungszeile: Fingerabdruck ungültig")
	}
	if l.Base, err = NormBase(base); err != nil {
		return ShareLink{}, err
	}
	return l, nil
}

// ---------------------------------------------------------------- Verschlüsselung

func seal(key []byte, id string, typ byte, payload []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+1+len(payload)+aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	plain := append([]byte{typ}, payload...)
	return aead.Seal(nonce, nonce, plain, []byte(id)), nil
}

func open(key []byte, id string, frame []byte) (byte, []byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return 0, nil, err
	}
	if len(frame) < aead.NonceSize()+aead.Overhead()+1 {
		return 0, nil, i18n.NewError("Rahmen zu kurz")
	}
	plain, err := aead.Open(nil, frame[:aead.NonceSize()], frame[aead.NonceSize():], []byte(id))
	if err != nil {
		return 0, nil, i18n.NewError("Rahmen lässt sich nicht entschlüsseln (falscher Link?)")
	}
	return plain[0], plain[1:], nil
}

// ---------------------------------------------------------------- Gastgeber

// ShareHost: offene Freigabe auf Seiten des Gastgebers.
type ShareHost struct {
	Link    ShareLink
	ws      *websocket.Conn
	viewers chan int
	done    chan struct{}
	once    sync.Once
}

// StartShare legt eine Freigabe an und verbindet sich als Gastgeber.
func (c *Client) StartShare(ctx context.Context) (*ShareHost, error) {
	var r struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, PathShare, map[string]string{}, &r); err != nil {
		return nil, err
	}
	if !shareIDRE.MatchString(r.ID) {
		return nil, i18n.NewError("Vermittler hat keine gültige Kennung geschickt")
	}
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	h := http.Header{}
	c.auth(h)
	ws, err := c.dialWS(ctx, PathShareSend+"?id="+r.ID, h)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(64 * 1024)
	sh := &ShareHost{Link: ShareLink{Base: c.Base, ID: r.ID, Key: key, FP: c.FP}, ws: ws,
		viewers: make(chan int, 8), done: make(chan struct{})}
	go sh.readCtrl()
	return sh, nil
}

func (c *Client) dialWS(ctx context.Context, path string, h http.Header) (*websocket.Conn, error) {
	ws, resp, err := websocket.Dial(ctx, c.Base+path, &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: h})
	if err != nil {
		if resp != nil && resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusSwitchingProtocols {
			var b []byte
			if resp.Body != nil {
				b, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<16))
			}
			return nil, &StatusError{Code: resp.StatusCode, Msg: errMsg(b)}
		}
		return nil, fmt.Errorf(i18n.T("Verbindung zum Vermittler: %w"), err)
	}
	return ws, nil
}

// readCtrl: Meldungen des Vermittlers (Zahl der Zuschauer).
func (h *ShareHost) readCtrl() {
	defer h.finish()
	for {
		typ, b, err := h.ws.Read(context.Background())
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var m struct {
			Typ string `json:"typ"`
			N   int    `json:"n"`
		}
		if json.Unmarshal(b, &m) == nil && m.Typ == "zuschauer" {
			select {
			case h.viewers <- m.N:
			default:
			}
		}
	}
}

func (h *ShareHost) finish() { h.once.Do(func() { close(h.done) }) }

// Viewers: neue Zahl der Zuschauer, sobald jemand kommt oder geht.
func (h *ShareHost) Viewers() <-chan int { return h.viewers }

// Done: die Freigabe ist zu Ende (Vermittler weg oder Close).
func (h *ShareHost) Done() <-chan struct{} { return h.done }

// Send verschlüsselt einen Rahmen und schickt ihn an alle Zuschauer.
func (h *ShareHost) Send(typ byte, payload []byte) error {
	for len(payload) > shareFrameMax-64 {
		if err := h.Send(typ, payload[:shareFrameMax-64]); err != nil {
			return err
		}
		payload = payload[shareFrameMax-64:]
	}
	f, err := seal(h.Link.Key, h.Link.ID, typ, payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.ws.Write(ctx, websocket.MessageBinary, f)
}

// Close beendet die Freigabe (Zuschauer bekommen „Ende“).
func (h *ShareHost) Close() {
	_ = h.Send(FrameEnd, nil)
	h.ws.Close(websocket.StatusNormalClosure, "")
	h.finish()
}

// ---------------------------------------------------------------- Zuschauer

// ShareView: Verbindung eines Zuschauers.
type ShareView struct {
	link ShareLink
	ws   *websocket.Conn
}

// WatchShare verbindet sich als Zuschauer (ohne Kopplung).
func WatchShare(ctx context.Context, l ShareLink) (*ShareView, error) {
	c, err := NewClient(l.Base, "", l.FP)
	if err != nil {
		return nil, err
	}
	ws, err := c.dialWS(ctx, PathShareWatch+"?id="+l.ID, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(shareFrameMax + 1024)
	return &ShareView{link: l, ws: ws}, nil
}

// Next liefert den nächsten Rahmen. io.EOF = Gastgeber hat beendet.
func (v *ShareView) Next(ctx context.Context) (byte, []byte, error) {
	for {
		typ, b, err := v.ws.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return 0, nil, io.EOF
			}
			return 0, nil, err
		}
		if typ != websocket.MessageBinary {
			continue
		}
		t, p, err := open(v.link.Key, v.link.ID, b)
		if err != nil {
			return 0, nil, err
		}
		if t == FrameEnd {
			return 0, nil, io.EOF
		}
		return t, p, nil
	}
}

// Close trennt den Zuschauer.
func (v *ShareView) Close() error { return v.ws.Close(websocket.StatusNormalClosure, "") }

// ---------------------------------------------------------------- Vermittler

type shareRoom struct {
	owner   string
	host    *websocket.Conn
	viewers map[*websocket.Conn]bool
	created time.Time
}

func (s *Server) shareRooms() map[string]*shareRoom {
	if s.shares == nil {
		s.shares = map[string]*shareRoom{}
	}
	return s.shares
}

// shareCreate: gekoppeltes Gerät legt eine Freigabe an ({"v":2} = mit Code
// und Zulassung, share2.go).
func (s *Server) shareCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		V   int `json:"v"`
		Min int `json:"min"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	_ = json.NewDecoder(r.Body).Decode(&in)
	cid, open, ok := s.shareOwner(w, r, true)
	if !ok {
		return
	}
	if in.V >= 2 {
		s.create2(w, cid, open, in.Min)
		return
	}
	if open || cid == "lokal" {
		fail(w, http.StatusForbidden, "nur mit Code (sshTermy zu alt?)")
		return
	}
	s.mu.Lock()
	rooms := s.shareRooms()
	mine := 0
	for id, x := range rooms {
		if x.host == nil && time.Since(x.created) > shareHostWait {
			delete(rooms, id) // nie verbunden
			continue
		}
		if x.owner == cid {
			mine++
		}
	}
	if len(rooms)+len(s.rooms2()) >= maxShares || mine >= maxSharesPer {
		s.mu.Unlock()
		fail(w, http.StatusTooManyRequests, "zu viele Freigaben")
		return
	}
	id := randID()
	rooms[id] = &shareRoom{owner: cid, viewers: map[*websocket.Conn]bool{}, created: time.Now()}
	s.mu.Unlock()
	s.log("teilen_neu", 1)
	reply(w, map[string]string{"id": id})
}

// shareSend: der Gastgeber schickt Rahmen, der Vermittler verteilt sie.
func (s *Server) shareSend(w http.ResponseWriter, r *http.Request) {
	cid, _, ok := s.shareOwner(w, r, false)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	s.mu.Lock()
	if rm := s.rooms2()[id]; rm != nil {
		s.mu.Unlock()
		s.send2(w, r, rm, cid)
		return
	}
	room := s.shareRooms()[id]
	bad := room == nil || room.owner != cid || room.host != nil
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
	s.mu.Lock()
	room.host = ws
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.shareRooms(), id)
		viewers := room.viewers
		room.viewers = map[*websocket.Conn]bool{}
		s.mu.Unlock()
		for v := range viewers {
			v.Close(websocket.StatusNormalClosure, "Ende")
		}
		ws.Close(websocket.StatusNormalClosure, "")
		s.log("teilen_ende", 1)
	}()
	for {
		typ, b, err := ws.Read(context.Background())
		if err != nil {
			return
		}
		if typ != websocket.MessageBinary {
			continue
		}
		s.mu.Lock()
		list := make([]*websocket.Conn, 0, len(room.viewers))
		for v := range room.viewers {
			list = append(list, v)
		}
		s.mu.Unlock()
		for _, v := range list {
			ctx, cancel := context.WithTimeout(context.Background(), shareWriteWait)
			err := v.Write(ctx, websocket.MessageBinary, b)
			cancel()
			if err != nil { // zu langsam oder weg: raus
				s.dropViewer(room, v)
			}
		}
	}
}

func (s *Server) dropViewer(room *shareRoom, v *websocket.Conn) {
	s.mu.Lock()
	_, had := room.viewers[v]
	delete(room.viewers, v)
	n, host := len(room.viewers), room.host
	s.mu.Unlock()
	if had {
		v.Close(websocket.StatusPolicyViolation, "zu langsam")
		notifyViewers(host, n)
	}
}

func notifyViewers(host *websocket.Conn, n int) {
	if host == nil {
		return
	}
	msg, _ := json.Marshal(map[string]any{"typ": "zuschauer", "n": n})
	ctx, cancel := context.WithTimeout(context.Background(), shareWriteWait)
	defer cancel()
	_ = host.Write(ctx, websocket.MessageText, msg)
}

// shareWatch: Zuschauer ohne Kopplung; die Kennung ist das Geheimnis für den
// Vermittler, der Schlüssel im Link das für den Inhalt.
func (s *Server) shareWatch(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("platz") {
		s.watch2(w, r)
		return
	}
	if !s.limit.allow("watch:"+s.ip(r), 30, time.Minute) {
		fail(w, http.StatusTooManyRequests, "zu viele Anfragen")
		return
	}
	id := r.URL.Query().Get("id")
	// Der Gastgeber ist evtl. gerade erst verbunden: sein Handler trägt die
	// Leitung kurz nach dem Handshake ein. So lange kurz warten statt 404.
	for i := 0; i < 20; i++ {
		s.mu.Lock()
		room := s.shareRooms()[id]
		ready := room == nil || room.host != nil
		s.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	room := s.shareRooms()[id]
	var err error
	switch {
	case room == nil || room.host == nil:
		err = errors.New("nicht gefunden")
	case len(room.viewers) >= maxViewers:
		err = errors.New("voll")
	}
	s.mu.Unlock()
	if err != nil {
		if err.Error() == "voll" {
			fail(w, http.StatusTooManyRequests, "schon zu viele Zuschauer")
		} else {
			fail(w, http.StatusNotFound, "Freigabe beendet oder unbekannt")
		}
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(1024)
	s.mu.Lock()
	if s.shareRooms()[id] != room {
		s.mu.Unlock()
		ws.Close(websocket.StatusNormalClosure, "Ende")
		return
	}
	room.viewers[ws] = true
	n, host := len(room.viewers), room.host
	s.mu.Unlock()
	notifyViewers(host, n)
	s.log("zuschauer", n)
	for { // Zuschauer schicken nichts; Lesen erkennt das Ende
		if _, _, err := ws.Read(context.Background()); err != nil {
			break
		}
	}
	s.dropViewerQuiet(room, ws)
}

func (s *Server) dropViewerQuiet(room *shareRoom, v *websocket.Conn) {
	s.mu.Lock()
	_, had := room.viewers[v]
	delete(room.viewers, v)
	n, host := len(room.viewers), room.host
	s.mu.Unlock()
	if had {
		notifyViewers(host, n)
	}
}
