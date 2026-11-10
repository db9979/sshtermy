package agent

import (
	"context"
	"regexp"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
)

// Entwurf eines Befehlssatzes (internal/cmdset) aus dem, was im Terminal
// steht. Die KI schreibt nur einen Entwurf; geprüft (nur Lesebefehle,
// gültige Muster) und gespeichert wird er erst in sshTermy, nach Bestätigung.

const sysCmdSet = "Du schreibst für das Admin-Werkzeug sshTermy eine Befehlssatz-Datei (YAML) für das Netzwerkgerät oder System, dessen Terminal-Ausgabe du bekommst. " +
	"Antworte NUR mit YAML, ohne Erklärung und ohne ```-Zaun. Felder (alle optional außer id und name):\n" +
	"id: kleinbuchstaben-mit-bindestrich (z. B. hersteller-modell)\nname: Anzeigename\nfamily: network (Switch/Router/Firewall) oder linux/bsd\n" +
	"hint: ein Absatz für die KI: welche CLI, typische Lesebefehle, wie man konfiguriert und speichert\n" +
	"detect:\n  screen: ['Go-Regex auf Login-Banner oder Prompt']\n  version: ['Go-Regex auf die Ausgabe des Versionsbefehls']\n" +
	"commands:\n  paging_off: nur einer von: terminal length 0 | no page | screen-length disable | screen-length 0 temporary | terminal pager 0 | set cli pager off | no paging (sonst weglassen)\n" +
	"  version: Befehl für Modell/Firmware\n  system: Befehl für CPU/Speicher\n  ports: Befehl für Port-Übersicht\n  vlans: Befehl für VLAN-Liste\n  config: Befehl, der die laufende Konfiguration ausgibt\n" +
	"parse:\n  firmware: 'Go-Regex mit einer Klammer'\n  model: 'Go-Regex mit einer Klammer'\n  name: '…'\n  uptime: '…'\n" +
	"logs:\n  commands: [Befehl für das Gerätelog]\n  reader: generic\n" +
	"syslog:\n  steps: ['Befehle, die Syslog an {{ip}} Port {{port}} schicken (Konfigurationsmodus betreten und verlassen)']\n  save: Befehl zum dauerhaften Speichern\n" +
	"Regeln: Unter commands und logs NUR Befehle, die nichts ändern (show/display/get …). Regex in einfachen Anführungszeichen. " +
	"Nimm nur Befehle, die es auf diesem Gerät sicher gibt; was du nicht weißt, lässt du weg. Erfinde keine Gefahrenregeln und keine read-Liste." + guard

var reYAMLFence = regexp.MustCompile("(?s)^```(?:ya?ml)?\\s*\\n(.*?)\\n```\\s*$")

// DraftCmdSet bittet die KI um einen Befehlssatz-Entwurf.
func (a *Agent) DraftCmdSet(ctx context.Context, screen string, c Context) (string, error) {
	body := strings.NewReplacer("<<<", "‹‹‹", ">>>", "›››").Replace(Redact(tail(screen, 6000)))
	res, err := a.C.CompleteResult(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysCmdSet)},
		{Role: "user", Content: material(c) + "\nTerminal:\n<<<\n" + body + "\n>>>"},
	}, llm.Opts{MaxTokens: goalTokens, Temperature: 0.2, NoThink: true, Task: TaskAdapt})
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(stripThink(res.Content))
	if m := reYAMLFence.FindStringSubmatch(out); m != nil {
		out = m[1]
	}
	if out == "" {
		return "", unreadable(res)
	}
	return out + "\n", nil
}
