// Package serialx öffnet serielle Konsolen (USB-Konsolenkabel für Switches,
// Router, Firewalls) unter Windows, macOS und Linux. Der Datenstrom geht roh
// an den Terminal-Emulator, genau wie bei einer SSH-Shell.
package serialx

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

// Port: eine gefundene serielle Schnittstelle.
type Port struct {
	Name string // COM3, /dev/ttyUSB0, /dev/cu.usbserial-1410
	Desc string // z. B. „FTDI FT232R“, leer wenn unbekannt
	USB  bool   // USB-Adapter (Konsolenkabel), nicht die eingebaute Schnittstelle
}

func (p Port) String() string {
	if p.Desc == "" {
		return p.Name
	}
	return p.Name + " (" + p.Desc + ")"
}

// List liefert die Schnittstellen dieses Rechners, USB-Kabel zuerst.
func List() []Port {
	ports := listPorts()
	sort.SliceStable(ports, func(i, j int) bool {
		if ports[i].USB != ports[j].USB {
			return ports[i].USB
		}
		return natLess(ports[i].Name, ports[j].Name)
	})
	return ports
}

// natLess sortiert COM2 vor COM10.
func natLess(a, b string) bool {
	if len(a) != len(b) && strings.TrimRight(a, "0123456789") == strings.TrimRight(b, "0123456789") {
		return len(a) < len(b)
	}
	return a < b
}

// ErrNoPort: kein Konsolenkabel eingesteckt.
var ErrNoPort = errors.New("no serial port found")

// AmbiguousError: „automatisch“, aber mehrere USB-Kabel stecken.
type AmbiguousError struct{ Ports []Port }

func (e *AmbiguousError) Error() string {
	n := make([]string, len(e.Ports))
	for i, p := range e.Ports {
		n[i] = p.Name
	}
	return "several serial ports: " + strings.Join(n, ", ")
}

// Resolve wählt die Schnittstelle: die gewünschte, wenn es sie gibt; leer
// (automatisch) oder nicht vorhanden heißt: das einzige USB-Kabel. So läuft
// eine synchronisierte Verbindung auch auf dem anderen Rechner, wo dasselbe
// Kabel COM5 statt /dev/cu.usbserial heißt.
func Resolve(want string, ports []Port) (name string, guessed bool, err error) {
	for _, p := range ports {
		if want != "" && strings.EqualFold(p.Name, want) {
			return p.Name, false, nil
		}
	}
	var usb []Port
	for _, p := range ports {
		if p.USB {
			usb = append(usb, p)
		}
	}
	switch {
	case len(usb) == 1:
		return usb[0].Name, want != "", nil
	case len(usb) > 1:
		return "", false, &AmbiguousError{usb}
	case want != "":
		// Nicht in der Liste (z. B. ein Pfad, den die Suche nicht kennt):
		// trotzdem versuchen, das Betriebssystem meldet den Fehler.
		return want, false, nil
	case len(ports) == 1:
		return ports[0].Name, true, nil
	}
	return "", false, ErrNoPort
}

// Mode baut die Einstellung aus Geschwindigkeit und Rahmen („8N1“, „7E1“ …).
func Mode(baud int, frame string) (*serial.Mode, error) {
	if baud <= 0 {
		baud = 9600
	}
	if frame == "" {
		frame = "8N1"
	}
	f := strings.ToUpper(strings.TrimSpace(frame))
	if len(f) < 3 {
		return nil, fmt.Errorf("invalid frame %q (e.g. 8N1)", frame)
	}
	m := &serial.Mode{BaudRate: baud}
	switch f[0] {
	case '5', '6', '7', '8':
		m.DataBits = int(f[0] - '0')
	default:
		return nil, fmt.Errorf("invalid data bits in %q", frame)
	}
	switch f[1] {
	case 'N':
		m.Parity = serial.NoParity
	case 'E':
		m.Parity = serial.EvenParity
	case 'O':
		m.Parity = serial.OddParity
	case 'M':
		m.Parity = serial.MarkParity
	case 'S':
		m.Parity = serial.SpaceParity
	default:
		return nil, fmt.Errorf("invalid parity in %q", frame)
	}
	switch f[2:] {
	case "1":
		m.StopBits = serial.OneStopBit
	case "1.5", "15":
		m.StopBits = serial.OnePointFiveStopBits
	case "2":
		m.StopBits = serial.TwoStopBits
	default:
		return nil, fmt.Errorf("invalid stop bits in %q", frame)
	}
	return m, nil
}

// Stream ist eine offene serielle Konsole.
type Stream struct {
	Name string     // tatsächlich geöffnete Schnittstelle
	Done chan error // nil = von uns geschlossen, sonst Fehler (Kabel gezogen)

	p      serial.Port
	mu     sync.Mutex
	closed atomic.Bool
}

// Open öffnet die Schnittstelle; alles, was ankommt, geht an out.
func Open(name string, baud int, frame string, out io.Writer) (*Stream, error) {
	mode, err := Mode(baud, frame)
	if err != nil {
		return nil, err
	}
	p, err := serial.Open(name, mode)
	if err != nil {
		return nil, explain(name, err)
	}
	// DTR/RTS an: manche Adapter (und Geräte) senden sonst nichts.
	_ = p.SetDTR(true)
	_ = p.SetRTS(true)
	s := &Stream{Name: name, Done: make(chan error, 1), p: p}
	go s.read(out)
	return s, nil
}

func (s *Stream) read(out io.Writer) {
	buf := make([]byte, 4096)
	idle := 0
	for {
		n, err := s.p.Read(buf)
		if n > 0 {
			idle = 0
			_, _ = out.Write(buf[:n])
		}
		if s.closed.Load() {
			s.Done <- nil
			return
		}
		if err != nil {
			s.Done <- err
			return
		}
		if n == 0 {
			// Read ohne Daten und ohne Fehler: unter Linux/macOS das Zeichen
			// für ein abgezogenes Kabel, wenn es sich ständig wiederholt.
			idle++
			if idle > 50 {
				s.Done <- errors.New("serial port gone")
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// Write schickt Tastendrücke an das Gerät (nebenläufig sicher).
func (s *Stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.Write(p)
}

// Resize: eine serielle Leitung kennt keine Fenstergröße.
func (s *Stream) Resize(cols, rows int) error { return nil }

// Break sendet ein Break-Signal (z. B. für die Passwort-Wiederherstellung
// oder um ein hängendes Gerät in den Monitor-Modus zu holen).
func (s *Stream) Break() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.Break(300 * time.Millisecond)
}

func (s *Stream) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	return s.p.Close()
}

// explain macht aus den Fehlern des Betriebssystems etwas Verständliches.
func explain(name string, err error) error {
	var pe *serial.PortError
	if errors.As(err, &pe) {
		switch pe.Code() {
		case serial.PortBusy:
			return fmt.Errorf("%s: %w", name, ErrBusy)
		case serial.PortNotFound:
			return fmt.Errorf("%s: %w", name, ErrNotFound)
		case serial.PermissionDenied:
			return fmt.Errorf("%s: %w", name, ErrPermission)
		}
	}
	return fmt.Errorf("%s: %w", name, err)
}

var (
	ErrBusy       = errors.New("port is in use by another program")
	ErrNotFound   = errors.New("port not found (cable plugged in?)")
	ErrPermission = errors.New("permission denied")
)
