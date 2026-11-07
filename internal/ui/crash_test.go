package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/i18n"
)

// TestCrashAfterUnlock: Ein harter Absturz der letzten Sitzung wird nach dem
// Entsperren zum Bericht, ohne Passwörter und Hosts aus dem Store.
func TestCrashAfterUnlock(t *testing.T) {
	g := newGold(t, i18n.DE, 100, 30, true)
	dir := crash.Dir(g.a.st.Dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := "panic: login db-01 (10.0.1.5) mit geheim-golden fehlgeschlagen\n\ngoroutine 7 [running]:\nmain.main()\n"
	if err := os.WriteFile(filepath.Join(dir, "fatal-prev.log"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	g.a.modal = nil
	g.a.checkCrash()
	if g.a.modal == nil || g.a.modal.kind != mCrash {
		t.Fatal("kein Absturz-Dialog")
	}
	b, err := os.ReadFile(g.a.modal.crash.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"geheim-golden", "db-01", "10.0.1.5"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("%q im Bericht:\n%s", bad, b)
		}
	}
	if crash.Pending(g.a.st.Dir) != "" {
		t.Error("Rohtext nicht aufgeräumt")
	}
	g.keys("esc")
	if g.a.modal != nil {
		t.Error("Esc schließt nicht")
	}
}
