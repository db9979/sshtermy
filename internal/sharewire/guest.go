package sharewire

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Ablauf einer Freigabe v2 zwischen Gastgeber und Zuschauer (über einen
// Vermittler oder den eingebauten im lokalen Netz):
//
//	Zuschauer  A: CPace-Start                       → Gastgeber
//	Gastgeber  B: CPace-Antwort + Bestätigung       → Zuschauer (Code falsch? hier merkt er es)
//	Zuschauer  C: Bestätigung + Hallo (verschlüsselt)→ Gastgeber (Code falsch? Einladung gesperrt)
//	Gastgeber  zeigt „Anna möchte zusehen“ + Prüfwörter, wartet auf j/n
//	Gastgeber  K{ok, Freigabe-Schlüssel, Kennung}   → Zuschauer, dann „zulassen“ beim Vermittler
//	danach     S: Rahmen (Freigabe-Schlüssel) an alle; K: Steuerung je Zuschauer

// Stage meldet Zwischenschritte der Anmeldung (Browser-Seite zeigt sie an).
var Stage = func(string) {}

// Conn: Leitung des Zuschauers zum Gastgeber (WebSocket, Binärnachrichten).
type Conn interface {
	Send(p []byte) error
	Recv(ctx context.Context) ([]byte, error)
	Close() error
}

// Hello: wer zusehen will (verschlüsselt übertragen).
type Hello struct {
	Name    string `json:"name"`
	Rechner string `json:"rechner"`
	Version string `json:"version"`
	Browser bool   `json:"browser,omitempty"` // Zuschauer ohne sshTermy (Seite /zusehen)
}

// ShareInfo: was Zuschauer über die Freigabe erfahren (Rahmen FrameInfo).
type ShareInfo struct {
	Tab   string `json:"tab"`
	Von   string `json:"von"`
	Bis   int64  `json:"bis"`             // Unix-Zeit, Ende der Freigabe
	Pause bool   `json:"pause,omitempty"` // Passworteingabe läuft
}

// Ctl: Steuernachricht zwischen Gastgeber und einem Zuschauer.
type Ctl struct {
	T    string     `json:"t"`              // ok, nein, schreiben, hand, tasten
	Key  []byte     `json:"key,omitempty"`  // ok: Freigabe-Schlüssel
	ID   string     `json:"id,omitempty"`   // ok: Kennung (zusätzliche Daten der Rahmen)
	Info *ShareInfo `json:"info,omitempty"` // ok
	Bis  int64      `json:"bis,omitempty"`  // schreiben: bis wann (0 = entzogen)
	Data []byte     `json:"d,omitempty"`    // tasten
	Why  string     `json:"grund,omitempty"`
}

// ---------------------------------------------------------------- Zuschauer

// Watch2: zugelassener Zuschauer.
type Watch2 struct {
	g     Conn
	keys  PakeKeys
	key   []byte
	id    string
	Info  ShareInfo
	Check [3]string
	Base  string
}

// ErrLocked: Einladung schon benutzt, abgelaufen oder gesperrt.
var ErrLocked = i18n.NewError("Einladung schon benutzt, abgelaufen oder gesperrt – bitte beim Gastgeber einen neuen Code holen")

// ErrDenied: der Gastgeber hat abgelehnt (oder der Code war falsch).
var ErrDenied = i18n.NewError("Der Gastgeber hat abgelehnt")

// Join: anmelden, Code prüfen, auf Zulassung warten. checked meldet die
// Prüfwörter, sobald der Code stimmt (vor dem Warten).
func Join(ctx context.Context, g Conn, inv Invite, hello Hello, checked func([3]string)) (w *Watch2, err error) {
	defer func() {
		if err != nil {
			_ = g.Close() // auch bei falschem Code: der Gastgeber sperrt dann die Einladung
		}
	}()
	return join(ctx, g, inv, hello, checked)
}

func join(ctx context.Context, g Conn, inv Invite, hello Hello, checked func([3]string)) (*Watch2, error) {
	gp, msgA, err := StartGuest(inv)
	if err != nil {
		return nil, err
	}
	if err := g.Send(append([]byte{MsgPakeA}, msgA...)); err != nil {
		return nil, err
	}
	Stage("gesendet")
	w := &Watch2{g: g}
	for {
		b, err := g.Recv(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, i18n.NewError("Der Gastgeber hat nicht rechtzeitig zugelassen")
			}
			return nil, errEnded(err)
		}
		if len(b) == 0 {
			continue
		}
		switch b[0] {
		case MsgNo:
			if strings.Contains(string(b[1:]), "Code") {
				return nil, ErrWrongCode
			}
			if strings.Contains(string(b[1:]), "gesperrt") {
				return nil, ErrLocked
			}
			return nil, ErrDenied
		case MsgPakeB:
			msgB, tag, ok := SplitPakeB(b)
			if !ok {
				return nil, ErrWrongCode
			}
			k, err := gp.Finish(msgB, tag)
			if err != nil {
				return nil, err
			}
			w.keys, w.Check = k, k.Check
			hb, _ := json.Marshal(hello)
			sealed, err := k.SealCtl(hb)
			if err != nil {
				return nil, err
			}
			msg := append([]byte{MsgHello}, k.Guest...)
			if err := g.Send(append(msg, sealed...)); err != nil {
				return nil, err
			}
			if checked != nil {
				checked(k.Check)
			}
		case MsgCtl:
			if w.keys.Key == nil {
				continue
			}
			p, err := w.keys.OpenCtl(b[1:])
			if err != nil {
				continue
			}
			var c Ctl
			if json.Unmarshal(p, &c) != nil || c.T != "ok" || len(c.Key) != chacha20poly1305.KeySize || !IDRE.MatchString(c.ID) {
				continue
			}
			w.key, w.id = c.Key, c.ID
			if c.Info != nil {
				w.Info = *c.Info
			}
			return w, nil
		}
	}
}

func errEnded(error) error { return i18n.NewError("Freigabe beendet") }

// Next: nächster Rahmen (typ FrameData/FrameSize/FramePause/…) oder eine
// Steuernachricht (typ 'c', p = JSON Ctl). io.EOF = Gastgeber hat beendet.
func (w *Watch2) Next(ctx context.Context) (byte, []byte, error) {
	for {
		b, err := w.g.Recv(ctx)
		if err != nil {
			return 0, nil, io.EOF
		}
		if len(b) == 0 {
			continue
		}
		switch b[0] {
		case MsgFrame:
			t, p, err := OpenFrame(w.key, w.id, b)
			if err != nil {
				return 0, nil, err
			}
			if t == FrameEnd {
				return 0, nil, io.EOF
			}
			if t == FrameInfo {
				_ = json.Unmarshal(p, &w.Info)
			}
			return t, p, nil
		case MsgCtl:
			p, err := w.keys.OpenCtl(b[1:])
			if err == nil {
				return 'c', p, nil
			}
		case MsgNo:
			return 0, nil, io.EOF
		}
	}
}

// SendCtl: Hand heben, Tasten.
func (w *Watch2) SendCtl(c Ctl) error {
	b, _ := json.Marshal(c)
	f, err := w.keys.SealCtl(b)
	if err != nil {
		return err
	}
	return w.g.Send(append([]byte{MsgCtl}, f...))
}

// Close trennt.
func (w *Watch2) Close() error { return w.g.Close() }
