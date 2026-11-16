// Package harden: kurzer, nur lesender Härtungs-Check für Linux-Server.
//
// Ein Skript sammelt Fakten (sshd-Einstellungen, sudo NOPASSWD, Firewall,
// lauschende Ports, automatische Updates, TLS-Zertifikate, Konten mit UID 0,
// Zeitabgleich, MAC-Schutz). Die Bewertung passiert hier, lokal. Nichts wird
// geändert; Vorschläge zum Beheben stehen als Text dabei.
package harden

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Level: Ampel.
type Level int

const (
	OK Level = iota
	Info
	Warn
	Bad
)

// Finding: ein Prüfpunkt.
type Finding struct {
	ID     string `json:"id"`
	Level  Level  `json:"level"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"` // Vorschlag, nie automatisch
}

// Report: Ergebnis eines Hosts.
type Report struct {
	At       time.Time `json:"at"`
	System   string    `json:"system,omitempty"`
	Root     bool      `json:"root,omitempty"` // mit root-Rechten gelesen (sonst lückenhaft)
	Findings []Finding `json:"findings"`
}

// Counts: rot, gelb, Hinweise.
func (r *Report) Counts() (bad, warn, info int) {
	if r == nil {
		return
	}
	for _, f := range r.Findings {
		switch f.Level {
		case Bad:
			bad++
		case Warn:
			warn++
		case Info:
			info++
		}
	}
	return
}

// Script liest nur. Mit root (oder sudo -n) sieht es mehr.
const Script = `export LC_ALL=C
P=""
if [ "$(id -u)" = 0 ]; then echo "root=1"; elif sudo -n true 2>/dev/null; then P="sudo -n"; echo "root=1"; fi
. /etc/os-release 2>/dev/null && echo "os=$PRETTY_NAME"
if command -v sshd >/dev/null 2>&1 || [ -x /usr/sbin/sshd ]; then
  S=$(command -v sshd || echo /usr/sbin/sshd)
  $P $S -T 2>/dev/null | grep -Ei '^(permitrootlogin|passwordauthentication|permitemptypasswords|x11forwarding|maxauthtries|kbdinteractiveauthentication|pubkeyauthentication) ' | sed 's/^/sshd=/'
fi
$P cat /etc/sudoers /etc/sudoers.d/* 2>/dev/null | grep -v '^[[:space:]]*#' | grep -E 'NOPASSWD' | sed 's/^/nopasswd=/' | head -20
if command -v ufw >/dev/null 2>&1; then echo "fw=ufw $($P ufw status 2>/dev/null | head -1)"; fi
if command -v firewall-cmd >/dev/null 2>&1; then echo "fw=firewalld $(firewall-cmd --state 2>&1 | head -1)"; fi
if command -v nft >/dev/null 2>&1; then echo "fw=nft $($P nft list ruleset 2>/dev/null | grep -cE '^\s*(chain|.*(drop|reject))' )"; fi
if command -v iptables >/dev/null 2>&1; then echo "fw=iptables $($P iptables -S 2>/dev/null | grep -cvE '^-P (INPUT|FORWARD|OUTPUT) ACCEPT$')"; fi
$P iptables -S INPUT 2>/dev/null | head -1 | sed 's/^/fwpol=/'
if command -v ss >/dev/null 2>&1; then ss -ltnpH 2>/dev/null | awk '{print "lis="$4"|"$6}' | head -60; fi
if [ -f /etc/apt/apt.conf.d/20auto-upgrades ]; then grep -h 'Unattended-Upgrade' /etc/apt/apt.conf.d/20auto-upgrades /etc/apt/apt.conf.d/10periodic 2>/dev/null | sed 's/^/auto=apt /'; fi
command -v unattended-upgrade >/dev/null 2>&1 && echo "auto=apt-installed"
if command -v systemctl >/dev/null 2>&1; then
  for t in dnf-automatic.timer dnf-automatic-install.timer dnf5-automatic.timer yum-cron.service apt-daily-upgrade.timer; do
    systemctl is-enabled "$t" >/dev/null 2>&1 && echo "auto=unit $t"
  done
  for u in fail2ban.service sshguard.service crowdsec.service; do
    systemctl is-active "$u" >/dev/null 2>&1 && echo "guard=$u"
  done
fi
command -v timedatectl >/dev/null 2>&1 && timedatectl show -p NTPSynchronized --value 2>/dev/null | sed 's/^/ntp=/'
[ -r /sys/fs/selinux/enforce ] && echo "selinux=$(cat /sys/fs/selinux/enforce)"
[ -d /sys/kernel/security/apparmor ] && echo "apparmor=1"
awk -F: '$3==0 {print "uid0="$1}' /etc/passwd 2>/dev/null
$P awk -F: '($2=="") {print "emptypw="$1}' /etc/shadow 2>/dev/null | head -10
for f in $( ( ls /etc/letsencrypt/live/*/cert.pem 2>/dev/null; $P grep -rhoE '^\s*(ssl_certificate|SSLCertificateFile)\s+[^ ;]+' /etc/nginx /etc/apache2 /etc/httpd 2>/dev/null | awk '{print $2}' ) | sort -u | head -30); do
  e=$($P openssl x509 -enddate -noout -in "$f" 2>/dev/null | cut -d= -f2)
  [ -n "$e" ] && echo "cert=$f|$e"
done
exit 0
`

// Facts: gelesene Fakten.
type Facts struct {
	Root     bool
	OS       string
	SSHD     map[string]string
	NoPasswd []string
	FW       []string // "ufw Status: active", "firewalld running", "nft 12", "iptables 3"
	FWPolicy string
	Listen   []Listener
	Auto     []string
	Guards   []string
	NTP      string
	SELinux  string
	AppArmor bool
	UID0     []string
	EmptyPW  []string
	Certs    []Cert
}

// Listener: lauschender TCP-Port.
type Listener struct {
	Addr string
	Port int
	Proc string
}

// Cert: Zertifikat auf dem Host.
type Cert struct {
	Path string
	End  time.Time
}

// Parse liest die Ausgabe von Script.
func Parse(out string) Facts {
	f := Facts{SSHD: map[string]string{}}
	for _, ln := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimRight(ln, "\r"), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "root":
			f.Root = v == "1"
		case "os":
			f.OS = v
		case "sshd":
			if a, b, ok := strings.Cut(v, " "); ok {
				f.SSHD[strings.ToLower(a)] = strings.ToLower(strings.TrimSpace(b))
			}
		case "nopasswd":
			f.NoPasswd = append(f.NoPasswd, v)
		case "fw":
			f.FW = append(f.FW, v)
		case "fwpol":
			f.FWPolicy = v
		case "lis":
			addr, proc, _ := strings.Cut(v, "|")
			if i := strings.LastIndexByte(addr, ':'); i > 0 {
				p, _ := strconv.Atoi(addr[i+1:])
				f.Listen = append(f.Listen, Listener{Addr: strings.Trim(addr[:i], "[]"), Port: p, Proc: procName(proc)})
			}
		case "auto":
			f.Auto = append(f.Auto, v)
		case "guard":
			f.Guards = append(f.Guards, v)
		case "ntp":
			f.NTP = v
		case "selinux":
			f.SELinux = v
		case "apparmor":
			f.AppArmor = v == "1"
		case "uid0":
			f.UID0 = append(f.UID0, v)
		case "emptypw":
			f.EmptyPW = append(f.EmptyPW, v)
		case "cert":
			p, e, _ := strings.Cut(v, "|")
			if t, err := time.Parse("Jan _2 15:04:05 2006 MST", e); err == nil {
				f.Certs = append(f.Certs, Cert{Path: p, End: t})
			}
		}
	}
	return f
}

// procName: users:(("nginx",pid=1,fd=6)) → nginx
func procName(s string) string {
	if i := strings.Index(s, `(("`); i >= 0 {
		s = s[i+3:]
		if j := strings.IndexByte(s, '"'); j >= 0 {
			return s[:j]
		}
	}
	return ""
}

// Ports, die nicht offen im Netz stehen sollten (unverschlüsselt oder ohne
// Anmeldung üblich).
var riskyPorts = map[int]string{
	23: "Telnet", 2375: "Docker-API ohne TLS", 6379: "Redis", 11211: "Memcached", 27017: "MongoDB",
	9200: "Elasticsearch", 5432: "PostgreSQL", 3306: "MySQL/MariaDB", 5984: "CouchDB", 2379: "etcd",
	10250: "Kubelet", 8500: "Consul", 5900: "VNC", 21: "FTP", 111: "rpcbind", 445: "SMB", 139: "NetBIOS",
}

var critPorts = map[int]bool{23: true, 2375: true, 6379: true, 11211: true, 27017: true, 9200: true, 2379: true, 10250: true}

// Check bewertet die Fakten.
func Check(f Facts, now time.Time) *Report {
	r := &Report{At: now, System: f.OS, Root: f.Root}
	add := func(id string, lv Level, title, detail, fix string) {
		r.Findings = append(r.Findings, Finding{ID: id, Level: lv, Title: title, Detail: detail, Fix: fix})
	}
	T := i18n.T

	// sshd
	if len(f.SSHD) == 0 {
		add("sshd", Info, T("sshd-Einstellungen nicht lesbar"), T("„sshd -T“ braucht root; ohne sudo -n bleibt dieser Teil leer."), "")
	} else {
		switch f.SSHD["permitrootlogin"] {
		case "yes":
			add("ssh-root", Bad, T("SSH: root darf sich mit Passwort anmelden"), "PermitRootLogin yes",
				T("In /etc/ssh/sshd_config „PermitRootLogin prohibit-password“ (oder no) setzen, dann sshd neu laden. Vorher prüfen, dass ein Schlüssel-Login funktioniert."))
		case "prohibit-password", "without-password":
			add("ssh-root", Info, T("SSH: root nur mit Schlüssel"), "PermitRootLogin "+f.SSHD["permitrootlogin"], T("Besser: Anmeldung als normaler Benutzer, dann sudo."))
		default:
			add("ssh-root", OK, T("SSH: root-Anmeldung aus"), "PermitRootLogin "+orDash(f.SSHD["permitrootlogin"]), "")
		}
		if f.SSHD["permitemptypasswords"] == "yes" {
			add("ssh-empty", Bad, T("SSH: leere Passwörter erlaubt"), "PermitEmptyPasswords yes", T("„PermitEmptyPasswords no“ setzen."))
		}
		if f.SSHD["passwordauthentication"] == "yes" || f.SSHD["kbdinteractiveauthentication"] == "yes" {
			lv := Warn
			d := "PasswordAuthentication yes"
			if len(f.Guards) > 0 {
				d += " · " + strings.Join(f.Guards, ", ")
			}
			add("ssh-pw", lv, T("SSH: Anmeldung mit Passwort möglich"), d,
				T("Wenn alle mit Schlüssel arbeiten: „PasswordAuthentication no“ und „KbdInteractiveAuthentication no“. Sonst fail2ban o. Ä. gegen Raten."))
		} else {
			add("ssh-pw", OK, T("SSH: nur Schlüssel-Anmeldung"), "", "")
		}
		if n, err := strconv.Atoi(f.SSHD["maxauthtries"]); err == nil && n > 6 {
			add("ssh-tries", Info, T("SSH: viele Anmeldeversuche je Verbindung"), fmt.Sprintf("MaxAuthTries %d", n), T("„MaxAuthTries 4“ reicht meist."))
		}
		if f.SSHD["x11forwarding"] == "yes" {
			add("ssh-x11", Info, T("SSH: X11-Weiterleitung an"), "X11Forwarding yes", T("Auf Servern ohne Grafik „X11Forwarding no“."))
		}
	}

	// sudo
	var all []string
	for _, l := range f.NoPasswd {
		if strings.Contains(l, "ALL") && !strings.Contains(l, "/") {
			all = append(all, strings.Join(strings.Fields(l), " "))
		}
	}
	if len(all) > 0 {
		add("sudo-nopw", Warn, T("sudo ohne Passwort für alle Befehle"), strings.Join(all, " · "),
			T("NOPASSWD nur für einzelne Befehle freigeben oder Passwort verlangen (visudo)."))
	}

	// Firewall
	active := false
	var fwd []string
	for _, x := range f.FW {
		lx := strings.ToLower(x)
		switch {
		case strings.HasPrefix(lx, "ufw") && strings.Contains(lx, "active") && !strings.Contains(lx, "inactive"):
			active = true
		case strings.HasPrefix(lx, "firewalld") && strings.Contains(lx, "running"):
			active = true
		case strings.HasPrefix(lx, "nft "), strings.HasPrefix(lx, "iptables "):
			if n, _ := strconv.Atoi(strings.TrimSpace(x[strings.IndexByte(x, ' ')+1:])); n > 2 {
				active = true
			}
		}
		fwd = append(fwd, x)
	}
	if strings.Contains(f.FWPolicy, "DROP") || strings.Contains(f.FWPolicy, "REJECT") {
		active = true
	}
	public := publicListeners(f.Listen)
	switch {
	case active:
		add("fw", OK, T("Firewall aktiv"), strings.Join(fwd, " · "), "")
	case !f.Root && len(f.FW) > 0:
		add("fw", Info, T("Firewall-Regeln nicht lesbar (braucht root)"), strings.Join(fwd, " · "), "")
	case len(public) > 1:
		add("fw", Warn, T("Keine Firewall erkannt"), fmt.Sprintf(T("%d Ports offen für alle Adressen"), len(public)),
			T("ufw bzw. firewalld einschalten und nur die nötigen Ports freigeben. Vorher SSH freigeben, sonst sperrst du dich aus."))
	default:
		add("fw", Info, T("Keine Firewall erkannt"), T("nur SSH offen"), "")
	}

	// Ports
	for _, l := range public {
		if name, ok := riskyPorts[l.Port]; ok {
			lv := Warn
			if critPorts[l.Port] {
				lv = Bad
			}
			add(fmt.Sprintf("port-%d", l.Port), lv, fmt.Sprintf(T("Port %d (%s) offen für alle Adressen"), l.Port, name),
				strings.TrimSpace(l.Addr+" "+l.Proc), T("Nur an 127.0.0.1 oder ein internes Netz binden oder per Firewall beschränken."))
		}
	}

	// automatische Updates
	auto := false
	for _, a := range f.Auto {
		la := strings.ToLower(a)
		switch {
		case strings.HasPrefix(la, "unit ") && !strings.Contains(la, "apt-daily-upgrade"):
			auto = true // dnf-automatic, yum-cron
		case strings.Contains(la, "unattended-upgrade") && strings.Contains(la, `"1"`):
			auto = true
		}
	}
	switch {
	case auto:
		add("auto", OK, T("Automatische Sicherheitsupdates an"), strings.Join(f.Auto, " · "), "")
	default:
		add("auto", Warn, T("Keine automatischen Sicherheitsupdates"), strings.Join(f.Auto, " · "),
			T("Debian/Ubuntu: unattended-upgrades einrichten; RHEL-Familie: dnf-automatic (nur security). Alternativ Patch-Runde regelmäßig."))
	}

	// Konten
	var extra []string
	for _, u := range f.UID0 {
		if u != "root" {
			extra = append(extra, u)
		}
	}
	if len(extra) > 0 {
		add("uid0", Bad, T("Weitere Konten mit root-Rechten (UID 0)"), strings.Join(extra, ", "), T("Prüfen, wer das angelegt hat; UID 0 nur für root."))
	}
	if len(f.EmptyPW) > 0 {
		add("emptypw", Bad, T("Konten ohne Passwort"), strings.Join(f.EmptyPW, ", "), T("Passwort setzen oder Konto sperren (passwd -l)."))
	}

	// Zertifikate
	for _, c := range f.Certs {
		days := int(c.End.Sub(now).Hours() / 24)
		switch {
		case days < 0:
			add("cert", Bad, T("TLS-Zertifikat abgelaufen"), fmt.Sprintf("%s · %s", c.Path, c.End.Format("2006-01-02")), T("Zertifikat erneuern (certbot renew) und Dienst neu laden."))
		case days < 14:
			add("cert", Bad, fmt.Sprintf(T("TLS-Zertifikat läuft in %d Tagen ab"), days), c.Path, T("Erneuerung prüfen (certbot renew --dry-run)."))
		case days < 30:
			add("cert", Warn, fmt.Sprintf(T("TLS-Zertifikat läuft in %d Tagen ab"), days), c.Path, T("Erneuerung prüfen (certbot renew --dry-run)."))
		}
	}

	// Kleinigkeiten
	if f.NTP == "no" {
		add("ntp", Info, T("Uhrzeit wird nicht abgeglichen"), "NTPSynchronized=no", T("chrony oder systemd-timesyncd einschalten; falsche Uhrzeit stört Logs und Zertifikate."))
	}
	if f.SELinux == "0" {
		add("mac", Info, T("SELinux nur im Warnmodus"), "enforce=0", "")
	}

	sort.SliceStable(r.Findings, func(i, j int) bool { return r.Findings[i].Level > r.Findings[j].Level })
	return r
}

func publicListeners(ls []Listener) []Listener {
	seen := map[int]bool{}
	var out []Listener
	for _, l := range ls {
		a := l.Addr
		if a == "0.0.0.0" || a == "::" || a == "*" || a == "" {
			if !seen[l.Port] {
				seen[l.Port] = true
				out = append(out, l)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
