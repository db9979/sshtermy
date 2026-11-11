package psremote

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/db9979/sshtermy/internal/termemu"
)

// fakeWSMan: WinRS-Endpunkt mit Basic-Anmeldung über HTTPS. Hinter jedem
// „Command“ steckt fakeProc (stream_test.go), so läuft das ganze
// SOAP-Hin-und-Her wie gegen einen echten Server.
type fakeWSMan struct {
	t      *testing.T
	ps     *fakePS
	mu     sync.Mutex
	out    map[string]chan string // CommandId → Ausgabe (stdout, base64)
	procs  map[string]proc
	done   map[string]bool
	shells int
	gone   int
	cmds   []string
}

var (
	reAction = regexp.MustCompile(`Action[^>]*>([^<]+)<`)
	reStream = regexp.MustCompile(`Stream[^>]*CommandId="([^"]+)"[^>]*>([^<]*)<`)
	reCmdID  = regexp.MustCompile(`CommandId="([^"]+)"`)
	reArgs   = regexp.MustCompile(`<[^>]*Command>(?:<!\[CDATA\[)?([^<\]]+)`)
)

const envHead = `<s:Envelope xml:lang="en-US" xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:x="http://schemas.xmlsoap.org/ws/2004/09/transfer" xmlns:w="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd" xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell"><s:Header><a:Action>%s</a:Action></s:Header><s:Body>%s</s:Body></s:Envelope>`

func (f *fakeWSMan) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, p, ok := r.BasicAuth()
	if !ok || u != "admin" || p != "geheim" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	b, _ := io.ReadAll(r.Body)
	body := string(b)
	m := reAction.FindStringSubmatch(body)
	if m == nil {
		w.WriteHeader(400)
		return
	}
	reply := func(action, content string) {
		w.Header().Set("Content-Type", "application/soap+xml;charset=UTF-8")
		fmt.Fprintf(w, envHead, action, content)
	}
	switch act := m[1]; {
	case strings.HasSuffix(act, "transfer/Create"):
		f.mu.Lock()
		f.shells++
		f.mu.Unlock()
		reply("http://schemas.xmlsoap.org/ws/2004/09/transfer/CreateResponse", `<rsp:Shell><rsp:ShellId>SHELL-1</rsp:ShellId></rsp:Shell>`)
	case strings.HasSuffix(act, "transfer/Delete"):
		f.mu.Lock()
		f.gone++
		f.mu.Unlock()
		reply("http://schemas.xmlsoap.org/ws/2004/09/transfer/DeleteResponse", "")
	case strings.HasSuffix(act, "shell/Command"):
		a := reArgs.FindStringSubmatch(body)
		f.mu.Lock()
		id := "CMD-" + strconv.Itoa(len(f.procs)+1)
		if a != nil {
			f.cmds = append(f.cmds, a[1])
		}
		ch := make(chan string, 1024)
		f.out[id] = ch
		f.mu.Unlock()
		pr, _ := f.ps.start("", handlers{
			out: func(b []byte) { ch <- base64.StdEncoding.EncodeToString(b) },
			err: func(b []byte) {},
			end: func(error) {},
		})
		f.mu.Lock()
		f.procs[id] = pr
		f.mu.Unlock()
		reply("http://schemas.microsoft.com/wbem/wsman/1/windows/shell/CommandResponse", `<rsp:CommandResponse><rsp:CommandId>`+id+`</rsp:CommandId></rsp:CommandResponse>`)
	case strings.HasSuffix(act, "shell/Send"):
		s := reStream.FindStringSubmatch(body)
		f.mu.Lock()
		pr := f.procs[s[1]]
		f.mu.Unlock()
		in, _ := base64.StdEncoding.DecodeString(s[2])
		_ = pr.send(in)
		reply("http://schemas.microsoft.com/wbem/wsman/1/windows/shell/SendResponse", "")
	case strings.HasSuffix(act, "shell/Signal"):
		id := reCmdID.FindStringSubmatch(body)[1]
		f.mu.Lock()
		f.done[id] = true
		f.mu.Unlock()
		reply("http://schemas.microsoft.com/wbem/wsman/1/windows/shell/SignalResponse", "")
	case strings.HasSuffix(act, "shell/Receive"):
		id := reCmdID.FindStringSubmatch(body)[1]
		f.mu.Lock()
		ch, done := f.out[id], f.done[id]
		f.mu.Unlock()
		if done {
			reply("http://schemas.microsoft.com/wbem/wsman/1/windows/shell/ReceiveResponse",
				`<rsp:ReceiveResponse><rsp:CommandState CommandId="`+id+`" State="http://schemas.microsoft.com/wbem/wsman/1/windows/shell/CommandState/Done"><rsp:ExitCode>0</rsp:ExitCode></rsp:CommandState></rsp:ReceiveResponse>`)
			return
		}
		select {
		case o := <-ch:
			reply("http://schemas.microsoft.com/wbem/wsman/1/windows/shell/ReceiveResponse",
				`<rsp:ReceiveResponse><rsp:Stream Name="stdout" CommandId="`+id+`">`+o+`</rsp:Stream><rsp:CommandState CommandId="`+id+`" State="http://schemas.microsoft.com/wbem/wsman/1/windows/shell/CommandState/Running"></rsp:CommandState></rsp:ReceiveResponse>`)
		case <-time.After(300 * time.Millisecond):
			// wie WinRM nach OperationTimeout ohne Ausgabe
			w.WriteHeader(500)
			fmt.Fprintf(w, envHead, "http://schemas.dmtf.org/wbem/wsman/1/wsman/fault",
				`<s:Fault><s:Code><s:Value>s:Receiver</s:Value><s:Subcode><s:Value>w:TimedOut</s:Value></s:Subcode></s:Code><s:Detail><f:WSManFault xmlns:f="http://schemas.microsoft.com/wbem/wsman/1/wsmanfault" Code="2150858793"/></s:Detail></s:Fault>`)
		}
	default:
		f.t.Errorf("unbekannte Aktion %s", act)
		w.WriteHeader(400)
	}
}

func startFakeWSMan(t *testing.T) (*fakeWSMan, Config) {
	f := &fakeWSMan{t: t, ps: &fakePS{}, out: map[string]chan string{}, procs: map[string]proc{}, done: map[string]bool{}}
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	p, _ := strconv.Atoi(port)
	return f, Config{Host: host, Port: p, HTTPS: true, Insecure: true, Auth: AuthBasic, User: "admin", Password: "geheim", Timeout: 5 * time.Second}
}

func TestWinRMEndToEnd(t *testing.T) {
	f, cfg := startFakeWSMan(t)
	h := &harness{t: t, f: f.ps}
	h.term = termemu.New(80, 24, &bytes.Buffer{})
	defer h.term.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Dial(ctx, cfg, 80, h)
	if err != nil {
		t.Fatal(err)
	}
	h.s = s
	h.wait(`[SRV01]: PS C:\Users\admin>`)
	f.mu.Lock()
	cmds := append([]string(nil), f.cmds...)
	f.mu.Unlock()
	if len(cmds) != 1 || cmds[0] != "powershell.exe" {
		t.Fatalf("Befehle %q", cmds)
	}
	// Eingabe kommt sofort an, obwohl Receive gerade wartet (zweiter Client)
	t0 := time.Now()
	h.typ("Get-Date\r")
	h.wait("Montag")
	if d := time.Since(t0); d > 2*time.Second {
		t.Fatalf("Antwort nach %s", d)
	}
	// Strg+C: Signal + neuer Befehl im selben Ordner
	h.typ("sleep 100\r")
	time.Sleep(50 * time.Millisecond)
	h.typ("\x03")
	h.until("Neustart", func() bool { return len(f.ps.starts) == 2 })
	h.wait("abgebrochen")
	s.Close()
	dl := time.Now().Add(3 * time.Second)
	for time.Now().Before(dl) {
		f.mu.Lock()
		gone := f.gone
		f.mu.Unlock()
		if gone == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Shell drüben nicht gelöscht")
}

func TestWinRMWrongPassword(t *testing.T) {
	_, cfg := startFakeWSMan(t)
	cfg.Password = "falsch"
	_, err := Dial(context.Background(), cfg, 80, io.Discard)
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestWinRMRefusedAndBasicHTTP(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	_, err := Dial(context.Background(), Config{Host: "127.0.0.1", Port: port, User: "a", Password: "b", Timeout: 2 * time.Second}, 80, io.Discard)
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	if _, err := Dial(context.Background(), Config{Host: "x", Auth: AuthBasic}, 80, io.Discard); !errors.Is(err, ErrBasicHTTP) {
		t.Fatalf("err = %v", err)
	}
}

// Portnummern wie 33401 sind kein HTTP-Status 401 (war ein Wackeltest).
func TestExplainPortNotStatus(t *testing.T) {
	err := explain(errors.New(`Post "http://127.0.0.1:33401/wsman": dial tcp 127.0.0.1:33401: connect: connection refused`), Config{})
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	if err := explain(errors.New("http error 401: Unauthorized"), Config{}); !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v", err)
	}
	if err := explain(errors.New("http response error: 401 - invalid content type"), Config{}); !errors.Is(err, ErrAuth) {
		t.Fatalf("err = %v", err)
	}
	if err := explain(errors.New("dial tcp 10.0.0.1:1404: i/o timeout"), Config{}); !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
}

func TestWinRMCertNotTrusted(t *testing.T) {
	_, cfg := startFakeWSMan(t)
	cfg.Insecure = false
	_, err := Dial(context.Background(), cfg, 80, io.Discard)
	if !errors.Is(err, ErrCert) {
		t.Fatalf("err = %v", err)
	}
}
