// fakeswitch startet den nachgebauten ProCurve aus internal/devfake, z. B. für
// Screenshots der Host-Übersicht: go run ./tools/dev/fakeswitch 127.0.0.1:2323
// Mit -telnet lauscht er per Telnet statt SSH (Benutzer/Passwort beliebig).
package main

import (
	"flag"
	"log"

	"github.com/db9979/sshtermy/internal/devfake"
)

func main() {
	tel := flag.Bool("telnet", false, "Telnet statt SSH")
	flag.Parse()
	addr := "127.0.0.1:2323"
	if flag.NArg() > 0 {
		addr = flag.Arg(0)
	}
	start := devfake.StartSwitch
	if *tel {
		start = devfake.StartTelnetSwitch
	}
	sw, err := start(addr)
	if err != nil {
		log.Fatal(err)
	}
	sw.PressAnyKey = true
	log.Printf("Fake-ProCurve auf %s (Telnet: %v, Benutzer/Passwort beliebig, 'vlan <n>' ändert die Config)", sw.Addr, *tel)
	select {}
}
