package safety

import (
	"regexp"
	"strings"
)

// ReadOnly: ändert der Befehl sicher nichts am System? Der Agent-Modus führt
// in der Stufe „Vorsichtig“ (und auf Produktiv-Hosts) nur solche Befehle ohne
// Rückfrage aus. Die Prüfung ist absichtlich streng: im Zweifel false.
func ReadOnly(cmd, kind string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	switch kind {
	case Network:
		return netReadOnly(cmd)
	case "windows":
		return winReadOnly(cmd)
	}
	return unixReadOnly(cmd)
}

// harmlose Umleitungen: Fehler verwerfen oder zusammenführen
var safeRedirect = regexp.MustCompile(`\s[12]?>\s*/dev/null|\s2>&1`)

// Schreibende Umleitungen, Befehlsersetzung und Hintergrundjobs ⇒ nie „nur lesen“.
var unsafeShell = regexp.MustCompile("[>`]|\\$\\(|<\\(|&\\s*$|\\s&\\s")

var splitOps = regexp.MustCompile(`\|\||&&|;|\|`)

func unixReadOnly(cmd string) bool {
	c := safeRedirect.ReplaceAllString(" "+cmd, " ")
	if unsafeShell.MatchString(c) || strings.Contains(c, "\n") {
		return false
	}
	for _, seg := range splitOps.Split(c, -1) {
		if !unixSegment(strings.Fields(seg)) {
			return false
		}
	}
	return true
}

// Befehle, die nur lesen, egal mit welchen Argumenten.
var unixRead = set("ls", "ll", "cat", "head", "tail", "grep", "egrep", "fgrep", "zgrep", "zcat", "wc", "df", "du", "free",
	"uptime", "uname", "id", "whoami", "groups", "ps", "pgrep", "stat", "file", "which", "whereis", "type",
	"echo", "printf", "printenv", "lsblk", "lsof", "ss", "netstat", "dig", "nslookup", "host", "sort",
	"cut", "tr", "column", "getent", "lscpu", "lsmod", "lspci", "lsusb", "findmnt", "readlink", "realpath",
	"basename", "dirname", "md5sum", "sha1sum", "sha256sum", "nproc", "last", "w", "who", "test", "[", "true", "pwd",
	"diff", "cmp", "jq", "nl", "rev", "tac", "seq", "vmstat", "iostat", "mpstat", "dmesg", "journalctl",
	"arch", "locale", "getconf", "apt-cache", "apt-config", "tty", "mount")

// Befehle, die nur mit bestimmten Unterbefehlen lesen.
var unixSub = map[string]map[string]bool{
	"systemctl":   set("status", "is-active", "is-enabled", "is-failed", "list-units", "list-unit-files", "list-timers", "show", "cat"),
	"service":     set("status", "--status-all"),
	"docker":      set("ps", "images", "inspect", "logs", "version", "info", "stats"),
	"podman":      set("ps", "images", "inspect", "logs", "version", "info"),
	"apt":         set("list", "show", "policy", "search"),
	"dnf":         set("list", "info", "search", "repolist"),
	"yum":         set("list", "info", "search", "repolist"),
	"git":         set("status", "log", "diff", "show"),
	"ip":          set("a", "addr", "address", "r", "route", "link", "l", "neigh", "-br", "-4", "-6", "-s"),
	"timedatectl": set("status", "show"),
	"hostnamectl": set("status"),
	"ufw":         set("status"),
	"kubectl":     set("get", "describe", "logs", "top", "version"),
	"crontab":     set("-l"),
	"dpkg":        set("-l", "-s", "-L", "--list", "--status"),
	"rpm":         set("-q", "-qa", "-qi", "-ql"),
	"logrotate":   set("-d", "--debug"),
	"nginx":       set("-t", "-T", "-v", "-V"),
	"iptables":    set("-L", "-S", "--list"),
	"nft":         set("list"),
}

// nvidia-smi: Schalter, die nur lesen (alles andere fragt nach)
var smiRead = set("-L", "--list-gpus", "-q", "--query", "-d", "--display", "-i", "--id", "-x", "--xml-format",
	"--query-gpu", "--query-compute-apps", "--query-supported-clocks", "--query-retired-pages",
	"--query-remapped-rows", "--query-accounted-apps", "--format", "-l", "--loop", "-lms", "--loop-ms",
	"-m", "-s", "-u", "--unit", "-h", "--help")

// ohne Argumente ebenfalls nur lesend
var unixBare = set("timedatectl", "hostnamectl", "ip", "systemctl", "docker", "git")

func unixSegment(f []string) bool {
	for len(f) > 0 && (f[0] == "sudo" || f[0] == "command" || f[0] == "time" || strings.Contains(f[0], "=")) {
		f = f[1:] // sudo ls, LANG=C df …
		for len(f) > 0 && strings.HasPrefix(f[0], "-") {
			return false // sudo -u … / sudo -s: lieber nachfragen
		}
	}
	if len(f) == 0 {
		return false
	}
	name := f[0]
	args := f[1:]
	switch name {
	case "find":
		for _, a := range args {
			switch a {
			case "-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls":
				return false
			}
		}
		return true
	case "sort":
		for _, a := range args {
			if strings.HasPrefix(a, "-o") || strings.HasPrefix(a, "--output") {
				return false
			}
		}
		return true
	case "date", "hostname", "env":
		// date -s, hostname neu, env cmd … ändern etwas oder starten andere Befehle
		for _, a := range args {
			if !strings.HasPrefix(a, "-") || a == "-s" || strings.HasPrefix(a, "--set") || a == "-F" || a == "--file" {
				if name == "date" && strings.HasPrefix(a, "+") {
					continue
				}
				return false
			}
		}
		return true
	case "ping":
		for _, a := range args {
			if a == "-c" || strings.HasPrefix(a, "-c") {
				return true // ohne -c läuft ping unter Linux endlos
			}
		}
		return false
	case "dmesg", "mount":
		if name == "mount" {
			return len(args) == 0
		}
		for _, a := range args {
			switch a {
			case "-c", "-C", "--clear", "--read-clear", "-n", "-D", "-E", "--console-off", "--console-on":
				return false
			}
			if strings.HasPrefix(a, "--console-level") || strings.HasPrefix(a, "-n") {
				return false
			}
		}
		return true
	case "nvidia-smi":
		// nur Abfragen; -r (GPU-Reset), -pl (Leistungsgrenze), -pm, -c … ändern etwas
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !smiRead[strings.SplitN(a, "=", 2)[0]] {
				return false
			}
		}
		return true
	case "sensors":
		for _, a := range args {
			if a == "-s" || a == "--set" {
				return false
			}
		}
		return true
	case "ss":
		for _, a := range args {
			if a == "--kill" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "K")) {
				return false // ss -K beendet Verbindungen
			}
		}
		return true
	case "git":
		for _, a := range args {
			if strings.HasPrefix(a, "--output") || strings.HasPrefix(a, "--ext-diff") {
				return false
			}
		}
	case "ip":
		for _, a := range args {
			switch a {
			case "add", "del", "delete", "set", "flush", "change", "replace", "append":
				return false
			}
		}
	case "journalctl":
		for _, a := range args {
			if strings.HasPrefix(a, "--vacuum") || a == "--rotate" || a == "--flush" {
				return false
			}
		}
		return true
	}
	if unixRead[name] {
		return true
	}
	if subs, ok := unixSub[name]; ok {
		if len(args) == 0 {
			return unixBare[name]
		}
		return subs[args[0]]
	}
	return false
}

var netShow = regexp.MustCompile(`(?i)^(show|sh|display|dis|ping|traceroute|tracert|dir)\b`)

// FortiOS „get …“, RouterOS „/… print“ und „/export“
var netVendorRead = regexp.MustCompile(`(?i)^(get\s+\S|/[\w/ -]*\bprint\b|/export\b)`)

// Junos „| save datei“, RouterOS „file=…“ schreiben eine Datei auf das Gerät.
var netWrites = regexp.MustCompile(`(?i)\|\s*save\b|\bfile=`)

func netReadOnly(cmd string) bool {
	if strings.ContainsAny(cmd, ";\n") || netWrites.MatchString(cmd) {
		return false
	}
	// show … | include x ist auf Switches üblich und harmlos
	return (netShow.MatchString(cmd) || netVendorRead.MatchString(cmd)) && !strings.Contains(strings.ToLower(cmd), " > ")
}

// Format-Table/List/Wide/Custom/Hex nur ausgeben; Format-Volume und „format C:“ formatieren!
var winVerb = regexp.MustCompile(`(?i)^((get|test|select|where|sort|measure|resolve)(-|$)|out-string$|format-(table|list|wide|custom|hex)$)`)
var winRead = set("dir", "ls", "type", "cat", "ipconfig", "systeminfo", "hostname", "whoami", "tasklist", "netstat",
	"nslookup", "ver", "echo", "findstr", "where", "gci", "gc", "gps", "gsv", "select", "sort", "ft", "fl")

func winReadOnly(cmd string) bool {
	if strings.ContainsAny(cmd, ";>`\n&{}") || strings.Contains(cmd, "$(") {
		return false
	}
	for _, seg := range strings.Split(cmd, "|") {
		f := strings.Fields(seg)
		if len(f) == 0 {
			return false
		}
		n := strings.ToLower(f[0])
		if !winVerb.MatchString(n) && !winRead[n] {
			return false
		}
	}
	return true
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
