package ui

import (
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestHighlightAlertInHiddenTab(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal = nil
	if err := a.st.UpdateSettings(func(s *domain.Settings) {
		s.Highlight.Rules = []string{"!rot %LINK-3-UPDOWN"}
		s.NoDesktopNotify = true
	}); err != nil {
		t.Fatal(err)
	}
	vis := &session{id: 1, conn: domain.Connection{Alias: "vorn"}, state: stConnected, chipArmed: -1}
	bg := &session{id: 2, conn: domain.Connection{Alias: "sw-keller"}, state: stConnected, chipArmed: -1}
	a.sessions, a.cur, a.scr = []*session{vis, bg}, 0, scrTerminal
	a.alertTick(bg) // legt den Scanner an
	sc := bg.alerts.Load()
	if sc == nil {
		t.Fatal("kein Scanner trotz Regel mit !")
	}
	sc.feed([]byte("\x1b[0mNov 8 %LINK-3-UPD"))
	sc.feed([]byte("OWN: Interface 1/1/3, changed state to down\r\nharmlos\n"))
	a.alertTick(bg)
	if bg.notice != noticeAlert {
		t.Fatalf("Tab nicht markiert: %v", bg.notice)
	}
	if len(a.toasts) == 0 || !strings.Contains(a.toasts[0].text, "sw-keller") || !strings.Contains(a.toasts[0].text, "%LINK-3-UPDOWN") {
		t.Fatalf("Einblendung: %+v", a.toasts)
	}
	// Ohne „!“-Regel kein Scanner mehr
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Highlight.Rules = nil })
	a.alertTick(bg)
	if bg.alerts.Load() != nil {
		t.Fatal("Scanner muss weg")
	}
	// Farbregeln aus: nichts zum Zeichnen
	_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Highlight.Off = true })
	if a.termHL() != nil {
		t.Fatal("aus heißt aus")
	}
}

func TestHighlightFormValidates(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal = nil
	a.openHighlightForm()
	a.form.get("r0").in.SetValue("Red  error 4[0-9]")
	a.form.get("r1").in.SetValue("pink x")
	if err := a.saveHighlight(); err == nil || !strings.Contains(err.Error(), "Regel 2") {
		t.Fatalf("kaputte Regel muss mit Nummer gemeldet werden: %v", err)
	}
	a.form.get("r1").in.SetValue("")
	a.form.get("defaults").on = false
	if err := a.saveHighlight(); err != nil {
		t.Fatal(err)
	}
	h := a.st.Settings().Highlight
	if len(h.Rules) != 1 || h.Rules[0] != "rot error 4[0-9]" || !h.NoDefaults {
		t.Fatalf("%+v", h)
	}
}

func TestEditorCommand(t *testing.T) {
	c, err := editorCommand(`"C:\Program Files\Notepad++\notepad++.exe" -multiInst`, "/tmp/x.conf")
	if err != nil || c.Args[0] != `C:\Program Files\Notepad++\notepad++.exe` || c.Args[1] != "-multiInst" || c.Args[2] != "/tmp/x.conf" {
		t.Fatalf("%v %q", err, c.Args)
	}
	if !terminalEditor("nvim -p") || terminalEditor("code --wait") {
		t.Fatal("terminalEditor")
	}
}
