// Package codeagent erkennt KI-Coding-Agenten (Claude Code, Codex, Gemini
// CLI, OpenCode, Copilot CLI, Cursor Agent …), die in einem Terminal laufen,
// und ihren Zustand: arbeitet, wartet auf eine Freigabe, bereit. sshTermy
// zeigt das am Tab und meldet sich, wenn ein Agent auf dem Server auf dich
// wartet oder fertig ist. Gelesen wird nur der sichtbare Bildschirm und der
// Fenstertitel; an den Agenten geht nichts.
package codeagent

import (
	"path"
	"regexp"
	"strings"
)

// State: was der Agent gerade tut.
type State int

const (
	Unknown State = iota // Agent erkannt, Zustand nicht
	Idle                 // wartet auf die nächste Aufgabe
	Working              // arbeitet
	Blocked              // wartet auf eine Freigabe oder Antwort
)

func (s State) String() string {
	return [...]string{"unknown", "idle", "working", "blocked"}[s]
}

type agentDef struct {
	name    string
	cmds    []string       // Programmnamen in der Befehlszeile
	marker  *regexp.Regexp // Kennzeichen auf dem Bildschirm oder im Titel
	idle    *regexp.Regexp // Fußzeile / Eingabefeld im Leerlauf
	working *regexp.Regexp // eigene Arbeitsanzeige
	blocked *regexp.Regexp // eigene Freigabe-Dialoge
}

var agents = []agentDef{
	{
		name:    "Claude Code",
		cmds:    []string{"claude", "claude-code"},
		marker:  regexp.MustCompile(`(?i)claude code|\? for shortcuts|✻ welcome to claude`),
		idle:    regexp.MustCompile(`(?i)\? for shortcuts|shift\+tab to cycle|auto-accept edits on|plan mode on`),
		working: regexp.MustCompile(`(?i)\besc to interrupt\b`),
		blocked: regexp.MustCompile(`(?i)do you want to (proceed|make this edit|create|allow|run)|❯\s*1\.\s*yes|enter to (confirm|select)\s*·\s*esc to cancel`),
	},
	{
		name:    "Codex",
		cmds:    []string{"codex"},
		marker:  regexp.MustCompile(`(?i)openai codex|>_ codex|\bcontext left\b`),
		idle:    regexp.MustCompile(`(?i)⏎ send|context left|ctrl\+j newline|⌃j newline`),
		working: regexp.MustCompile(`(?i)\besc to interrupt\b`),
		blocked: regexp.MustCompile(`(?i)would you like to (run|make|apply)|allow command\?|approve this|›\s*1\.\s*yes`),
	},
	{
		name:    "Gemini CLI",
		cmds:    []string{"gemini"},
		marker:  regexp.MustCompile(`(?i)gemini cli|type your message or @path`),
		idle:    regexp.MustCompile(`(?i)type your message or @path`),
		working: regexp.MustCompile(`(?i)\besc to cancel\b`),
		blocked: regexp.MustCompile(`(?i)allow execution|apply this change\?|●\s*1\.\s*yes|waiting for user confirmation`),
	},
	{
		name:    "OpenCode",
		cmds:    []string{"opencode"},
		marker:  regexp.MustCompile(`(?i)\bopencode\b`),
		idle:    regexp.MustCompile(`(?i)ctrl\+p commands|tab switch agent`),
		working: regexp.MustCompile(`(?i)\besc (to )?interrupt\b`),
		blocked: regexp.MustCompile(`(?i)permission required|allow once|allow always`),
	},
	{
		name:    "Copilot CLI",
		cmds:    []string{"copilot"},
		marker:  regexp.MustCompile(`(?i)github copilot`),
		working: regexp.MustCompile(`(?i)\besc to (cancel|interrupt)\b`),
		blocked: regexp.MustCompile(`(?i)do you want to (run|allow|proceed)|❯\s*1\.\s*yes`),
	},
	{
		name:    "Cursor Agent",
		cmds:    []string{"cursor-agent", "agent"},
		marker:  regexp.MustCompile(`(?i)cursor agent`),
		working: regexp.MustCompile(`(?i)ctrl\+c to stop|\besc to (interrupt|cancel)\b`),
		blocked: regexp.MustCompile(`(?i)run this command\?|\(y\) \(enter\)|allow \(y\)|reject \(`),
	},
	{
		name:    "Qwen Code",
		cmds:    []string{"qwen"},
		marker:  regexp.MustCompile(`(?i)qwen code`),
		idle:    regexp.MustCompile(`(?i)type your message or @path`),
		working: regexp.MustCompile(`(?i)\besc to cancel\b`),
		blocked: regexp.MustCompile(`(?i)allow execution|apply this change\?|waiting for user confirmation`),
	},
}

// FromCommand: Name des Agenten, wenn die Befehlszeile ihn startet
// („claude“, „npx @openai/codex“, „/usr/local/bin/gemini -p …“), sonst "".
func FromCommand(cmd string) string {
	f := strings.Fields(cmd)
	// Vorsätze überspringen: sudo, env, npx/bunx/pnpm dlx, VAR=wert
	for len(f) > 0 {
		w := f[0]
		switch {
		case w == "sudo" || w == "env" || w == "npx" || w == "bunx" || w == "exec" || w == "nohup" || w == "time":
			f = f[1:]
			continue
		case (w == "pnpm" || w == "yarn") && len(f) > 1 && f[1] == "dlx":
			f = f[2:]
			continue
		case strings.Contains(w, "=") && !strings.HasPrefix(w, "-"):
			f = f[1:]
			continue
		case strings.HasPrefix(w, "-"):
			f = f[1:] // Schalter von npx & Co.
			continue
		}
		break
	}
	if len(f) == 0 {
		return ""
	}
	prog := path.Base(strings.ReplaceAll(f[0], `\`, "/"))
	prog = strings.TrimSuffix(strings.TrimSuffix(prog, ".exe"), ".cmd")
	// npm-Pakete: @anthropic-ai/claude-code, @openai/codex, @google/gemini-cli
	switch {
	case strings.Contains(f[0], "claude-code"):
		return "Claude Code"
	case strings.HasSuffix(f[0], "/codex"):
		return "Codex"
	case strings.Contains(f[0], "gemini-cli"):
		return "Gemini CLI"
	}
	for _, d := range agents {
		for _, c := range d.cmds {
			if prog == c {
				if c == "agent" && d.name == "Cursor Agent" {
					continue // „agent“ allein ist zu allgemein
				}
				return d.name
			}
		}
	}
	return ""
}

// FromScreen: Name des Agenten anhand von Titel und Bildschirm, sonst "".
func FromScreen(screen, title string) string {
	for _, d := range agents {
		if d.marker.MatchString(title) || d.marker.MatchString(screen) {
			return d.name
		}
	}
	return ""
}

func def(name string) *agentDef {
	for i := range agents {
		if agents[i].name == name {
			return &agents[i]
		}
	}
	return nil
}

// allgemeine Zeichen, die viele Agenten teilen
var (
	anyWorking = regexp.MustCompile(`(?i)\besc to (interrupt|cancel)\b`)
	anyBlocked = regexp.MustCompile(`(?i)do you want to (proceed|run|allow|make)|would you like to run|allow (once|always|execution)|permission required`)
	// Spinner vorn im Fenstertitel (Claude Code, Codex): arbeitet
	titleSpin = regexp.MustCompile(`^[⠀-⣿◐◓◑◒✳✻✽✶·✢*]\s`)
)

// Detect: Zustand des Agenten name auf diesem Bildschirm (die letzten
// Zeilen genügen). seen = der Agent ist am Bildschirm zu erkennen (sonst
// hat er sich vermutlich beendet).
func Detect(name, screen, title string) (st State, seen bool) {
	d := def(name)
	if d == nil {
		return Unknown, false
	}
	// nur der untere Teil zählt: Freigaben und Fußzeilen stehen unten,
	// ältere Ausgabe weiter oben soll nichts auslösen
	lines := strings.Split(screen, "\n")
	if len(lines) > 14 {
		lines = lines[len(lines)-14:]
	}
	tail := strings.Join(lines, "\n")
	match := func(rx *regexp.Regexp) bool { return rx != nil && rx.MatchString(tail) }
	seen = match(d.marker) || match(d.idle) || match(d.working) || match(d.blocked) ||
		d.marker.MatchString(title)
	switch {
	case match(d.blocked) || anyBlocked.MatchString(tail):
		return Blocked, true
	case match(d.working) || anyWorking.MatchString(tail) || titleSpin.MatchString(title):
		return Working, true
	case match(d.idle):
		return Idle, true
	}
	if seen {
		return Idle, true
	}
	return Unknown, false
}
