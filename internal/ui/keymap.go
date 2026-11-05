package ui

import (
	"strings"
	"unicode/utf8"
)

// Frei belegbare Tastenkürzel. Jede Aktion hat Standardtasten; in den
// Einstellungen (Settings.Keys) lässt sich pro Aktion eine eigene Taste
// hinterlegen, die dann die Standardtasten ersetzt.
//
// Standard ohne Alt+⏎: Windows Terminal schaltet damit in den Vollbildmodus,
// die Taste kommt bei sshTermy gar nicht an.

type action string

const (
	actPalette   action = "palette"
	actHelp      action = "help"
	actAbout     action = "about"
	actQuit      action = "quit"
	actGoal      action = "goal"
	actAsk       action = "ask"
	actTunnels   action = "tunnels"
	actFiles     action = "files"
	actPrevTab   action = "prev_tab"
	actNextTab   action = "next_tab"
	actManager   action = "manager"
	actChip1     action = "chip1"
	actChip2     action = "chip2"
	actChip3     action = "chip3"
	actChip4     action = "chip4"
	actNewline   action = "newline"
	actImport    action = "import"
	actKeys      action = "keys"
	actNextLook  action = "theme"
	actCheat     action = "cheat"
	actQuickEdit action = "quick_edit"
	actBroadcast action = "broadcast"
	actScan      action = "scan"
)

// Schnellbefehle ⇧F1 … ⇧F8 (manche Terminals senden dafür F13 … F20).
var actQuick = [maxQuick]action{"quick1", "quick2", "quick3", "quick4", "quick5", "quick6", "quick7", "quick8"}

type actionDef struct {
	id    action
	label string
	area  string
	def   []string // bubbletea-Tastennamen; die erste wird angezeigt
}

var actionDefs = []actionDef{
	{actPalette, "Befehlspalette", "Überall", []string{"ctrl+space", "ctrl+@"}},
	{actHelp, "Hilfe", "Überall", []string{"f1"}},
	{actAbout, "Über & Update", "Überall", []string{"f12"}},
	{actQuit, "Beenden", "Überall", []string{"ctrl+q"}},
	{actManager, "Manager ↔ Terminal", "Überall", []string{"f10"}},
	{actCheat, "Spickzettel an/aus", "Überall", []string{"f9"}},
	{actGoal, "Ziel-Modus (Agent)", "Terminal", []string{"f2"}},
	{actAsk, "KI fragen", "Terminal", []string{"f3"}},
	{actTunnels, "Tunnel", "Terminal", []string{"f4"}},
	{actFiles, "Dateien (SFTP)", "Terminal", []string{"f5"}},
	{actPrevTab, "Tab zurück", "Terminal", []string{"f7"}},
	{actNextTab, "Tab vor", "Terminal", []string{"f8"}},
	{actChip1, "Folgebefehl 1 einfügen", "Terminal", []string{"alt+1"}},
	{actChip2, "Folgebefehl 2 einfügen", "Terminal", []string{"alt+2"}},
	{actChip3, "Folgebefehl 3 einfügen", "Terminal", []string{"alt+3"}},
	{actChip4, "Folgebefehl 4 einfügen", "Terminal", []string{"alt+4"}},
	{actQuick[0], "Schnellbefehl 1", "Terminal", []string{"shift+f1", "f13"}},
	{actQuick[1], "Schnellbefehl 2", "Terminal", []string{"shift+f2", "f14"}},
	{actQuick[2], "Schnellbefehl 3", "Terminal", []string{"shift+f3", "f15"}},
	{actQuick[3], "Schnellbefehl 4", "Terminal", []string{"shift+f4", "f16"}},
	{actQuick[4], "Schnellbefehl 5", "Terminal", []string{"shift+f5", "f17"}},
	{actQuick[5], "Schnellbefehl 6", "Terminal", []string{"shift+f6", "f18"}},
	{actQuick[6], "Schnellbefehl 7", "Terminal", []string{"shift+f7", "f19"}},
	{actQuick[7], "Schnellbefehl 8", "Terminal", []string{"shift+f8", "f20"}},
	{actNewline, "Neue Zeile in KI-Eingabe", "KI-Eingabe", []string{"shift+enter", "ctrl+j"}},
	{actImport, "Verbindungen importieren", "Manager", []string{"i"}},
	{actNextLook, "Aussehen wechseln", "Manager", []string{"a"}},
	{actKeys, "Tastenkürzel anpassen", "Manager", []string{"b"}},
	{actQuickEdit, "Schnellbefehle bearbeiten", "Manager", []string{"q"}},
	{actBroadcast, "Broadcast an mehrere Hosts", "Manager", []string{"m"}},
	{actScan, "Netzwerk-Scan", "Manager", []string{"f"}},
}

func defOf(id action) actionDef {
	for _, d := range actionDefs {
		if d.id == id {
			return d
		}
	}
	return actionDef{id: id}
}

// keymap: aktuelle Belegung Aktion → Tasten.
type keymap struct {
	keys map[action][]string
}

func newKeymap(over map[string]string) keymap {
	km := keymap{keys: map[action][]string{}}
	for _, d := range actionDefs {
		km.keys[d.id] = d.def
		if k := strings.TrimSpace(over[string(d.id)]); k != "" {
			km.keys[d.id] = []string{k}
		}
	}
	return km
}

// activeKeys: Belegung für Stellen ohne App-Zugriff (Hilfe, Eingabefelder).
var activeKeys = newKeymap(nil)

func (km keymap) is(id action, key string) bool {
	for _, k := range km.keys[id] {
		if k == key {
			return true
		}
	}
	return false
}

// which: welche Aktion liegt auf der Taste (oder "")?
func (km keymap) which(key string, among ...action) action {
	for _, id := range among {
		if km.is(id, key) {
			return id
		}
	}
	return ""
}

// label: angezeigte Taste der Aktion, z. B. „F2“ oder „Strg+G“.
func (km keymap) label(id action) string {
	ks := km.keys[id]
	if len(ks) == 0 {
		return "—"
	}
	return keyLabel(ks[0])
}

// conflicts: Aktionen, die sich eine Taste teilen (nur im selben oder im
// globalen Bereich ein Problem).
func (km keymap) conflicts() map[action]action {
	out := map[action]action{}
	for i, a := range actionDefs {
		for _, b := range actionDefs[i+1:] {
			if a.area != b.area && a.area != "Überall" && b.area != "Überall" {
				continue
			}
			for _, ka := range km.keys[a.id] {
				if km.is(b.id, ka) {
					out[a.id], out[b.id] = b.id, a.id
				}
			}
		}
	}
	return out
}

// keyLabel macht aus „ctrl+g“ die Anzeige „Strg+G“ (umkehrbar mit keyFromLabel).
func keyLabel(k string) string {
	parts := strings.Split(k, "+")
	if len(parts) > 1 && parts[len(parts)-1] == "" { // „ctrl++“
		parts = append(parts[:len(parts)-2], "+")
	}
	for i, p := range parts {
		if i < len(parts)-1 {
			parts[i] = map[string]string{"ctrl": "Strg", "alt": "Alt", "shift": "Shift", "super": "Super", "meta": "Meta"}[p]
			if parts[i] == "" {
				parts[i] = p
			}
			continue
		}
		switch {
		case p == "enter":
			parts[i] = "⏎"
		case p == "space":
			parts[i] = "Space"
		case p == "esc":
			parts[i] = "Esc"
		case p == "tab":
			parts[i] = "Tab"
		case p == "backspace":
			parts[i] = "⌫"
		case p == "delete":
			parts[i] = "Entf"
		case p == "@":
			parts[i] = "@"
		case len(p) >= 2 && p[0] == 'f' && p[1] >= '0' && p[1] <= '9':
			parts[i] = strings.ToUpper(p)
		case utf8.RuneCountInString(p) == 1 && i > 0:
			parts[i] = strings.ToUpper(p)
		default:
			parts[i] = p
		}
	}
	return strings.Join(parts, "+")
}

// reservedKey: Tasten, die sich nicht sinnvoll belegen lassen.
func reservedKey(k string) string {
	switch k {
	case "esc", "enter", "tab", "backspace", "up", "down", "left", "right", "space":
		return "wird zum Bedienen gebraucht"
	case "alt+enter":
		return "Windows Terminal schaltet damit auf Vollbild"
	case "alt+f4":
		return "schließt unter Windows das Fenster"
	case "ctrl+c", "ctrl+d", "ctrl+z", "ctrl+v":
		return "gehört der Shell bzw. dem Einfügen"
	}
	if utf8.RuneCountInString(k) == 1 {
		return "einzelne Zeichen braucht die Shell (nur im Manager sinnvoll)"
	}
	return ""
}

// overrides: nur die Abweichungen vom Standard (für Settings.Keys).
func (km keymap) overrides() map[string]string {
	out := map[string]string{}
	for _, d := range actionDefs {
		ks := km.keys[d.id]
		if len(ks) == 1 && (len(d.def) != 1 || d.def[0] != ks[0]) {
			out[string(d.id)] = ks[0]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// relabelDefault ersetzt eine Standard-Tastenbeschriftung (Hilfe-Tabelle)
// durch die aktuelle Belegung. Einzelne Buchstaben (Manager-Aktionen) nur
// im Manager-Abschnitt, sonst würde z. B. „i“ im Ziel-Modus umbenannt.
func relabelDefault(lbl string, ctx helpCtx) string {
	switch lbl {
	case "Alt+1…4":
		if activeKeys.label(actChip1) == "Alt+1" && activeKeys.label(actChip4) == "Alt+4" {
			return lbl
		}
		return activeKeys.label(actChip1) + "…"
	case "F7 / F8":
		return activeKeys.label(actPrevTab) + " / " + activeKeys.label(actNextTab)
	case "⇧⏎ / Strg+J":
		ks := activeKeys.keys[actNewline]
		if len(ks) == 2 && ks[0] == "shift+enter" && ks[1] == "ctrl+j" {
			return lbl // passt in die Spalte
		}
		if len(ks) == 2 {
			return keyLabel(ks[0]) + " / " + keyLabel(ks[1])
		}
		return activeKeys.label(actNewline)
	}
	for _, d := range actionDefs {
		if d.area == "Manager" && ctx != hcManager {
			continue
		}
		if len(d.def) > 0 && keyLabel(d.def[0]) == lbl {
			return activeKeys.label(d.id)
		}
	}
	return lbl
}
