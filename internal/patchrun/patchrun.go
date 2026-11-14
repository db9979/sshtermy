// Package patchrun: Bausteine für die Patch-Runde über einen Ordner.
//
// Eine Runde spielt Updates gestaffelt ein: erst ein Host (Kanarienvogel),
// dann nach Freigabe der Rest, einer nach dem anderen. Vor und nach dem
// Update hält sie einen Zustand fest (laufende Dienste, lauschende Ports,
// fehlgeschlagene Units, Boot-Kennung) und vergleicht beide. Hier stehen nur
// die Skripte, das Lesen ihrer Ausgabe und der Vergleich; Verbinden und
// Ablauf macht die Oberfläche.
package patchrun

import (
	"fmt"
	"sort"
	"strings"
)

// Manager: Paketverwaltung auf dem Host.
type Manager string

const (
	MgrNone   Manager = ""
	MgrApt    Manager = "apt"
	MgrDnf    Manager = "dnf"
	MgrYum    Manager = "yum"
	MgrZypper Manager = "zypper"
	MgrApk    Manager = "apk"
	MgrMicro  Manager = "microdnf" // schlanke RHEL-Abbilder
)

// StateScript: liest den Zustand, nur lesend. Ausgabe Zeile für Zeile
// „schlüssel=wert“.
const StateScript = `export LC_ALL=C
[ "$(id -u)" = 0 ] && echo "root=1"
if command -v apt-get >/dev/null 2>&1; then echo "pm=apt"
elif command -v dnf >/dev/null 2>&1; then echo "pm=dnf"
elif command -v yum >/dev/null 2>&1; then echo "pm=yum"
elif command -v zypper >/dev/null 2>&1; then echo "pm=zypper"
elif command -v apk >/dev/null 2>&1; then echo "pm=apk"
elif command -v microdnf >/dev/null 2>&1; then echo "pm=microdnf"
fi
[ -r /proc/sys/kernel/random/boot_id ] && echo "boot=$(cat /proc/sys/kernel/random/boot_id)"
echo "kernel=$(uname -r)"
if command -v systemctl >/dev/null 2>&1; then
  systemctl list-units --type=service --state=running --no-legend --plain 2>/dev/null | awk '{print "svc="$1}'
  systemctl list-units --state=failed --no-legend --plain 2>/dev/null | awk '{print "fail="$1}'
elif command -v rc-status >/dev/null 2>&1; then
  rc-status -s 2>/dev/null | awk '/started/ {print "svc="$1}'
fi
if command -v ss >/dev/null 2>&1; then
  ss -ltnH 2>/dev/null | awk '{print "port="$4}'
elif command -v netstat >/dev/null 2>&1; then
  netstat -ltn 2>/dev/null | awk '/LISTEN/ {print "port="$4}'
fi
[ -f /var/run/reboot-required ] && echo "reboot=1"
if command -v needs-restarting >/dev/null 2>&1; then
  needs-restarting -r >/dev/null 2>&1; [ $? -eq 1 ] && echo "reboot=1"
fi
if command -v zypper >/dev/null 2>&1; then
  zypper -q ps -s 2>/dev/null | grep -qi "reboot is required\|neustart" && echo "reboot=1"
fi
if [ -f /etc/alpine-release ] && [ -d /lib/modules ]; then
  ls /lib/modules 2>/dev/null | grep -qx "$(uname -r)" || echo "reboot=1"
fi
exit 0
`

// State: Zustand eines Hosts zu einem Zeitpunkt.
type State struct {
	Root     bool
	Manager  Manager
	Boot     string
	Kernel   string
	Services []string // laufende Dienste
	Failed   []string // fehlgeschlagene Units
	Ports    []string // lauschende TCP-Ports „addr:port“
	Reboot   bool     // Neustart nötig
}

// ParseState liest die Ausgabe von StateScript.
func ParseState(out string) State {
	var s State
	seen := map[string]bool{}
	add := func(dst *[]string, k, v string) {
		if v == "" || seen[k+v] {
			return
		}
		seen[k+v] = true
		*dst = append(*dst, v)
	}
	for _, ln := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(ln), "=")
		if !ok {
			continue
		}
		switch k {
		case "root":
			s.Root = v == "1"
		case "pm":
			s.Manager = Manager(v)
		case "boot":
			s.Boot = v
		case "kernel":
			s.Kernel = v
		case "svc":
			add(&s.Services, k, v)
		case "fail":
			add(&s.Failed, k, v)
		case "port":
			add(&s.Ports, k, normPort(v))
		case "reboot":
			s.Reboot = s.Reboot || v == "1"
		}
	}
	sort.Strings(s.Services)
	sort.Strings(s.Failed)
	sort.Strings(s.Ports)
	return s
}

// normPort: „0.0.0.0:22“, „[::]:22“, „*:22“ → gleiche Schreibweise, damit
// IPv4/IPv6-Doppel nicht als zwei Ports zählen.
func normPort(v string) string {
	i := strings.LastIndexByte(v, ':')
	if i < 0 {
		return v
	}
	addr, port := v[:i], v[i+1:]
	switch strings.Trim(addr, "[]") {
	case "0.0.0.0", "::", "*", "":
		return "*:" + port
	}
	if strings.HasPrefix(addr, "127.") || addr == "[::1]" || addr == "::1" {
		return "lo:" + port
	}
	return addr + ":" + port
}

// Diff: was nach dem Update anders ist als vorher.
type Diff struct {
	StoppedServices []string // liefen vorher, jetzt nicht
	NewFailed       []string // jetzt fehlgeschlagen, vorher nicht
	ClosedPorts     []string // waren offen, jetzt nicht
	Rebooted        bool
}

// Problems: Zahl der Auffälligkeiten.
func (d Diff) Problems() int { return len(d.StoppedServices) + len(d.NewFailed) + len(d.ClosedPorts) }

// Compare vergleicht vorher und nachher.
func Compare(before, after State) Diff {
	d := Diff{Rebooted: before.Boot != "" && after.Boot != "" && before.Boot != after.Boot}
	d.StoppedServices = minus(before.Services, after.Services)
	d.NewFailed = minus(after.Failed, before.Failed)
	d.ClosedPorts = minus(before.Ports, after.Ports)
	return d
}

func minus(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}

// Scope: welche Updates eine Runde einspielt.
type Scope int

const (
	ScopeAll      Scope = iota // alle verfügbaren Updates
	ScopeSecurity              // nur Pakete aus dem Sicherheitscheck (bzw. dnf --security)
)

// UpdateScript: Skript, das als root läuft. pkgs nur bei ScopeSecurity
// (Binärpakete aus dem Sicherheitscheck). "" = nichts zu tun oder unbekannte
// Paketverwaltung.
func UpdateScript(m Manager, scope Scope, pkgs []string) string {
	q := quoteAll(pkgs)
	if scope == ScopeSecurity && len(pkgs) == 0 {
		return ""
	}
	switch m {
	case MgrApt:
		pre := "export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a LC_ALL=C\napt-get update -q || exit $?\n"
		opts := "-y -q -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold"
		if scope == ScopeSecurity {
			return pre + "apt-get install --only-upgrade " + opts + " " + q + "\n"
		}
		return pre + "apt-get upgrade " + opts + "\n"
	case MgrDnf, MgrYum, MgrMicro:
		if scope == ScopeSecurity {
			return "LC_ALL=C " + string(m) + " -y upgrade " + q + "\n"
		}
		return "LC_ALL=C " + string(m) + " -y upgrade\n"
	case MgrZypper:
		if scope == ScopeSecurity {
			return "LC_ALL=C zypper -n --non-interactive update " + q + "\n"
		}
		return "LC_ALL=C zypper -n --non-interactive update\n"
	case MgrApk:
		if scope == ScopeSecurity {
			return "apk update -q || exit $?\napk upgrade " + q + "\n"
		}
		return "apk update -q || exit $?\napk upgrade\n"
	}
	return ""
}

// RebootScript: Neustart in einer Sekunde, damit die Sitzung noch sauber
// endet.
const RebootScript = `(sleep 1; if command -v systemctl >/dev/null 2>&1; then systemctl reboot; else reboot; fi) >/dev/null 2>&1 &
exit 0
`

func quoteAll(ps []string) string {
	q := make([]string, 0, len(ps))
	for _, p := range ps {
		if p == "" || strings.ContainsAny(p, " \t\n") {
			continue
		}
		q = append(q, "'"+strings.ReplaceAll(p, "'", `'\''`)+"'")
	}
	return strings.Join(q, " ")
}

// Tail: die letzten n Zeilen (für den Bericht).
func Tail(out string, n int) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > n {
		lines = append([]string{fmt.Sprintf("… (%d Zeilen)", len(lines)-n)}, lines[len(lines)-n:]...)
	}
	return strings.Join(lines, "\n")
}

// Upgraded: Pakete, die laut Ausgabe aktualisiert wurden (grob, für den
// Bericht; 0 = unbekannt oder nichts).
func Upgraded(m Manager, out string) int {
	n := 0
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		switch m {
		case MgrApt:
			if strings.HasPrefix(ln, "Unpacking ") {
				n++
			}
		case MgrDnf, MgrYum:
			if strings.HasPrefix(ln, "Upgrading ") || strings.HasPrefix(ln, "Upgrade ") && strings.Contains(ln, "Package") {
				continue
			}
			if strings.HasPrefix(ln, "Upgraded:") {
				continue
			}
			if strings.HasPrefix(ln, "Verifying ") {
				n++
			}
		case MgrZypper:
			if strings.HasPrefix(ln, "(") && strings.Contains(ln, "Installing:") {
				n++
			}
		case MgrApk:
			if strings.HasPrefix(ln, "(") && strings.Contains(ln, "Upgrading ") {
				n++
			}
		}
	}
	if m == MgrDnf || m == MgrYum {
		n /= 2 // „Verifying“ je Paket für neu und alt
	}
	return n
}
