//go:build linux

package ui

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/db9979/sshtermy/internal/devfake"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/hostinfo"
)

// openPTY: Pseudo-Terminal als virtuelle serielle Leitung.
func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	// Nicht blockierend öffnen, damit Close ein laufendes Read beendet
	// (sonst hält Go die Leitung offen, bis das Read zurückkommt).
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skip("kein /dev/ptmx:", err)
	}
	m := os.NewFile(uintptr(fd), "/dev/ptmx")
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Skip("unlockpt:", err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Skip("ptsname:", err)
	}
	return m, "/dev/pts/" + strconv.Itoa(n)
}

// Serielle Konsole über ein Pseudo-Terminal: „Press any key“ wird vom ersten
// Enter erledigt, Befehle laufen, die Host-Übersicht funktioniert, ein
// verschwundenes Kabel führt zum Auto-Reconnect statt zum stillen Ende.
func TestSerialSession(t *testing.T) {
	a, st := rawTestApp(t)
	master, slave := openPTY(t)
	defer master.Close()
	sw := devfake.NewConsole()
	sw.PressAnyKey = true
	go sw.ServeConsole(master)

	closed := make(chan shellClosedMsg, 1)
	go func() {
		for m := range a.events {
			if c, ok := m.(shellClosedMsg); ok {
				closed <- c
			}
		}
	}()
	c, err := st.UpsertConnection(domain.Connection{Alias: "Aruba Konsole", Protocol: domain.ProtoSerial,
		SerialPort: slave, Baud: 9600})
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
	if s.state != stConnected || !s.netDev() {
		t.Fatal("nicht verbunden")
	}
	// Der Prompt muss stehen bleiben: nach „Press any key“ kann ein zweiter
	// Weck-Tastendruck noch eine Leerzeile und einen neuen Prompt auslösen,
	// dazwischen ist die Cursorzeile kurz leer.
	atPrompt := func() bool { return hostinfo.IsPrompt(s.term.CursorLine()) }
	waitFor(t, func() bool {
		if !atPrompt() {
			return false
		}
		time.Sleep(150 * time.Millisecond)
		return atPrompt()
	})
	cmd := a.hostNetCmd(s, true, false)
	if cmd == nil {
		t.Fatalf("keine Abfrage: %s", s.host.err)
	}
	a.Update(cmd())
	if s.host.err != "" || s.host.sw == nil {
		t.Fatalf("Übersicht über die Konsole: %q", s.host.err)
	}
	if v, _ := a.viewTerminal(); !strings.Contains(v, "⌁ pts/") {
		t.Fatal("Konsolen-Plakette fehlt")
	}
	if cmd := a.sendBreak(s); cmd != nil {
		t.Fatal("Break liefert Kommando")
	}

	// Kabel weg: Gegenseite schließen
	master.Close()
	select {
	case m := <-closed:
		if !m.dropped {
			t.Fatal("abgezogenes Kabel nicht als Abbruch erkannt")
		}
		a.Update(m)
		if s.state != stReconnect && s.state != stClosed {
			t.Fatalf("Zustand %v", s.state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("kein Ende nach Schließen der Leitung")
	}
}
