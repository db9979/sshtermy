package importer

import (
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestSSHConfig(t *testing.T) {
	cfg := `
# Kommentar
Host web1 web2
    HostName %h.example.com
    User deploy
Host db
    HostName 10.0.0.5
    Port 2222
    IdentityFile ~/.ssh/id_db
    ProxyJump web1
Host *.lan !nas.lan
    User pi
Host *
    User fallback
    IdentityFile ~/.ssh/id_all
Match host foo
    User nobody
Host="quoted"
    HostName = 1.2.3.4
`
	es, err := ParseSSHConfig(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Entry{}
	for _, e := range es {
		got[e.Conn.Alias] = e
	}
	if len(es) != 4 {
		t.Fatalf("4 Einträge erwartet, %d: %+v", len(es), es)
	}
	w := got["web2"].Conn
	if w.Host != "web2.example.com" || w.Username != "deploy" || w.Port != 22 || w.Auth != domain.AuthKeyFile || !strings.HasSuffix(w.KeyPath, "id_all") {
		t.Errorf("web2: %+v", w)
	}
	d := got["db"]
	if d.Conn.Host != "10.0.0.5" || d.Conn.Port != 2222 || d.Conn.Username != "fallback" || !strings.HasSuffix(d.Conn.KeyPath, "id_db") || d.Jump != "web1" {
		t.Errorf("db: %+v", d)
	}
	if q := got["quoted"].Conn; q.Host != "1.2.3.4" {
		t.Errorf("quoted: %+v", q)
	}
}

func TestHostMatches(t *testing.T) {
	if !hostMatches("pi.lan", []string{"*.lan", "!nas.lan"}) || hostMatches("nas.lan", []string{"*.lan", "!nas.lan"}) {
		t.Fatal("Muster falsch")
	}
}

func utf16reg(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := []byte{0xff, 0xfe}
	for _, x := range u {
		b = append(b, byte(x), byte(x>>8))
	}
	return b
}

func TestPuTTYReg(t *testing.T) {
	reg := "Windows Registry Editor Version 5.00\r\n\r\n" +
		"[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\Default%20Settings]\r\n\"HostName\"=\"\"\r\n\r\n" +
		"[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\Core%20Switch]\r\n" +
		"\"HostName\"=\"admin@10.1.1.1\"\r\n\"PortNumber\"=dword:00000016\r\n\"Protocol\"=\"ssh\"\r\n" +
		"\"PublicKeyFile\"=\"C:\\\\Keys\\\\sw.ppk\"\r\n\r\n" +
		"[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\router]\r\n" +
		"\"HostName\"=\"r1\"\r\n\"PortNumber\"=dword:00000017\r\n\"Protocol\"=\"telnet\"\r\n"
	txt := decodeText(utf16reg(reg))
	es, err := ParsePuTTYReg(txt)
	if err != nil || len(es) != 1 {
		t.Fatalf("%v %+v", err, es)
	}
	c := es[0].Conn
	if c.Alias != "Core Switch" || c.Host != "10.1.1.1" || c.Username != "admin" || c.Port != 22 || c.Auth != domain.AuthAgent || len(es[0].Notes) == 0 {
		t.Errorf("%+v %v", c, es[0].Notes)
	}
}

func TestMoba(t *testing.T) {
	ini := "[Misc]\r\nFoo=bar\r\n[Bookmarks]\r\nSubRep=\r\nImgNum=42\r\n" +
		"nas= #109#0%192.168.1.10%2222%dominik%%-1%-1%%%22%%0%0%0%_ProfileDir_\\.ssh\\id_ed25519%%-1%0%0%0%%1080%%0%0%1#MobaFont%10%0#0# #-1\r\n" +
		"rdp= #91#4%win%3389%admin#MobaFont%10\r\n" +
		"[Bookmarks_1]\r\nSubRep=Netz\\Switches\r\nImgNum=41\r\n" +
		"aruba= #109#0%10.0.0.2%22%manager%%-1%-1%%jump.lan%2200%ops%0%0%0%%%-1#MobaFont#0# #-1\r\n"
	es, err := ParseMoba(ini)
	if err != nil || len(es) != 2 {
		t.Fatalf("%v %+v", err, es)
	}
	n := es[0].Conn
	if n.Alias != "nas" || n.Host != "192.168.1.10" || n.Port != 2222 || n.Username != "dominik" || n.Auth != domain.AuthKeyFile || !strings.HasSuffix(n.KeyPath, "id_ed25519") || strings.Contains(n.KeyPath, "_ProfileDir_") {
		t.Errorf("nas: %+v", n)
	}
	a := es[1]
	if a.Conn.Group != "Netz / Switches" || a.Conn.Username != "manager" || a.Jump != "ops@jump.lan:2200" || a.Conn.Auth != domain.AuthPassword {
		t.Errorf("aruba: %+v", a)
	}
}

func TestIsKnown(t *testing.T) {
	ex := []domain.Connection{{Alias: "web", Host: "h", Port: 22, Username: "u"}}
	if !IsKnown(domain.Connection{Alias: "WEB"}, ex) || !IsKnown(domain.Connection{Alias: "x", Host: "H", Username: "u"}, ex) || IsKnown(domain.Connection{Alias: "y", Host: "h", Username: "v"}, ex) {
		t.Fatal("IsKnown")
	}
}
