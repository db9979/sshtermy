package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
)

// ownConsole: Windows hat für sshTermy eigens ein Konsolenfenster geöffnet
// (Doppelklick im Explorer, Startmenü) oder es gibt gar keins (Neustart nach
// Update aus dem Fenster). Aus PowerShell, cmd oder Windows Terminal hängen
// weitere Prozesse an der Konsole: dann bleibt es beim Terminal.
func ownConsole() bool {
	var list [4]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&list[0])), uintptr(len(list)))
	return n <= 1
}

// hideConsole schließt das leere schwarze Konsolenfenster, sobald das eigene
// Fenster übernimmt.
func hideConsole() {
	if ownConsole() {
		_, _, _ = procFreeConsole.Call()
	}
}
