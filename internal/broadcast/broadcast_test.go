package broadcast

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/sshx"
)

type fakeExec struct {
	net   bool
	fail  string // Befehl mit Exit 1
	mu    *sync.Mutex
	execs *[]string
}

func (f fakeExec) NetDevice() bool { return f.net }
func (f fakeExec) Learned() bool   { return false }
func (f fakeExec) Close() error    { return nil }
func (f fakeExec) Exec(_ context.Context, cmd string) (sshx.RunResult, error) {
	f.mu.Lock()
	*f.execs = append(*f.execs, cmd)
	f.mu.Unlock()
	if cmd == f.fail {
		return sshx.RunResult{Output: []byte("nein\n"), Exit: 1}, nil
	}
	return sshx.RunResult{Output: []byte("ok:" + cmd + "\n")}, nil
}
func (f fakeExec) RunInShell(_ context.Context, cmds []string) (sshx.RunResult, error) {
	return sshx.RunResult{Output: []byte("sw# " + strings.Join(cmds, "\nsw# ")), Exit: -1}, nil
}

func TestRunMixed(t *testing.T) {
	conns := []domain.Connection{{ID: "a", Alias: "web"}, {ID: "b", Alias: "db"}, {ID: "c", Alias: "switch"}, {ID: "d", Alias: "weg"}}
	var mu sync.Mutex
	var execs []string
	dial := func(_ context.Context, c domain.Connection) (Executor, error) {
		switch c.ID {
		case "d":
			return nil, errors.New("connection refused")
		case "c":
			return fakeExec{net: true, mu: &mu, execs: &execs}, nil
		case "b":
			return fakeExec{fail: "eins", mu: &mu, execs: &execs}, nil
		}
		return fakeExec{mu: &mu, execs: &execs}, nil
	}
	var umu sync.Mutex
	updates := 0
	res := Run(context.Background(), Job{Conns: conns, Steps: []string{"eins", "zwei"}, Parallel: 2}, dial,
		func(int, Result) { umu.Lock(); updates++; umu.Unlock() })

	if r := res[0]; !r.OK() || r.Exit != 0 || r.Output != "$ eins\nok:eins\n\n$ zwei\nok:zwei\n" {
		t.Errorf("web: %+v", r)
	}
	if r := res[1]; r.OK() || r.Exit != 1 || !strings.Contains(r.Err, "Schritt 1") || strings.Contains(r.Output, "zwei") {
		t.Errorf("db sollte nach Schritt 1 abbrechen: %+v", r)
	}
	if r := res[2]; !r.OK() || !r.Net || r.Output != "sw# eins\nsw# zwei" {
		t.Errorf("switch: %+v", r)
	}
	if r := res[3]; r.State != Failed || r.Err != "connection refused" {
		t.Errorf("weg: %+v", r)
	}
	if ok, failed, running := Summary(res); ok != 2 || failed != 2 || running != 0 {
		t.Errorf("Summary %d/%d/%d", ok, failed, running)
	}
	if updates < len(conns)*3 {
		t.Errorf("zu wenige Updates: %d", updates)
	}
}

func TestRunCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dial := func(ctx context.Context, c domain.Connection) (Executor, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	done := make(chan []Result)
	go func() {
		done <- Run(ctx, Job{Conns: []domain.Connection{{ID: "a"}, {ID: "b"}, {ID: "c"}}, Steps: []string{"x"}, Parallel: 1}, dial, func(int, Result) {})
	}()
	select {
	case res := <-done:
		for _, r := range res {
			if r.State != Canceled {
				t.Errorf("%s: %v %q", r.ConnID, r.State, r.Err)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run hängt nach Abbruch")
	}
}

func TestDangers(t *testing.T) {
	d := Dangers([]string{"uptime", "sudo rm -rf /var/tmp/x", "Restart-Computer"})
	if len(d) != 2 || d[0].Step != 2 || d[1].Step != 3 {
		t.Fatalf("%+v", d)
	}
	if !Elevated([]string{"ls", "sudo apt update"}) || Elevated([]string{"ls"}) {
		t.Fatal("Elevated falsch")
	}
}
