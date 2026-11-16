package ui

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestParseHeaderCmd(t *testing.T) {
	for in, want := range map[string]struct {
		cmd   string
		every int
	}{
		"date":                     {"date", 0},
		"curl -s ifconfig.me @300": {"curl -s ifconfig.me", 300},
		"echo a@b":                 {"echo a@b", 0},
		"uptime @1":                {"uptime", headerEveryMin},
	} {
		c, e := parseHeaderCmd(in)
		if c != want.cmd || e != want.every {
			t.Errorf("%q → %q %d", in, c, e)
		}
	}
}

func TestHeaderFieldsClockAndCmd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	a := dangerApp(t)
	_ = a.st.UpdateSettings(func(s *domain.Settings) {
		s.Header = domain.HeaderConfig{Clock: true, Cmd: "printf '\\n VPN: an \\033[31m\\nrest'"}
	})
	cmd := a.headerTick(time.Now())
	if cmd == nil {
		t.Fatal("Befehl nicht gestartet")
	}
	if a.headerTick(time.Now()) != nil {
		t.Fatal("läuft doppelt")
	}
	a.coreMsg(cmd())
	got := ansi.Strip(a.headerFields())
	if !strings.Contains(got, "VPN: an [31m") || !strings.Contains(got, time.Now().Format("15:")) {
		t.Fatalf("Felder = %q", got)
	}
	if a.headerTick(time.Now()) != nil {
		t.Fatal("vor Ablauf erneut gestartet")
	}
	if a.headerTick(time.Now().Add(31*time.Second)) == nil {
		t.Fatal("nach 30 s nicht erneut")
	}
}
