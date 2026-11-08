package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
)

func TestAuditToggleAndExport(t *testing.T) {
	a := newPaletteTestApp(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	a.palEnter("settings")
	it, ok := hasLabel(a.paletteItems(), "Audit-Protokoll (wer")
	if !ok || it.state != palOff {
		t.Fatalf("Schalter: %+v", it)
	}
	it.run()
	if !a.auditOn() || !a.st.Settings().Audit {
		t.Fatal("nicht eingeschaltet")
	}
	a.auditAdd(audit.EvSettings, auditConn(domain.Connection{Alias: "sw1", Host: "10.0.0.1", Username: "admin", Port: 2222}), "Test")
	s := &session{conn: domain.Connection{Alias: "web", Host: "web.lan", Username: "root"}}
	a.auditCommand(s, &cmdRun{cmd: "rm -rf /tmp/x", agent: true}, false, nil)
	a.auditCommand(s, &cmdRun{cmd: "geheim", private: true}, false, nil)
	a.exportAudit()
	m, _ := filepath.Glob(filepath.Join(home, "sshtermy-audit-*.txt"))
	if len(m) != 1 {
		t.Fatalf("Export fehlt: %v", m)
	}
	b, _ := os.ReadFile(m[0])
	txt := string(b)
	for _, want := range []string{"lückenlos", "sw1 (admin@10.0.0.1:2222)", "agent", "rm -rf /tmp/x", "(privat)"} {
		if !strings.Contains(txt, want) {
			t.Errorf("%q fehlt:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "geheim") {
		t.Error("privater Befehl im Audit")
	}
	// Ausschalten wird selbst protokolliert.
	a.toggleAudit()
	if a.auditOn() {
		t.Fatal("nicht aus")
	}
	l, _ := audit.Open(a.st.Dir, mustKey(t, a))
	r := l.Verify()
	if !r.OK() || r.Entries[len(r.Entries)-1].Detail != "Audit-Protokoll aus" {
		t.Fatalf("%v %+v", r.Problems, r.Entries)
	}
}

func mustKey(t *testing.T, a *App) []byte {
	k, err := a.st.V.DeriveKey("audit-log")
	if err != nil {
		t.Fatal(err)
	}
	return k
}
