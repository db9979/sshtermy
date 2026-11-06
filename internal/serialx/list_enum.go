//go:build !darwin

package serialx

import (
	"strings"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// listPorts: unter Windows und Linux mit USB-Angaben (Hersteller, Produkt).
func listPorts() []Port {
	det, err := enumerator.GetDetailedPortsList()
	if err != nil || len(det) == 0 {
		names, _ := serial.GetPortsList()
		out := make([]Port, 0, len(names))
		for _, n := range names {
			out = append(out, Port{Name: n, USB: looksUSB(n)})
		}
		return out
	}
	out := make([]Port, 0, len(det))
	for _, d := range det {
		p := Port{Name: d.Name, USB: d.IsUSB || looksUSB(d.Name)}
		if d.Product != "" {
			p.Desc = strings.TrimSpace(d.Product)
		} else if d.IsUSB && d.VID != "" {
			p.Desc = usbVendor(d.VID)
		}
		out = append(out, p)
	}
	return out
}

func looksUSB(name string) bool {
	return strings.Contains(name, "ttyUSB") || strings.Contains(name, "ttyACM")
}
