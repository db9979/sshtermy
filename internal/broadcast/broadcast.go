// Package broadcast führt einen Befehl oder die Schritte eines Runbooks
// parallel auf mehreren gespeicherten Verbindungen aus (z. B. „alle
// Aruba-Switches“) und sammelt die Ergebnisse pro Host.
//
// Server bekommen jeden Schritt als eigene Exec-Sitzung (mit Exit-Code, Abbruch
// beim ersten Fehler). Netzwerkgeräte vertragen kein Exec: Dort werden die
// Schritte in eine Shell getippt, einen Exit-Code gibt es dann nicht.
//
// Die Gefahrenregeln aus internal/safety gelten auch hier: Dangers liefert die
// Treffer, die Oberfläche verlangt dafür eine zweite Bestätigung.
package broadcast

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/safety"
	"github.com/db9979/sshtermy/internal/sshx"
)

// State eines Hosts im Broadcast.
type State int

const (
	Waiting State = iota
	Connecting
	Running
	Done
	Failed
	Canceled
)

// Result pro Host. Exit -1 = unbekannt (Netzwerkgerät oder Abbruch).
type Result struct {
	ConnID  string
	Alias   string
	State   State
	Step    int // laufender Schritt, ab 1
	Output  string
	Exit    int
	Err     string
	Net     bool // als Netzwerkgerät behandelt (Shell statt Exec)
	Learned bool // Netzwerkgerät erst beim Verbinden erkannt
	Start   time.Time
	End     time.Time
}

// Finished: Host ist fertig (gut oder schlecht).
func (r Result) Finished() bool { return r.State >= Done }

// OK: fertig ohne Fehler (Exit 0 oder, bei Netzwerkgeräten, unbekannt).
func (r Result) OK() bool { return r.State == Done && r.Exit <= 0 && r.Err == "" }

// Executor ist eine offene Verbindung, auf der Befehle laufen.
type Executor interface {
	NetDevice() bool
	Learned() bool
	Exec(ctx context.Context, cmd string) (sshx.RunResult, error)
	RunInShell(ctx context.Context, cmds []string) (sshx.RunResult, error)
	Close() error
}

// Dialer baut die Verbindung zu einem Host auf.
type Dialer func(ctx context.Context, c domain.Connection) (Executor, error)

// Job: was wo laufen soll.
type Job struct {
	Conns    []domain.Connection
	Steps    []string
	Parallel int           // gleichzeitige Hosts, Standard 8
	Timeout  time.Duration // pro Host, Standard 5 min
}

// Run startet alle Hosts und meldet jede Zustandsänderung an update (aus
// verschiedenen Goroutinen, immer mit dem vollständigen Stand des Hosts).
// Run kehrt zurück, wenn alle Hosts fertig sind oder ctx abgebrochen wurde.
func Run(ctx context.Context, j Job, dial Dialer, update func(i int, r Result)) []Result {
	par := j.Parallel
	if par <= 0 {
		par = 8
	}
	tmo := j.Timeout
	if tmo <= 0 {
		tmo = 5 * time.Minute
	}
	res := make([]Result, len(j.Conns))
	for i, c := range j.Conns {
		res[i] = Result{ConnID: c.ID, Alias: c.Alias, Exit: -1}
		update(i, res[i])
	}
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for i, c := range j.Conns {
		wg.Add(1)
		go func(i int, c domain.Connection) {
			defer wg.Done()
			r := &res[i]
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				r.State, r.Err = Canceled, "abgebrochen"
				update(i, *r)
				return
			}
			hctx, cancel := context.WithTimeout(ctx, tmo)
			defer cancel()
			runHost(hctx, c, j.Steps, dial, r, func() { update(i, *r) })
		}(i, c)
	}
	wg.Wait()
	return res
}

func runHost(ctx context.Context, c domain.Connection, steps []string, dial Dialer, r *Result, emit func()) {
	r.Start = time.Now()
	r.State = Connecting
	emit()
	finish := func(st State, err error) {
		r.End = time.Now()
		r.State = st
		if err != nil {
			r.Err = errText(err)
			if errors.Is(err, context.Canceled) {
				r.State = Canceled
			}
		}
		emit()
	}
	ex, err := dial(ctx, c)
	if err != nil {
		finish(Failed, err)
		return
	}
	defer ex.Close()
	r.Net, r.Learned = ex.NetDevice(), ex.Learned()
	r.State, r.Step = Running, 1
	emit()

	if r.Net {
		out, err := ex.RunInShell(ctx, steps)
		r.Output, r.Exit = string(out.Output), -1
		if err != nil {
			finish(Failed, err)
			return
		}
		r.Step = len(steps)
		finish(Done, nil)
		return
	}
	var b strings.Builder
	for i, cmd := range steps {
		r.Step = i + 1
		if len(steps) > 1 {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("$ " + cmd + "\n")
		}
		out, err := ex.Exec(ctx, cmd)
		b.Write(out.Output)
		r.Output, r.Exit = b.String(), out.Exit
		if err != nil {
			finish(Failed, err)
			return
		}
		if out.Exit != 0 {
			if len(steps) > 1 && i < len(steps)-1 {
				r.Err = fmt.Sprintf("Schritt %d mit Exit %d abgebrochen", i+1, out.Exit)
			}
			finish(Done, nil)
			return
		}
		emit()
	}
	finish(Done, nil)
}

func errText(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Zeitlimit überschritten"
	case errors.Is(err, context.Canceled):
		return "abgebrochen"
	}
	return err.Error()
}

// Danger: ein Schritt, auf den eine Gefahrenregel zutrifft.
type Danger struct {
	Step    int // ab 1
	Command string
	Reasons []string
}

// Dangers prüft alle Schritte mit den festen Regeln. Das Zielsystem ist vor
// dem Verbinden nicht bekannt, deshalb gelten Unix- und Windows-Regeln.
// sudo allein ist keine Gefahr, wird aber mit "läuft mit sudo" markiert.
func Dangers(steps []string) []Danger {
	var out []Danger
	for i, cmd := range steps {
		seen := map[string]bool{}
		var reasons []string
		for _, kind := range []string{"unix", "windows"} {
			for _, f := range safety.Check(cmd, kind) {
				if !seen[f.Reason] {
					seen[f.Reason] = true
					reasons = append(reasons, f.Reason)
				}
			}
		}
		if len(reasons) > 0 {
			out = append(out, Danger{Step: i + 1, Command: cmd, Reasons: reasons})
		}
	}
	return out
}

// Elevated: läuft mindestens ein Schritt mit erhöhten Rechten?
func Elevated(steps []string) bool {
	for _, s := range steps {
		if safety.Elevated(s) {
			return true
		}
	}
	return false
}

// SSH macht aus einer sshx-Verbindung einen Executor.
func SSH(c *sshx.Client, tm sshx.ShellTiming) Executor { return sshExec{c, tm} }

type sshExec struct {
	c  *sshx.Client
	tm sshx.ShellTiming
}

func (e sshExec) NetDevice() bool { return e.c.NetDevice }
func (e sshExec) Learned() bool   { return e.c.Learned }
func (e sshExec) Close() error    { return e.c.Close() }
func (e sshExec) Exec(ctx context.Context, cmd string) (sshx.RunResult, error) {
	return e.c.Exec(ctx, cmd)
}
func (e sshExec) RunInShell(ctx context.Context, cmds []string) (sshx.RunResult, error) {
	return e.c.RunInShell(ctx, cmds, e.tm)
}

// Summary: „3 ok · 1 Fehler · 2 laufen“.
func Summary(rs []Result) (ok, failed, running int) {
	for _, r := range rs {
		switch {
		case !r.Finished():
			running++
		case r.OK():
			ok++
		default:
			failed++
		}
	}
	return
}
