package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/agent"
	"github.com/db9979/sshtermy/internal/domain"
)

func TestGoalDiffText(t *testing.T) {
	a := dangerApp(t)
	s := &session{conn: domain.Connection{Alias: "web"}}
	list := []fileBackup{
		{Path: "/etc/nginx/nginx.conf", Existed: true, Content: []byte("a\nb\nc\n")},
		{Path: "/etc/new.conf"},
		{Path: "/etc/same", Existed: true, Content: []byte("x\n")},
		{Path: "/etc/secret", Existed: true, Content: []byte("x")},
	}
	now := []fileNow{
		{Path: "/etc/nginx/nginx.conf", Exists: true, Content: []byte("a\nB\nc\nd\n")},
		{Path: "/etc/new.conf", Exists: true, Content: []byte("neu\n")},
		{Path: "/etc/same", Exists: true, Content: []byte("x\n")},
		{Path: "/etc/secret", Err: "nicht lesbar"},
	}
	title, lines := a.goalDiffText(s, list, now)
	body := ansiStrip(strings.Join(lines, "\n"))
	for _, want := range []string{"+ B", "- b", "+ d", "neu angelegt", "unverändert", "nicht lesbar"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q fehlt:\n%s", want, body)
		}
	}
	if tt := ansiStrip(title); !strings.Contains(tt, "2 Dateien") || !strings.Contains(tt, "+3") || !strings.Contains(tt, "−1") {
		t.Errorf("Titel: %q", tt)
	}
}

func TestViewVerify(t *testing.T) {
	a := dangerApp(t)
	s := &session{conn: domain.Connection{Alias: "web"}}
	s.goal.done = true
	var b strings.Builder
	s.goal.verify = &verifyState{busy: true, model: "m"}
	a.viewVerify(&b, s, 60)
	if !strings.Contains(ansiStrip(b.String()), "läuft") {
		t.Error(b.String())
	}
	b.Reset()
	s.goal.verify = &verifyState{v: agent.Verdict{OK: false, Finding: "Port 443 zu"}, checks: []string{"ss -ltn"}}
	a.viewVerify(&b, s, 60)
	if v := ansiStrip(b.String()); !strings.Contains(v, "Zweifel") || !strings.Contains(v, "Port 443 zu") || !strings.Contains(v, "ss -ltn") {
		t.Error(v)
	}
	_ = a.toggleVerify()
	if a.verifyOn() {
		t.Error("Schalter")
	}
}

func TestTasksFlow(t *testing.T) {
	a := dangerApp(t)
	c, err := a.st.UpsertConnection(domain.Connection{Alias: "web", Host: "web.lan", Username: "root", Group: "Prod / Web"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addTask(c.ID, "", "Zertifikat erneuern bis 1.1.2020"); err != nil {
		t.Fatal(err)
	}
	if err := a.addTask("", "Prod", "Kernel-Updates einspielen"); err != nil {
		t.Fatal(err)
	}
	_ = a.openTasks(c.ID, c.Group)
	refs := a.taskRefs()
	if len(refs) != 2 || refs[0].t.Text != "Zertifikat erneuern" || refs[1].folder != "Prod" {
		t.Fatalf("Liste: %+v", refs)
	}
	v := ansiStrip(a.tasksView())
	if !strings.Contains(v, "fällig 01.01.") || !strings.Contains(v, "Prod") {
		t.Errorf("Ansicht:\n%s", v)
	}
	if b := tasksBadge(append(c.Tasks, refs[0].t), time.Now()); !strings.Contains(b, "fällig") {
		t.Errorf("Badge %q", b)
	}
	// ohne offenen Tab: nichts starten
	if cmd := a.tasksKey("enter"); cmd != nil || a.modal == nil {
		t.Error("ohne Tab gestartet")
	}
	// mit Tab: Ziel startet, Aufgabe läuft
	s, _ := typedSession(c, "root@web:~# ", "")
	a.sessions = []*session{s}
	a.cur = 0
	_ = a.tasksKey("enter")
	if !s.goal.active || s.goal.goal != "Zertifikat erneuern" || s.goal.task == nil {
		t.Fatalf("Ziel nicht gestartet: %+v", s.goal.goal)
	}
	got, _ := a.st.Connection(c.ID)
	if got.Tasks[0].State != domain.TaskRunning {
		t.Errorf("Zustand %q", got.Tasks[0].State)
	}
	// Ziel erreicht → zur Abnahme, Gegenprüfung hängt den Befund an
	s.goal.doneReason = "neues Zertifikat bis 2027"
	a.taskGoalEnded(s, "done")
	a.taskVerified(s, true, "Ablaufdatum 2027 belegt")
	got, _ = a.st.Connection(c.ID)
	if got.Tasks[0].State != domain.TaskReview || !strings.Contains(got.Tasks[0].Note, "Ablaufdatum 2027") {
		t.Errorf("nach dem Ziel: %+v", got.Tasks[0])
	}
	// abhaken, löschen
	_ = a.openTasks(c.ID, c.Group)
	_ = a.tasksKey("a")
	got, _ = a.st.Connection(c.ID)
	if got.Tasks[0].State != domain.TaskDone {
		t.Error("nicht abgehakt")
	}
	a.taskL.sel = 0
	refs = a.taskRefs()
	for i, r := range refs {
		if r.folder == "Prod" {
			a.taskL.sel = i
		}
	}
	_ = a.tasksKey("delete")
	if len(a.st.FolderTasks("Prod")) != 0 {
		t.Error("Ordner-Aufgabe nicht gelöscht")
	}
	_ = a.tasksKey("esc")
	if a.modal != nil {
		t.Error("Esc schließt")
	}
	// u im Manager
	a.scr = scrManager
	_, _ = a.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
}
