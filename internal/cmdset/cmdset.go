// Package cmdset beschreibt Befehlssätze: alles, was sshTermy über ein
// System oder einen Gerätehersteller weiß, in einer Datei pro Satz.
//
// Ein Befehlssatz sagt, woran man das System erkennt (SSH-Kennung,
// Terminaltext, Ausgabe des Versionsbefehls, Dateien auf Servern), welche
// Befehle die Host-Übersicht (F6), die Config-Sicherung und der Log-Blick
// tippen, wie deren Ausgabe gelesen wird, wie Syslog eingerichtet wird und
// welchen Hinweis die KI bekommt. Varianten decken alte und neue Modelle ab.
//
// # Herkunft
//
// Eingebaute Sätze liegen als YAML-Dateien im Programm (builtin/). Eigene
// Sätze liegen im Konfigordner unter befehlssaetze/, Sätze aus dem Team
// unter befehlssaetze/team/. Team-Sätze sind erst aktiv, wenn der Nutzer
// genau diesen Inhalt freigegeben hat (Prüfsumme).
//
// # Sicherheit
//
//   - Eigene Sätze können Gefahrenregeln nur hinzufügen, nie entfernen.
//   - Jeder Befehl, den sshTermy selbst tippt (F6, Sicherung, Log-Blick),
//     muss die „Nur lesen“-Prüfung bestehen; das Blättern-aus-Kommando muss
//     auf einer festen Liste stehen. Lesebefehle freigeben („read“) dürfen
//     nur eingebaute Sätze.
//   - Syslog-Schritte ändern das Gerät: Sie werden jedes Mal gezeigt und
//     Zeile für Zeile gegen die Gefahrenregeln geprüft. Platzhalter gibt es
//     nur für IP und Port, und beide werden vorher geprüft.
//   - Eigene Sätze haben eigene Kennungen; ihre Erkennung greift erst, wenn
//     kein eingebauter Satz passt.
//   - Größe, Anzahl und Länge der Einträge sind begrenzt; Go-Suchmuster
//     (RE2) laufen in linearer Zeit.
package cmdset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Familien (wie domain.Family*).
const (
	FamilyLinux   = "linux"
	FamilyMacOS   = "macos"
	FamilyBSD     = "bsd"
	FamilyWindows = "windows"
	FamilyNetwork = "network"
)

// Source: woher ein Satz kommt.
type Source int

const (
	Builtin Source = iota // im Programm
	Own                   // eigene Datei im Konfigordner
	Team                  // aus dem Team-Ordner (nur nach Freigabe aktiv)
)

// Set ist ein Befehlssatz, wie er in der Datei steht.
type Set struct {
	ID       string    `yaml:"id"`
	Name     string    `yaml:"name"`
	NameEN   string    `yaml:"name_en,omitempty"`
	Family   string    `yaml:"family,omitempty"`     // linux, bsd, windows, macos, network (Standard)
	Base     string    `yaml:"base,omitempty"`       // baut auf diesem Satz auf
	Order    int       `yaml:"order,omitempty"`      // Reihenfolge bei der Erkennung (kleiner zuerst)
	ReadKind string    `yaml:"read_check,omitempty"` // Regelgruppe für die Nur-lesen-Prüfung: network, unix, windows
	Hint     string    `yaml:"hint,omitempty"`       // Hinweis für die KI
	Detect   Detect    `yaml:"detect,omitempty"`
	Commands Commands  `yaml:"commands,omitempty"`
	Parse    Parse     `yaml:"parse,omitempty"`
	Logs     Logs      `yaml:"logs,omitempty"`
	Syslog   Syslog    `yaml:"syslog,omitempty"`
	Read     []string  `yaml:"read,omitempty"`   // nur eingebaut: weitere Lesebefehle (ganze Zeile)
	Danger   []Danger  `yaml:"danger,omitempty"` // zusätzliche Gefahrenregeln
	Variants []Variant `yaml:"variants,omitempty"`
	Samples  []Sample  `yaml:"samples,omitempty"`

	Source Source `yaml:"-"`
	File   string `yaml:"-"` // Pfad der Datei (eigene und Team-Sätze)
	Sum    string `yaml:"-"` // Prüfsumme des Dateiinhalts
	Raw    []byte `yaml:"-"`
	From   string `yaml:"-"` // Team: wer ihn geteilt hat (aus dem Paket)
	famEff string // geerbte Familie (Install)
}

// Detect: Merkmale, an denen man das System erkennt (Suchmuster).
type Detect struct {
	Banner  []string `yaml:"banner,omitempty"`  // SSH-Kennung
	Screen  []string `yaml:"screen,omitempty"`  // Terminaltext (Login, Prompt, show version)
	Version []string `yaml:"version,omitempty"` // Ausgabe des Versionsbefehls
	Probe   []string `yaml:"probe,omitempty"`   // Server: diese Datei gibt es
}

// Commands: Befehle, die sshTermy am Gerät tippt (alle nur lesend).
type Commands struct {
	PagingOff string  `yaml:"paging_off,omitempty"`
	Version   string  `yaml:"version,omitempty"`
	System    string  `yaml:"system,omitempty"`
	Memory    string  `yaml:"memory,omitempty"` // Ausgabe wird mit system zusammen gelesen
	Ports     string  `yaml:"ports,omitempty"`
	VLANs     string  `yaml:"vlans,omitempty"`
	Config    string  `yaml:"config,omitempty"`
	Extra     []Extra `yaml:"extra,omitempty"`
}

// Extra: Zusatzzeile in der Host-Übersicht, z. B. „VMs: 12“.
type Extra struct {
	Label   string `yaml:"label"`
	LabelEN string `yaml:"label_en,omitempty"`
	Cmd     string `yaml:"cmd"`
	Count   string `yaml:"count,omitempty"` // Wert = Anzahl der passenden Zeilen
	Pick    string `yaml:"pick,omitempty"`  // Wert = erste Klammer des ersten Treffers
}

// Parse: wie die Ausgaben gelesen werden. Ohne Angabe gilt der allgemeine
// Leser; die Muster überschreiben einzelne Werte. Wert = erste nicht leere
// Klammer. cpu: Klammer = belegt in %, benannt (?P<idle>…) = frei in %.
// mem: Klammer = belegt in %, oder benannt (?P<used>…)/(?P<free>…) mit
// (?P<total>…).
type Parse struct {
	Reader   string `yaml:"reader,omitempty"` // eingebauter Leser: junos, routeros, fortios, unifi
	Model    string `yaml:"model,omitempty"`
	Firmware string `yaml:"firmware,omitempty"`
	Name     string `yaml:"name,omitempty"`
	Uptime   string `yaml:"uptime,omitempty"`
	CPU      string `yaml:"cpu,omitempty"`
	Mem      string `yaml:"mem,omitempty"`
	PortUp   string `yaml:"port_up,omitempty"` // Zeile = aktiver Port
	PortDown string `yaml:"port_down,omitempty"`
	VLAN     string `yaml:"vlan,omitempty"` // Zeile mit ID und Name: Klammern 1/2 oder (?P<id>…)/(?P<name>…)
}

// Logs: Log-Blick (Alt+E) und Logwache.
type Logs struct {
	Commands []string `yaml:"commands,omitempty"`
	Ports    string   `yaml:"ports,omitempty"`
	Reader   string   `yaml:"reader,omitempty"` // procurve, arubaos-cx, routeros, fortios, busybox, junos, vrp, generic
}

// Syslog: Schritte, mit denen das Gerät Meldungen an sshTermy schickt.
type Syslog struct {
	Steps []string `yaml:"steps,omitempty"` // Platzhalter {{ip}} und {{port}}
	Note  string   `yaml:"note,omitempty"`  // statt Schritten: so geht es von Hand
	Save  string   `yaml:"save,omitempty"`  // Hinweis, wie man dauerhaft speichert
	// Abort: verwirft eine abgebrochene Einrichtung und verlässt den
	// Konfigurationsmodus. Nur bekannte Befehle (end, quit, return, abort …).
	Abort []string `yaml:"abort,omitempty"`
}

// Danger: zusätzliche Gefahrenregel (Suchmuster pro Zeile, Grund).
type Danger struct {
	Rx       string `yaml:"rx"`
	Reason   string `yaml:"reason"`
	ReasonEN string `yaml:"reason_en,omitempty"`
}

// Variant: Abweichungen für bestimmte Modelle oder Firmware-Stände.
type Variant struct {
	Name     string   `yaml:"name"`
	Model    string   `yaml:"model,omitempty"`    // Suchmuster auf das Modell
	Firmware string   `yaml:"firmware,omitempty"` // Suchmuster auf die Firmware
	Hint     string   `yaml:"hint,omitempty"`     // kommt zum Hinweis dazu
	Commands Commands `yaml:"commands,omitempty"`
	Parse    Parse    `yaml:"parse,omitempty"`
	Logs     Logs     `yaml:"logs,omitempty"`
	Syslog   Syslog   `yaml:"syslog,omitempty"`
}

// Sample: Beispielausgaben für den Selbsttest.
type Sample struct {
	Name    string `yaml:"name,omitempty"`
	Banner  string `yaml:"banner,omitempty"`
	Screen  string `yaml:"screen,omitempty"`
	Version string `yaml:"version,omitempty"`
	System  string `yaml:"system,omitempty"`
	Ports   string `yaml:"ports,omitempty"`
	VLANs   string `yaml:"vlans,omitempty"`
	Logs    string `yaml:"logs,omitempty"`
	Expect  Expect `yaml:"expect,omitempty"`
}

// Expect: was der Selbsttest erwartet (leer = egal).
type Expect struct {
	Model     string `yaml:"model,omitempty"`
	Firmware  string `yaml:"firmware,omitempty"`
	Name      string `yaml:"name,omitempty"`
	Uptime    string `yaml:"uptime,omitempty"`
	CPU       *int   `yaml:"cpu,omitempty"`
	Mem       *int   `yaml:"mem,omitempty"`
	PortsUp   *int   `yaml:"ports_up,omitempty"`
	PortsDown *int   `yaml:"ports_down,omitempty"`
	VLANs     *int   `yaml:"vlans,omitempty"`
	LogLines  *int   `yaml:"log_lines,omitempty"` // erkannte Log-Einträge
	Variant   string `yaml:"variant,omitempty"`
}

// Grenzen für Dateien (eigene und Team-Sätze, eingebaute ebenso).
const (
	MaxFileSize  = 64 << 10
	maxPatterns  = 32
	maxPattern   = 512
	maxCommand   = 300
	maxHint      = 2000
	maxVariants  = 16
	maxSamples   = 16
	maxExtras    = 8
	maxSteps     = 12
	maxDanger    = 32
	maxBaseDepth = 4
)

var (
	reID        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
	reProbePath = regexp.MustCompile(`^/[A-Za-z0-9._/+-]{1,120}$`)
	rePlace     = regexp.MustCompile(`\{\{\s*([^}]*?)\s*\}\}`)

	// pagingOK: Befehle, die nur das Blättern dieser Sitzung abschalten.
	pagingOK = regexp.MustCompile(`(?i)^(no page|terminal length 0|terminal pager 0|terminal datadump|screen-length disable|screen-length 0 temporary|` +
		`set cli screen-length 0|set cli pager off|skip-page-display|disable clipaging|terminal more disable|terminal no more|` +
		`set terminal length 0|terminal length 0 temporary|config paging disable|no paging)$`)

	readers    = map[string]bool{"": true, "junos": true, "routeros": true, "fortios": true, "unifi": true}
	logReaders = map[string]bool{"": true, "generic": true, "procurve": true, "arubaos-cx": true, "routeros": true, "fortios": true, "busybox": true, "junos": true, "vrp": true}
	families   = map[string]bool{FamilyLinux: true, FamilyMacOS: true, FamilyBSD: true, FamilyWindows: true, FamilyNetwork: true}
	readKinds  = map[string]bool{"": true, "network": true, "unix": true, "windows": true}
)

// CheckRead prüft, ob ein Befehl nur liest (kind: network, unix, windows).
// Das Paket safety trägt sich hier ein; ohne Prüfung lehnt Validate eigene
// Sätze ab.
var CheckRead func(cmd, kind string) bool

// Parse liest eine Datei.
func ParseFile(raw []byte) (*Set, error) {
	if len(raw) > MaxFileSize {
		return nil, fmt.Errorf(i18n.T("Datei zu groß (höchstens %d KB)"), MaxFileSize>>10)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var s Set
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf(i18n.T("Datei nicht lesbar: %v"), err)
	}
	s.Raw = raw
	s.Sum = Sum(raw)
	return &s, nil
}

// Sum: Prüfsumme eines Dateiinhalts (für Freigaben).
func Sum(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// EffFamily: Familie mit Standard.
func (s *Set) EffFamily() string {
	switch {
	case s.Family != "":
		return s.Family
	case s.famEff != "":
		return s.famEff
	}
	return FamilyNetwork
}

// DisplayName: Name in der aktiven Sprache.
func (s *Set) DisplayName() string {
	if i18n.English() && s.NameEN != "" {
		return s.NameEN
	}
	return s.Name
}

// readKind: Regelgruppe der Nur-lesen-Prüfung.
func (s *Set) readKind() string {
	if s.ReadKind != "" {
		return s.ReadKind
	}
	if s.Family == "" && s.famEff == "" && s.Base != "" && s.Base != s.ID {
		// Familie nicht angegeben: wie die Grundlage (vor Install noch unbekannt)
		if b := Resolve(s.Base, "", ""); b != nil {
			return b.readKind()
		}
	}
	switch s.EffFamily() {
	case FamilyNetwork:
		return "network"
	case FamilyWindows:
		return "windows"
	}
	return "unix"
}

// Validate prüft einen Satz. Eingebaute Sätze unterliegen denselben Regeln,
// dürfen aber zusätzlich Lesebefehle freigeben.
func (s *Set) Validate() error {
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(i18n.T(format), a...)) }
	if !reID.MatchString(s.ID) {
		bad("Kennung „%s“: nur a–z, 0–9 und -, 2 bis 32 Zeichen", s.ID)
	}
	if strings.TrimSpace(s.Name) == "" || len([]rune(s.Name)) > 60 {
		bad("Name fehlt oder ist länger als 60 Zeichen")
	}
	if s.Family != "" && !families[s.Family] {
		bad("Familie „%s“ unbekannt (linux, bsd, macos, windows, network)", s.Family)
	}
	if !readKinds[s.ReadKind] {
		bad("read_check „%s“ unbekannt (network, unix, windows)", s.ReadKind)
	}
	if s.Base == s.ID && s.ID != "" {
		bad("Ein Satz kann nicht auf sich selbst aufbauen")
	}
	if len([]rune(s.Hint)) > maxHint {
		bad("KI-Hinweis länger als %d Zeichen", maxHint)
	}
	pats := func(what string, list []string, anchored bool) {
		if len(list) > maxPatterns {
			bad("%s: höchstens %d Muster", what, maxPatterns)
		}
		for _, p := range list {
			pat(what, p, anchored, bad)
		}
	}
	pats("detect.banner", s.Detect.Banner, false)
	pats("detect.screen", s.Detect.Screen, false)
	pats("detect.version", s.Detect.Version, false)
	if len(s.Detect.Probe) > maxPatterns {
		bad("detect.probe: höchstens %d Pfade", maxPatterns)
	}
	for _, p := range s.Detect.Probe {
		if !reProbePath.MatchString(p) {
			bad("detect.probe „%s“: nur ein absoluter Pfad ohne Leer- und Sonderzeichen", p)
		}
	}
	if len(s.Read) > 0 && s.Source != Builtin {
		bad("Lesebefehle freigeben (read) dürfen nur eingebaute Befehlssätze")
	}
	pats("read", s.Read, true)
	if len(s.Danger) > maxDanger {
		bad("danger: höchstens %d Regeln", maxDanger)
	}
	for _, d := range s.Danger {
		pat("danger", d.Rx, false, bad)
		if strings.TrimSpace(d.Reason) == "" || len([]rune(d.Reason)) > 120 {
			bad("danger: Grund fehlt oder ist länger als 120 Zeichen")
		}
	}
	s.checkCommands("commands", s.Commands, bad)
	s.checkParse("parse", s.Parse, bad)
	s.checkLogs("logs", s.Logs, bad)
	checkSyslog("syslog", s.Syslog, bad)
	if len(s.Variants) > maxVariants {
		bad("höchstens %d Varianten", maxVariants)
	}
	for i, v := range s.Variants {
		w := fmt.Sprintf("variants[%d]", i+1)
		if strings.TrimSpace(v.Name) == "" {
			bad("%s: Name fehlt", w)
		}
		if v.Model == "" && v.Firmware == "" {
			bad("%s: braucht model oder firmware", w)
		}
		pat(w+".model", v.Model, false, bad)
		pat(w+".firmware", v.Firmware, false, bad)
		if len([]rune(v.Hint)) > maxHint {
			bad("%s: KI-Hinweis zu lang", w)
		}
		s.checkCommands(w+".commands", v.Commands, bad)
		s.checkParse(w+".parse", v.Parse, bad)
		s.checkLogs(w+".logs", v.Logs, bad)
		checkSyslog(w+".syslog", v.Syslog, bad)
	}
	if len(s.Samples) > maxSamples {
		bad("höchstens %d Beispiele", maxSamples)
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func pat(what, p string, anchored bool, bad func(string, ...any)) {
	if p == "" {
		return
	}
	if len(p) > maxPattern {
		bad("%s: Muster länger als %d Zeichen", what, maxPattern)
		return
	}
	if _, err := regexp.Compile(p); err != nil {
		bad("%s: Muster „%s“ ungültig: %v", what, p, err)
		return
	}
	if anchored && (!strings.HasPrefix(p, "^") || !strings.HasSuffix(p, "$")) {
		bad("%s: Muster „%s“ muss mit ^ beginnen und mit $ enden", what, p)
	}
}

// checkCommands: alles, was sshTermy tippt, muss lesen; Blättern aus nur
// aus der festen Liste.
func (s *Set) checkCommands(w string, c Commands, bad func(string, ...any)) {
	if c.PagingOff != "" && !pagingOK.MatchString(strings.TrimSpace(c.PagingOff)) {
		bad("%s.paging_off „%s“: nur bekannte Befehle, die das Blättern dieser Sitzung abschalten", w, c.PagingOff)
	}
	for _, x := range []struct{ k, v string }{{"version", c.Version}, {"system", c.System}, {"memory", c.Memory}, {"ports", c.Ports}, {"vlans", c.VLANs}, {"config", c.Config}} {
		s.readCmd(w+"."+x.k, x.v, bad)
	}
	if len(c.Extra) > maxExtras {
		bad("%s.extra: höchstens %d Zeilen", w, maxExtras)
	}
	for _, e := range c.Extra {
		if strings.TrimSpace(e.Label) == "" || len([]rune(e.Label)) > 12 {
			bad("%s.extra: Beschriftung fehlt oder ist länger als 12 Zeichen", w)
		}
		if strings.TrimSpace(e.Cmd) == "" {
			bad("%s.extra „%s“: Befehl fehlt", w, e.Label)
		}
		s.readCmd(w+".extra", e.Cmd, bad)
		pat(w+".extra.count", e.Count, false, bad)
		pat(w+".extra.pick", e.Pick, false, bad)
	}
}

func (s *Set) readCmd(w, cmd string, bad func(string, ...any)) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	if len(cmd) > maxCommand || strings.ContainsAny(cmd, "\r\n") {
		bad("%s: Befehl zu lang oder mehrzeilig", w)
		return
	}
	if s.Source == Builtin && s.readBuiltin(cmd) {
		return
	}
	if CheckRead == nil {
		bad("%s: Nur-lesen-Prüfung nicht verfügbar", w)
		return
	}
	if !CheckRead(cmd, s.readKind()) {
		bad("%s: „%s“ ist kein reiner Lesebefehl; sshTermy tippt nur Befehle, die nichts ändern", w, cmd)
	}
}

// readBuiltin: steht der Befehl auf der Leseliste dieses (eingebauten) Satzes?
func (s *Set) readBuiltin(cmd string) bool {
	for _, p := range s.Read {
		if rx, err := regexp.Compile(p); err == nil && rx.MatchString(cmd) {
			return true
		}
	}
	return false
}

func (s *Set) checkParse(w string, p Parse, bad func(string, ...any)) {
	if !readers[p.Reader] {
		bad("%s.reader „%s“ unbekannt (junos, routeros, fortios, unifi)", w, p.Reader)
	}
	for _, x := range []string{p.Model, p.Firmware, p.Name, p.Uptime, p.CPU, p.Mem, p.PortUp, p.PortDown, p.VLAN} {
		pat(w, x, false, bad)
	}
}

func (s *Set) checkLogs(w string, l Logs, bad func(string, ...any)) {
	if !logReaders[l.Reader] {
		bad("%s.reader „%s“ unbekannt (generic, procurve, arubaos-cx, routeros, fortios, busybox, junos, vrp)", w, l.Reader)
	}
	if len(l.Commands) > 6 {
		bad("%s.commands: höchstens 6 Befehle", w)
	}
	for _, c := range l.Commands {
		s.readCmd(w+".commands", c, bad)
	}
	s.readCmd(w+".ports", l.Ports, bad)
}

func checkSyslog(w string, sl Syslog, bad func(string, ...any)) {
	if len(sl.Steps) > maxSteps {
		bad("%s.steps: höchstens %d Schritte", w, maxSteps)
	}
	for _, st := range sl.Steps {
		if len(st) > maxCommand || strings.ContainsAny(st, "\r\n") || strings.TrimSpace(st) == "" {
			bad("%s.steps: Schritt leer, zu lang oder mehrzeilig", w)
		}
		for _, m := range rePlace.FindAllStringSubmatch(st, -1) {
			if m[1] != "ip" && m[1] != "port" {
				bad("%s.steps: Platzhalter {{%s}} unbekannt (nur {{ip}} und {{port}})", w, m[1])
			}
		}
	}
	if len(sl.Abort) > 4 {
		bad("%s.abort: höchstens 4 Befehle", w)
	}
	for _, c := range sl.Abort {
		if !abortOK.MatchString(c) {
			bad("%s.abort: „%s“ ist kein bekannter Befehl zum Verlassen (end, exit, quit, return, abort, rollback, exit configuration-mode)", w, c)
		}
	}
	if len([]rune(sl.Note)) > 400 || len([]rune(sl.Save)) > 120 {
		bad("%s: note oder save zu lang", w)
	}
}

var abortOK = regexp.MustCompile(`(?i)^\s*(end|exit|quit|return|abort|rollback|exit configuration-mode|top)\s*$`)

// SyslogAbort: Befehle, die eine abgebrochene Einrichtung verwerfen. Ohne
// Angabe im Satz aus dem ersten Schritt abgeleitet.
func (s *Set) SyslogAbort() []string {
	if len(s.Syslog.Abort) > 0 {
		return s.Syslog.Abort
	}
	if len(s.Syslog.Steps) == 0 {
		return nil
	}
	first := strings.ToLower(strings.TrimSpace(s.Syslog.Steps[0]))
	switch {
	case first == "system-view":
		return []string{"return"}
	case strings.HasPrefix(first, "config log ") || strings.HasPrefix(first, "config system "):
		return []string{"abort"}
	case first == "configure" || first == "configure terminal" || first == "config":
		return []string{"end"}
	}
	return nil
}

// SyslogSteps setzt IP und Port ein. Beide werden geprüft, damit nichts
// anderes in die Gerätekonfiguration gelangt.
func SyslogSteps(steps []string, ip string, port int) ([]string, error) {
	if net.ParseIP(ip) == nil {
		return nil, fmt.Errorf(i18n.T("„%s“ ist keine IP-Adresse"), ip)
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf(i18n.T("Port %d ungültig"), port)
	}
	out := make([]string, 0, len(steps))
	for _, st := range steps {
		out = append(out, rePlace.ReplaceAllStringFunc(st, func(m string) string {
			switch strings.TrimSpace(strings.Trim(m, "{}")) {
			case "ip":
				return ip
			case "port":
				return strconv.Itoa(port)
			}
			return m
		}))
	}
	return out, nil
}
