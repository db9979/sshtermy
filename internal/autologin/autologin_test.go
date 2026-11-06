package autologin

import "testing"

func TestLogin(t *testing.T) {
	var got []string
	h := New("manager", "geheim", func(s string) { got = append(got, s) })
	h.Write([]byte("HP J9773A\r\n\r\nPress any key to continue\x1b[?25h"))
	h.Write([]byte("\r\n\r\nUser"))
	h.Write([]byte("name: "))
	h.Write([]byte("manager\r\nPassword: "))
	h.Write([]byte("\r\nUsername: ")) // zweiter Versuch: nicht noch einmal tippen
	want := []string{" ", "manager\r", "geheim\r"}
	if len(got) != len(want) {
		t.Fatalf("got %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestNoCredentials(t *testing.T) {
	n := 0
	h := New("", "", func(string) { n++ })
	h.Write([]byte("login: "))
	h.Write([]byte("Password: "))
	if n != 0 {
		t.Fatalf("typed %d times without credentials", n)
	}
}

func TestPromptInsideText(t *testing.T) {
	n := 0
	h := New("admin", "x", func(string) { n++ })
	h.Write([]byte("Last login: Mon Nov  3 10:00:00 2026\r\n"))
	if n != 0 {
		t.Fatal("reacted to a line that is no prompt")
	}
}
