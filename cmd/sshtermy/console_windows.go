package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procShowWindow            = user32.NewProc("ShowWindow")
)

// Das leere Konsolenfenster beim Doppelklick so früh wie möglich verstecken,
// noch vor Store und Schriften: sonst blitzt es sichtbar auf. Nur ohne
// Argumente (Explorer, Startmenü) bzw. mit -window; alles andere entscheidet
// main wie gewohnt.
func init() {
	if os.Getenv("SSHTERMY_WINDOW") == "0" {
		return
	}
	args := os.Args[1:]
	if len(args) > 1 || (len(args) == 1 && args[0] != "-window" && args[0] != "--window") {
		return
	}
	if ownConsole() {
		if h, _, _ := procGetConsoleWindow.Call(); h != 0 {
			const swHide = 0
			_, _, _ = procShowWindow.Call(h, swHide)
		}
	}
}

// ownConsole: Windows hat für sshTermy eigens ein Konsolenfenster geöffnet
// (Doppelklick im Explorer, Startmenü) oder es gibt gar keins (Neustart nach
// Update aus dem Fenster). Aus PowerShell, cmd oder Windows Terminal hängen
// weitere Prozesse an der Konsole: dann bleibt es beim Terminal.
func ownConsole() bool {
	var list [4]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&list[0])), uintptr(len(list)))
	return n <= 1
}

// hideConsole gibt das (schon versteckte) Konsolenfenster ganz frei, sobald
// das eigene Fenster übernimmt.
func hideConsole() {
	if ownConsole() {
		_, _, _ = procFreeConsole.Call()
	}
}
