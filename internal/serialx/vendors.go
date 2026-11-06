package serialx

import "strings"

// usbVendor: Hersteller der verbreiteten Konsolenkabel-Chips.
func usbVendor(vid string) string {
	switch strings.ToLower(vid) {
	case "0403":
		return "FTDI"
	case "067b":
		return "Prolific"
	case "10c4":
		return "Silicon Labs"
	case "1a86":
		return "WCH"
	case "05ad":
		return "Cisco"
	}
	return "USB " + vid
}
