package store

import "testing"

func TestCmdStats(t *testing.T) {
	t.Setenv("SSHTERMY_HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init([]byte("correct horse"), false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.NoteCommand("c1", "docker  ps -a"); err != nil {
			t.Fatal(err)
		}
	}
	u, _ := s.NoteCommand("c1", "df -h")
	if u.N != 1 {
		t.Fatalf("df -h: %+v", u)
	}
	top := s.TopCommands("c1", 0)
	if len(top) != 2 || top[0].Cmd != "docker ps -a" || top[0].N != 3 {
		t.Fatalf("Reihenfolge: %+v", top)
	}
	if err := s.MarkCommand("c1", "df -h", true, true); err != nil {
		t.Fatal(err)
	}
	if len(s.TopCommands("c1", 0)) != 1 {
		t.Fatal("ausgeblendeter Befehl darf nicht mehr kommen")
	}
	// nach Sperren und Entsperren wieder da (verschlüsselt gespeichert)
	s.Lock()
	if _, err := s.NoteCommand("c1", "uptime"); err == nil {
		t.Fatal("gesperrt darf nichts gezählt werden")
	}
	if err := s.V.UnlockPassword([]byte("correct horse")); err != nil {
		t.Fatal(err)
	}
	if err := s.AfterUnlock(); err != nil {
		t.Fatal(err)
	}
	if u := s.CommandUse("c1", "docker ps -a"); u.N != 3 {
		t.Fatalf("nach Entsperren: %+v", u)
	}
	s.dropStats("c1")
	if len(s.TopCommands("c1", 0)) != 0 {
		t.Fatal("Statistik der gelöschten Verbindung muss weg")
	}
}
