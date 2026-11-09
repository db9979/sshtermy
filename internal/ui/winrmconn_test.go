package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/psremote"
	"github.com/db9979/sshtermy/internal/store"
)

func TestParseQuickWinRM(t *testing.T) {
	for in, want := range map[string]domain.Connection{
		"winrm srv01":            {Host: "srv01", Port: 5985},
		"winrm admin@srv01 5986": {Host: "srv01", Port: 5986, Username: "admin", WinRMHTTPS: true},
		"Enter-PSSession srv01":  {Host: "srv01", Port: 5985},
		"Enter-PSSession -ComputerName srv01 -Credential FIRMA\\admin -UseSSL": {Host: "srv01", Port: 5986, Username: "FIRMA\\admin", WinRMHTTPS: true},
		"etsn 10.0.0.5": {Host: "10.0.0.5", Port: 5985},
	} {
		c, ok := parseQuickWinRM(in)
		if !ok || c.Protocol != domain.ProtoWinRM || c.Host != want.Host || c.Port != want.Port || c.Username != want.Username || c.WinRMHTTPS != want.WinRMHTTPS {
			t.Errorf("%q → %+v %v", in, c, ok)
		}
	}
	for _, in := range []string{"winrm", "winrm a b c", "Enter-PSSession -Session $s", "pwsh", "telnet srv01", "winrm srv01 99999"} {
		if _, ok := parseQuickWinRM(in); ok {
			t.Errorf("%q erkannt", in)
		}
	}
}

func TestConnFormWinRM(t *testing.T) {
	a, st := rawTestApp(t)
	a.openConnForm(newConnDefaults())
	f := a.form
	f.get("alias").in.SetValue("DC01")
	f.get("host").in.SetValue("dc01.firma.local")
	setChoice(t, f, "proto", "PowerShell")
	syncProtoPort(f)
	if got := f.str("port"); got != "5985" {
		t.Fatalf("Port nach Wechsel auf PowerShell %q", got)
	}
	if fl := f.get("auth"); !fl.hidden(f) {
		t.Fatal("SSH-Verfahren bei WinRM sichtbar")
	}
	for _, k := range []string{"password", "winrm_https", "winrm_auth", "pwsrc"} {
		if fl := f.get(k); fl.hidden != nil && fl.hidden(f) {
			t.Fatalf("%s bei WinRM versteckt", k)
		}
	}
	if fl := f.get("winrm_insecure"); !fl.hidden(f) {
		t.Fatal("Zertifikat-Schalter ohne HTTPS sichtbar")
	}
	f.get("user").in.SetValue(`FIRMA\admin`)
	f.get("password").in.SetValue("geheim")
	f.get("winrm_https").on = true
	f.get("winrm_insecure").on = true
	if err := a.saveConn(); err != nil {
		t.Fatal(err)
	}
	cs := st.Connections()
	if len(cs) != 1 {
		t.Fatalf("%+v", cs)
	}
	c := cs[0]
	if c.Protocol != domain.ProtoWinRM || c.Port != 5986 || !c.WinRMHTTPS || !c.WinRMInsecure || c.Password.Reveal() != "geheim" || c.Auth != domain.AuthPassword {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(c.Target(), "PowerShell") || shortTarget(c) != `PS FIRMA\admin@dc01.firma.local` {
		t.Fatalf("Anzeige %q / %q", c.Target(), shortTarget(c))
	}

	// Basic über HTTP lehnt das Formular ab
	a.openConnForm(c)
	f = a.form
	f.get("winrm_https").on = false
	setChoice(t, f, "winrm_auth", "basic")
	if err := a.saveConn(); err == nil {
		t.Fatal("Basic über HTTP gespeichert")
	}
}

func TestValidateWinRM(t *testing.T) {
	c := domain.Connection{Alias: "x", Host: "srv", Protocol: domain.ProtoWinRM}
	if store.Validate(c) == nil {
		t.Fatal("ohne Benutzer gültig")
	}
	c.Username = "admin"
	if err := store.Validate(c); err != nil {
		t.Fatal(err)
	}
	if c.DefaultPort() != 5985 {
		t.Fatal(c.DefaultPort())
	}
	c.WinRMHTTPS = true
	if c.DefaultPort() != 5986 {
		t.Fatal(c.DefaultPort())
	}
}

func TestWinRMErrorHints(t *testing.T) {
	c := domain.Connection{Protocol: domain.ProtoWinRM, Host: "srv"}
	err := winrmError(&psremote.Error{Kind: psremote.ErrRefused, Cause: errors.New("dial tcp: connection refused")}, c)
	if !strings.Contains(err.Error(), "Enable-PSRemoting") || !strings.Contains(err.Error(), "5985") {
		t.Fatal(err)
	}
	err = winrmError(&psremote.Error{Kind: psremote.ErrAuth, Cause: errors.New("http error 401")}, c)
	if !isAuthFailure(err) {
		t.Fatal("Anmeldefehler nicht erkannt:", err)
	}
	if plain := errors.New("x"); winrmError(plain, c) != plain {
		t.Fatal("fremder Fehler verändert")
	}
}

func TestWinRMProfile(t *testing.T) {
	p := winrmProfile(nil)
	if p.Family != domain.FamilyWindows || p.Dialect != "powershell" {
		t.Fatalf("%+v", p)
	}
}
