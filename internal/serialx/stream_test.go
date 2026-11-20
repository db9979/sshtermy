package serialx

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.bug.st/serial"
)

// fakePort: Konsolenkabel ohne Kabel. Liefert die Stücke aus in, danach
// je nach gone: (0, nil) wie ein abgezogenes Kabel unter Linux/macOS oder
// blockiert, bis Close kommt.
type fakePort struct {
	mu      sync.Mutex
	in      [][]byte
	readErr error
	gone    bool
	closed  chan struct{}
	written bytes.Buffer
	breaks  int
	dtr     bool
}

func newFake(chunks ...string) *fakePort {
	f := &fakePort{closed: make(chan struct{})}
	for _, c := range chunks {
		f.in = append(f.in, []byte(c))
	}
	return f
}

func (f *fakePort) Read(p []byte) (int, error) {
	f.mu.Lock()
	if len(f.in) > 0 {
		n := copy(p, f.in[0])
		f.in = f.in[1:]
		f.mu.Unlock()
		return n, nil
	}
	err, gone := f.readErr, f.gone
	f.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if gone {
		return 0, nil
	}
	<-f.closed
	return 0, errors.New("closed")
}
func (f *fakePort) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.Write(p)
}
func (f *fakePort) SetMode(*serial.Mode) error         { return nil }
func (f *fakePort) Drain() error                       { return nil }
func (f *fakePort) ResetInputBuffer() error            { return nil }
func (f *fakePort) ResetOutputBuffer() error           { return nil }
func (f *fakePort) SetDTR(on bool) error               { f.dtr = on; return nil }
func (f *fakePort) SetRTS(bool) error                  { return nil }
func (f *fakePort) SetReadTimeout(time.Duration) error { return nil }
func (f *fakePort) Break(time.Duration) error          { f.breaks++; return nil }
func (f *fakePort) Close() error                       { close(f.closed); return nil }
func (f *fakePort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func withFake(t *testing.T, f *fakePort) {
	t.Helper()
	old := openPort
	openPort = func(string, *serial.Mode) (serial.Port, error) { return f, nil }
	t.Cleanup(func() { openPort = old })
}

func waitDone(t *testing.T, s *Stream) error {
	t.Helper()
	select {
	case err := <-s.Done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Stream meldet kein Ende")
		return nil
	}
}

func TestStreamReadWriteClose(t *testing.T) {
	f := newFake("Switch login: ", "\r\n")
	withFake(t, f)
	var out syncBuf
	s, err := Open("/dev/ttyUSB0", 9600, "8N1", &out)
	if err != nil {
		t.Fatal(err)
	}
	if !f.dtr {
		t.Fatal("DTR nicht gesetzt (manche Adapter schweigen sonst)")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(out.String(), "login:") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(out.String(), "Switch login: ") {
		t.Fatalf("Ausgabe: %q", out.String())
	}
	if _, err := s.Write([]byte("admin\r")); err != nil || f.written.String() != "admin\r" {
		t.Fatalf("Write: %v %q", err, f.written.String())
	}
	if err := s.Break(); err != nil || f.breaks != 1 {
		t.Fatal("Break nicht gesendet")
	}
	if s.Resize(80, 24) != nil {
		t.Fatal("Resize")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s.Close() != nil {
		t.Fatal("zweites Close")
	}
	if err := waitDone(t, s); err != nil {
		t.Fatalf("selbst geschlossen muss nil melden: %v", err)
	}
}

// Kabel gezogen: Read liefert ständig 0 Bytes ohne Fehler.
func TestStreamCableGone(t *testing.T) {
	f := newFake("x")
	f.gone = true
	withFake(t, f)
	s, err := Open("/dev/ttyUSB0", 115200, "", &syncBuf{})
	if err != nil {
		t.Fatal(err)
	}
	if err := waitDone(t, s); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("abgezogenes Kabel: %v", err)
	}
}

func TestStreamReadError(t *testing.T) {
	f := newFake()
	f.readErr = errors.New("I/O error")
	withFake(t, f)
	s, err := Open("COM3", 9600, "8N1", &syncBuf{})
	if err != nil {
		t.Fatal(err)
	}
	if err := waitDone(t, s); err == nil || err.Error() != "I/O error" {
		t.Fatalf("Lesefehler: %v", err)
	}
}

func TestOpenErrors(t *testing.T) {
	if _, err := Open("COM3", 9600, "9Z9", &syncBuf{}); err == nil {
		t.Fatal("kaputter Rahmen angenommen")
	}
	old := openPort
	defer func() { openPort = old }()
	openPort = func(string, *serial.Mode) (serial.Port, error) { return nil, errors.New("kaputt") }
	if _, err := Open("COM9", 9600, "8N1", &syncBuf{}); err == nil || !strings.HasPrefix(err.Error(), "COM9: ") {
		t.Fatalf("Fehler ohne Schnittstellenname: %v", err)
	}
}

func TestPortNamesAndVendors(t *testing.T) {
	if (Port{Name: "COM3"}).String() != "COM3" || (Port{Name: "COM3", Desc: "FTDI"}).String() != "COM3 (FTDI)" {
		t.Fatal("Port.String")
	}
	for vid, want := range map[string]string{"0403": "FTDI", "067B": "Prolific", "10c4": "Silicon Labs", "1a86": "WCH", "05ad": "Cisco", "dead": "USB dead"} {
		if got := usbVendor(vid); got != want {
			t.Errorf("%s: %q", vid, got)
		}
	}
	e := &AmbiguousError{Ports: []Port{{Name: "COM3"}, {Name: "COM4"}}}
	if e.Error() != "several serial ports: COM3, COM4" {
		t.Fatal(e.Error())
	}
}
