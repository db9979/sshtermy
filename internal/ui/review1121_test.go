package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/domain"
)

// KI-Zeitplan: ohne bestätigte Befehle läuft nichts; geänderter Auftrag
// macht die Bestätigung ungültig.
func TestScheduleAINeedsConfirmedChecks(t *testing.T) {
	a := dangerApp(t)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "web.lan", Username: "root", Group: "Prod"})
	if err != nil {
		t.Fatal(err)
	}
	sc := domain.Schedule{ID: "s1", Kind: domain.ScheduleAI, Folder: "Prod", Prompt: "läuft nginx?"}
	cmd := a.startScheduleAI(sc, []domain.Connection{c})
	if m, ok := cmd().(schedDoneMsg); !ok || m.err == nil || !strings.Contains(m.err.Error(), "bestätigt") {
		t.Fatalf("ohne Bestätigung gelaufen: %#v", cmd())
	}
	sc.Checks, sc.ChecksFor = []string{"systemctl is-active nginx"}, "läuft nginx?"
	if !sc.ChecksPinned() {
		t.Fatal("bestätigt erwartet")
	}
	sc.Prompt = "läuft apache?"
	if sc.ChecksPinned() {
		t.Fatal("neuer Auftrag braucht neue Bestätigung")
	}
	sc.Prompt, sc.Checks = "läuft nginx?", []string{"sudo cat /etc/shadow"}
	if m, ok := a.startScheduleAI(sc, []domain.Connection{c})().(schedDoneMsg); !ok || m.err == nil {
		t.Fatal("Geheimnis-Befehl trotz Bestätigung gelaufen")
	}
}

func TestRunsWhenOpened(t *testing.T) {
	for n, want := range map[string]bool{"x.exe": true, "Rechnung.PDF.bat": true, "a.lnk": true, "x.exe.": true, "run.command": true,
		"bild.png": false, "doc.pdf": false, "notes.txt": false, "README": false} {
		if runsWhenOpened(n) != want {
			t.Errorf("%s: %v", n, !want)
		}
	}
}

func TestResolveConnIPsParallel(t *testing.T) {
	var conns []domain.Connection
	for i := 0; i < 40; i++ {
		conns = append(conns, domain.Connection{ID: fmt.Sprint("c", i), Host: fmt.Sprintf("tot%d.lan", i)})
	}
	conns = append(conns, domain.Connection{ID: "ip", Host: "10.0.0.9"}, domain.Connection{ID: "ok", Host: "gut.lan"})
	start := time.Now()
	m := resolveConnIPs(context.Background(), conns, func(ctx context.Context, h string) ([]string, error) {
		if h == "gut.lan" {
			return []string{"10.0.0.7"}, nil
		}
		<-ctx.Done() // hängt bis zum Zeitlimit
		return nil, ctx.Err()
	})
	if m["10.0.0.9"] != "ip" || m["10.0.0.7"] != "ok" {
		t.Fatalf("%v", m)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("zu langsam: %v", d)
	}
}

// Esc-Ziel per Maus finden (klickbare Zone über dem Wort).
func escZone(a *App, words ...string) (int, int, bool) {
	lines := strings.Split(ansi.Strip(a.View().Content), "\n")
	for _, w := range words {
		for y, l := range lines {
			if i := strings.Index(l, w); i >= 0 {
				x := ansi.StringWidth(l[:i])
				if a.z.At(x, y, false) != nil {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// M1: Bei schmalem Fenster bleibt „Esc“ in der Fußzeile klickbar.
func TestFooterKeepsEsc(t *testing.T) {
	for _, w := range []int{80, 100, 120} {
		a := newPaletteTestApp(t)
		a.pal, a.intro, a.w = nil, nil, w
		a.openLogs()
		if a.scr != scrLogs {
			t.Fatal("Protokolle nicht offen")
		}
		x, y, ok := escZone(a, "Esc")
		if !ok {
			t.Fatalf("Breite %d: kein klickbares Esc", w)
		}
		a.mouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		if a.scr == scrLogs {
			t.Fatalf("Breite %d: Klick auf Esc führt nicht zurück", w)
		}
	}
}

// M2 + M4: Textfenster blättert mit dem Rad und schließt per Klick daneben.
func TestTextModalWheelAndClickAway(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal, a.intro = nil, nil
	a.scr = scrManager
	m := &modal{kind: mText, title: "Diff", lines: strings.Split(strings.Repeat("Zeile\n", 200), "\n")}
	a.modal = m
	_ = a.View()
	a.mouse(tea.MouseWheelMsg{X: a.w / 2, Y: a.h / 2, Button: tea.MouseWheelDown})
	if m.scroll != 3 {
		t.Fatalf("Rad: scroll %d", m.scroll)
	}
	a.mouse(tea.MouseClickMsg{X: 0, Y: 1, Button: tea.MouseLeft})
	if a.modal != nil {
		t.Fatal("Klick daneben schließt das Textfenster nicht")
	}
}

// M3: Seitentaste „Zurück“ schließt Dialog und führt im Untermenü zurück.
func TestMouseBackButton(t *testing.T) {
	a := newPaletteTestApp(t)
	a.intro = nil
	a.scr = scrManager
	a.palEnter("settings")
	back := tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseBackward}
	a.mouse(back)
	if a.pal == nil || a.pal.menu != "" {
		t.Fatalf("Untermenü: %+v", a.pal)
	}
	a.mouse(back)
	if a.pal != nil {
		t.Fatal("Palette bleibt offen")
	}
	a.modal = &modal{kind: mInfo, title: "x", body: "y"}
	a.mouse(back)
	if a.modal != nil {
		t.Fatal("Dialog bleibt offen")
	}
	// Rechtsklick ins Leere im Untermenü: eine Ebene zurück
	a.openPalette()
	a.palEnter("help")
	_ = a.View()
	a.mouse(tea.MouseClickMsg{X: 0, Y: a.h - 3, Button: tea.MouseRight})
	if a.pal == nil || a.pal.menu != "" {
		t.Fatalf("Rechtsklick im Untermenü: %+v", a.pal)
	}
}

// M5: Rechtsklick auf freie Fläche im Manager zeigt ein Menü mit „Neuer Ordner“.
func TestManagerBlankMenu(t *testing.T) {
	a := newPaletteTestApp(t)
	a.pal, a.intro = nil, nil
	a.scr = scrManager
	_, _ = a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "web.lan", Username: "root"})
	_ = a.View()
	a.mouse(tea.MouseClickMsg{X: 3, Y: a.h - 4, Button: tea.MouseRight})
	if a.modal == nil || a.modal.kind != mMenu {
		t.Fatal("kein Menü")
	}
	found := false
	for _, it := range a.modal.items {
		if strings.HasPrefix(it.label, "Neuer Ordner") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Neuer Ordner fehlt: %+v", a.modal.items)
	}
}
