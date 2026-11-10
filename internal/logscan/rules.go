package logscan

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Rule: bekanntes Muster mit Titel, Hinweis und lesenden Prüfbefehlen.
// $1 … $9 in Key, Title und Checks stehen für Gruppen aus Re.
type Rule struct {
	Kinds  []Kind // leer = überall
	IDs    []string
	Re     *regexp.Regexp
	Sev    Sev // -1 = Schwere der Zeile behalten
	Key    string
	Title  string
	Hint   string
	Checks []string
	// Down: passt die Zeile hierauf, gilt sie mindestens als Warnung
	// (Link down), sonst bleibt ihre Schwere (Link up = Info).
	Down *regexp.Regexp
	// Burst: in der Logwache gelten 20 gleiche Zeilen in 10 Minuten als
	// Fehler (Angriff auf Anmeldungen).
	Burst bool
	after func(*Finding)
}

const keep Sev = -1

var unix, win, network = []Kind{KindUnix}, []Kind{KindWindows}, []Kind{KindNetwork}

func re(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// flapping: viele Wechsel zwischen up und down auf einem Port.
func flapping(f *Finding) {
	if f.Count >= 6 {
		f.Sev = SevCrit
		port := firstPort(f)
		if i := strings.LastIndex(f.Key, "link:"); i >= 0 && f.Key[i+5:] != "" {
			port = f.Key[i+5:]
		}
		f.Title = fmt.Sprintf(i18n.T("Port %s flattert: Link %d× up/down"), port, f.Count)
		f.Hint = i18n.T("Viele Link-Wechsel: Kabel, Stecker/SFP, Gegenstelle (Energiesparen, Neustarts) oder eine Schleife prüfen.")
	}
}

func firstPort(f *Finding) string {
	if len(f.Ports) > 0 {
		return f.Ports[0]
	}
	return "?"
}

// manyFails: Anmeldeversuche ab 20 sind ein Angriff oder ein falsch
// eingetragenes Passwort, darunter Tippfehler.
func manyFails(f *Finding) {
	if f.Count < 5 {
		f.Sev = SevInfo
	}
}

var rules = []*Rule{
	// ------------------------------------------------ Linux/BSD
	{Kinds: unix, Re: re(`Out of memory: Killed process \d+ \(([^)]+)\)`), Sev: SevCrit, Key: "oom:$1",
		Title: "Speicher voll: Kernel beendet $1 (OOM)", Hint: "Der Kernel hat einen Prozess beendet, weil der Arbeitsspeicher voll war. Danach prüfen, ob der Dienst wieder läuft.",
		Checks: []string{"free -h", "journalctl -k --since -24h | grep -i -B2 -A8 'out of memory'", "ps aux --sort=-rss | head"}},
	{Kinds: unix, Re: re(`([\w@.:-]+)\.service: (?:Failed with result|Main process exited, code=(?:exited|killed|dumped))`), Sev: SevErr, Key: "unit:$1",
		Title: "Dienst $1 ist fehlgeschlagen", Checks: []string{"systemctl status $1", "journalctl -u $1 -n 50 --no-pager"}},
	{Kinds: unix, Re: re(`^FAILED-UNIT (\S+?)(?:\.service)?$`), Sev: SevErr, Key: "unit:$1",
		Title: "Dienst $1 ist fehlgeschlagen", Checks: []string{"systemctl status $1", "journalctl -u $1 -n 50 --no-pager"}},
	{Kinds: unix, Re: re(`(?i)Failed password for|authentication (?:failure|error)|Invalid user \S+ from|Connection closed by authenticating user`), Sev: SevWarn, Key: "authfail", Burst: true,
		Title: "SSH: fehlgeschlagene Anmeldungen", Hint: "Viele Versuche von fremden Adressen sind meist Scanner. Passwort-Login abschalten oder fail2ban nutzen.",
		Checks: []string{"sudo lastb -n 20", "grep -c 'Failed password' /var/log/auth.log"}, after: manyFails},
	{Kinds: unix, Re: re(`(?i)EXT4-fs error|XFS .*(?:error|corruption)|BTRFS (?:error|critical)|I/O error|Buffer I/O error|blk_update_request|critical medium error|Unrecovered read error`), Sev: SevCrit, Key: "disk-io",
		Title: "Platte/Dateisystem meldet E/A-Fehler", Hint: "Lese- oder Schreibfehler: Platte, Kabel oder Controller prüfen und Sicherung bereithalten.",
		Checks: []string{"dmesg -T | grep -iE 'error|fail' | tail -20", "sudo smartctl -H /dev/sda", "lsblk -o NAME,SIZE,MODEL,STATE"}},
	{Kinds: unix, Re: re(`(?i)No space left on device`), Sev: SevCrit, Key: "nospace",
		Title: "Platte voll („No space left on device“)", Checks: []string{"df -h", "df -i", "sudo du -xh / --max-depth=2 | sort -h | tail"}},
	{Kinds: unix, Re: re(`(?i)EXT4-fs warning.*Directory index full`), Sev: SevWarn, Key: "dirfull",
		Title: "Verzeichnisindex voll (sehr viele Dateien in einem Ordner)", Checks: []string{"df -i"}},
	{Kinds: unix, Re: re(`(\S+)\[\d+\]: segfault at`), Sev: SevErr, Key: "segv:$1",
		Title: "Programm abgestürzt: $1 (segfault)", Checks: []string{"journalctl --since -24h | grep -i segfault | tail"}},
	{Kinds: unix, Re: re(`(?i)nf_conntrack: table full`), Sev: SevCrit, Key: "conntrack",
		Title: "Verbindungstabelle voll (nf_conntrack), Pakete werden verworfen", Checks: []string{"sysctl net.netfilter.nf_conntrack_count net.netfilter.nf_conntrack_max"}},
	{Kinds: unix, Re: re(`(?i)(?:timesyncd|chronyd|ntpd).*(?:Timeout|no (?:NTP )?server|unreachable|Can't synchronise|No suitable source)`), Sev: SevWarn, Key: "ntp",
		Title: "Zeitabgleich (NTP) erreicht keinen Server", Hint: "Oft blockiert eine Firewall UDP 123 nach außen.", Checks: []string{"timedatectl", "chronyc sources 2>/dev/null"}},
	{Kinds: unix, Re: re(`(?i)temperature above threshold|thermal.*(?:critical|throttl)`), Sev: SevWarn, Key: "thermal",
		Title: "CPU zu heiß, Takt wird gedrosselt", Checks: []string{"sensors 2>/dev/null"}},
	{Kinds: unix, Re: re(`(?i)upstream timed out`), Sev: SevWarn, Key: "upstream-timeout",
		Title: "nginx: Backend antwortet zu langsam (upstream timed out)", Checks: []string{"systemctl --type=service --state=running | grep -iE 'php|fpm|gunicorn|node|uwsgi'"}},
	{Kinds: unix, Re: re(`(?i)Kernel panic|BUG: unable to handle|general protection fault|watchdog: BUG: soft lockup`), Sev: SevCrit, Key: "kpanic",
		Title: "Kernel-Fehler (Panic, Lockup oder Oops)", Checks: []string{"journalctl -k -b -1 -n 80 --no-pager"}},
	{Kinds: unix, Re: re(`(?i)too many (?:clients|connections)`), Sev: SevErr, Key: "toomany",
		Title: "Datenbank: zu viele Verbindungen", Checks: []string{"ss -tan state established | wc -l"}},

	// ------------------------------------------------ Windows (nach Ereignis-ID)
	{Kinds: win, IDs: []string{"41"}, Sev: SevCrit, Key: "41", Title: "Unerwarteter Neustart (Kernel-Power 41)",
		Hint:   "Windows wurde nicht sauber heruntergefahren: Stromausfall, Absturz oder hart ausgeschaltet.",
		Checks: []string{"Get-WinEvent -FilterHashtable @{LogName='System';Id=41,6008,1001} -MaxEvents 10 | ft TimeCreated,Id,Message -Wrap"}},
	{Kinds: win, IDs: []string{"6008"}, Sev: SevCrit, Key: "6008", Title: "Unerwartetes Herunterfahren (6008)"},
	{Kinds: win, IDs: []string{"1001"}, Re: re(`(?i)bugcheck|bluescreen|0x[0-9a-f]{8}`), Sev: SevCrit, Key: "bugcheck", Title: "Bluescreen (BugCheck)",
		Checks: []string{"Get-ChildItem C:\\Windows\\Minidump | Sort-Object LastWriteTime | Select-Object -Last 3"}},
	{Kinds: win, IDs: []string{"7031", "7034"}, Re: re(`(?i)(?:the |der dienst ")?"?([^"]+?)"? (?:service terminated|wurde unerwartet|dienst wurde unerwartet)`), Sev: SevErr, Key: "svccrash:$1",
		Title: "Dienst unerwartet beendet: $1", Checks: []string{"Get-Service | Where-Object DisplayName -like '$1*' | ft Status,Name,DisplayName"}},
	{Kinds: win, IDs: []string{"7031", "7034"}, Sev: SevErr, Key: "svccrash", Title: "Dienst unerwartet beendet"},
	{Kinds: win, IDs: []string{"7000", "7009", "7023", "7024", "1069"}, Sev: SevErr, Key: "svcstart:$0", Title: "Dienst startet nicht"},
	{Kinds: win, IDs: []string{"7", "11", "51", "129", "153", "157"}, Sev: SevErr, Key: "disk",
		Title:  "Datenträger: E/A-Fehler oder Wiederholungen",
		Hint:   "Fehler oder Zeitüberschreitungen beim Zugriff auf eine Platte: Zustand der Platte, Kabel und Controller prüfen.",
		Checks: []string{"Get-PhysicalDisk | ft FriendlyName,HealthStatus,OperationalStatus", "Get-PhysicalDisk | Get-StorageReliabilityCounter | ft DeviceId,ReadErrorsTotal,WriteErrorsTotal,Temperature"}},
	{Kinds: win, IDs: []string{"55", "98", "137"}, Sev: SevCrit, Key: "ntfs", Title: "Dateisystem beschädigt (NTFS)",
		Checks: []string{"Get-Volume | ft DriveLetter,FileSystemLabel,HealthStatus,SizeRemaining"}},
	{Kinds: win, IDs: []string{"4625"}, Re: re(`^4625 (\S*) @ (\S*)`), Sev: SevWarn, Key: "4625:$1", Burst: true,
		Title: "Anmeldung fehlgeschlagen: Konto „$1“", Hint: "Viele Versuche auf ein Konto: Angriff oder ein Dienst mit altem Passwort.",
		Checks: []string{"Get-WinEvent -FilterHashtable @{LogName='Security';Id=4625} -MaxEvents 20 | ft TimeCreated,@{n='Konto';e={$_.Properties[5].Value}},@{n='Von';e={$_.Properties[19].Value}}"},
		after:  manyFails},
	{Kinds: win, IDs: []string{"4740"}, Sev: SevErr, Key: "4740", Title: "Konto gesperrt (zu viele Fehlversuche)"},
	{Kinds: win, IDs: []string{"1102"}, Sev: SevCrit, Key: "1102", Title: "Sicherheitsprotokoll wurde gelöscht"},
	{Kinds: win, IDs: []string{"1000", "1002"}, Re: re(`(?i)(?:faulting application name|name der fehlerhaften anwendung|anwendung):?\s*([\w.-]+\.exe)`), Sev: SevErr, Key: "appcrash:$1",
		Title: "Programm abgestürzt: $1"},
	{Kinds: win, IDs: []string{"1000", "1002"}, Sev: SevErr, Key: "appcrash", Title: "Programm abgestürzt"},
	{Kinds: win, IDs: []string{"5719", "1129", "1053"}, Sev: SevErr, Key: "nodc", Title: "Kein Domänencontroller erreichbar",
		Checks: []string{"nltest /dsgetdc:", "Test-ComputerSecureChannel"}},
	{Kinds: win, IDs: []string{"1014"}, Sev: SevWarn, Key: "dns1014", Title: "DNS-Client: Namensauflösung läuft in Zeitüberschreitung"},
	{Kinds: win, IDs: []string{"10016"}, Sev: SevInfo, Key: "dcom", Title: "DCOM-Berechtigung (10016, meist harmlos)"},
	{Kinds: win, IDs: []string{"36887", "36874", "36888"}, Sev: SevWarn, Key: "schannel", Title: "TLS-Verbindung abgelehnt (Schannel)"},
	{Kinds: win, IDs: []string{"STOPPED"}, Re: re(`^(\S+) (.*)$`), Sev: SevWarn, Key: "stopped:$1",
		Title: "Automatischer Dienst läuft nicht: $2", Checks: []string{"Get-Service $1 | fl *", "Get-WinEvent -FilterHashtable @{LogName='System';ProviderName='Service Control Manager'} -MaxEvents 20 | Where-Object Message -like '*$2*' | ft TimeCreated,Id,Message -Wrap"}},

	// ------------------------------------------------ Switches und Firewalls
	{Kinds: network, Re: re(`(?i)port (\S+?) is now (?:off|on)-line|Interface (\S+?), changed state to (?:up|down)|(?:ether\S*|port|interface) (\S+?) link (?:up|down)|SNMP_TRAP_LINK_(?:UP|DOWN).*ifName (\S+?)[ ,]|Link (?:up|down) on port (\S+)|link status changed.*port (\S+)|\b((?:ether|sfp|combo|qsfp|wlan)[\w-]*) link (?:up|down)`),
		Sev: keep, Down: re(`(?i)off-line|\bdown\b|_DOWN`), Key: "link:$1$2$3$4$5$6$7", Title: "Port $1$2$3$4$5$6$7: Link wechselt (up/down)", after: flapping,
		Checks: []string{"show interfaces $1$2$3$4$5$6$7", "show lldp info remote-device $1$2$3$4$5$6$7"}},
	{Kinds: network, Re: re(`(?i)loop[- ]?protect.*?port (\S+)|loop (?:detected|guard).*?(?:port|interface) (\S+)|(?:port|interface) (\S+?)[ ,:].*loop detected|ERR-DISABLE.*?(?:on|interface) (\S+?)[ ,.]`), Sev: SevCrit, Key: "loop:$1$2$3$4",
		Title: "Schleife erkannt: Port $1$2$3$4 gesperrt", Hint: "Eine Netzwerkschleife (z. B. Kabel zwischen zwei Dosen, falsch gesteckter Switch) hat den Port abschalten lassen.",
		Checks: []string{"show loop-protect", "show interfaces $1$2$3$4"}},
	{Kinds: network, Re: re(`(?i)topology change|TCN|SPANTREE|STP.*(?:root|change)|MSTP.*change`), Sev: SevErr, Key: "stp",
		Title: "Spanning Tree: Topologieänderungen", Hint: "Häufige Topologieänderungen kommen meist von einem flatternden Port oder einer Schleife.",
		Checks: []string{"show spanning-tree"}},
	{Kinds: network, Re: re(`(?i)PoE.*(?:denied|fault|overload|over.?current|short|budget)|power (?:denied|fault)|insufficient power`), Sev: SevWarn, Key: "poe",
		Title: "PoE: Leistung verweigert oder Fehler", Checks: []string{"show power-over-ethernet brief"}},
	{Kinds: network, Re: re(`(?i)CRC|FCS|alignment error|input errors|excessive collisions|DUPLEX_MISMATCH|duplex mismatch`), Sev: SevWarn, Key: "crc",
		Title: "Übertragungsfehler (CRC/FCS, Duplex)", Hint: "Fehlerhafte Pakete deuten auf ein schlechtes Kabel, einen Stecker oder Duplex-Mismatch hin."},
	{Kinds: network, Re: re(`(?i)(?:invalid (?:password|user)|login (?:failed|failure)|authentication fail\w*|admin login failed|failed login).*?(?:from|for user|user)\s*:?\s*"?([^\s",]*)`), Sev: SevWarn, Key: "authfail", Burst: true,
		Title: "Fehlgeschlagene Anmeldungen am Gerät", after: manyFails},
	{Kinds: network, Re: re(`(?i)(?:IPsec|tunnel|vpn|IKE|phase ?[12]).*(?:down|fail|dpd|timeout|negotiate error)|(?:down|dpd).*(?:tunnel|vpn)`), Sev: SevErr, Key: "vpn",
		Title: "VPN: Tunnel bricht ab", Checks: []string{"get vpn ipsec tunnel summary"}},
	{Kinds: network, Re: re(`(?i)\bHA\b.*(?:failover|state chang|heartbeat|member (?:down|lost)|role change)|(?:failover|switchover) (?:to|occurred)`), Sev: SevCrit, Key: "ha",
		Title: "Hochverfügbarkeit: Umschaltung", Checks: []string{"get system ha status", "diagnose sys ha history read"}},
	{Kinds: network, Re: re(`(?i)conserve mode|memory (?:low|exhausted)|out of memory|malloc fail`), Sev: SevCrit, Key: "mem",
		Title: "Speicher knapp (conserve mode / out of memory)"},
	{Kinds: network, Re: re(`(?i)fan|power supply|PSU|temperature|over.?temp`), Sev: keep, Key: "hw",
		Title: "Hardware: Lüfter, Netzteil oder Temperatur", Checks: []string{"show system", "show environment"}},
	{Kinds: network, Re: re(`(?i)(?:dropped|deny|denied|blocked).*?(?:src(?:ip)?[= ]|from )(\d+\.\d+\.\d+\.\d+)`), Sev: SevInfo, Key: "deny:$1",
		Title: "Verworfen: Verkehr von $1"},
	{Kinds: network, Re: re(`(?i)duplicate (?:ip|address)|MAC (?:flap|move)|MACFLAP|address conflict`), Sev: SevErr, Key: "dup",
		Title: "Doppelte IP oder springende MAC-Adresse"},
}

// match: erste Regel, die zur Zeile passt.
func match(k Kind, e *Entry) (*Rule, []string) {
	for _, rl := range rules {
		if len(rl.Kinds) > 0 && !kindIn(k, rl.Kinds) {
			continue
		}
		if len(rl.IDs) > 0 {
			if !contains(rl.IDs, e.ID) {
				continue
			}
			if rl.Re == nil {
				return rl, []string{e.ID}
			}
		}
		if rl.Re == nil {
			continue
		}
		m := rl.Re.FindStringSubmatch(e.Msg)
		if m == nil && e.Source != "" {
			m = rl.Re.FindStringSubmatch(e.Source + ": " + e.Msg)
		}
		if m == nil {
			continue
		}
		return rl, m
	}
	return nil, nil
}

func kindIn(k Kind, ks []Kind) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

// defaultChecks: Prüfbefehle für Meldungen ohne Regel.
func defaultChecks(k Kind, e *Entry) []string {
	src := strings.TrimSpace(e.Source)
	switch k {
	case KindUnix:
		if src == "" || src == "kernel" {
			return []string{"journalctl -k --since -24h -p warning --no-pager | tail -40"}
		}
		return []string{"journalctl -t " + shellWord(src) + " --since -24h --no-pager | tail -40"}
	case KindWindows:
		if e.ID != "" && src != "" {
			return []string{fmt.Sprintf("Get-WinEvent -FilterHashtable @{ProviderName='%s';Id=%s} -MaxEvents 10 | fl TimeCreated,Message", strings.ReplaceAll(src, "'", "''"), e.ID)}
		}
	}
	return nil
}

func shellWord(s string) string {
	for _, r := range s {
		if !(r == '-' || r == '_' || r == '.' || r == '@' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}
