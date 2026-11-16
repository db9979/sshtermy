//go:build js && wasm

// Zusehen im Browser (ohne sshTermy): dieselbe Anmeldung wie der
// Zuschauer-Tab in sshTermy (internal/sharewire), als WebAssembly. Die Seite
// (internal/webwatch/assets) liefert der Vermittler oder – im lokalen Netz –
// das sshTermy des Gastgebers selbst aus. Der Code bleibt im Browser: Er
// steht hinter „#“ und geht nie an den Server; Schlüssel entstehen per PAKE.
//
// Bauen: make zusehen (schreibt internal/webwatch/assets/zusehen.wasm.gz).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/coder/websocket"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sharewire"
)

type conn struct{ ws *websocket.Conn }

func (c *conn) Send(p []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageBinary, p)
}

func (c *conn) Recv(ctx context.Context) ([]byte, error) {
	for {
		typ, b, err := c.ws.Read(ctx)
		if err != nil {
			return nil, err
		}
		if typ == websocket.MessageBinary {
			return b, nil
		}
	}
}

// Close wartet nicht: ws.Close blockiert im Browser, bis das close-Ereignis
// kommt, und das bleibt bei manchen Fehlern (Zertifikat, Safari) ganz aus.
func (c *conn) Close() error {
	go func() { _ = c.ws.Close(websocket.StatusNormalClosure, "") }()
	return nil
}

// dial: wie websocket.Dial, aber bei Zeitüberschreitung ohne Warten auf das
// close-Ereignis (websocket.Dial bleibt dann sonst ewig in Close hängen).
func dial(ctx context.Context, url string, wait time.Duration) (*websocket.Conn, error) {
	type res struct {
		ws  *websocket.Conn
		err error
	}
	ch := make(chan res, 1)
	dctx, cancel := context.WithCancel(ctx)
	go func() {
		ws, _, err := websocket.Dial(dctx, url, nil)
		ch <- res{ws, err}
	}()
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case r := <-ch:
		cancel()
		return r.ws, r.err
	case <-t.C:
		cancel() // der Versuch räumt im Hintergrund auf
		return nil, context.DeadlineExceeded
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

// session: eine Anmeldung (die Seite kann nach einem Fehler neu beginnen).
type session struct {
	h      js.Value // Rückrufe der Seite
	w      *sharewire.Watch2
	cancel context.CancelFunc
}

var cur *session

func (s *session) call(name string, args ...any) {
	if f := s.h.Get(name); f.Type() == js.TypeFunction {
		f.Invoke(args...)
	}
}

func bytesJS(p []byte) js.Value {
	a := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(a, p)
	return a
}

func infoJSON(i sharewire.ShareInfo) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// join(opts, handlers): opts = {code, ws, name, rechner, lang}.
func join(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return "join(opts, handlers)"
	}
	o, h := args[0], args[1]
	if l := o.Get("lang").String(); strings.HasPrefix(strings.ToLower(l), "de") {
		i18n.Set(i18n.DE)
	} else {
		i18n.Set(i18n.EN)
	}
	inv, err := sharewire.ParseInvite(o.Get("code").String())
	if err != nil {
		return err.Error()
	}
	wsURL := o.Get("ws").String()
	if !strings.HasPrefix(wsURL, "wss://") {
		return i18n.T("Nur über https")
	}
	if cur != nil {
		cur.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{h: h, cancel: cancel}
	cur = s
	hello := sharewire.Hello{Name: sharewire.Clean(o.Get("name").String(), 40), Rechner: sharewire.Clean(o.Get("rechner").String(), 60),
		Version: "Browser", Browser: true}
	go s.run(ctx, wsURL+fmt.Sprintf("?platz=%d", inv.Platz), inv, hello)
	return nil
}

func (s *session) run(ctx context.Context, wsURL string, inv sharewire.Invite, hello sharewire.Hello) {
	fail := func(err error) {
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		s.call("end", msg)
	}
	s.call("stage", "dial")
	ws, err := dial(ctx, wsURL, 15*time.Second)
	if err != nil {
		// Häufigster Grund außer „beendet“: Safari lässt WebSockets zu selbst
		// ausgestellten Zertifikaten nicht zu, auch wenn die Seite erlaubt wurde.
		fail(errors.New(i18n.T("Keine Verbindung zur Freigabe (beendet, Code schon benutzt oder abgelaufen?). Safari: bitte Chrome, Edge oder Firefox nehmen – Safari erlaubt hier keine Verbindung zu selbst ausgestellten Zertifikaten.")))
		return
	}
	ws.SetReadLimit(1<<20 + 1024)
	c := &conn{ws}
	s.call("stage", "code")
	jctx, jcancel := context.WithTimeout(ctx, 4*time.Minute)
	// Antwortet der Gastgeber nicht binnen 20 s auf den Code, nicht ewig
	// warten. Die Uhr läuft erst, wenn der Code gesendet ist: Ohne JIT (Edge
	// „Sicherheit im Web erhöhen“) rechnet der Browser vorher lange.
	var checked atomic.Bool
	var quiet *time.Timer
	sharewire.Stage = func(st string) {
		s.call("stage", st)
		if st == "gesendet" {
			quiet = time.AfterFunc(20*time.Second, func() {
				if !checked.Load() {
					jcancel()
				}
			})
		}
	}
	w, err := sharewire.Join(jctx, c, inv, hello, func(chk [3]string) {
		checked.Store(true)
		s.call("check", strings.ToUpper(strings.Join(chk[:], " · ")))
	})
	if quiet != nil {
		quiet.Stop()
	}
	jcancel()
	if err != nil {
		if !checked.Load() && ctx.Err() == nil {
			err = errors.New(i18n.T("Der Gastgeber antwortet nicht auf den Code (sshTermy beim Gastgeber noch offen und aktuell?)"))
		}
		fail(err)
		return
	}
	s.w = w
	s.call("admitted", infoJSON(w.Info))
	for {
		typ, p, err := w.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			fail(err)
			return
		}
		switch typ {
		case sharewire.FrameData:
			s.call("data", bytesJS(p))
		case sharewire.FrameSize:
			var c, r int
			if _, err := fmt.Sscanf(string(p), "%d %d", &c, &r); err == nil && c >= 10 && r >= 3 && c <= 1000 && r <= 500 {
				s.call("size", c, r)
			}
		case sharewire.FramePause:
			s.call("pause")
		case sharewire.FrameResume:
			s.call("resume")
		case sharewire.FrameInfo:
			s.call("info", infoJSON(w.Info))
		case 'c':
			var ctl sharewire.Ctl
			if json.Unmarshal(p, &ctl) == nil && ctl.T == "schreiben" {
				s.call("write", float64(ctl.Bis))
			}
		}
	}
}

// keys(Uint8Array): Tasten (gehen nur an, solange Mitschreiben erlaubt ist;
// das prüft der Gastgeber).
func keys(_ js.Value, args []js.Value) any {
	if cur == nil || cur.w == nil || len(args) < 1 {
		return nil
	}
	b := make([]byte, args[0].Get("length").Int())
	js.CopyBytesToGo(b, args[0])
	w := cur.w
	go func() { _ = w.SendCtl(sharewire.Ctl{T: "tasten", Data: b}) }()
	return nil
}

// hand(): um Mitschreiben bitten.
func hand(_ js.Value, _ []js.Value) any {
	if cur == nil || cur.w == nil {
		return i18n.T("noch nicht zugelassen")
	}
	w := cur.w
	go func() { _ = w.SendCtl(sharewire.Ctl{T: "hand"}) }()
	return nil
}

// leave(): trennen.
func leave(_ js.Value, _ []js.Value) any {
	if cur != nil {
		cur.cancel()
		if w := cur.w; w != nil {
			go func() { _ = w.Close() }()
		}
	}
	return nil
}

// codeOK(text): leerer Text = Code lesbar, sonst der Grund.
func codeOK(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return ""
	}
	if _, err := sharewire.ParseInvite(args[0].String()); err != nil {
		return err.Error()
	}
	return ""
}

func main() {
	api := js.Global().Get("Object").New()
	api.Set("join", js.FuncOf(join))
	api.Set("keys", js.FuncOf(keys))
	api.Set("hand", js.FuncOf(hand))
	api.Set("leave", js.FuncOf(leave))
	api.Set("codeOK", js.FuncOf(codeOK))
	js.Global().Set("sshtermy", api)
	if f := js.Global().Get("sshtermyReady"); f.Type() == js.TypeFunction {
		f.Invoke()
	}
	select {}
}
