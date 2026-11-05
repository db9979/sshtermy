// fakeswitch startet den nachgebauten ProCurve aus internal/devfake, z. B. für
// Screenshots der Host-Übersicht: go run ./tools/dev/fakeswitch 127.0.0.1:2323
package main

import (
	"log"
	"os"

	"github.com/db9979/sshtermy/internal/devfake"
)

func main() {
	addr := "127.0.0.1:2323"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	sw, err := devfake.StartSwitch(addr)
	if err != nil {
		log.Fatal(err)
	}
	sw.PressAnyKey = true
	log.Printf("Fake-ProCurve auf %s (Benutzer/Passwort beliebig, 'vlan <n>' ändert die Config)", sw.Addr)
	select {}
}
