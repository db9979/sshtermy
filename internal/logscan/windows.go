package logscan

import (
	"bufio"
	"strconv"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// WindowsScript: nur lesendes PowerShell-Skript. Ereignisprotokolle System
// und Anwendung ab Stufe Warnung, aus dem Sicherheitsprotokoll nur
// Fehlanmeldungen, Kontosperren und Löschen des Protokolls (braucht
// Administratorrechte, sonst fehlt es still). Dazu gestoppte automatische
// Dienste, Startzeit und Windows-Updates.
func WindowsScript(hours int) string {
	return strings.ReplaceAll(windowsScript, "@H@", strconv.Itoa(hours))
}

// WindowsSinceScript: nur neue Ereignisse ab Zeitpunkt (Unix-Sekunden),
// Stufe bis maxLevel (2 = Fehler, 3 = Warnung). Für die Logwache.
func WindowsSinceScript(since int64, maxLevel int) string {
	return strings.NewReplacer("@T@", strconv.FormatInt(since, 10), "@L@", strconv.Itoa(maxLevel)).Replace(windowsSince)
}

const windowsScript = `$ErrorActionPreference='SilentlyContinue'
$s=(Get-Date).AddHours(-@H@)
function u($d){[int64](($d.ToUniversalTime()-[datetime]'1970-01-01').TotalSeconds)}
$e=@(Get-WinEvent -FilterHashtable @{LogName='System','Application';Level=1,2,3;StartTime=$s} -MaxEvents 4000)
$e+=@(Get-WinEvent -FilterHashtable @{LogName='Security';Id=4625,4740,1102;StartTime=$s} -MaxEvents 2000)
foreach($x in $e){$m=(($x.Message+'') -split "` + "`" + `r?` + "`" + `n")[0]
if($x.Id -eq 4625){$m='4625 '+$x.Properties[5].Value+' @ '+$x.Properties[19].Value}
"E|$(u $x.TimeCreated)|$($x.Level)|$($x.Id)|$($x.ProviderName)|$m"}
Get-CimInstance Win32_Service -Filter "StartMode='Auto' AND State<>'Running'"|%{"S|$($_.Name)|$($_.DisplayName)"}
"B|$(u (Get-CimInstance Win32_OperatingSystem).LastBootUpTime)"
Get-WinEvent -FilterHashtable @{LogName='System';ProviderName='Microsoft-Windows-WindowsUpdateClient';Id=19;StartTime=$s}|%{"U|$(u $_.TimeCreated)|$($_.Properties[0].Value)"}
"N|$(u (Get-Date))"
`

const windowsSince = `$ErrorActionPreference='SilentlyContinue'
$s=([datetime]'1970-01-01').AddSeconds(@T@).ToLocalTime()
function u($d){[int64](($d.ToUniversalTime()-[datetime]'1970-01-01').TotalSeconds)}
$l=1..@L@
$e=@(Get-WinEvent -FilterHashtable @{LogName='System','Application';Level=$l;StartTime=$s} -MaxEvents 200)
foreach($x in $e){$m=(($x.Message+'') -split "` + "`" + `r?` + "`" + `n")[0]
"E|$(u $x.TimeCreated)|$($x.Level)|$($x.Id)|$($x.ProviderName)|$m"}
"N|$(u (Get-Date))"
`

// Dienste, die auf „Automatisch“ stehen, aber nur bei Bedarf laufen.
var quietServices = map[string]bool{
	"gupdate": true, "gupdatem": true, "edgeupdate": true, "edgeupdatem": true, "mapsbroker": true, "sppsvc": true,
	"remoteregistry": true, "trustedinstaller": true, "tiledatamodelsvc": true, "cdpsvc": true, "usosvc": true,
	"wbiosrvc": true, "sysmain": true, "bits": true, "wuauserv": true, "shellhwdetection": true, "dosvc": true,
	"gpsvc": true, "clr_optimization_v4.0.30319_32": true, "clr_optimization_v4.0.30319_64": true, "msiscsi": true,
	"onesyncsvc": true, "wslservice": true, "stisvc": true, "tabletinputservice": true, "googleupdaterservice": true,
	"googleupdaterinternalservice": true, "intelaudioservice": true, "sense": true, "wisvc": true, "lfsvc": true,
}

// ParseWindows liest die Ausgabe von WindowsScript bzw. WindowsSinceScript.
func ParseWindows(out string, hours int, now time.Time) *Report {
	r := &Report{Kind: KindWindows, Hours: hours, At: now,
		Read: []string{i18n.T("Get-WinEvent System/Application (ab Warnung)"), "Security 4625/4740/1102", i18n.T("Get-Service (Auto, gestoppt)")}}
	var upd []Mark
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if len(line) < 2 || line[1] != '|' {
			continue
		}
		f := strings.SplitN(line[2:], "|", 5)
		switch line[0] {
		case 'E':
			if len(f) < 5 {
				continue
			}
			r.Lines++
			sec, _ := strconv.ParseInt(f[0], 10, 64)
			lvl, _ := strconv.Atoi(f[1])
			e := Entry{Time: time.Unix(sec, 0), Sev: sevOfWinLevel(lvl), ID: f[2], Source: winSource(f[3]), Msg: strings.TrimSpace(f[4])}
			if e.ID == "4625" || e.ID == "4740" || e.ID == "1102" {
				e.Sev = SevWarn
			}
			if e.Msg == "" {
				e.Msg = i18n.T("(ohne Text)")
			}
			r.add(e)
		case 'S':
			if len(f) < 2 || quietServices[strings.ToLower(f[0])] {
				continue
			}
			r.add(Entry{Time: now, Sev: SevWarn, ID: "STOPPED", Source: "Service Control Manager", Msg: f[0] + " " + f[1]})
		case 'B':
			if sec, err := strconv.ParseInt(f[0], 10, 64); err == nil && sec > 0 {
				r.Boot = time.Unix(sec, 0)
			}
		case 'U':
			if len(f) < 2 {
				continue
			}
			if sec, err := strconv.ParseInt(f[0], 10, 64); err == nil {
				upd = append(upd, Mark{Time: time.Unix(sec, 0), Kind: MarkPkg, Text: trimUpdate(f[1])})
			}
		case 'N':
			if sec, err := strconv.ParseInt(f[0], 10, 64); err == nil && sec > 0 {
				r.At = time.Unix(sec, 0) // Uhr des Servers für die nächste Abfrage
			}
		}
	}
	if !r.Boot.IsZero() && r.Boot.After(r.Since()) {
		r.Marks = append(r.Marks, Mark{Time: r.Boot, Kind: MarkBoot, Text: i18n.T("Neustart")})
	}
	r.Marks = append(r.Marks, groupPkgs(upd, r.Since())...)
	sortEntries(r.Entries)
	Group(r)
	return r
}

func sevOfWinLevel(l int) Sev {
	switch l {
	case 1:
		return SevCrit
	case 2:
		return SevErr
	case 3:
		return SevWarn
	}
	return SevInfo
}

// winSource: „Microsoft-Windows-Kernel-Power“ → „Kernel-Power“.
func winSource(p string) string {
	return strings.TrimPrefix(strings.TrimSpace(p), "Microsoft-Windows-")
}

func trimUpdate(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 60 {
		s = string(r[:59]) + "…"
	}
	return s
}
