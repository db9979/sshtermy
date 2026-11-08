// Package netwatch erkennt, dass der Rechner aus dem Ruhezustand kommt oder
// das Netz gewechselt hat (anderes WLAN, VPN an/aus, Kabel ab). Die
// Oberfläche prüft dann sofort alle Verbindungen, statt auf den
// Keepalive-Ablauf zu warten, und verbindet abgerissene neu.
//
// Ruhezustand: Die monotone Uhr (time.Since) steht während des Schlafs
// still, die Wanduhr läuft weiter. Läuft die Wanduhr deutlich weiter als
// die monotone, oder kommt ein Takt viel zu spät, hat der Rechner geschlafen.
//
// Netzwechsel: Die Menge der eigenen Adressen (ohne Loopback und
// Link-Local) ändert sich.
package netwatch

import (
	"net"
	"sort"
	"strings"
	"time"
)

// Event: was passiert ist.
type Event int

const (
	None Event = iota
	Resumed
	NetChanged
)

// Schwellen: so viel Abweichung gilt als Schlaf.
const (
	sleepGap = 10 * time.Second
	lateTick = 30 * time.Second
)

// Watcher merkt sich den letzten Stand. Nicht nebenläufig benutzen.
type Watcher struct {
	last  time.Time
	addrs string
	init  bool
	// Addrs liefert die Adressen (austauschbar für Tests).
	Addrs func() []string
}

// New: Watcher mit den echten Netzwerkadressen.
func New() *Watcher { return &Watcher{Addrs: LocalAddrs} }

// Check vergleicht mit dem letzten Aufruf. now muss von time.Now() kommen
// (mit monotoner Uhr). Online meldet, ob es überhaupt eine Adresse gibt.
func (w *Watcher) Check(now time.Time) (ev Event, online bool) {
	addrs := strings.Join(w.Addrs(), ",")
	online = addrs != ""
	if !w.init {
		w.init, w.last, w.addrs = true, now, addrs
		return None, online
	}
	mono := now.Sub(w.last)
	wall := now.Round(0).Sub(w.last.Round(0))
	w.last = now
	changed := addrs != w.addrs
	w.addrs = addrs
	switch {
	case wall-mono > sleepGap || mono > lateTick:
		return Resumed, online
	case changed:
		return NetChanged, online
	}
	return None, online
}

// Online: gibt es eine Adresse außer Loopback/Link-Local?
func (w *Watcher) Online() bool { return w.addrs != "" }

// LocalAddrs: eigene Adressen aktiver Schnittstellen, sortiert.
func LocalAddrs() []string {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range as {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.IsLoopback() || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ipn.IP.String())
		}
	}
	sort.Strings(out)
	return out
}
