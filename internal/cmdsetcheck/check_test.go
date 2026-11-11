package cmdsetcheck

import (
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/cmdset"
	"github.com/db9979/sshtermy/internal/hostinfo"
	"github.com/db9979/sshtermy/internal/safety"
)

// Alle eingebauten Sätze: gültig, Befehle nur lesend, Beispiele stimmen.
func TestBuiltinSetsValid(t *testing.T) {
	if err := cmdset.ValidateBuiltin(); err != nil {
		t.Fatal(err)
	}
	if n := len(cmdset.Builtins()); n < 45 {
		t.Fatalf("nur %d eingebaute Sätze", n)
	}
}

func TestBuiltinSamples(t *testing.T) {
	for _, s := range cmdset.Builtins() {
		if fails := SelfTest(s); len(fails) > 0 {
			t.Errorf("%s:\n  %s", s.ID, strings.Join(fails, "\n  "))
		}
	}
}

// Die Versionsausgabe eines Beispiels erkennt genau diesen Satz (nicht
// einen früher stehenden anderen Hersteller).
func TestBuiltinDetectionUnique(t *testing.T) {
	for _, s := range cmdset.Builtins() {
		for _, smp := range s.Samples {
			if smp.Version == "" || s.EffFamily() != cmdset.FamilyNetwork {
				continue
			}
			if got := hostinfo.DetectVendor(smp.Version); string(got) != s.ID {
				t.Errorf("%s/%s: erkannt als %q", s.ID, smp.Name, got)
			}
		}
	}
}

// Jeder Befehl, den sshTermy selbst tippt, ist für die Agent-/MCP-Prüfung
// „nur lesen“ (auch über die Varianten).
func TestBuiltinCommandsReadOnly(t *testing.T) {
	for _, s := range cmdset.Builtins() {
		kind := "network"
		if s.EffFamily() != cmdset.FamilyNetwork || s.ReadKind == "unix" {
			kind = "unix"
		}
		cs := []cmdset.Commands{s.Commands}
		ls := []cmdset.Logs{s.Logs}
		for _, v := range s.Variants {
			cs, ls = append(cs, v.Commands), append(ls, v.Logs)
		}
		var cmds []string
		for _, c := range cs {
			cmds = append(cmds, c.Version, c.System, c.Memory, c.Ports, c.VLANs, c.Config)
			for _, e := range c.Extra {
				cmds = append(cmds, e.Cmd)
			}
		}
		for _, l := range ls {
			cmds = append(cmds, l.Commands...)
			cmds = append(cmds, l.Ports)
		}
		for _, c := range cmds {
			if c != "" && !safety.ReadOnly(c, kind) {
				t.Errorf("%s: %q gilt nicht als nur lesend", s.ID, c)
			}
		}
	}
}

// Syslog-Schritte der eingebauten Sätze treffen keine Gefahrenregel.
func TestBuiltinSyslogStepsNotDangerous(t *testing.T) {
	for _, s := range cmdset.Builtins() {
		steps, err := cmdset.SyslogSteps(s.Syslog.Steps, "192.0.2.10", 5514)
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range steps {
			for _, k := range safety.Kinds {
				if f := safety.Check(st, k); len(f) > 0 {
					t.Errorf("%s: %q → %v", s.ID, st, f)
				}
			}
		}
	}
}

// Wer Syslog-Schritte in den Konfigurationsmodus hat, muss ihn bei einem
// Fehler auch wieder verlassen können.
func TestBuiltinSyslogAbort(t *testing.T) {
	for _, s := range cmdset.Builtins() {
		r := cmdset.Resolve(s.ID, "", "")
		if len(r.Syslog.Steps) < 2 || s.ID == "routeros" || s.ID == "cisco-wlc" {
			continue // einzelne Befehle ohne Konfigurationsmodus
		}
		if len(r.SyslogAbort()) == 0 {
			t.Errorf("%s: Syslog-Schritte ohne Abbruch-Befehle", s.ID)
		}
	}
	if _, err := cmdset.ParseOwn([]byte("id: x-abort\nname: X\nsyslog:\n  steps: [configure, 'logging {{ip}}', end]\n  abort: ['reload']\n")); err == nil {
		t.Error("unbekannter Abbruch-Befehl angenommen")
	}
}
