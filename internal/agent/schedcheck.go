package agent

// KI-Prüfung nach Zeitplan: Ein Prüfauftrag in eigenen Worten („Läuft
// nginx, ist die Platte unter 90 %, gibt es neue Fehler im Journal?“) läuft
// ohne Aufsicht über alle Server eines Ordners. Die KI schlägt dafür einmal
// bis zu fünf Befehle vor, die nur lesen (was nicht sicher nur liest, fällt
// weg), sie laufen auf jedem Host, und die KI urteilt danach je Host:
// in Ordnung oder Befund.

import (
	"context"
	"fmt"
	"strings"

	"github.com/db9979/sshtermy/internal/llm"
	"github.com/db9979/sshtermy/internal/safety"
)

// TaskSchedCheck: KI-Prüfung nach Zeitplan (KI-Status, Verbrauch).
const TaskSchedCheck = "sched-check"

// MaxSchedChecks: so viele Prüfbefehle höchstens.
const MaxSchedChecks = 5

const sysSchedChecks = "Du prüfst regelmäßig und ohne Aufsicht mehrere Server. Du bekommst einen Prüfauftrag in eigenen Worten. " +
	"Schlag bis zu 5 kurze Befehle vor, die auf jedem der Server laufen und NUR LESEN (Status, Füllstand, Logs der letzten Zeit, " +
	"Versionen). Nichts, was etwas ändert, neu startet, installiert, löscht oder schreibt, und nichts Interaktives (kein less, top, " +
	"watch, tail -f). Antworte nur mit JSON " + `{"befehle": ["befehl", …]}.` + guard

const sysSchedJudge = "Du prüfst regelmäßig und ohne Aufsicht mehrere Server. Beurteile anhand des Prüfauftrags und der Ausgaben " +
	"je Host, ob alles in Ordnung ist. Sei sachlich: nur melden, was die Ausgaben belegen. Antworte nur mit JSON " +
	`{"hosts": [{"host": "name", "ok": true|false, "befund": "ein kurzer Satz"}], "zusammenfassung": "ein Satz für alle"}.` + guard

// HostOutput: Ausgaben der Prüfbefehle auf einem Host.
type HostOutput struct {
	Host    string
	Results []CheckResult
	Err     string // Host nicht erreicht
}

// HostVerdict: Urteil je Host.
type HostVerdict struct {
	Host    string
	OK      bool
	Finding string
}

// SchedChecks: rein lesende Befehle für den Prüfauftrag.
func (a *Agent) SchedChecks(ctx context.Context, task string, hosts []string, c Context) ([]string, error) {
	user := material(c) + "\n\n<<<\nPrüfauftrag: " + oneLineText(task) + "\nServer: " + strings.Join(hosts, ", ") + "\n>>>"
	if !c.Raw {
		user = Redact(user)
	}
	out, err := a.C.Complete(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysSchedChecks)},
		{Role: "user", Content: user},
	}, llm.Opts{MaxTokens: 400, Temperature: 0.1, NoThink: true, JSON: true, Task: TaskSchedCheck})
	if err != nil {
		return nil, err
	}
	var v struct {
		B []string `json:"befehle"`
	}
	if !strictJSON(out, &v) {
		return nil, ErrUnreadable
	}
	kind := c.kind()
	var cmds []string
	for _, x := range v.B {
		line, ok := oneLine(x, maxCmd)
		if !ok || line == "" || !safety.ReadOnly(line, kind) {
			continue
		}
		cmds = append(cmds, line)
		if len(cmds) == MaxSchedChecks {
			break
		}
	}
	return cmds, nil
}

// SchedJudge: Urteil je Host und eine Zusammenfassung.
func (a *Agent) SchedJudge(ctx context.Context, task string, outs []HostOutput, c Context) ([]HostVerdict, string, error) {
	n := 0
	for _, o := range outs {
		n += max(1, len(o.Results))
	}
	per := max(200, 24000/max(1, n)) // alles zusammen bleibt überschaubar
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n<<<\nPrüfauftrag: %s\n", Today(c.Now), oneLineText(task))
	for _, o := range outs {
		fmt.Fprintf(&b, "\n## %s\n", o.Host)
		if o.Err != "" {
			fmt.Fprintf(&b, "nicht erreicht: %s\n", oneLineText(o.Err))
			continue
		}
		for _, x := range o.Results {
			fmt.Fprintf(&b, "$ %s   (Exit %d)\n%s\n", x.Command, x.Exit, indent(tailRunes(strings.TrimSpace(x.Output), per)))
		}
	}
	b.WriteString(">>>")
	user := b.String()
	if !c.Raw {
		user = Redact(user)
	}
	out, err := a.C.Complete(ctx, []llm.Message{
		{Role: "system", Content: a.sys(sysSchedJudge)},
		{Role: "user", Content: user},
	}, llm.Opts{MaxTokens: 300 + 60*len(outs), Temperature: 0.1, NoThink: true, JSON: true, Task: TaskSchedCheck})
	if err != nil {
		return nil, "", err
	}
	var v struct {
		Hosts []struct {
			Host string `json:"host"`
			OK   *bool  `json:"ok"`
			F    string `json:"befund"`
		} `json:"hosts"`
		Sum string `json:"zusammenfassung"`
	}
	if !strictJSON(out, &v) {
		return nil, "", ErrUnreadable
	}
	known := map[string]bool{}
	for _, o := range outs {
		known[o.Host] = true
	}
	var vs []HostVerdict
	for _, h := range v.Hosts {
		if h.OK == nil || !known[h.Host] {
			continue // nur Hosts, die es gibt
		}
		f, _ := oneLine(Clean(h.F), 300)
		vs = append(vs, HostVerdict{Host: h.Host, OK: *h.OK, Finding: f})
	}
	sum, _ := oneLine(Clean(v.Sum), 300)
	return vs, sum, nil
}
