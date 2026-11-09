package hostinfo

import (
	"sort"
	"strings"
)

// Disk: ein Dateisystem des Servers (eine Zeile aus df).
type Disk struct {
	Mount       string // Einhängepunkt, z. B. /home
	Dev         string // Gerät, z. B. /dev/sda1
	Type        string // ext4, xfs, … ("" = unbekannt, z. B. macOS)
	Total, Used uint64 // Bytes
	Avail       uint64 // frei für normale Benutzer; 0 = unbekannt
}

// Pct: Belegung in Prozent wie bei df (belegt / (belegt + frei), aufgerundet),
// damit reservierte Blöcke nicht als frei zählen. Ohne „frei“: belegt / gesamt.
func (d Disk) Pct() int {
	if base := d.Used + d.Avail; d.Avail > 0 && base > 0 {
		return int((d.Used*100 + base - 1) / base)
	}
	if d.Total == 0 {
		return 0
	}
	return int(d.Used * 100 / d.Total)
}

// pseudoFS: Dateisysteme ohne eigene Platte (Arbeitsspeicher, Container,
// Kernel-Schnittstellen, Snap-Images). Sie füllen die Liste nur auf.
var pseudoFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "ramfs": true, "overlay": true, "overlayfs": true, "aufs": true,
	"squashfs": true, "proc": true, "sysfs": true, "cgroup": true, "cgroup2": true, "devpts": true,
	"devfs": true, "autofs": true, "efivarfs": true, "nsfs": true, "debugfs": true, "tracefs": true,
	"securityfs": true, "pstore": true, "bpf": true, "hugetlbfs": true, "mqueue": true, "configfs": true,
	"fusectl": true, "binfmt_misc": true, "rpc_pipefs": true, "nullfs": true, "fdescfs": true,
	"fuse.lxcfs": true, "fuse.snapfuse": true, "fuse.gvfsd-fuse": true, "fuse.portal": true,
	"shm": true, "none": true, "iso9660": true, "udf": true,
}

// pseudoMount: Einhängepunkte, unter denen nur Systeminternes liegt.
func pseudoMount(m string) bool {
	under := func(p string) bool { return m == p || strings.HasPrefix(m, p+"/") }
	switch {
	case under("/proc"), under("/sys"), under("/dev"), under("/snap"),
		under("/var/lib/docker"), under("/var/lib/containers"), under("/var/lib/kubelet"),
		under("/var/snap"), under("/private/var/vm"):
		return true
	case under("/run"):
		return !under("/run/media") && !under("/run/mount")
	case under("/System/Volumes"):
		return !under("/System/Volumes/Data") // macOS: VM, Preboot, Update, xarts …
	}
	return false
}

// parseFS liest eine fs=-Zeile aus ServerScript:
// "typ total_kb used_kb avail_kb gerät mount…".
func parseFS(v string) (Disk, bool) {
	f := strings.Fields(v)
	if len(f) < 6 {
		return Disk{}, false
	}
	d := Disk{Type: f[0], Total: kb(f[1]), Used: kb(f[2]), Avail: kb(f[3]), Dev: f[4]}
	if d.Type == "?" {
		d.Type = ""
	}
	// Einhängepunkt darf Leerzeichen enthalten: Rest nach dem fünften Feld.
	rest := strings.TrimSpace(v)
	for i := 0; i < 5; i++ {
		rest = strings.TrimSpace(rest[len(strings.Fields(rest)[0]):])
	}
	d.Mount = rest
	return d, d.Mount != ""
}

// realDisks: Pseudo-Dateisysteme weg, doppelt eingehängte Geräte (bind
// mounts) nur einmal (kürzester Pfad), "/" zuerst, dann nach Pfad.
func realDisks(in []Disk) []Disk {
	byDev := map[string]int{}
	var out []Disk
	for _, d := range in {
		if d.Total == 0 || pseudoFS[strings.ToLower(d.Type)] || pseudoFS[d.Dev] || strings.HasPrefix(d.Dev, "map ") || pseudoMount(d.Mount) {
			continue
		}
		if strings.HasPrefix(d.Dev, "/") {
			if i, ok := byDev[d.Dev]; ok {
				if len(d.Mount) < len(out[i].Mount) {
					out[i] = d
				}
				continue
			}
			byDev[d.Dev] = len(out)
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Mount == "/") != (out[j].Mount == "/") {
			return out[i].Mount == "/"
		}
		return out[i].Mount < out[j].Mount
	})
	return out
}
