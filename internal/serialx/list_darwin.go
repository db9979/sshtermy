//go:build darwin

package serialx

import (
	"strings"

	"go.bug.st/serial"
)

// listPorts: unter macOS nur die cu.*-Geräte (tty.* wartet auf das
// Trägersignal und blockiert bei Konsolenkabeln). Die USB-Details bräuchten
// cgo; der Name verrät den Adapter aber meist.
func listPorts() []Port {
	names, _ := serial.GetPortsList()
	var out []Port
	for _, n := range names {
		base := strings.TrimPrefix(n, "/dev/")
		if !strings.HasPrefix(base, "cu.") {
			continue
		}
		l := strings.ToLower(base)
		if strings.Contains(l, "bluetooth") || strings.Contains(l, "debug-console") {
			continue
		}
		p := Port{Name: n}
		switch {
		case strings.Contains(l, "usbserial"), strings.Contains(l, "usbmodem"):
			p.USB = true
		case strings.Contains(l, "slab"):
			p.USB, p.Desc = true, "Silicon Labs CP210x"
		case strings.Contains(l, "wchusb"):
			p.USB, p.Desc = true, "WCH CH34x"
		case strings.Contains(l, "pl2303"):
			p.USB, p.Desc = true, "Prolific PL2303"
		}
		out = append(out, p)
	}
	return out
}
