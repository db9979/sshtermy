package ui

import (
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestParseQuick(t *testing.T) {
	cases := []struct {
		in         string
		ok, strong bool
		user, host string
		port       int
	}{
		{"root@10.0.0.5", true, true, "root", "10.0.0.5", 22},
		{"admin@sw-core:2222", true, true, "admin", "sw-core", 2222},
		{"ssh -p 2200 dominik@spark.local", true, true, "dominik", "spark.local", 2200},
		{"ssh -l pi raspi", true, true, "pi", "raspi", 22},
		{"u@[fe80::1]:22", true, true, "u", "fe80::1", 22},
		{"u@fe80::1", true, true, "u", "fe80::1", 22},
		{"192.168.1.1", true, true, "", "192.168.1.1", 22},
		{"server.example.com", true, false, "", "server.example.com", 22},
		{"web", false, false, "", "", 0},
		{"installiere nginx auf web01", false, false, "", "", 0},
		{"u@host:abc", false, false, "", "", 0},
		{"ssh -i key u@h", false, false, "", "", 0},
		{"@host", false, false, "", "", 0},
		{"u@-oProxyCommand=x", false, false, "", "", 0},
	}
	for _, c := range cases {
		conn, strong, ok := parseQuick(c.in)
		if ok != c.ok || (ok && strong != c.strong) {
			t.Errorf("%q: ok=%v strong=%v, want %v %v", c.in, ok, strong, c.ok, c.strong)
			continue
		}
		if !ok {
			continue
		}
		if c.user != "" && conn.Username != c.user || conn.Host != c.host || conn.Port != c.port {
			t.Errorf("%q: %s@%s:%d", c.in, conn.Username, conn.Host, conn.Port)
		}
		if conn.Auth != domain.AuthAuto || conn.ID == "" || conn.Username == "" {
			t.Errorf("%q: unvollständig %+v", c.in, conn)
		}
	}
}

func TestParseMux(t *testing.T) {
	cases := map[string]muxState{
		"tmux":                    {kind: "tmux"},
		"tmux new -s work":        {kind: "tmux", name: "work"},
		"tmux new-session -swork": {kind: "tmux", name: "work"},
		"tmux a -t 3":             {kind: "tmux", name: "3"},
		"tmux -2 attach -t x":     {kind: "tmux", name: "x"},
		"screen":                  {kind: "screen"},
		"screen -S build":         {kind: "screen", name: "build"},
		"screen -dr build":        {kind: "screen", name: "build"},
		"screen -r":               {kind: "screen"},
	}
	for in, want := range cases {
		got, ok := parseMux(in)
		if !ok || got != want {
			t.Errorf("%q: %+v %v, want %+v", in, got, ok, want)
		}
	}
	for _, in := range []string{"tmux ls", "tmux kill-server", "screen -ls", "ls", "vim tmux.conf"} {
		if _, ok := parseMux(in); ok {
			t.Errorf("%q darf kein Anhängen sein", in)
		}
	}
	if c := (muxState{kind: "tmux", name: "a b"}).attachCmd(); c != "tmux attach -d -t 'a b'" {
		t.Errorf("Quoting: %s", c)
	}
}

func TestPickAttachedTmux(t *testing.T) {
	if m := pickAttachedTmux("0 alt\n1 work\n"); m.name != "work" {
		t.Errorf("got %+v", m)
	}
	if m := pickAttachedTmux("1 a\n1 b\n"); m.kind != "" {
		t.Errorf("zwei angehängte: nicht raten, got %+v", m)
	}
	if m := pickAttachedTmux(""); m.kind != "" {
		t.Errorf("leer: %+v", m)
	}
}

func TestQuitIntent(t *testing.T) {
	s := &session{}
	s.noteCommand("ls -la")
	if s.wantedEnd() {
		t.Fatal("ls ist kein Abmelden")
	}
	s.noteCommand("logout")
	if !s.wantedEnd() {
		t.Fatal("logout nicht erkannt")
	}
	s.quitAt = time.Now().Add(-time.Minute)
	if s.wantedEnd() {
		t.Fatal("altes exit darf späteren Abbruch nicht blockieren")
	}
}

func TestIsProduction(t *testing.T) {
	yes := []domain.Connection{{Group: "Produktion"}, {Tags: []string{"live"}}, {Alias: "db-prod-1"}, {Tags: []string{"PROD"}}}
	no := []domain.Connection{{Group: "Homelab"}, {Alias: "olive"}, {Tags: []string{"test"}}}
	for _, c := range yes {
		if !isProduction(c) {
			t.Errorf("%+v sollte produktiv sein", c)
		}
	}
	for _, c := range no {
		if isProduction(c) {
			t.Errorf("%+v ist nicht produktiv", c)
		}
	}
}

func TestFmtDuration(t *testing.T) {
	if got := fmtDuration(83 * time.Second); got != "1:23" {
		t.Error(got)
	}
	if got := fmtDuration(3723 * time.Second); got != "1:02:03" {
		t.Error(got)
	}
}
