// Package sysprof erkennt, was für ein System hinter einer Verbindung steckt
// (domain.SystemProfile), ohne dabei etwas am Gerät anzustoßen:
//
//   - FromBanner liest nur die SSH-Kennung, die der Server ohnehin schickt.
//   - FromProbe wertet die unsichtbare Exec-Abfrage aus (nur bei Servern;
//     Netzwerkgeräte bekommen nie einen Exec-Kanal, siehe sshx.Dial).
//   - FromScreen liest mit, was im Terminal steht (Login-Banner, Ausgabe von
//     „show version“ usw.), und schärft das Profil nach.
//   - Rejection erkennt Fehlermeldungen wie „% Invalid input“, damit sich das
//     Profil abgelehnte Befehle merkt.
//
// Merge führt ein gespeichertes und ein frisch erkanntes Profil zusammen;
// Eingaben des Nutzers (Manual, Notizen) bleiben dabei erhalten.
package sysprof

import (
	"regexp"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

// FromBanner wertet die SSH-Kennung aus, z. B. "SSH-2.0-OpenSSH_9.6p1
// Ubuntu-3ubuntu13.5" oder "SSH-2.0-Mocana SSH 6.3".
func FromBanner(banner string) domain.SystemProfile {
	p := domain.SystemProfile{Banner: strings.TrimSpace(banner)}
	l := strings.ToLower(banner)
	set := func(fam, os, dia string) { p.Family, p.OS, p.Dialect = fam, os, dia }
	switch {
	case strings.Contains(l, "openssh_for_windows"):
		set(domain.FamilyWindows, "Windows (OpenSSH)", "")
	case strings.Contains(l, "rosssh") || strings.Contains(l, "mikrotik"):
		set(domain.FamilyNetwork, "MikroTik RouterOS", "routeros")
	case strings.Contains(l, "mocana") || strings.Contains(l, "procurve"):
		// HPE/Aruba ProCurve (ArubaOS-Switch) meldet sich mit Mocana SSH.
		set(domain.FamilyNetwork, "ArubaOS-Switch (ProCurve)", "procurve")
	case strings.Contains(l, "cisco"):
		set(domain.FamilyNetwork, "Cisco IOS", "cisco-ios")
	case strings.Contains(l, "huawei"):
		set(domain.FamilyNetwork, "Huawei VRP", "huawei-vrp")
	case strings.Contains(l, "comware") || strings.Contains(l, "h3c"):
		set(domain.FamilyNetwork, "HPE Comware", "comware")
	case strings.Contains(l, "fortissh"):
		set(domain.FamilyNetwork, "FortiOS", "fortios")
	case strings.Contains(l, "ubuntu"):
		set(domain.FamilyLinux, "Ubuntu", "posix")
	case strings.Contains(l, "debian"):
		set(domain.FamilyLinux, "Debian", "posix")
	case strings.Contains(l, "raspbian"):
		set(domain.FamilyLinux, "Raspberry Pi OS", "posix")
	case strings.Contains(l, "freebsd"):
		set(domain.FamilyBSD, "FreeBSD", "bsd")
	}
	return p
}

// FromProbe wertet die Ausgabe von ProbeCommand aus (eine Angabe pro Zeile).
func FromProbe(out string) domain.SystemProfile {
	lines := strings.Split(strings.ReplaceAll(out, "\r", ""), "\n")
	if len(lines) < probeLines {
		// Die echte Abfrage gibt immer mindestens fünf Zeilen aus. Kürzeres
		// ist eine Fehlermeldung des Geräts (z. B. ArubaOS-CX: „Command not
		// supported.“) und darf nicht als Systemname gelten.
		return domain.SystemProfile{}
	}
	get := func(i int) string {
		if i < len(lines) {
			return strings.TrimSpace(lines[i])
		}
		return ""
	}
	if u, ok := unifiProbe(get(6)); ok {
		u.Shell = get(2)
		return u
	}
	p := domain.SystemProfile{OS: get(0), Version: get(1), Shell: get(2), PkgMgr: get(3), Init: get(4)}
	k := strings.ToLower(p.Version + " " + p.OS)
	switch {
	case p.OS == "" && p.Version == "":
		return domain.SystemProfile{}
	case strings.Contains(k, "darwin") || strings.Contains(k, "macos"):
		p.Family, p.Dialect = domain.FamilyMacOS, "macos"
		if p.Init == "" || p.Init == "launchd" {
			p.Init = "launchd"
		}
	case strings.Contains(k, "bsd"):
		p.Family, p.Dialect = domain.FamilyBSD, "bsd"
	default:
		p.Family, p.Dialect = domain.FamilyLinux, "posix"
	}
	if p.OS == "" {
		p.OS = firstWord(p.Version)
	}
	p.Init = initName(p.Init)
	return p
}

// probeLines: so viele Zeilen hat die Ausgabe von ProbeCommand mindestens
// (fünf Angaben, jede mit Zeilenende).
const probeLines = 5

// ProbeRejected erkennt, dass ein Gerät die unsichtbare Abfrage gar nicht
// ausgeführt, sondern mit einer kurzen Fehlermeldung beantwortet hat, wie es
// Switches mit eigener CLI tun (ArubaOS-CX: „Command not supported.“). Dann
// ist es ein Netzwerkgerät. Rückgabe: die Meldung, sonst "".
func ProbeRejected(out string) string {
	out = strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
	if out == "" || strings.Count(out, "\n") >= probeLines-1 {
		return ""
	}
	if Rejection(out) != "" || probeRefusal.MatchString(out) {
		line, _, _ := strings.Cut(out, "\n")
		return strings.TrimSpace(line)
	}
	return ""
}

var probeRefusal = regexp.MustCompile(`(?i)not supported|invalid (input|command)|unknown command|unrecognized command|not allowed|not permitted|syntax error`)

// ProbeCommand: läuft unsichtbar in einer eigenen Exec-Sitzung (nur Server).
// Bewusst über sh, damit auch fish/zsh/csh als Login-Shell nichts ändern.
// Zeilen: System, Kernel, Shell, Paketmanager, Prozess 1, Locale und
// Ubiquiti-Kennung („board.name|/etc/version“, sonst nur „|“).
const ProbeCommand = `sh -c '(. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME") || (sw_vers -productName 2>/dev/null | tr "\n" " "; sw_vers -productVersion 2>/dev/null) || echo; uname -srm 2>/dev/null || echo; echo "$SHELL"; pm=; for x in apt-get dnf yum zypper pacman apk pkg brew; do if command -v $x >/dev/null 2>&1; then pm=$x; break; fi; done; echo "$pm"; cat /proc/1/comm 2>/dev/null || ps -p 1 -o comm= 2>/dev/null || echo; echo "${LC_ALL:-${LC_CTYPE:-$LANG}}"; echo "$(sed -n "s/^board\.name=//p" /etc/board.info 2>/dev/null | head -n 1)|$(head -n 1 /etc/version 2>/dev/null)"'`

// rxUniFiFW: /etc/version eines UniFi-Switches, z. B.
// „US.bcm5334x.v4.3.13.11253.200430.1420“ oder „USW.mt7621.v6.6.55.15189…“.
var rxUniFiFW = regexp.MustCompile(`^(US[A-Z]*)\.[\w-]+\.v(\d+\.\d+\.\d+(?:\.\d+)?)`)

// unifiProbe: Zeile 7 der Abfrage. Ubiquiti-Geräte haben /etc/board.info;
// Switches erkennt man am Modell (US-…, USW-…) bzw. an /etc/version „US…“.
func unifiProbe(line string) (domain.SystemProfile, bool) {
	board, ver, ok := strings.Cut(strings.TrimSpace(line), "|")
	board, ver = strings.TrimSpace(board), strings.TrimSpace(ver)
	if !ok || (board == "" && ver == "") {
		return domain.SystemProfile{}, false
	}
	m := rxUniFiFW.FindStringSubmatch(ver)
	b := strings.ToUpper(board)
	if m == nil && (!strings.HasPrefix(b, "US") || strings.HasPrefix(b, "USG")) {
		return domain.SystemProfile{}, false // UniFi-AP, EdgeRouter …: bleibt Linux
	}
	p := domain.SystemProfile{Family: domain.FamilyNetwork, OS: unifiName, Dialect: "unifi", Model: board}
	if m != nil {
		p.Version = m[2]
	}
	return p, true
}

const unifiName = "UniFi-Switch (Ubiquiti)"

func initName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	switch s {
	case "systemd", "launchd", "openrc", "runit", "s6-svscan", "tini", "dumb-init", "":
		return s
	case "init":
		return "sysvinit/init"
	}
	return s
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// ---------------------------------------------------------------- Bildschirm

type screenRule struct {
	rx                  *regexp.Regexp
	family, os, dialect string
}

// screenRules: Texte, an denen man ein System in der Terminal-Ausgabe
// sicher erkennt (Login-Banner, „show version“, typische Prompts). Reihenfolge
// zählt: Spezifisches zuerst.
var screenRules = []screenRule{
	{regexp.MustCompile(`(?i)arubaos-cx|\baos-cx\b`), domain.FamilyNetwork, "ArubaOS-CX", "arubaos-cx"},
	{regexp.MustCompile(`(?i)arubaos-switch|procurve|image stamp:|\bsoftware revision\s+:?\s*[A-Z]{1,2}\.\d{2}\.`), domain.FamilyNetwork, "ArubaOS-Switch (ProCurve)", "procurve"},
	{regexp.MustCompile(`(?i)cisco nexus|\bNX-OS\b`), domain.FamilyNetwork, "Cisco NX-OS", "cisco-ios"},
	{regexp.MustCompile(`(?i)cisco ios|cisco internetwork operating system|IOS-XE`), domain.FamilyNetwork, "Cisco IOS", "cisco-ios"},
	{regexp.MustCompile(`(?i)\bjunos\b`), domain.FamilyNetwork, "Juniper Junos", "junos"},
	{regexp.MustCompile(`(?i)mikrotik routeros|\[[\w.-]+@[\w.-]+\] >`), domain.FamilyNetwork, "MikroTik RouterOS", "routeros"},
	{regexp.MustCompile(`(?i)huawei versatile routing platform`), domain.FamilyNetwork, "Huawei VRP", "huawei-vrp"},
	{regexp.MustCompile(`(?i)comware`), domain.FamilyNetwork, "HPE Comware", "comware"},
	{regexp.MustCompile(`(?i)fortigate|fortios`), domain.FamilyNetwork, "FortiOS", "fortios"},
	{regexp.MustCompile(`(?im)welcome to unifi\s+us[^g\s!]|^\(UBNT\)\s*[>#]|^\s*model:\s+us[^g\s!][\w-]*\s*$`), domain.FamilyNetwork, unifiName, "unifi"},
	{regexp.MustCompile(`(?m)^PS [A-Z]:\\[^>]*> ?$`), domain.FamilyWindows, "", "powershell"},
	{regexp.MustCompile(`(?m)^[A-Z]:\\[^>]*> ?$`), domain.FamilyWindows, "", "cmd"},
}

var (
	rxProcurveRev   = regexp.MustCompile(`(?i)software revision\s*:?\s*([A-Z]{1,2}\.\d{2}\.\d{2}[\w.]*)`)
	rxProcurveModel = regexp.MustCompile(`(?m)^\s*((?:HPE?|Aruba)\s+(?:J\w{5}|JL\d{3}A)\s+[\w+./-]+)\s+Switch`)
	rxCXVersion     = regexp.MustCompile(`(?im)^\s*version\s*:\s*([A-Z]{2}\.\d{2}\.\d{2}[\w.]*)`)
	rxCXProduct     = regexp.MustCompile(`(?im)^\s*product name\s*:\s*(.+?)\s*$`)
	rxCiscoVersion  = regexp.MustCompile(`(?i)cisco ios.*?version\s+([\w.()]+)`)
	rxJunosVersion  = regexp.MustCompile(`(?im)^\s*junos:\s*(\S+)`)
	rxROSVersion    = regexp.MustCompile(`(?im)^\s*version:\s*(\d+\.\d+[\w.]*)`)
	rxUniFiVersion  = regexp.MustCompile(`(?im)^\s*version:\s+(\d+\.\d+\.\d+[\d.]*)`)
	rxUniFiModel    = regexp.MustCompile(`(?im)(?:^\s*model:\s+|welcome to unifi\s+)(US[^Gg\s!][\w-]*)`)
)

// FromScreen sucht in Terminaltext nach Merkmalen eines Systems. Gefunden
// wird nur, was eindeutig ist; sonst bleibt das Ergebnis leer.
func FromScreen(text string) domain.SystemProfile {
	var p domain.SystemProfile
	for _, r := range screenRules {
		if r.rx.MatchString(text) {
			p.Family, p.OS, p.Dialect = r.family, r.os, r.dialect
			break
		}
	}
	sub := func(rx *regexp.Regexp) string {
		if m := rx.FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
		return ""
	}
	switch p.Dialect {
	case "procurve":
		p.Version, p.Model = sub(rxProcurveRev), sub(rxProcurveModel)
	case "arubaos-cx":
		p.Version, p.Model = sub(rxCXVersion), sub(rxCXProduct)
	case "cisco-ios":
		p.Version = sub(rxCiscoVersion)
	case "junos":
		p.Version = sub(rxJunosVersion)
	case "routeros":
		p.Version = sub(rxROSVersion)
	case "unifi":
		p.Version, p.Model = sub(rxUniFiVersion), sub(rxUniFiModel)
	case "powershell", "cmd":
		p.OS = "Windows"
	}
	return p
}

// ---------------------------------------------------------------- Ablehnung

// rejections: Fehlermeldungen, mit denen ein System einen Befehl ablehnt, den
// es nicht kennt (falscher Dialekt), nicht einen, der nur scheitert.
var rejections = []*regexp.Regexp{
	regexp.MustCompile(`% ?Invalid input[^\n]*`),
	regexp.MustCompile(`(?m)^\s*Command not supported\.?`),
	regexp.MustCompile(`(?m)^\s*Invalid input:[^\n]*`),
	regexp.MustCompile(`% ?Unknown command[^\n]*`),
	regexp.MustCompile(`% ?Unrecognized command[^\n]*`),
	regexp.MustCompile(`(?m)^\s*unknown command[^\n]*`),
	regexp.MustCompile(`(?m)^\s*syntax error, expecting[^\n]*`),
	regexp.MustCompile(`bad command name[^\n]*`),
	regexp.MustCompile(`(?m)^\s*Command fail\. Return code[^\n]*`),
	regexp.MustCompile(`(?m)[:\s]command not found\s*$`),
	regexp.MustCompile(`is not recognized as (?:an internal or external command|the name of a cmdlet)[^\n]*`),
}

// Rejection prüft die Ausgabe nach einem Befehl auf eine Ablehnung wie
// „% Invalid input detected at '^' marker.“ und gibt die Meldung zurück.
func Rejection(output string) string {
	for _, rx := range rejections {
		if m := rx.FindString(output); m != "" {
			return strings.TrimSpace(strings.Join(strings.Fields(m), " "))
		}
	}
	return ""
}

// OutputAfter liefert den Text nach der letzten Zeile, in der cmd eingegeben
// wurde (also die Ausgabe dieses Befehls), sonst "".
func OutputAfter(screen, cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	lines := strings.Split(screen, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasSuffix(strings.TrimRight(lines[i], " "), cmd) {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return ""
}

// ---------------------------------------------------------------- Zusammenführen

// Merge legt ein frisch erkanntes Profil über ein gespeichertes. Leere Felder
// im neuen Profil ändern nichts; bei Manual bleiben System und Befehlssatz
// so, wie der Nutzer sie gesetzt hat. changed sagt, ob sich etwas geändert hat
// (dann lohnt sich Speichern).
func Merge(old domain.SystemProfile, fresh domain.SystemProfile) (domain.SystemProfile, bool) {
	p := old
	set := func(dst *string, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = v
		}
	}
	// Als Server gemerkt, jetzt aber als Netzwerkgerät erkannt (z. B. ein
	// ArubaOS-CX, dessen Fehlermeldung früher als Systemname gespeichert
	// wurde): die Server-Angaben stimmen nicht, also neu anfangen.
	if !p.Manual && fresh.Family == domain.FamilyNetwork && serverFamily(p.Family) {
		p.Family, p.OS, p.Dialect, p.Version, p.Shell, p.PkgMgr, p.Init = "", "", "", "", "", "", ""
	}
	// Ein spezifischerer Befund schlägt einen allgemeinen: "Netzwerkgerät
	// (unbekannt)" wird von "ArubaOS-CX" ersetzt, aber nie umgekehrt.
	generic := func(d string) bool { return d == "" || d == "network" }
	if !p.Manual {
		if fresh.Dialect != "" && (generic(p.Dialect) || !generic(fresh.Dialect)) {
			if fresh.Dialect != p.Dialect || fresh.OS != "" {
				set(&p.OS, fresh.OS)
			}
			p.Dialect = fresh.Dialect
		} else if p.OS == "" {
			set(&p.OS, fresh.OS)
		}
		if p.Family == "" || (fresh.Family != "" && !generic(fresh.Dialect)) {
			set(&p.Family, fresh.Family)
		}
	}
	set(&p.Version, fresh.Version)
	set(&p.Model, fresh.Model)
	set(&p.PkgMgr, fresh.PkgMgr)
	set(&p.Init, fresh.Init)
	set(&p.Shell, fresh.Shell)
	set(&p.Banner, fresh.Banner)
	changed := !same(old, p)
	if changed || p.DetectedAt.IsZero() {
		p.DetectedAt = time.Now()
	}
	return p, changed
}

func serverFamily(f string) bool {
	return f == domain.FamilyLinux || f == domain.FamilyBSD || f == domain.FamilyMacOS
}

func same(a, b domain.SystemProfile) bool {
	return a.Family == b.Family && a.OS == b.OS && a.Version == b.Version && a.Model == b.Model &&
		a.Dialect == b.Dialect && a.PkgMgr == b.PkgMgr && a.Init == b.Init && a.Shell == b.Shell && a.Banner == b.Banner
}

// FailedEntry: so steht ein abgelehnter Befehl im Profil.
func FailedEntry(cmd, msg string) string {
	cmd = strings.Join(strings.Fields(cmd), " ")
	if r := []rune(cmd); len(r) > 120 {
		cmd = string(r[:120]) + "…"
	}
	if r := []rune(msg); len(r) > 80 {
		msg = string(r[:80]) + "…"
	}
	return cmd + " → " + msg
}
