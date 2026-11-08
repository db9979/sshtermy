// Package hostinfo sammelt Kennzahlen eines Hosts für die Host-Übersicht (F6):
// bei Servern CPU, RAM, Platte, Laufzeit und offene Updates (ein kleines
// sh-Skript in einer Exec-Sitzung), bei Netzwerkgeräten Firmware, Ports und
// VLANs (show-Befehle über die interaktive Shell, siehe switch.go). Dazu
// kommen das Säubern mitgeschnittener Shell-Ausgabe und der Zeilen-Diff für
// die Config-Sicherung. Das Paket kennt weder SSH noch die Oberfläche.
package hostinfo

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// ServerScript läuft per "sh -s" (Skript über stdin), damit es auch bei fish
// oder csh als Login-Shell funktioniert. Jede Zeile ist key=wert. Alles ist
// nur lesend; Paketlisten werden nicht aktualisiert (kein apt update).
const ServerScript = `LC_ALL=C; export LC_ALL
T=""; command -v timeout >/dev/null 2>&1 && T="timeout 20"
echo "cpus=$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null)"
if [ -r /proc/loadavg ]; then echo "load=$(cut -d' ' -f1-3 /proc/loadavg)"; else echo "load=$(sysctl -n vm.loadavg 2>/dev/null | tr -d '{}')"; fi
if [ -r /proc/meminfo ]; then awk '/^MemTotal:/{t=$2}/^MemAvailable:/{a=$2}END{print "mem_kb="t" "a}' /proc/meminfo
else
  echo "mem_b=$(sysctl -n hw.memsize 2>/dev/null)"
  ps=$(sysctl -n hw.pagesize 2>/dev/null)
  vm_stat 2>/dev/null | awk -v ps="$ps" '/Pages free/{f=$3}/Pages inactive/{i=$3}/Pages speculative/{s=$3}END{gsub(/\./,"",f);gsub(/\./,"",i);gsub(/\./,"",s);if(ps!="")print "memfree_b="(f+i+s)*ps}'
fi
if [ -r /proc/stat ]; then echo "stat1=$(head -n 1 /proc/stat)"; sleep 1; echo "stat2=$(head -n 1 /proc/stat)"
else echo "cpu_top=$(top -l 2 -n 0 -s 1 2>/dev/null | grep 'CPU usage' | tail -n 1)"; fi
df -Pk / 2>/dev/null | awk 'NR==2{print "disk_kb="$2" "$3" "$4}'
if [ -r /proc/uptime ]; then echo "uptime_s=$(cut -d' ' -f1 /proc/uptime)"; else echo "boot=$(sysctl -n kern.boottime 2>/dev/null | sed 's/^[^0-9]*sec = \([0-9]*\).*/\1/')"; fi
echo "now=$(date +%s)"
if command -v apt-get >/dev/null 2>&1; then
  u=$($T apt-get -s -o Debug::NoLocking=1 upgrade 2>/dev/null | grep '^Inst ')
  echo "upd=apt $(printf '%s\n' "$u" | grep -c '^Inst') $(printf '%s\n' "$u" | grep -ci 'security')"
elif command -v dnf >/dev/null 2>&1; then
  u=$($T dnf -q -C check-update 2>/dev/null); rc=$?
  if [ $rc = 1 ]; then echo "upd=dnf ? ?"; else echo "upd=dnf $(printf '%s\n' "$u" | grep -c '^[[:alnum:]]') ?"; fi
elif command -v yum >/dev/null 2>&1; then
  u=$($T yum -q -C check-update 2>/dev/null); rc=$?
  if [ $rc = 1 ]; then echo "upd=yum ? ?"; else echo "upd=yum $(printf '%s\n' "$u" | grep -c '^[[:alnum:]]') ?"; fi
elif command -v apk >/dev/null 2>&1; then
  echo "upd=apk $($T apk list -u 2>/dev/null | grep -c .) ?"
elif command -v pacman >/dev/null 2>&1; then
  echo "upd=pacman $(pacman -Qu 2>/dev/null | grep -c .) ?"
elif command -v softwareupdate >/dev/null 2>&1; then
  echo "upd=softwareupdate ? ?"
fi
[ -f /var/run/reboot-required ] && echo "reboot=1"
exit 0
`

// Server: Kennzahlen eines Servers. Unbekannte Werte sind 0 bzw. -1.
type Server struct {
	CPUs              int
	CPUPct            int // Auslastung über eine Sekunde, -1 = unbekannt
	Load              [3]float64
	HasLoad           bool
	MemTotal, MemFree uint64 // Bytes (frei = verfügbar)
	DiskTotal         uint64 // Bytes, Dateisystem /
	DiskUsed          uint64
	Uptime            time.Duration
	Updates           int    // -1 = unbekannt
	Security          int    // -1 = unbekannt
	UpdateTool        string // apt, dnf, …
	RebootNeeded      bool
	At                time.Time
}

// ParseServer liest die Ausgabe von ServerScript.
func ParseServer(out string) (Server, error) {
	s := Server{Updates: -1, Security: -1, CPUPct: -1, At: time.Now()}
	var boot, now int64
	var stat1, stat2 string
	seen := 0
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		seen++
		switch k {
		case "cpus":
			s.CPUs, _ = strconv.Atoi(strings.TrimSpace(v))
		case "load":
			for i := 0; i < 3 && i < len(f); i++ {
				x, err := strconv.ParseFloat(strings.ReplaceAll(f[i], ",", "."), 64)
				if err == nil {
					s.Load[i], s.HasLoad = x, true
				}
			}
		case "stat1":
			stat1 = v
		case "stat2":
			stat2 = v
		case "cpu_top":
			if m := topIdle.FindStringSubmatch(v); m != nil {
				if idle, err := strconv.ParseFloat(m[1], 64); err == nil && idle >= 0 && idle <= 100 {
					s.CPUPct = int(100 - idle + 0.5)
				}
			}
		case "mem_kb":
			if len(f) == 2 {
				s.MemTotal, s.MemFree = kb(f[0]), kb(f[1])
			}
		case "mem_b":
			s.MemTotal = num(v)
		case "memfree_b":
			s.MemFree = num(v)
		case "disk_kb":
			if len(f) >= 2 {
				s.DiskTotal, s.DiskUsed = kb(f[0]), kb(f[1])
			}
		case "uptime_s":
			if x, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				s.Uptime = time.Duration(x) * time.Second
			}
		case "boot":
			boot = int64(num(v))
		case "now":
			now = int64(num(v))
		case "upd":
			if len(f) >= 1 {
				s.UpdateTool = f[0]
			}
			if len(f) >= 2 {
				if n, err := strconv.Atoi(f[1]); err == nil {
					s.Updates = n
				}
			}
			if len(f) >= 3 {
				if n, err := strconv.Atoi(f[2]); err == nil {
					s.Security = n
				}
			}
		case "reboot":
			s.RebootNeeded = true
		}
	}
	if p, ok := statPct(stat1, stat2); ok {
		s.CPUPct = p
	}
	if s.Uptime == 0 && boot > 0 && now > boot {
		s.Uptime = time.Duration(now-boot) * time.Second
	}
	if seen == 0 || (s.CPUs == 0 && s.MemTotal == 0 && s.DiskTotal == 0) {
		return s, errors.New(i18n.T("keine Kennzahlen (nur Linux/macOS mit sh)"))
	}
	return s, nil
}

var topIdle = regexp.MustCompile(`([\d.]+)%\s*idle`)

// statPct: CPU-Auslastung aus zwei "cpu …"-Zeilen von /proc/stat (user nice
// system idle iowait irq softirq steal …). Leerlauf = idle + iowait.
func statPct(a, b string) (int, bool) {
	parse := func(l string) (total, idle uint64, ok bool) {
		f := strings.Fields(l)
		if len(f) < 5 || f[0] != "cpu" {
			return 0, 0, false
		}
		for i, x := range f[1:] {
			if i >= 8 { // guest/guest_nice stecken schon in user
				break
			}
			n, err := strconv.ParseUint(x, 10, 64)
			if err != nil {
				return 0, 0, false
			}
			total += n
			if i == 3 || i == 4 {
				idle += n
			}
		}
		return total, idle, true
	}
	t1, i1, ok1 := parse(a)
	t2, i2, ok2 := parse(b)
	if !ok1 || !ok2 || t2 <= t1 || i2 < i1 {
		return 0, false
	}
	dt, di := t2-t1, i2-i1
	if di > dt {
		return 0, false
	}
	return int(((dt-di)*100 + dt/2) / dt), true
}

func num(v string) uint64 {
	n, _ := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
	return n
}
func kb(v string) uint64 { return num(v) * 1024 }

// MemPct / DiskPct: Belegung in Prozent (0, wenn unbekannt).
func (s Server) MemPct() int {
	if s.MemTotal == 0 || s.MemFree > s.MemTotal {
		return 0
	}
	return int((s.MemTotal - s.MemFree) * 100 / s.MemTotal)
}

func (s Server) DiskPct() int {
	if s.DiskTotal == 0 {
		return 0
	}
	return int(s.DiskUsed * 100 / s.DiskTotal)
}

// Summary: eine Zeile für die Sitzungsleiste.
func (s Server) Summary() string {
	parts := []string{}
	if s.CPUPct >= 0 {
		parts = append(parts, fmt.Sprintf("CPU %d%%", s.CPUPct))
	} else if s.HasLoad {
		parts = append(parts, fmt.Sprintf(i18n.T("Last %.2f"), s.Load[0]))
	}
	if s.MemTotal > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("RAM %d%%"), s.MemPct()))
	}
	if s.DiskTotal > 0 {
		parts = append(parts, fmt.Sprintf("/ %d%%", s.DiskPct()))
	}
	if s.Uptime > 0 {
		parts = append(parts, "up "+Duration(s.Uptime))
	}
	if s.Updates > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", s.Updates))
	}
	return strings.Join(parts, " · ")
}

// Bytes: 1536 → "1.5 KiB".
func Bytes(n uint64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	d, e := uint64(u), 0
	for x := n / u; x >= u; x /= u {
		d *= u
		e++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(d), "KMGTPE"[e])
}

// Duration: kurze deutsche Laufzeit, z. B. "12 T 4 Std" oder "35 Min".
func Duration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf(i18n.T("%d T %d Std"), days, hours)
	case hours > 0:
		return fmt.Sprintf(i18n.T("%d Std %d Min"), hours, mins)
	default:
		return fmt.Sprintf(i18n.T("%d Min"), mins)
	}
}
