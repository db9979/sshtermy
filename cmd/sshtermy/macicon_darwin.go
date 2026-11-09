package main

import (
	"os"

	"github.com/db9979/sshtermy/assets/icon"
)

// fixMacAppIcon gibt einer per install.sh angelegten sshTermy.app ihr Icon,
// auch wenn sie vor dem Icon installiert wurde (Auto-Update tauscht nur die
// Programmdatei). Fehler sind egal: dann bleibt das Standard-Icon.
func fixMacAppIcon() {
	if exe, err := os.Executable(); err == nil {
		installMacIcon(exe, icon.ICNS)
	}
}
