package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/db9979/sshtermy/internal/webwatch"
	"golang.org/x/crypto/ssh"
)

// Server ist der eigene Vermittler (sshtermy vermittler start). Er kennt
// gekoppelte sshTermy-Geräte und Server mit Anker und reicht Bytes zwischen
// beiden durch. Er sieht nie ein Passwort, einen Schlüssel oder einen Befehl:
// SSH läuft verschlüsselt durch ihn hindurch.
type Server struct {
	Dir        string
	TrustProxy bool // X-Forwarded-For gilt (nur hinter einem eigenen Reverse-Proxy)
	Logf       func(string, ...any)
	OpenWait   time.Duration // so lange hat ein Anker, um einen Kanal zu öffnen

	mu      sync.Mutex
	st      *State
	stMod   stateStamp
	anchors map[string]*anchorLink // Anker-ID → Steuerleitung
	pending map[string]*pendingChan
	active  map[string]*activeChan
	shares  map[string]*shareRoom // Sitzung live teilen (share.go)
	shares2 map[string]*room2     // Teilen mit Code und Zulassung (share2.go)
	limit   *limiter

	// Open: Freigaben auch ohne Kopplung (öffentlicher Vermittler nur fürs
	// Teilen, `vermittler start --offen`). Gebremst je Adresse.
	Open bool
	// LocalKey: eingebauter Vermittler im lokalen Netz (lan.go). Nur wer
	// diesen Schlüssel hat (sshTermy selbst), darf Freigaben anlegen.
	LocalKey string
}

type anchorLink struct {
	ws     *websocket.Conn
	cancel context.CancelFunc
	since  time.Time
}

type pendingChan struct {
	anchor string
	got    chan *websocket.Conn
	done   chan struct{}
	once   sync.Once
}

func (p *pendingChan) finish() { p.once.Do(func() { close(p.done) }) }

type activeChan struct {
	client, anchor string
	cancel         context.CancelFunc
}

const (
	maxBody     = 4096
	ctrlLimit   = 64 * 1024
	pingEvery   = 25 * time.Second
	recheck     = 5 * time.Second
	maxChannels = 64
)

// NewServer lädt den Zustand aus dir.
func NewServer(dir string) (*Server, error) {
	st, err := LoadState(dir)
	if err != nil {
		return nil, err
	}
	return &Server{Dir: dir, st: st, stMod: stateMtime(dir), anchors: map[string]*anchorLink{},
		pending: map[string]*pendingChan{}, active: map[string]*activeChan{}, limit: newLimiter(),
		OpenWait: 15 * time.Second, Logf: log.Printf}, nil
}

// Handler: alle Wege des Protokolls.
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("POST "+PathPair, s.pairClient)
	m.HandleFunc("GET "+PathSystems, s.systems)
	m.HandleFunc("GET "+PathChannel, s.channel)
	m.HandleFunc("POST "+PathAnchorPair, s.pairAnchor)
	m.HandleFunc("GET "+PathAnchor, s.anchorCtrl)
	m.HandleFunc("GET "+PathAnchorChannel, s.anchorChannel)
	m.HandleFunc("POST "+PathCodes, s.codesCreate)
	m.HandleFunc("POST "+PathShare, s.shareCreate)
	m.HandleFunc("GET "+PathShareSend, s.shareSend)
	m.HandleFunc("GET "+PathShareWatch, s.shareWatch)
	webwatch.Register(m) // Zusehen im Browser
	m.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "sshTermy-Vermittler (Protokoll v1, Teilen v2)\n")
	})
	return m
}

// Watch trennt entfernte Geräte und Server (Änderung an state.json durch
// `sshtermy vermittler entfernen`), bis ctx endet.
func (s *Server) Watch(ctx context.Context) {
	t := time.NewTicker(recheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			s.reload()
			for id, l := range s.anchors {
				if _, ok := s.st.Anchors[id]; !ok {
					l.cancel()
					delete(s.anchors, id)
					s.log("anker_entfernt", 1)
				}
			}
			for _, a := range s.active {
				_, c := s.st.Clients[a.client]
				_, an := s.st.Anchors[a.anchor]
				if !c || !an {
					a.cancel()
				}
			}
			s.mu.Unlock()
		}
	}
}

// log: feste Wörter und Zahlen, nie Namen, Schlüssel oder Adressen.
func (s *Server) log(word string, n int) {
	if s.Logf != nil {
		s.Logf("vermittler: %s | %d", word, n)
	}
}

// reload liest state.json neu, wenn die Datei sich geändert hat (Aufrufer hält s.mu).
func (s *Server) reload() {
	if s.Dir == "" {
		return // eingebauter Vermittler ohne Datenordner
	}
	if m := stateMtime(s.Dir); !m.mod.Equal(s.stMod.mod) || m.size != s.stMod.size {
		if st, err := LoadState(s.Dir); err == nil {
			s.st, s.stMod = st, m
		}
	}
}

// update ändert den Zustand und speichert ihn (Aufrufer hält s.mu).
func (s *Server) save() error {
	if err := SaveState(s.Dir, s.st); err != nil {
		return err
	}
	s.stMod = stateMtime(s.Dir)
	return nil
}

func fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"fehler": msg})
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if k, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(k)
	}
	return ""
}

func (s *Server) ip(r *http.Request) string {
	if s.TrustProxy {
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			return strings.TrimSpace(strings.Split(f, ",")[0])
		}
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return false
	}
	return true
}

// peer prüft den Schlüssel einer Anfrage (Client oder Anker).
func (s *Server) peer(w http.ResponseWriter, r *http.Request, anchors bool) (string, bool) {
	ip := s.ip(r)
	if !s.limit.allow("auth:"+ip, 120, time.Minute) {
		fail(w, http.StatusTooManyRequests, "zu viele Anfragen")
		return "", false
	}
	s.mu.Lock()
	s.reload()
	m := s.st.Clients
	if anchors {
		m = s.st.Anchors
	}
	id, p := byKey(m, bearer(r))
	s.mu.Unlock()
	if p == nil {
		fail(w, http.StatusUnauthorized, "unbekannter Schlüssel")
		s.log("schluessel_falsch", 1)
		return "", false
	}
	return id, true
}

// ---------------------------------------------------------------- Kopplung

func (s *Server) pairGate(w http.ResponseWriter, r *http.Request) bool {
	if !s.limit.allow("pair:"+s.ip(r), 10, 10*time.Minute) || !s.limit.allow("pair", 60, 10*time.Minute) {
		fail(w, http.StatusTooManyRequests, "zu viele Versuche")
		s.log("koppeln_gebremst", 1)
		return false
	}
	return true
}

func (s *Server) pairClient(w http.ResponseWriter, r *http.Request) {
	if !s.pairGate(w, r) {
		return
	}
	var in struct{ Code, Name string }
	if !readJSON(w, r, &in) {
		return
	}
	kind, ok := takeCodeKind(s.Dir, in.Code, time.Now())
	if !ok || (kind != CodeClient && kind != CodeAdmin) {
		fail(w, http.StatusForbidden, "Code ungültig oder abgelaufen")
		s.log("koppeln_code_falsch", 1)
		return
	}
	key, sha := newKey()
	s.mu.Lock()
	s.reload()
	if len(s.st.Clients) >= MaxPeers {
		s.mu.Unlock()
		fail(w, http.StatusConflict, "zu viele Geräte")
		return
	}
	s.st.Clients[newID("c_")] = &Peer{Name: name(in.Name, "sshTermy"), KeySHA: sha, Since: time.Now().UTC(), Admin: kind == CodeAdmin}
	err := s.save()
	nm := s.st.Name
	s.mu.Unlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	s.log("koppeln_geraet", 1)
	reply(w, PairResult{Key: key, Name: nm, Kind: KindSSHTermy, Admin: kind == CodeAdmin})
}

func (s *Server) pairAnchor(w http.ResponseWriter, r *http.Request) {
	if !s.pairGate(w, r) {
		return
	}
	var in struct{ Code, Name, OS, HostKey string }
	if !readJSON(w, r, &in) {
		return
	}
	hk, ok := normHostKey(in.HostKey)
	if !ok {
		fail(w, http.StatusBadRequest, "Host-Key fehlt oder ist ungültig")
		return
	}
	if !takeCode(s.Dir, in.Code, CodeAnchor, time.Now()) {
		fail(w, http.StatusForbidden, "Code ungültig oder abgelaufen")
		s.log("koppeln_code_falsch", 1)
		return
	}
	key, sha := newKey()
	id := newID("a_")
	s.mu.Lock()
	s.reload()
	if len(s.st.Anchors) >= MaxPeers {
		s.mu.Unlock()
		fail(w, http.StatusConflict, "zu viele Server")
		return
	}
	s.st.Anchors[id] = &Peer{Name: name(in.Name, "Server"), OS: clean(in.OS, 60), HostKey: hk, KeySHA: sha, Since: time.Now().UTC()}
	err := s.save()
	s.mu.Unlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	s.log("koppeln_anker", 1)
	reply(w, map[string]string{"id": id, "key": key})
}

func name(s, def string) string {
	s = strings.TrimSpace(clean(s, 40))
	if s == "" {
		return def
	}
	return s
}

// normHostKey: genau ein öffentlicher SSH-Schlüssel im authorized_keys-Format.
func normHostKey(s string) (string, bool) {
	k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(s)))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k))), true
}

// ---------------------------------------------------------------- Liste

func (s *Server) systems(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.peer(w, r, false); !ok {
		return
	}
	s.mu.Lock()
	l := SystemList{Name: s.st.Name, Kind: KindSSHTermy, Systems: []System{}}
	for id, p := range s.st.Anchors {
		_, on := s.anchors[id]
		l.Systems = append(l.Systems, System{ID: id, Name: p.Name, OS: p.OS, Online: on, HostKey: p.HostKey, Folder: p.Folder})
	}
	s.mu.Unlock()
	sort.Slice(l.Systems, func(i, j int) bool { return strings.ToLower(l.Systems[i].Name) < strings.ToLower(l.Systems[j].Name) })
	reply(w, l)
}

// ---------------------------------------------------------------- Kanäle

func randID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// channel: sshTermy will zum sshd eines Systems. Der Vermittler bittet den
// Anker um einen Kanal und verbindet beide, sobald er da ist. Erst dann wird
// der WebSocket angenommen, damit Fehler als HTTP-Status ankommen.
func (s *Server) channel(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.peer(w, r, false)
	if !ok {
		return
	}
	sys := r.URL.Query().Get("system")
	s.mu.Lock()
	_, known := s.st.Anchors[sys]
	link := s.anchors[sys]
	full := len(s.active)+len(s.pending) >= maxChannels
	var p *pendingChan
	id := randID()
	if known && link != nil && !full {
		p = &pendingChan{anchor: sys, got: make(chan *websocket.Conn, 1), done: make(chan struct{})}
		s.pending[id] = p
	}
	s.mu.Unlock()
	switch {
	case !known:
		fail(w, http.StatusNotFound, "System unbekannt")
		return
	case link == nil:
		fail(w, http.StatusConflict, "System offline")
		s.log("kanal_offline", 1)
		return
	case full:
		fail(w, http.StatusTooManyRequests, "zu viele Kanäle")
		return
	}
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		p.finish()
	}()
	msg, _ := json.Marshal(map[string]string{"typ": "kanal", "id": id})
	wctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	err := link.ws.Write(wctx, websocket.MessageText, msg)
	cancel()
	if err != nil {
		fail(w, http.StatusConflict, "System offline")
		return
	}
	var aws *websocket.Conn
	select {
	case aws = <-p.got:
	case <-time.After(s.OpenWait):
		fail(w, http.StatusGatewayTimeout, "Anker hat keinen Kanal geöffnet (sshd erreichbar?)")
		s.log("kanal_zeit", 1)
		return
	case <-r.Context().Done():
		return
	}
	cws, err := websocket.Accept(w, r, nil)
	if err != nil {
		aws.Close(websocket.StatusGoingAway, "")
		return
	}
	ctx, stop := context.WithCancel(context.Background())
	s.mu.Lock()
	s.active[id] = &activeChan{client: cid, anchor: sys, cancel: stop}
	n := len(s.active)
	s.mu.Unlock()
	s.log("kanal_auf", n)
	splice(ctx, cws, aws)
	stop()
	s.mu.Lock()
	delete(s.active, id)
	n = len(s.active)
	s.mu.Unlock()
	s.log("kanal_zu", n)
}

// anchorChannel: der Anker liefert den erbetenen Kanal.
func (s *Server) anchorChannel(w http.ResponseWriter, r *http.Request) {
	aid, ok := s.peer(w, r, true)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	s.mu.Lock()
	p := s.pending[id]
	if p != nil && p.anchor == aid {
		delete(s.pending, id)
	} else {
		p = nil
	}
	s.mu.Unlock()
	if p == nil {
		fail(w, http.StatusNotFound, "kein solcher Kanal")
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	p.got <- ws // gepuffert: blockiert nie
	<-p.done
}

// anchorCtrl: Steuerleitung eines Ankers. Er meldet sich mit „hallo“ (OS,
// Host-Key) und bekommt „kanal“-Bitten. Eine neue Leitung ersetzt die alte.
func (s *Server) anchorCtrl(w http.ResponseWriter, r *http.Request) {
	aid, ok := s.peer(w, r, true)
	if !ok {
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(ctrlLimit)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	l := &anchorLink{ws: ws, cancel: cancel, since: time.Now()}
	s.mu.Lock()
	if old := s.anchors[aid]; old != nil {
		old.cancel()
	}
	s.anchors[aid] = l
	n := len(s.anchors)
	s.mu.Unlock()
	s.log("anker_auf", n)
	defer func() {
		s.mu.Lock()
		if s.anchors[aid] == l {
			delete(s.anchors, aid)
		}
		n := len(s.anchors)
		s.mu.Unlock()
		ws.Close(websocket.StatusNormalClosure, "")
		s.log("anker_zu", n)
	}()
	go func() {
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, c := context.WithTimeout(ctx, 15*time.Second)
				err := ws.Ping(pctx)
				c()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	for {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var m struct {
			Typ, OS, HostKey string
		}
		if json.Unmarshal(b, &m) != nil || m.Typ != "hallo" {
			continue
		}
		hk, okKey := normHostKey(m.HostKey)
		s.mu.Lock()
		s.reload()
		if p := s.st.Anchors[aid]; p != nil {
			changed := false
			if os := clean(m.OS, 60); os != "" && os != p.OS {
				p.OS, changed = os, true
			}
			// Ein neuer Host-Key ersetzt den alten nicht stillschweigend:
			// sshTermy prüft gegen den gemeldeten Schlüssel, und der darf sich
			// nur durch neues Koppeln ändern.
			if okKey && p.HostKey == "" {
				p.HostKey, changed = hk, true
			} else if okKey && hk != p.HostKey {
				s.log("anker_hostkey_anders", 1)
			}
			if changed {
				_ = s.save()
			}
		}
		s.mu.Unlock()
	}
}

// splice verbindet zwei WebSockets Byte für Byte, bis einer endet.
func splice(ctx context.Context, a, b *websocket.Conn) {
	a.SetReadLimit(-1)
	b.SetReadLimit(-1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ca := websocket.NetConn(ctx, a, websocket.MessageBinary)
	cb := websocket.NetConn(ctx, b, websocket.MessageBinary)
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(ca, cb); done <- struct{}{} }()
	go func() { _, _ = io.Copy(cb, ca); done <- struct{}{} }()
	select {
	case <-done:
	case <-ctx.Done():
	}
	_ = ca.Close()
	_ = cb.Close()
}

// ---------------------------------------------------------------- Ratenbremse

type limiter struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{m: map[string][]time.Time{}} }

// allow: höchstens n Ereignisse je Schlüssel im Zeitfenster.
func (l *limiter) allow(key string, n int, per time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.m) > 10000 {
		l.m = map[string][]time.Time{}
	}
	ts := l.m[key][:0]
	for _, t := range l.m[key] {
		if now.Sub(t) < per {
			ts = append(ts, t)
		}
	}
	if len(ts) >= n {
		l.m[key] = ts
		return false
	}
	l.m[key] = append(ts, now)
	return true
}

// codesCreate: ein Gerät mit Verwaltungsrecht holt eine Kopplungszeile für ein
// weiteres Gerät oder einen Server, ohne sich am Vermittler-Rechner anzumelden.
func (s *Server) codesCreate(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.peer(w, r, false)
	if !ok {
		return
	}
	var in struct {
		Art string `json:"art"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	s.mu.Lock()
	p := s.st.Clients[cid]
	admin := p != nil && p.Admin
	st := *s.st
	s.mu.Unlock()
	if !admin {
		fail(w, http.StatusForbidden, "dieses Gerät darf keine Codes holen (mit „vermittler code --verwalten“ koppeln)")
		return
	}
	kind := CodeClient
	switch in.Art {
	case CodeAnchor, CodeAdmin:
		kind = in.Art
	}
	code, err := NewCode(s.Dir, kind)
	if err != nil {
		fail(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	s.log("code_aus_sshtermy", 1)
	reply(w, map[string]string{"zeile": PairLine{Base: st.URL, Code: code, FP: st.FP}.String()})
}
