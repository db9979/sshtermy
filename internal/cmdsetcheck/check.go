// Package cmdsetcheck prüft Befehlssätze gegen ihre Beispielausgaben
// (Selbsttest). Eigenes Paket, weil dafür Erkennung (cmdset), Leser
// (hostinfo) und Log-Leser (logscan) zusammenkommen.
package cmdsetcheck

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/logscan"
	_ "github.com/db9979/sshtermy/internal/safety" // Nur-lesen-Prüfung für cmdset
)

// SelfTest prüft alle Beispiele eines (aktiven) Satzes. Rückgabe: was nicht
// stimmt, leer = alles gut.
func SelfTest(s *cmdset.Set) []string {
	var fails []string
	for i, smp := range s.Samples {
		name := smp.Name
		if name == "" {
			name = fmt.Sprintf(i18n.T("Beispiel %d"), i+1)
		}
		bad := func(format string, a ...any) { fails = append(fails, name+": "+fmt.Sprintf(i18n.T(format), a...)) }
		if smp.Banner != "" && len(s.Detect.Banner) > 0 && !anyMatch(s.Detect.Banner, smp.Banner) {
			bad("SSH-Kennung wird nicht erkannt")
		}
		if smp.Screen != "" && len(s.Detect.Screen) > 0 && !anyMatch(s.Detect.Screen, smp.Screen) {
			bad("Bildschirmtext wird nicht erkannt")
		}
		if smp.Version != "" && len(s.Detect.Version)+len(s.Detect.Screen) > 0 && !anyMatch(append(append([]string(nil), s.Detect.Version...), s.Detect.Screen...), smp.Version) {
			bad("Versionsausgabe wird nicht erkannt")
		}
		eff := cmdset.Resolve(s.ID, "", "")
		if eff == nil {
			eff = s
		}
		e := smp.Expect
		if smp.Version != "" || smp.System != "" || smp.Ports != "" || smp.VLANs != "" {
			sw := hostinfo.ParseWith(eff, smp.Version, smp.System, smp.Ports, smp.VLANs)
			if v := cmdset.Resolve(s.ID, sw.Model, sw.Firmware); v != nil {
				sw = hostinfo.ParseWith(v, smp.Version, smp.System, smp.Ports, smp.VLANs)
			}
			str := func(what, want, got string) {
				if want != "" && strings.TrimSpace(want) != got {
					bad("%s: erwartet „%s“, gelesen „%s“", what, want, got)
				}
			}
			str("Modell", e.Model, sw.Model)
			str("Firmware", e.Firmware, sw.Firmware)
			str("Name", e.Name, sw.Name)
			str("Laufzeit", e.Uptime, sw.Uptime)
			n := func(what string, want *int, got int) {
				if want != nil && *want != got {
					bad("%s: erwartet %d, gelesen %d", what, *want, got)
				}
			}
			n("CPU", e.CPU, sw.CPUPct)
			n("RAM", e.Mem, sw.MemPct)
			n("Ports aktiv", e.PortsUp, sw.PortsUp)
			n("Ports inaktiv", e.PortsDown, sw.PortsDown)
			n("VLANs", e.VLANs, len(sw.VLANs))
			if e.Variant != "" {
				if got := cmdset.VariantName(s.ID, sw.Model, sw.Firmware); got != e.Variant {
					bad("Variante: erwartet „%s“, gewählt „%s“", e.Variant, got)
				}
			}
		}
		if smp.Logs != "" && e.LogLines != nil {
			if got := logscan.CountLines(eff.Logs.Reader, smp.Logs, time.Now()); got != *e.LogLines {
				bad("Log-Zeilen: erwartet %d, erkannt %d", *e.LogLines, got)
			}
		}
	}
	return fails
}

func anyMatch(pats []string, text string) bool {
	for _, p := range pats {
		if rx, err := regexp.Compile(p); err == nil && rx.MatchString(text) {
			return true
		}
	}
	return false
}
