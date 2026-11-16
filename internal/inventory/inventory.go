// Package inventory: Inventar eines Linux-Servers (Pakete, Benutzer,
// Admins, Cron, lauschende Ports, aktivierte Dienste, Kernel) und was sich
// seit dem letzten Stand geändert hat. Nur lesend.
package inventory

import (
	"encoding/csv"
	"sort"
	"strings"
	"time"
)

// Script liest nur. Mit root (oder sudo -n) sieht es auch fremde Crontabs
// und die Prozesse hinter Ports.
const Script = `export LC_ALL=C
P=""
if [ "$(id -u)" = 0 ]; then :; elif sudo -n true 2>/dev/null; then P="sudo -n"; fi
. /etc/os-release 2>/dev/null && echo "os=$PRETTY_NAME"
echo "kernel=$(uname -r)"
if command -v dpkg-query >/dev/null 2>&1; then
  dpkg-query -W -f '${db:Status-Abbrev}|${Package}|${Version}\n' 2>/dev/null | awk -F'|' '$1 ~ /^ii/ {print "pkg="$2"|"$3}'
elif command -v rpm >/dev/null 2>&1; then
  rpm -qa --qf 'pkg=%{NAME}.%{ARCH}|%{EPOCHNUM}:%{VERSION}-%{RELEASE}\n' 2>/dev/null | sed 's/|0:/|/'
elif command -v apk >/dev/null 2>&1; then
  apk info -v 2>/dev/null | sed -E 's/^(.+)-([0-9][^-]*-r[0-9]+)$/pkg=\1|\2/' | grep '^pkg='
fi
awk -F: '($3==0 || $3>=1000) && $3!=65534 {print "user="$1"|"$3"|"$7}' /etc/passwd 2>/dev/null
for g in sudo wheel admin; do getent group "$g" 2>/dev/null | awk -F: '{n=split($4,a,","); for(i=1;i<=n;i++) if(a[i]!="") print "admin="a[i]"|"$1}'; done
for f in /etc/crontab /etc/cron.d/* /var/spool/cron/crontabs/* /var/spool/cron/*; do
  [ -f "$f" ] || continue
  $P cat "$f" 2>/dev/null | grep -vE '^\s*(#|$|[A-Z_]+=)' | sed "s|^|cron=$f\||"
done
if command -v ss >/dev/null 2>&1; then $P ss -ltnupH 2>/dev/null | awk '{print "port="$1"|"$5"|"$7}'; fi
if command -v systemctl >/dev/null 2>&1; then
  systemctl list-unit-files --type=service --state=enabled --no-legend --plain 2>/dev/null | awk '{print "svc="$1}'
elif command -v rc-update >/dev/null 2>&1; then
  rc-update show 2>/dev/null | awk '{print "svc="$1}'
fi
exit 0
`

// Snapshot: Inventar zu einem Zeitpunkt.
type Snapshot struct {
	At       time.Time         `json:"at"`
	OS       string            `json:"os,omitempty"`
	Kernel   string            `json:"kernel,omitempty"`
	Packages map[string]string `json:"pkgs,omitempty"`   // Name → Version
	Users    map[string]string `json:"users,omitempty"`  // Name → „uid shell“
	Admins   []string          `json:"admins,omitempty"` // „name (gruppe)“
	Cron     []string          `json:"cron,omitempty"`   // „datei: zeile“
	Ports    []string          `json:"ports,omitempty"`  // „tcp *:80 nginx“
	Services []string          `json:"svcs,omitempty"`
}

// Parse liest die Ausgabe von Script.
func Parse(out string, now time.Time) Snapshot {
	s := Snapshot{At: now, Packages: map[string]string{}, Users: map[string]string{}}
	set := map[string]bool{}
	add := func(dst *[]string, v string) {
		if v != "" && !set[v] {
			set[v] = true
			*dst = append(*dst, v)
		}
	}
	for _, ln := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimRight(ln, "\r"), "=")
		if !ok {
			continue
		}
		switch k {
		case "os":
			s.OS = v
		case "kernel":
			s.Kernel = v
		case "pkg":
			if n, ver, ok := strings.Cut(v, "|"); ok && n != "" {
				s.Packages[n] = ver
			}
		case "user":
			p := strings.SplitN(v, "|", 3)
			if len(p) == 3 {
				s.Users[p[0]] = "uid " + p[1] + " " + p[2]
			}
		case "admin":
			if n, g, ok := strings.Cut(v, "|"); ok {
				add(&s.Admins, n+" ("+g+")")
			}
		case "cron":
			if f, l, ok := strings.Cut(v, "|"); ok {
				add(&s.Cron, f+": "+strings.Join(strings.Fields(l), " "))
			}
		case "port":
			p := strings.SplitN(v, "|", 3)
			if len(p) == 3 {
				add(&s.Ports, strings.TrimSpace(p[0]+" "+normAddr(p[1])+" "+procName(p[2])))
			}
		case "svc":
			add(&s.Services, v)
		}
	}
	sort.Strings(s.Admins)
	sort.Strings(s.Cron)
	sort.Strings(s.Ports)
	sort.Strings(s.Services)
	return s
}

func normAddr(a string) string {
	i := strings.LastIndexByte(a, ':')
	if i < 0 {
		return a
	}
	switch strings.Trim(a[:i], "[]") {
	case "0.0.0.0", "::", "*", "":
		return "*:" + a[i+1:]
	}
	return a
}

func procName(s string) string {
	if i := strings.Index(s, `(("`); i >= 0 {
		s = s[i+3:]
		if j := strings.IndexByte(s, '"'); j >= 0 {
			return s[:j]
		}
	}
	return ""
}

// Change: eine Änderung.
type Change struct {
	Kind string `json:"k"`           // pkg, user, admin, cron, port, svc, kernel, os
	Op   string `json:"op"`          // + neu, - weg, ~ geändert
	Item string `json:"i"`           // Name
	From string `json:"f,omitempty"` // alter Wert (bei ~)
	To   string `json:"t,omitempty"` // neuer Wert
}

// ChangeSet: Änderungen zwischen zwei Ständen.
type ChangeSet struct {
	Since   time.Time `json:"since"`
	At      time.Time `json:"at"`
	Changes []Change  `json:"changes"`
}

// Diff vergleicht zwei Stände.
func Diff(prev, cur Snapshot) []Change {
	var out []Change
	mapDiff := func(kind string, a, b map[string]string) {
		var keys []string
		for k := range a {
			keys = append(keys, k)
		}
		for k := range b {
			if _, ok := a[k]; !ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			x, inA := a[k]
			y, inB := b[k]
			switch {
			case !inA:
				out = append(out, Change{Kind: kind, Op: "+", Item: k, To: y})
			case !inB:
				out = append(out, Change{Kind: kind, Op: "-", Item: k, From: x})
			case x != y:
				out = append(out, Change{Kind: kind, Op: "~", Item: k, From: x, To: y})
			}
		}
	}
	listDiff := func(kind string, a, b []string) {
		in := func(xs []string) map[string]bool {
			m := map[string]bool{}
			for _, x := range xs {
				m[x] = true
			}
			return m
		}
		ma, mb := in(a), in(b)
		for _, x := range b {
			if !ma[x] {
				out = append(out, Change{Kind: kind, Op: "+", Item: x})
			}
		}
		for _, x := range a {
			if !mb[x] {
				out = append(out, Change{Kind: kind, Op: "-", Item: x})
			}
		}
	}
	if prev.Kernel != "" && cur.Kernel != "" && prev.Kernel != cur.Kernel {
		out = append(out, Change{Kind: "kernel", Op: "~", Item: "kernel", From: prev.Kernel, To: cur.Kernel})
	}
	if prev.OS != "" && cur.OS != "" && prev.OS != cur.OS {
		out = append(out, Change{Kind: "os", Op: "~", Item: "os", From: prev.OS, To: cur.OS})
	}
	mapDiff("user", prev.Users, cur.Users)
	listDiff("admin", prev.Admins, cur.Admins)
	listDiff("port", prev.Ports, cur.Ports)
	listDiff("svc", prev.Services, cur.Services)
	listDiff("cron", prev.Cron, cur.Cron)
	mapDiff("pkg", prev.Packages, cur.Packages)
	return out
}

// Record: was je Verbindung gespeichert wird.
type Record struct {
	Last    Snapshot    `json:"last"`
	History []ChangeSet `json:"history,omitempty"` // neueste zuerst
}

// MaxHistory: so viele Änderungsstände bleiben.
const MaxHistory = 30

// Update: neuen Stand übernehmen. Gibt die Änderungen zurück (nil beim
// ersten Stand).
func (r *Record) Update(cur Snapshot) []Change {
	if r.Last.At.IsZero() {
		r.Last = cur
		return nil
	}
	ch := Diff(r.Last, cur)
	if len(ch) > 0 {
		r.History = append([]ChangeSet{{Since: r.Last.At, At: cur.At, Changes: ch}}, r.History...)
		if len(r.History) > MaxHistory {
			r.History = r.History[:MaxHistory]
		}
	}
	r.Last = cur
	return ch
}

// Important: Änderungen ohne reine Paket-Versionssprünge (die stehen nur in
// der Zahl), für Meldungen.
func Important(ch []Change) []Change {
	var out []Change
	for _, c := range ch {
		if c.Kind == "pkg" && c.Op == "~" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// CSV: Inventar als CSV (Kategorie, Name, Wert).
func (s Snapshot) CSV(host string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"host", "kategorie", "name", "wert"})
	row := func(k, n, v string) { _ = w.Write([]string{host, k, n, v}) }
	row("os", s.OS, "")
	row("kernel", s.Kernel, "")
	names := func(m map[string]string) []string {
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return ks
	}
	for _, n := range names(s.Users) {
		row("user", n, s.Users[n])
	}
	for _, x := range s.Admins {
		row("admin", x, "")
	}
	for _, x := range s.Ports {
		row("port", x, "")
	}
	for _, x := range s.Services {
		row("service", x, "")
	}
	for _, x := range s.Cron {
		row("cron", x, "")
	}
	for _, n := range names(s.Packages) {
		row("package", n, s.Packages[n])
	}
	w.Flush()
	return b.String()
}
