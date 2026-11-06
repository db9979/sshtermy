package serialx

import (
	"errors"
	"testing"

	"go.bug.st/serial"
)

func TestResolve(t *testing.T) {
	one := []Port{{Name: "COM1"}, {Name: "COM5", USB: true}}
	if n, g, err := Resolve("", one); n != "COM5" || g || err != nil {
		t.Fatalf("auto: %s %v %v", n, g, err)
	}
	if n, g, _ := Resolve("/dev/ttyUSB0", one); n != "COM5" || !g {
		t.Fatalf("anderer Rechner: %s %v", n, g)
	}
	if n, g, _ := Resolve("com1", one); n != "COM1" || g {
		t.Fatalf("gewählt: %s %v", n, g)
	}
	two := append(one, Port{Name: "COM7", USB: true})
	var amb *AmbiguousError
	if _, _, err := Resolve("", two); !errors.As(err, &amb) || len(amb.Ports) != 2 {
		t.Fatalf("mehrdeutig: %v", err)
	}
	if _, _, err := Resolve("", nil); !errors.Is(err, ErrNoPort) {
		t.Fatalf("leer: %v", err)
	}
	if n, _, _ := Resolve("/dev/pts/3", nil); n != "/dev/pts/3" {
		t.Fatal("unbekannter Pfad nicht versucht")
	}
}

func TestMode(t *testing.T) {
	m, err := Mode(0, "7E1")
	if err != nil || m.BaudRate != 9600 || m.DataBits != 7 || m.Parity != serial.EvenParity || m.StopBits != serial.OneStopBit {
		t.Fatalf("%+v %v", m, err)
	}
	if m, _ := Mode(115200, "8N2"); m.StopBits != serial.TwoStopBits || m.BaudRate != 115200 {
		t.Fatalf("%+v", m)
	}
	for _, bad := range []string{"9N1", "8X1", "8N3", "N"} {
		if _, err := Mode(9600, bad); err == nil {
			t.Errorf("%q angenommen", bad)
		}
	}
}

func TestList(t *testing.T) {
	_ = List() // darf auf keinem System abstürzen
	if !natLess("COM2", "COM10") {
		t.Fatal("Sortierung")
	}
}
