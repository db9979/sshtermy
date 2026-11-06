package i18n

// Texte für Telnet und serielle Konsole (eigene Datei, damit parallele
// Änderungen an en.go nicht kollidieren).
var enConsole = map[string]string{
	"Aruba/HPE/Cisco: 9600 · viele neuere Geräte: 115200": "Aruba/HPE/Cisco: 9600 · many newer devices: 115200",
	"Baudrate ungültig":                                   "Invalid baud rate",
	"Break gesendet":                                      "Break sent",
	"Break gibt es nur an der seriellen Konsole":          "Break only exists on the serial console",
	"Break senden (serielle Konsole)":                     "Send break (serial console)",
	"Datenbits, Parität, Stoppbits · fast immer 8N1":      "Data bits, parity, stop bits · almost always 8N1",
	"Dateien und Tunnel gibt es nur bei SSH-Verbindungen": "Files and tunnels are only available on SSH connections",
	"Datenformat": "Data format",
	"Die Schnittstelle benutzt gerade ein anderes Programm (PuTTY, screen, minicom …).":         "Another program is using the port (PuTTY, screen, minicom …).",
	"Kabel eingesteckt? Sonst in der Verbindung eine andere Schnittstelle wählen.":              "Is the cable plugged in? Otherwise pick another port in the connection.",
	"Kein Konsolenkabel gefunden. Ist der USB-Adapter eingesteckt und der Treiber installiert?": "No console cable found. Is the USB adapter plugged in and its driver installed?",
	"Konsole":       "Console",
	"Konsolenkabel": "console cable",
	"Konsolenkabel an %s gefunden (gespeichert: %s)":                                                         "Console cable found at %s (saved: %s)",
	"Linux: Benutzer in die Gruppe „dialout“ aufnehmen (sudo usermod -aG dialout $USER), dann neu anmelden.": "Linux: add your user to the “dialout” group (sudo usermod -aG dialout $USER), then log in again.",
	"Mehrere Konsolenkabel gefunden (%s): bitte in der Verbindung eins auswählen":                            "Several console cables found (%s): please pick one in the connection",
	"Schnittstelle":           "Port",
	"Seriell (Konsolenkabel)": "Serial (console cable)",
	"Standard 22 · Telnet 23": "Default 22 · Telnet 23",
	"Telnet = unverschlüsselt (ältere Switches) · Seriell = USB-Konsolenkabel": "Telnet = unencrypted (older switches) · Serial = USB console cable",
	"Telnet/Konsole: optional, wird eingetippt, sobald das Gerät fragt":        "Telnet/console: optional, typed in as soon as the device asks",
	"Verbindung über": "Connect via",
	"automatisch = das einzige USB-Konsolenkabel (auch auf anderen Rechnern)": "automatic = the only USB console cable (also on other computers)",
	"automatisch, sobald das Gerät fragt":                                     "automatic, as soon as the device asks",
	"kein Kabel gefunden · automatisch = das einzige USB-Konsolenkabel":       "no cable found · automatic = the only USB console cable",
	"○ serielle Konsole, Kabel wird beim Verbinden gesucht":                   "○ serial console, cable is looked for when connecting",
	"telnet · unverschlüsselt":                                                "telnet · unencrypted",
	"von Hand am Gerät":                                                       "by hand on the device",
}

func init() {
	for k, v := range enConsole {
		en[k] = v
	}
}
