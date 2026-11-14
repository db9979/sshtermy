package vuln

import (
	"sort"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Paketliste eines Servers: ein nur lesendes sh-Skript (per "sh -s") gibt
// System, Pakete, laufende Programme mit ihren Paketen und – wenn docker
// oder podman ohne Passwort gehen – die Pakete laufender Container aus.
// Jede Zeile ist schlüssel=wert; Zeilen eines Containers beginnen mit "@".

// pkgScript: Pakete + System; läuft auf dem Host und in Containern.
const pkgScript = `LC_ALL=C; export LC_ALL
if [ -r /etc/os-release ]; then . /etc/os-release; elif [ -r /usr/lib/os-release ]; then . /usr/lib/os-release; fi
echo "os=$ID|$VERSION_ID|$PRETTY_NAME|$ID_LIKE"
if command -v dpkg-query >/dev/null 2>&1; then
  dpkg-query -W -f='deb=${db:Status-Abbrev}|${Package}|${Version}|${source:Package}|${source:Version}\n' 2>/dev/null | grep '^deb=ii'
elif command -v rpm >/dev/null 2>&1; then
  rpm -qa --qf 'rpm=%{NAME}|%{EPOCH}:%{VERSION}-%{RELEASE}|%{SOURCERPM}\n' 2>/dev/null
elif [ -r /lib/apk/db/installed ]; then
  awk -F: '/^P:/{p=substr($0,3)}/^V:/{v=substr($0,3)}/^o:/{o=substr($0,3)}/^$/{if(p!="")print "apk="p"|"v"|"o;p="";v="";o=""}END{if(p!="")print "apk="p"|"v"|"o}' /lib/apk/db/installed
elif [ -r /var/lib/dpkg/status ]; then
  awk '/^Package:/{p=$2}/^Status:/{s=$4}/^Version:/{v=$2}/^Source:/{o=$2;ov=$3}/^$/{if(p!=""&&s=="installed"){gsub(/[()]/,"",ov);print "deb=ii |"p"|"v"|"o"|"ov};p="";s="";v="";o="";ov=""}' /var/lib/dpkg/status
fi
`

// Script: vollständiges Skript für den Host (Pakete, laufende Programme,
// Container). withContainers=false lässt die Container aus.
func Script(withContainers bool) string {
	var b strings.Builder
	b.WriteString(pkgScript)
	b.WriteString(`S=""; [ "$(id -u)" = 0 ] || { sudo -n true 2>/dev/null && S="sudo -n"; }
[ -n "$S" ] || [ "$(id -u)" = 0 ] && echo "root=1"
echo "kernel=$(uname -r)"
P=$($S sh -c 'for p in /proc/[0-9]*; do e=$(readlink "$p/exe" 2>/dev/null) || continue; echo "${p#/proc/} ${e% (deleted)}"; done' 2>/dev/null)
printf '%s\n' "$P" | awk 'NF==2{print "pid="$1" "$2}'
if command -v ss >/dev/null 2>&1; then $S ss -ltnupH 2>/dev/null | awk '{print "lis="$1" "$5" "$NF}'; fi
E=$(printf '%s\n' "$P" | awk 'NF==2{print $2}' | sort -u)
if command -v dpkg >/dev/null 2>&1; then
  for f in $E; do echo "$f"; case "$f" in /usr/*) echo "${f#/usr}";; esac; done | xargs dpkg -S 2>/dev/null | sed 's/^/own=/'
elif command -v rpm >/dev/null 2>&1; then
  for f in $E; do o=$(rpm -qf --qf '%{NAME}' "$f" 2>/dev/null) && echo "own=$o: $f"; done
elif command -v apk >/dev/null 2>&1; then
  for f in $E; do o=$(apk info -qW "$f" 2>/dev/null | sed -n 's/.* is owned by //p') && [ -n "$o" ] && echo "own=$o: $f"; done
fi
`)
	if withContainers {
		b.WriteString(`T=""; command -v timeout >/dev/null 2>&1 && T="timeout 20"
I=$(cat <<'SSHTERMY_INV'
` + pkgScript + `SSHTERMY_INV
)
n=0
for rt in docker podman; do
  command -v $rt >/dev/null 2>&1 || continue
  D=$rt; $D ps -q >/dev/null 2>&1 || { [ -n "$S" ] && $S $rt ps -q >/dev/null 2>&1 && D="$S $rt"; } || { echo "ctrerr=$rt"; continue; }
  for c in $($D ps -q 2>/dev/null); do
    n=$((n+1)); [ $n -gt 25 ] && { echo "ctrmore=1"; break; }
    echo "ctr=$c|$($D inspect -f '{{.Name}}|{{.Config.Image}}' $c 2>/dev/null)|$rt"
    out=$($T $D exec $c sh -c "$I" 2>/dev/null)
    if [ -z "$out" ]; then
      out=$( { $T $D exec $c cat /etc/os-release 2>/dev/null | sed 's/^/osr=/'; $T $D exec $c cat /var/lib/dpkg/status 2>/dev/null | sed 's/^/dst=/'; $T $D exec $c cat /lib/apk/db/installed 2>/dev/null | sed 's/^/apkdb=/'; } )
    fi
    printf '%s\n' "$out" | sed 's/^/@/'
    echo "ctrend=$c"
  done
done
`)
	}
	b.WriteString("exit 0\n")
	return b.String()
}

// Package: ein installiertes Paket.
type Package struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Source        string `json:"source,omitempty"`         // Quellpaket (deb, apk origin, rpm Quell-RPM)
	SourceVersion string `json:"source_version,omitempty"` // nur deb
}

// Container: ein laufender Container mit eigener Paketliste.
type Container struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Image   string     `json:"image"`
	Runtime string     `json:"runtime"`
	Inv     *Inventory `json:"inv,omitempty"`
}

// Inventory: was auf einem System installiert ist.
type Inventory struct {
	OSID      string    `json:"os_id"`
	OSVersion string    `json:"os_version"`
	OSName    string    `json:"os_name"`
	OSLike    string    `json:"os_like,omitempty"`
	Kernel    string    `json:"kernel,omitempty"`
	Format    Format    `json:"format,omitempty"`
	Packages  []Package `json:"packages,omitempty"`
	Root      bool      `json:"root,omitempty"` // alle Prozesse sichtbar
	// Running: Paket → laufende Programme mit offenen Ports („sshd :22“).
	Running    map[string][]string `json:"running,omitempty"`
	Containers []Container         `json:"containers,omitempty"`
	Notes      []string            `json:"notes,omitempty"`
}

// ParseInventory liest die Ausgabe von Script.
func ParseInventory(out string) *Inventory {
	inv := &Inventory{}
	var cur *Container
	var sub []string
	pids := map[string]string{}  // pid → Programm
	owner := map[string]string{} // Programm → Paket
	type lis struct{ proto, addr, pid, prog string }
	var listens []lis
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if cur != nil {
			if strings.HasPrefix(line, "@") {
				sub = append(sub, line[1:])
				continue
			}
			if strings.HasPrefix(line, "ctrend=") {
				cur.Inv = parsePkgs(sub)
				inv.Containers = append(inv.Containers, *cur)
				cur, sub = nil, nil
				continue
			}
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "root":
			inv.Root = true
		case "kernel":
			inv.Kernel = v
		case "pid":
			if p, e, ok := strings.Cut(v, " "); ok {
				pids[p] = e
			}
		case "lis":
			f := strings.Fields(v)
			if len(f) >= 3 {
				for _, u := range usersOf(f[2]) {
					listens = append(listens, lis{proto: f[0], addr: f[1], pid: u[1], prog: u[0]})
				}
			}
		case "own":
			// "pkg1, pkg2:arch: /pfad" bzw. "pkg: /pfad"
			i := strings.LastIndex(v, ": ")
			if i < 0 {
				continue
			}
			pk, path := v[:i], strings.TrimSpace(v[i+2:])
			pk = strings.TrimSpace(strings.Split(pk, ",")[0])
			if j := strings.IndexByte(pk, ':'); j > 0 {
				pk = pk[:j]
			}
			owner[path] = pk
			if !strings.HasPrefix(path, "/usr/") {
				if _, ok := owner["/usr"+path]; !ok {
					owner["/usr"+path] = pk
				}
			}
		case "ctr":
			f := strings.Split(v, "|")
			for len(f) < 4 {
				f = append(f, "")
			}
			cur = &Container{ID: f[0], Name: strings.TrimPrefix(f[1], "/"), Image: f[2], Runtime: f[3]}
		case "ctrerr":
			inv.Notes = append(inv.Notes, i18n.F("%s: keine Rechte (Gruppe %s oder sudo ohne Passwort fehlt)", v, v))
		case "ctrmore":
			inv.Notes = append(inv.Notes, i18n.T("mehr als 25 Container: nur die ersten 25 geprüft"))
		}
	}
	host := parsePkgs(strings.Split(out, "\n"))
	inv.OSID, inv.OSVersion, inv.OSName, inv.OSLike = host.OSID, host.OSVersion, host.OSName, host.OSLike
	inv.Format, inv.Packages = host.Format, host.Packages
	// laufende Programme → Pakete, mit Ports
	ports := map[string][]string{} // Programm → Ports
	for _, l := range listens {
		exe := pids[l.pid]
		if exe == "" {
			continue
		}
		p := l.addr
		if i := strings.LastIndex(p, ":"); i >= 0 {
			p = p[i+1:]
		}
		port := ":" + p
		if l.proto == "udp" {
			port += "/udp"
		}
		if !contains(ports[exe], port) {
			ports[exe] = append(ports[exe], port)
		}
	}
	for _, exe := range pids {
		pk := owner[exe]
		if pk == "" {
			continue
		}
		name := exe[strings.LastIndex(exe, "/")+1:]
		if ps := ports[exe]; len(ps) > 0 {
			sort.Strings(ps)
			name += " " + strings.Join(ps, ",")
		}
		if inv.Running == nil {
			inv.Running = map[string][]string{}
		}
		if !contains(inv.Running[pk], name) {
			inv.Running[pk] = append(inv.Running[pk], name)
		}
	}
	for k := range inv.Running {
		sort.Strings(inv.Running[k])
	}
	return inv
}

// usersOf: users:(("sshd",pid=812,fd=3),("sshd",pid=900,fd=3)) → [[sshd 812] …]
func usersOf(s string) [][2]string {
	var out [][2]string
	for _, part := range strings.Split(s, "(\"")[1:] {
		name, rest, ok := strings.Cut(part, "\"")
		if !ok {
			continue
		}
		i := strings.Index(rest, "pid=")
		if i < 0 {
			continue
		}
		pid := rest[i+4:]
		if j := strings.IndexAny(pid, ",)"); j >= 0 {
			pid = pid[:j]
		}
		out = append(out, [2]string{name, pid})
	}
	return out
}

// parsePkgs: os=, deb=, rpm=, apk= und die Rohdateien eines Containers
// ohne Shell (osr=, dst=, apkdb=).
func parsePkgs(lines []string) *Inventory {
	inv := &Inventory{}
	var osr, dst, apkdb []string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "os":
			f := strings.Split(v, "|")
			for len(f) < 4 {
				f = append(f, "")
			}
			inv.OSID, inv.OSVersion, inv.OSName, inv.OSLike = strings.ToLower(f[0]), f[1], f[2], f[3]
		case "deb":
			f := strings.Split(v, "|")
			if len(f) < 3 || f[1] == "" {
				continue
			}
			p := Package{Name: f[1], Version: f[2]}
			if len(f) >= 5 {
				p.Source, p.SourceVersion = f[3], f[4]
			}
			inv.Format = FormatDeb
			inv.Packages = append(inv.Packages, p)
		case "rpm":
			f := strings.Split(v, "|")
			if len(f) < 2 || f[0] == "" || strings.HasPrefix(f[0], "gpg-pubkey") {
				continue
			}
			p := Package{Name: f[0], Version: strings.TrimPrefix(f[1], "(none):")}
			if len(f) >= 3 {
				p.Source = srpmName(f[2])
			}
			inv.Format = FormatRPM
			inv.Packages = append(inv.Packages, p)
		case "apk":
			f := strings.Split(v, "|")
			if len(f) < 2 || f[0] == "" {
				continue
			}
			p := Package{Name: f[0], Version: f[1]}
			if len(f) >= 3 {
				p.Source = f[2]
			}
			inv.Format = FormatAPK
			inv.Packages = append(inv.Packages, p)
		case "osr":
			osr = append(osr, v)
		case "dst":
			dst = append(dst, v)
		case "apkdb":
			apkdb = append(apkdb, v)
		}
	}
	if inv.OSID == "" && len(osr) > 0 {
		kv := map[string]string{}
		for _, l := range osr {
			if k, v, ok := strings.Cut(l, "="); ok {
				kv[k] = strings.Trim(v, `"'`)
			}
		}
		inv.OSID, inv.OSVersion, inv.OSName, inv.OSLike = strings.ToLower(kv["ID"]), kv["VERSION_ID"], kv["PRETTY_NAME"], kv["ID_LIKE"]
	}
	if len(inv.Packages) == 0 && len(dst) > 0 {
		inv.Format, inv.Packages = FormatDeb, parseDpkgStatus(dst)
	}
	if len(inv.Packages) == 0 && len(apkdb) > 0 {
		inv.Format, inv.Packages = FormatAPK, parseApkDB(apkdb)
	}
	return inv
}

// srpmName: openssl-3.0.7-27.el9.src.rpm → openssl
func srpmName(s string) string {
	s = strings.TrimSuffix(strings.TrimSuffix(s, ".rpm"), ".src")
	s = strings.TrimSuffix(s, ".nosrc")
	for range 2 {
		if i := strings.LastIndexByte(s, '-'); i > 0 {
			s = s[:i]
		}
	}
	if s == "(none)" {
		return ""
	}
	return s
}

func parseDpkgStatus(lines []string) []Package {
	var out []Package
	var p Package
	installed := false
	flush := func() {
		if p.Name != "" && installed {
			out = append(out, p)
		}
		p, installed = Package{}, false
	}
	for _, l := range lines {
		switch {
		case l == "":
			flush()
		case strings.HasPrefix(l, "Package: "):
			p.Name = strings.TrimSpace(l[9:])
		case strings.HasPrefix(l, "Status: "):
			installed = strings.HasSuffix(strings.TrimSpace(l), " installed")
		case strings.HasPrefix(l, "Version: "):
			p.Version = strings.TrimSpace(l[9:])
		case strings.HasPrefix(l, "Source: "):
			f := strings.Fields(l[8:])
			if len(f) > 0 {
				p.Source = f[0]
			}
			if len(f) > 1 {
				p.SourceVersion = strings.Trim(f[1], "()")
			}
		}
	}
	flush()
	return out
}

func parseApkDB(lines []string) []Package {
	var out []Package
	var p Package
	for _, l := range append(lines, "") {
		switch {
		case l == "":
			if p.Name != "" {
				out = append(out, p)
			}
			p = Package{}
		case strings.HasPrefix(l, "P:"):
			p.Name = l[2:]
		case strings.HasPrefix(l, "V:"):
			p.Version = l[2:]
		case strings.HasPrefix(l, "o:"):
			p.Source = l[2:]
		}
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Ecosystems: OSV-Ökosysteme des Systems (leer = OSV kennt es nicht).
func (inv *Inventory) Ecosystems() []string {
	id, ver := inv.OSID, inv.OSVersion
	major, minor, _ := strings.Cut(ver, ".")
	if i := strings.IndexByte(minor, '.'); i >= 0 {
		minor = minor[:i]
	}
	switch id {
	case "debian":
		if major != "" {
			return []string{"Debian:" + major}
		}
	case "ubuntu":
		if major != "" && minor != "" {
			e := "Ubuntu:" + major + "." + minor
			if n := atoi(major); minor == "04" && n%2 == 0 {
				e += ":LTS"
			}
			return []string{e}
		}
	case "alpine":
		if major != "" && minor != "" {
			return []string{"Alpine:v" + major + "." + minor}
		}
	case "rocky":
		return []string{"Rocky Linux:" + major}
	case "almalinux":
		return []string{"AlmaLinux:" + major}
	case "rhel":
		if major != "" {
			p := "Red Hat:enterprise_linux:" + major + "::"
			return []string{p + "baseos", p + "appstream", p + "crb"}
		}
	case "opensuse-leap":
		return []string{"openSUSE:Leap " + ver}
	case "opensuse-tumbleweed":
		return []string{"openSUSE:Tumbleweed"}
	case "sles", "sled":
		sp := ""
		if minor != "" && minor != "0" {
			sp = " SP" + minor
		}
		var out []string
		for _, m := range []string{"Server", "Server" + sp + "-LTSS", "Module for Basesystem", "Module for Server Applications",
			"Module for Web and Scripting", "Module for Development Tools", "Module for Containers", "Module for Legacy", "Module for Public Cloud", "Module for Python 3"} {
			if strings.HasSuffix(m, "-LTSS") {
				out = append(out, "SUSE:Linux Enterprise "+strings.Replace(m, "Server", "Server "+major, 1))
				continue
			}
			out = append(out, "SUSE:Linux Enterprise "+m+" "+major+sp)
		}
		return out
	}
	return nil
}

// Base: Ordner des Ökosystems in der OSV-Datenbank (Debian:12 → Debian).
func Base(eco string) string {
	b, _, _ := strings.Cut(eco, ":")
	return b
}

// QueryName: unter welchem Namen OSV das Paket führt. Debian, Ubuntu,
// Alpine und SUSE führen Quellpakete, Red Hat, Rocky und Alma die Binärpakete.
func QueryName(eco string, p Package) (name, version string) {
	switch Base(eco) {
	case "Debian", "Ubuntu":
		if p.Source != "" {
			v := p.SourceVersion
			if v == "" {
				v = p.Version
			}
			return p.Source, v
		}
	case "Alpine", "SUSE", "openSUSE":
		if p.Source != "" {
			return p.Source, p.Version
		}
	}
	return p.Name, p.Version
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			break
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}
