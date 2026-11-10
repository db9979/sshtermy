package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
)

func TestExplainConnErr(t *testing.T) {
	c := domain.Connection{Host: "sw1", Port: 22, Username: "admin"}
	for raw, want := range map[string]string{
		"ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain": "auth",
		"dial tcp: lookup sw1: no such host":                                                         "dns",
		"dial tcp 10.0.0.1:22: i/o timeout":                                                          "timeout",
		"dial tcp 10.0.0.1:22: connect: connection refused":                                          "refused",
		"dial tcp 10.0.0.1:22: connect: no route to host":                                            "unreach",
		"ssh: handshake failed: read tcp 10.0.0.2:5000->10.0.0.1:22: read: connection reset by peer": "reset",
		"ssh: handshake failed: ssh: no common algorithm for key exchange":                           "algo",
		"etwas ganz anderes": "other",
	} {
		if got := explainConnErr(errors.New(raw), c).kind; got != want {
			t.Errorf("%q: %s, erwartet %s", raw, got, want)
		}
	}
	hc := &sshx.HostKeyChangedError{Host: "sw1:22"}
	if p := explainConnErr(fmt.Errorf("ssh: handshake failed: %w", hc), c); p.kind != "hostkey" || len(p.keys) == 0 || p.keys[0][0] != "k" {
		t.Fatalf("Host-Key: %+v", p)
	}
	if p := explainConnErr(context.Canceled, c); p.kind != "cancel" {
		t.Fatalf("Abbruch: %+v", p)
	}
	ser := domain.Connection{Protocol: domain.ProtoSerial}
	if p := explainConnErr(errors.New("open /dev/ttyUSB0: permission denied"), ser); p.kind != "serial" {
		t.Fatalf("Seriell: %+v", p)
	}
}

func TestClosedViewShowsPlainText(t *testing.T) {
	a := tabsApp(1)
	s := a.sessions[0]
	s.conn.Host, s.conn.Port = "sw1", 22
	s.errv = errors.New("dial tcp 10.0.0.1:22: i/o timeout")
	s.err = s.errv.Error()
	v := ansi.Strip(a.closedView(s, 100, 20))
	for _, want := range []string{"Keine Antwort von sw1:22", "Jump-Host", "i/o timeout", "bearbeiten", "neu verbinden"} {
		if !strings.Contains(v, want) {
			t.Errorf("fehlt %q in\n%s", want, v)
		}
	}
}

func TestQuitAsksWithOpenConnections(t *testing.T) {
	a := tabsApp(2)
	a.scr = scrTerminal
	a.sessions[1].state = stConnected
	a.askQuit()
	if a.modal == nil || !a.modal.quit || !strings.Contains(a.modal.body, "host-b") || strings.Contains(a.modal.body, "host-a") {
		t.Fatalf("Rückfrage fehlt oder falsch: %+v", a.modal)
	}
}

func TestUndoToastAndExpire(t *testing.T) {
	a := tabsApp(0)
	done := false
	a.offerUndo("„x“ gelöscht", nil, func() { done = true }, nil)
	if a.undo == nil || len(a.toasts) != 1 || !a.toasts[0].undo || !strings.Contains(a.toasts[0].text, "Strg+Z") {
		t.Fatalf("Hinweis fehlt: %+v", a.toasts)
	}
	a.expireUndo(time.Now())
	if done || a.undo == nil {
		t.Fatal("zu früh endgültig")
	}
	a.expireUndo(time.Now().Add(undoLife + time.Second))
	if !done || a.undo != nil || len(a.toasts) != 0 {
		t.Fatal("nach Ablauf nicht endgültig")
	}
}

func TestConnTestResult(t *testing.T) {
	a := tabsApp(0)
	c := domain.Connection{Host: "srv", Port: 22, Username: "root"}
	a.connTestResult(connTestMsg{conn: c, step: sshx.StepAuth, err: errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]")})
	if a.modal == nil || !strings.Contains(a.modal.body, "✓ Erreichbar") || !strings.Contains(a.modal.body, "× Anmeldung") || !strings.Contains(a.modal.body, "Anmeldung abgelehnt") {
		t.Fatalf("Ergebnis falsch: %+v", a.modal)
	}
	a.connTestResult(connTestMsg{conn: c})
	if !strings.Contains(a.modal.body, "✓ Anmeldung") || a.modal.title != "Test erfolgreich" {
		t.Fatalf("Erfolg falsch: %+v", a.modal)
	}
}

func TestSimpleHidesProKeys(t *testing.T) {
	a := tabsApp(0)
	keys := [][2]string{{"n", "neu"}, {a.keys.label(actBroadcast), "Broadcast"}, {"t", "Tunnel"}, {a.keys.label(actHelp), "Hilfe"}}
	if got := a.simpleKeys(keys); len(got) != len(keys) {
		t.Fatal("ohne Store darf nichts fehlen")
	}
	items := a.simpleHide([]palItem{{label: "x", area: "multi"}, {label: "y", pro: true}, {label: "z"}})
	if items[0].hidden || items[1].hidden {
		t.Fatal("einfacher Modus ist aus")
	}
}

func TestSimpleModeWithStore(t *testing.T) {
	a := dangerApp(t)
	a.scr = scrManager
	a.toggleSimple()
	if !a.simple() {
		t.Fatal("einfacher Modus nicht an")
	}
	keys := a.simpleKeys([][2]string{{"n", "neu"}, {a.keys.label(actBroadcast), "Broadcast"}, {"t", "Tunnel"}})
	if len(keys) != 1 {
		t.Fatalf("Profi-Tasten bleiben: %v", keys)
	}
	for _, it := range a.palMenuItems() {
		if strings.HasPrefix(it.label, "MCP") && !it.hidden {
			t.Fatal("MCP im einfachen Modus sichtbar")
		}
	}
	// Undo im Manager: Verbindung löschen und zurückholen
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "h", Username: "u", Auth: domain.AuthAgent})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := a.st.DeleteConnectionTrash(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	a.offerUndo("weg", tr, nil, nil)
	a.doUndo()
	if _, ok := a.st.Connection(c.ID); !ok {
		t.Fatal("nicht wiederhergestellt")
	}
}
