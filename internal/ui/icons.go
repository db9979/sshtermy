package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Symbole der Menüs. Windows Terminal nutzt Cascadia Mono; viele hübsche
// Unicode-Zeichen (⚙ ⚿ ⌕ ⇅ …) fehlen dort und kommen aus einer Ersatzschrift
// mit anderer Breite und Strichstärke. Darum gibt es genau zwei Stile:
//
//   - Text-Tags (Standard): [+] [k] [/] … aus reinem ASCII, sieht mit jeder
//     Schrift gleich aus.
//   - Nerd Font: echte Bildsymbole, nur wenn eine Nerd Font eingestellt ist
//     (z. B. „Cascadia Mono NF“), Schalter unter Einstellungen › Aussehen.
//
// Im übrigen Text stehen nur Zeichen, die Cascadia Mono selbst hat
// (TestGlyphsInCascadia).

// iconSet: "" = Text-Tags, "nerd" = Nerd-Font-Symbole (aus den Einstellungen).
var iconSet = ""

const iconNerd = "nerd"

type iconDef struct{ tag, nerd string }

var iconDefs = map[string]iconDef{
	"manager":   {"[=]", "\uf233"},     // server
	"new":       {"[+]", "\uf067"},     // plus
	"sshkey":    {"[k]", "\uf084"},     // key
	"broadcast": {"[*]", "\uf0a1"},     // bullhorn
	"scan":      {"[/]", "\uf002"},     // search
	"search":    {"[/]", "\uf002"},     // search
	"logs":      {"[#]", "\uf0f6"},     // file-text
	"sync":      {"[s]", "\uf021"},     // refresh
	"settings":  {"[:]", "\uf013"},     // gear
	"help":      {"[?]", "\uf059"},     // question-circle
	"goal":      {"[@]", "\uf05b"},     // crosshairs
	"ai":        {"[~]", "\U000f06a9"}, // robot
	"rewrite":   {"[~]", "\uf0ec"},     // exchange
	"fix":       {"[!]", "\uf0ad"},     // wrench
	"command":   {"[$]", "\uf120"},     // terminal
	"runbook":   {"[>]", "\uf04b"},     // play
	"open":      {"[o]", "\uf08e"},     // external-link
	"tunnel":    {"[t]", "\uf0ec"},     // exchange
	"quick":     {"[q]", "\uf0e7"},     // bolt
	"connect":   {"[>]", "\uf0e7"},     // bolt
	"host":      {"[i]", "\uf0e4"},     // dashboard
	"record":    {"[r]", "\uf03d"},     // video-camera
	"export":    {"[e]", "\uf019"},     // download
	"import":    {"[e]", "\uf090"},     // sign-in
	"backup":    {"[b]", "\uf0c7"},     // floppy
	"errors":    {"[!]", "\uf071"},     // warning
	"split":     {"[|]", "\uf0db"},     // columns
	"detach":    {"[^]", "\uf2d2"},     // window-restore
	"attach":    {"[v]", "\uf2d0"},     // window-maximize
	"break":     {"[z]", "\uf1e6"},     // plug
	"restart":   {"[u]", "\uf021"},     // refresh
	"theme":     {"[c]", "\uf1fc"},     // paint-brush
	"colors":    {"[c]", "\U000f03d8"}, // palette
	"icons":     {"[c]", "\uf03e"},     // image
	"language":  {"[l]", "\uf1ab"},     // language
	"keys":      {"[^]", "\uf11c"},     // keyboard
	"relay":     {"[n]", "\uf0e8"},     // sitemap
	"llmcheck":  {"[r]", "\uf012"},     // signal
	"tour":      {"[>]", "\uf04b"},     // play
	"news":      {"[n]", "\uf005"},     // star
	"about":     {"[i]", "\uf05a"},     // info-circle
	"mouse":     {"[m]", "\U000f037d"}, // mouse
	"reconnect": {"[u]", "\uf021"},     // refresh
	"reach":     {"[r]", "\uf012"},     // signal
	"sesslog":   {"[#]", "\uf0f6"},     // file-text
	"paste":     {"[p]", "\uf0ea"},     // clipboard
	"danger":    {"[!]", "\uf071"},     // warning
	"intro":     {"[~]", "\uf135"},     // rocket
	"explain":   {"[?]", "\uf059"},     // question-circle
	"lock":      {"[k]", "\uf023"},     // lock
	"more":      {"[=]", "\uf233"},     // server
	"team":      {"[&]", "\uf0c0"},     // users
	"watch":     {"[w]", "\uf06e"},     // eye
	"copy":      {"[p]", "\uf0c5"},     // copy
}

// ic: Symbol einer Menü-Aktion im eingestellten Stil.
func ic(name string) string {
	d, ok := iconDefs[name]
	if !ok {
		return "[ ]"
	}
	if iconSet == iconNerd {
		return d.nerd
	}
	return d.tag
}

// iconCol: Breite der Symbolspalte in Listen (Tags 3, Nerd Font 1).
func iconCol() int {
	if iconSet == iconNerd {
		return 1
	}
	return 3
}

// Gerätesymbole: Text-Stil nimmt das Prompt-Zeichen des Systems
// ($ Linux, % macOS/zsh, > Windows, # Switch …), Nerd Font die Logos.
var devTags = map[devClass]string{
	devUnknown: "·", devLinux: "$", devMac: "%", devBSD: "$", devWindows: ">",
	devSwitch: "#", devRouter: "@", devFirewall: "!",
}

var devNerd = map[devClass]string{
	devUnknown: "·", devLinux: "\uf17c", devMac: "\uf179", devBSD: "\uf30c", devWindows: "\uf17a",
	devSwitch: "\U000f0807", devRouter: "\U000f0484", devFirewall: "\uf132",
}

func devGlyph(d devClass) string {
	if iconSet == iconNerd {
		return devNerd[d]
	}
	return devTags[d]
}

// padIcon bringt ein Symbol auf die Breite der Symbolspalte: einzelne
// Zeichen (Gerät, Status) bekommen im Text-Stil Klammern, z. B. [$].
func padIcon(s string) string {
	w := ansi.StringWidth(s)
	switch {
	case w >= iconCol():
		return s
	case iconCol() == 3 && w == 1:
		return "[" + s + "]"
	}
	return s + strings.Repeat(" ", iconCol()-w)
}

// groupHead: Gruppenüberschrift im Terminal-Stil („// verbindungen“).
func groupHead(s string) string {
	return "// " + strings.ToLower(s)
}
