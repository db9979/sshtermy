package ui

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/devfake"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/store"
)

func rawTestApp(t *testing.T) (*App, *store.Store) {
	t.Helper()
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	a, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	a.w, a.h, a.scr = 140, 40, scrTerminal
	return a, st
}

// Telnet zum nachgebauten ProCurve: Anmeldung läuft automatisch, Fenstergröße
// und Terminaltyp kommen an, die Host-Übersicht arbeitet über die Shell,
// logout beendet die Sitzung ohne Auto-Reconnect.
func TestTelnetSession(t *testing.T) {
	a, st := rawTestApp(t)
	sw, err := devfake.StartTelnetSwitch("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sw.Close()
	sw.PressAnyKey = true
	closed := make(chan shellClosedMsg, 1)
	go func() {
		for m := range a.events {
			if c, ok := m.(shellClosedMsg); ok {
				closed <- c
			}
		}
	}()
	host, port, _ := net.SplitHostPort(sw.Addr)
	p, _ := strconv.Atoi(port)
	c, err := st.UpsertConnection(domain.Connection{Alias: "sw-telnet", Host: host, Port: p,
		Protocol: domain.ProtoTelnet, Username: "manager", Password: "geheim"})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: 1, conn: c, chipArmed: -1}
	a.sessions = []*session{s}
	msg := a.connectCmd(s)()
	if e, ok := msg.(connectErrMsg); ok {
		t.Fatal(e.err)
	}
	a.Update(msg)
	if s.state != stConnected || s.client != nil || !s.netDev() {
		t.Fatalf("state %v client %v net %v", s.state, s.client, s.netDev())
	}
	waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })
	info := devfake.LastTelnet()
	if info.User != "manager" || info.Pass != "geheim" {
		t.Fatalf("Anmeldung: %+v", info)
	}
	if info.TermType != "XTERM" || info.Cols == 0 || info.Rows == 0 {
		t.Fatalf("Aushandlung: %+v", info)
	}

	// Host-Übersicht über die sichtbare Shell
	cmd := a.hostNetCmd(s, true, false)
	if cmd == nil {
		t.Fatalf("keine Abfrage: %s", s.host.err)
	}
	a.Update(cmd())
	if s.host.err != "" || s.host.sw == nil || s.host.sw.Firmware != "YA.16.10.0009" {
		t.Fatalf("Übersicht: %q %+v", s.host.err, s.host.sw)
	}

	// Ansicht und SSH-only-Funktionen dürfen nicht stolpern
	if v, _ := a.viewTerminal(); !strings.Contains(v, "telnet") {
		t.Fatal("Telnet-Plakette fehlt in der Statuszeile")
	}
	if a.toggleFiles(s); s.files.open {
		t.Fatal("Dateibrowser bei Telnet geöffnet")
	}
	_ = a.agentContext(s, true)

	waitFor(t, func() bool { return strings.HasSuffix(strings.TrimSpace(s.term.CursorLine()), "#") })
	_, _ = s.shell.Write([]byte("logout\r"))
	select {
	case m := <-closed:
		if m.dropped {
			t.Fatalf("logout als Abbruch gewertet: %v", m.err)
		}
		a.Update(m)
		if s.state != stClosed {
			t.Fatal("Sitzung nicht beendet")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Sitzung endet nicht")
	}
}

func TestParseQuickRaw(t *testing.T) {
	for _, c := range []struct {
		in    string
		proto domain.Protocol
		host  string
		port  int
		sport string
		baud  int
	}{
		{"telnet 10.0.0.5", domain.ProtoTelnet, "10.0.0.5", 23, "", 0},
		{"telnet sw1 2323", domain.ProtoTelnet, "sw1", 2323, "", 0},
		{"telnet://admin@sw1:24", domain.ProtoTelnet, "sw1", 24, "", 0},
		{"COM3", domain.ProtoSerial, "", 0, "COM3", 0},
		{"/dev/ttyUSB0 115200", domain.ProtoSerial, "", 0, "/dev/ttyUSB0", 115200},
		{"konsole", domain.ProtoSerial, "", 0, "", 0},
		{"console COM4 9600", domain.ProtoSerial, "", 0, "COM4", 9600},
	} {
		got, ok := parseQuickRaw(c.in)
		if !ok || got.Protocol != c.proto || got.Host != c.host || got.Port != c.port || got.SerialPort != c.sport || got.Baud != c.baud {
			t.Errorf("%q → %+v %v", c.in, got, ok)
		}
	}
	for _, in := range []string{"telnet", "ssh host", "user@host", "comet", "telnet a b c", "konsole wie geht das"} {
		if _, ok := parseQuickRaw(in); ok {
			t.Errorf("%q erkannt", in)
		}
	}
}

func TestConnFormSerial(t *testing.T) {
	a, st := rawTestApp(t)
	serialPorts = func() []serialxPort { return []serialxPort{{Name: "COM3", Desc: "FTDI", USB: true}} }
	defer func() { serialPorts = defaultSerialPorts }()
	a.openConnForm(newConnDefaults())
	f := a.form
	f.get("alias").in.SetValue("Aruba Konsole")
	setChoice(t, f, "proto", protoLabels[2])
	if fl := f.get("host"); fl.hidden == nil || !fl.hidden(f) {
		t.Fatal("Host bei seriell sichtbar")
	}
	if fl := f.get("auth"); !fl.hidden(f) {
		t.Fatal("SSH-Anmeldung bei seriell sichtbar")
	}
	if fl := f.get("password"); fl.hidden(f) {
		t.Fatal("Passwort für die automatische Anmeldung fehlt")
	}
	setChoice(t, f, "sport", "COM3")
	setChoice(t, f, "baud", "115200")
	if err := a.saveConn(); err != nil {
		t.Fatal(err)
	}
	cs := st.Connections()
	if len(cs) != 1 || cs[0].Protocol != domain.ProtoSerial || cs[0].SerialPort != "COM3" || cs[0].Baud != 115200 || cs[0].Frame != "8N1" {
		t.Fatalf("%+v", cs)
	}
	if got := cs[0].Addr(); got != "COM3 · 115200 8N1" {
		t.Fatalf("Addr %q", got)
	}

	// Telnet: Port 22 der Vorbelegung wird zu 23
	a.openConnForm(newConnDefaults())
	f = a.form
	f.get("alias").in.SetValue("alt")
	f.get("host").in.SetValue("10.0.0.9")
	setChoice(t, f, "proto", "Telnet")
	if err := a.saveConn(); err != nil {
		t.Fatal(err)
	}
	for _, c := range st.Connections() {
		if c.Alias == "alt" && (c.Port != 23 || c.Protocol != domain.ProtoTelnet) {
			t.Fatalf("%+v", c)
		}
	}
}

func setChoice(t *testing.T, f *form, key, val string) {
	t.Helper()
	fl := f.get(key)
	for i, o := range fl.options {
		if o == val {
			fl.opt = i
			return
		}
	}
	t.Fatalf("%s: keine Option %q in %v", key, val, fl.options)
}
