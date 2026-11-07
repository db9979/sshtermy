package trzsz

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeServer spielt die Server-Seite (trz/tsz von trzsz, Protokoll 1)
// nach der Referenz-Implementierung trzsz-go: Zeilen "#TYP:…\n".
type fakeServer struct {
	t       *testing.T
	toCli   *Transfer      // Ausgabe des Servers landet hier (Feed)
	in      *io.PipeReader // Eingabe vom Client
	lines   chan string
	bufsize int64
}

func newPair(t *testing.T) (*Transfer, *fakeServer) {
	pr, pw := io.Pipe()
	cli := New(pw)
	cli.Timeout = 3 * time.Second
	s := &fakeServer{t: t, toCli: cli, in: pr, lines: make(chan string, 1000), bufsize: 4096}
	go func() {
		var acc []byte
		buf := make([]byte, 64<<10)
		for {
			n, err := pr.Read(buf)
			acc = append(acc, buf[:n]...)
			for {
				i := bytes.IndexByte(acc, '\n')
				if i < 0 {
					break
				}
				s.lines <- string(acc[:i])
				acc = acc[i+1:]
			}
			if err != nil {
				close(s.lines)
				return
			}
		}
	}()
	return cli, s
}

func (s *fakeServer) out(typ, payload string) { s.toCli.Feed([]byte("#" + typ + ":" + payload + "\n")) }
func (s *fakeServer) outStr(typ, v string)    { s.out(typ, encode([]byte(v))) }
func (s *fakeServer) recv(typ string) string {
	select {
	case l, ok := <-s.lines:
		if !ok {
			s.t.Fatalf("Client hat zugemacht, erwartet %s", typ)
		}
		if !strings.HasPrefix(l, "#"+typ+":") {
			s.t.Fatalf("erwartet #%s, bekommen %.80q", typ, l)
		}
		return l[len(typ)+2:]
	case <-time.After(3 * time.Second):
		s.t.Fatalf("keine Zeile %s", typ)
	}
	return ""
}
func (s *fakeServer) recvStr(typ string) string {
	b, err := decode(s.recv(typ))
	if err != nil {
		s.t.Fatal(err)
	}
	return string(b)
}

func (s *fakeServer) handshake() action {
	var a action
	if err := json.Unmarshal([]byte(s.recvStr("ACT")), &a); err != nil {
		s.t.Fatal(err)
	}
	if a.Confirm {
		s.outStr("CFG", `{"lang":"py","timeout":20,"bufsize":`+strconv.FormatInt(s.bufsize, 10)+`}`)
	}
	return a
}

func TestDetect(t *testing.T) {
	tr, ok := Detect([]byte("$ tsz a.txt\r\n\x1b7\x07::TRZSZ:TRANSFER:S:1.1.7:1730971234567:0\r\n"))
	if !ok || tr.Mode != 'S' || tr.Version != "1.1.7" || tr.ID != "1730971234567" {
		t.Fatalf("Detect = %+v %v", tr, ok)
	}
	if _, ok := Detect([]byte("::TRZSZ:TRANSFER:")); ok {
		t.Fatal("unvollständig erkannt")
	}
}

// tsz: der Server schickt zwei Dateien, eine davon größer als ein Block,
// eine mit einem Namen, der lokal schon existiert.
func TestDownload(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("alt"), 0o644)
	cli, srv := newPair(t)
	big := bytes.Repeat([]byte("sshTermy-trzsz "), 1000) // 15 kB
	files := map[string][]byte{"a.bin": big, "../../b.txt": []byte("neu\n")}
	order := []string{"a.bin", "../../b.txt"}
	var lastDone int64
	cli.Progress = func(_ string, done, _ int64) { lastDone = done }

	errc := make(chan error, 1)
	go func() {
		a := srv.handshake()
		if !a.Confirm || a.SupportBinary || a.SupportDirectory {
			t.Errorf("Zusage = %+v", a)
		}
		srv.out("NUM", "2")
		if srv.recv("SUCC") != "2" {
			t.Error("NUM nicht bestätigt")
		}
		for _, name := range order {
			data := files[name]
			srv.outStr("NAME", name)
			local := srv.recvStr("SUCC")
			if strings.Contains(local, "/") {
				t.Errorf("lokaler Name mit Pfad: %q", local)
			}
			srv.out("SIZE", strconv.Itoa(len(data)))
			srv.recv("SUCC")
			for off := 0; off < len(data); off += int(srv.bufsize) {
				chunk := data[off:min(off+int(srv.bufsize), len(data))]
				srv.out("DATA", encode(chunk))
				if srv.recv("SUCC") != strconv.Itoa(len(chunk)) {
					t.Error("Block nicht bestätigt")
				}
			}
			sum := md5.Sum(data)
			srv.out("MD5", encode(sum[:]))
			srv.recv("SUCC")
		}
		srv.recvStr("EXIT")
		errc <- nil
	}()
	got, err := cli.Download(dir)
	if err != nil {
		t.Fatal(err)
	}
	<-errc
	if len(got) != 2 || filepath.Base(got[1]) != "b.txt.0" {
		t.Fatalf("Pfade = %v", got)
	}
	if b, _ := os.ReadFile(got[0]); !bytes.Equal(b, big) {
		t.Fatal("Inhalt a.bin falsch")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "b.txt")); string(b) != "alt" {
		t.Fatal("vorhandene Datei überschrieben")
	}
	if lastDone != 4 {
		t.Fatalf("Fortschritt zuletzt %d", lastDone)
	}
}

// trz: der Client lädt eine Datei hoch, der Server prüft Inhalt und MD5.
func TestUpload(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.txt")
	content := bytes.Repeat([]byte("vlan 20\n"), 2000)
	os.WriteFile(p, content, 0o600)
	cli, srv := newPair(t)
	done := make(chan []byte, 1)
	go func() {
		srv.handshake()
		n := srv.recv("NUM")
		srv.out("SUCC", n)
		name := srv.recvStr("NAME")
		srv.outStr("SUCC", name+".1")
		size := srv.recv("SIZE")
		srv.out("SUCC", size)
		var got []byte
		want, _ := strconv.Atoi(size)
		for len(got) < want {
			d, err := decode(srv.recv("DATA"))
			if err != nil {
				t.Error(err)
				return
			}
			got = append(got, d...)
			srv.out("SUCC", strconv.Itoa(len(d)))
		}
		sum := srv.recv("MD5")
		srv.out("SUCC", sum)
		srv.recvStr("EXIT")
		done <- got
	}()
	names, err := cli.Upload([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	if got := <-done; !bytes.Equal(got, content) {
		t.Fatalf("Server bekam %d Bytes", len(got))
	}
	if len(names) != 1 || names[0] != "config.txt.1" {
		t.Fatalf("Namen = %v", names)
	}
}

// Fehler des Servers und Abbruch durch den Nutzer.
func TestRemoteFailAndCancel(t *testing.T) {
	cli, srv := newPair(t)
	go func() {
		srv.handshake()
		srv.outStr("FAIL", "No such file: x")
	}()
	_, err := cli.Download(t.TempDir())
	if re, ok := err.(*RemoteError); !ok || !strings.Contains(re.Msg, "No such file") {
		t.Fatalf("Fehler = %v", err)
	}

	cli, srv = newPair(t)
	go func() {
		srv.handshake()
		// Server wartet; der Nutzer bricht ab
	}()
	go func() { time.Sleep(100 * time.Millisecond); cli.Cancel() }()
	if _, err := cli.Download(t.TempDir()); err != ErrCanceled {
		t.Fatalf("Abbruch = %v", err)
	}
	if l := srv.recv("fail"); l == "" {
		t.Fatal("Server erfährt nichts vom Abbruch")
	}

	// Ordner hochladen: höflich ablehnen
	cli, srv = newPair(t)
	go func() {
		if a := srv.handshake(); a.Confirm {
			t.Error("Ordner zugesagt")
		}
	}()
	if _, err := cli.Upload([]string{t.TempDir()}); err == nil {
		t.Fatal("Ordner ohne Fehler")
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{"../../etc/passwd": "passwd", `C:\x\y.txt`: "y.txt", "..": "", "a\x1bb": "a_b", "ok.txt": "ok.txt"} {
		if got := SafeName(in); got != want {
			t.Errorf("%q = %q, will %q", in, got, want)
		}
	}
}
