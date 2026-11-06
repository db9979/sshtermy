package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Akzentfarben pro Verbindung: ein farbiger Streifen in Tab, Statuszeile,
// Bereichstitel (geteilte Ansicht) und Manager-Liste. Ohne eigene Wahl
// leitet sshTermy die Farbe aus Ordner, Tags und Alias ab: Produktiv rot,
// Test grün, Labor blau. So sieht man auf einen Blick, wo man gerade tippt.

// accentDef: gespeicherte ID, Anzeigename (deutsch, über i18n übersetzt).
type accentDef struct{ id, label string }

const (
	accentAuto = ""
	accentNone = "none"
)

var accentDefs = []accentDef{
	{accentAuto, "automatisch"},
	{accentNone, "keine"},
	{"red", "Rot"},
	{"orange", "Orange"},
	{"yellow", "Gelb"},
	{"green", "Grün"},
	{"cyan", "Türkis"},
	{"blue", "Blau"},
	{"purple", "Violett"},
}

func accentLabels() []string {
	l := make([]string, len(accentDefs))
	for i, d := range accentDefs {
		l[i] = d.label
	}
	return l
}

func accentLabel(id string) string {
	for _, d := range accentDefs {
		if d.id == id {
			return d.label
		}
	}
	return accentDefs[0].label
}

func accentFromLabel(l string) string {
	for _, d := range accentDefs {
		if d.label == l {
			return d.id
		}
	}
	return accentAuto
}

// accentColor: Farbe passend zum Theme (auf Hell dunkle, kräftige Töne).
func accentColor(t Theme, id string) color.Color {
	switch id {
	case "red":
		return t.Danger
	case "yellow":
		return t.Warn
	case "green":
		return t.OK
	case "blue":
		return t.Accent
	case "purple":
		return t.AI
	case "orange":
		switch t.Name {
		case "hell":
			return lipgloss.Color("#b34700")
		case "kontrast":
			return lipgloss.Color("#ff9000")
		}
		return lipgloss.Color("#ff9e64")
	case "cyan":
		switch t.Name {
		case "hell":
			return lipgloss.Color("#00727f")
		case "kontrast":
			return lipgloss.Color("#00ffff")
		}
		return lipgloss.Color("#7dcfff")
	}
	return nil
}

// envKind: Umgebung aus Ordner, Tags und Alias. "prod", "test", "lab" oder "".
func envKind(c domain.Connection) string {
	if isProduction(c) {
		return "prod"
	}
	words := append([]string{c.Group, c.Alias}, c.Tags...)
	has := func(keys ...string) bool {
		for _, w := range words {
			l := strings.ToLower(w)
			for _, k := range keys {
				if strings.Contains(l, k) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("test", "staging", "stage", "dev", "qa", "abnahme"):
		return "test"
	case has("lab", "labor"):
		return "lab"
	}
	return ""
}

// envLabel: kurzer Name der Umgebung für die Statuszeile.
func envLabel(kind string) string {
	switch kind {
	case "prod":
		return i18n.T("PRODUKTIV")
	case "test":
		return "TEST"
	case "lab":
		return i18n.T("LABOR")
	}
	return ""
}

// connAccent: Akzent-ID der Verbindung ("" = keine Farbe).
func connAccent(c domain.Connection) string {
	switch c.Color {
	case accentNone:
		return ""
	case accentAuto:
		return map[string]string{"prod": "red", "test": "green", "lab": "blue"}[envKind(c)]
	}
	return c.Color
}

// accentOf: Farbe der Verbindung im aktuellen Theme (nil = keine).
func (a *App) accentOf(c domain.Connection) color.Color {
	return accentColor(a.s.T, connAccent(c))
}

// stripe: schmaler farbiger Balken vor Tab, Listenzeile und Statuszeile.
func (a *App) stripe(c domain.Connection, bg color.Color) (styled, plain string) {
	col := a.accentOf(c)
	if col == nil {
		return " ", " "
	}
	st := lipgloss.NewStyle().Foreground(col)
	if bg != nil {
		st = st.Background(bg)
	}
	return st.Render("▌"), "▌"
}

// ---------------------------------------------------------------- Gerätesymbole

// devClass: Geräteklasse aus Systemprofil und Gerätetyp.
type devClass string

const (
	devUnknown  devClass = ""
	devLinux    devClass = "linux"
	devMac      devClass = "macos"
	devBSD      devClass = "bsd"
	devWindows  devClass = "windows"
	devSwitch   devClass = "switch"
	devRouter   devClass = "router"
	devFirewall devClass = "firewall"
)

// deviceClass: was hinter der Verbindung steckt. Firewalls und Router
// werden an Befehlssatz, Modell und Systemname erkannt.
func deviceClass(c domain.Connection) devClass {
	var p domain.SystemProfile
	if c.Profile != nil {
		p = *c.Profile
	}
	hay := strings.ToLower(p.OS + " " + p.Model + " " + p.Banner)
	for _, k := range []string{"fortigate", "fortios", "pfsense", "opnsense", "firewall", "sophos", "palo alto", "pan-os", "asa", "checkpoint", "sonicwall", "watchguard"} {
		if containsWord(hay, k) {
			return devFirewall
		}
	}
	switch p.Dialect {
	case "fortios":
		return devFirewall
	case "routeros":
		return devRouter
	case "procurve", "arubaos-cx", "cisco-ios", "junos", "comware", "huawei-vrp", "network":
		if containsWord(hay, "router") || containsWord(hay, "isr") || containsWord(hay, "mx") {
			return devRouter
		}
		return devSwitch
	case "powershell", "cmd":
		return devWindows
	case "macos":
		return devMac
	case "bsd":
		return devBSD
	}
	switch p.Family {
	case domain.FamilyLinux:
		return devLinux
	case domain.FamilyWindows:
		return devWindows
	case domain.FamilyMacOS:
		return devMac
	case domain.FamilyBSD:
		return devBSD
	case domain.FamilyNetwork:
		return devSwitch
	}
	switch c.Device {
	case domain.DeviceNetwork:
		return devSwitch
	}
	return devUnknown
}

// containsWord: k kommt in s als eigenes Wort vor (asa ≠ „Kasachstan“).
func containsWord(s, k string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], k)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isAlnum(s[j-1])
		end := j + len(k)
		after := end >= len(s) || !isAlnum(s[end])
		if before && after {
			return true
		}
		i = j + 1
	}
}

func isAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// Symbole bewusst aus einfachen Unicode-Blöcken (eine Spalte breit, auch in
// Consolas, Cascadia und Menlo vorhanden), keine Emojis und keine Nerd-Font.
var devIcons = map[devClass]string{
	devUnknown:  "·",
	devLinux:    "❯",
	devMac:      "⌘",
	devBSD:      "❯",
	devWindows:  "⊞",
	devSwitch:   "▦",
	devRouter:   "⊕",
	devFirewall: "▤",
}

func devName(d devClass) string {
	return map[devClass]string{
		devUnknown: i18n.T("noch unbekannt"), devLinux: "Linux", devMac: "macOS", devBSD: "BSD",
		devWindows: "Windows", devSwitch: "Switch", devRouter: "Router", devFirewall: "Firewall",
	}[d]
}

func (a *App) devColor(d devClass) color.Color {
	t := a.s.T
	switch d {
	case devLinux:
		return t.OK
	case devWindows:
		return t.Accent
	case devMac, devBSD:
		return t.Text
	case devSwitch, devRouter:
		return t.Warn
	case devFirewall:
		return t.Danger
	}
	return t.Muted
}

// devIcon: Symbol der Verbindung und seine Farbe.
func (a *App) devIcon(c domain.Connection) (string, color.Color) {
	d := deviceClass(c)
	return devIcons[d], a.devColor(d)
}

// glyph färbt ein einzelnes Zeichen, optional auf Hintergrund.
func glyph(g string, fg, bg color.Color) string {
	st := lipgloss.NewStyle().Foreground(fg)
	if bg != nil {
		st = st.Background(bg)
	}
	return st.Render(g)
}
