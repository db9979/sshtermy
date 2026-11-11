package psremote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKrbUser(t *testing.T) {
	for _, c := range []struct{ user, host, def, name, realm string }{
		{"admin@corp.example", "10.0.0.5", "", "admin", "CORP.EXAMPLE"},
		{`CORP\admin`, "srv01.corp.example", "", "admin", "CORP.EXAMPLE"},
		{"admin", "srv01.corp.example", "", "admin", "CORP.EXAMPLE"},
		{"admin", "srv01", "lab.local", "admin", "LAB.LOCAL"},
		{"admin", "srv01", "", "admin", ""},
	} {
		n, r := krbUser(c.user, c.host, c.def)
		if n != c.name || r != c.realm {
			t.Errorf("%q @ %q: %q %q, erwartet %q %q", c.user, c.host, n, r, c.name, c.realm)
		}
	}
}

func TestKerberosTransport(t *testing.T) {
	t.Setenv("KRB5_CONFIG", filepath.Join(t.TempDir(), "fehlt.conf"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if _, err := kerberosTransport(Config{Host: "10.0.0.5", User: "a@corp.example", Auth: AuthKerberos}); !errors.Is(err, ErrKerberosIP) {
		t.Fatalf("IP: %v", err)
	}
	if _, err := kerberosTransport(Config{Host: "srv01", User: "admin", Auth: AuthKerberos}); !errors.Is(err, ErrKerberosRealm) && defaultRealm() == "" {
		t.Fatalf("ohne Realm: %v", err)
	}
	tr, err := kerberosTransport(Config{Host: "srv01.corp.example", Port: 5985, User: "admin", Password: "x", Auth: AuthKerberos})
	if err != nil || tr == nil {
		t.Fatalf("Transport: %v", err)
	}
	if _, err := os.Stat("/etc/krb5.conf"); err == nil {
		return // vorhandene krb5.conf hat Vorrang
	}
	p, err := krbConfPath("CORP.EXAMPLE")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "default_realm = CORP.EXAMPLE") || !strings.Contains(string(b), "dns_lookup_kdc = true") {
		t.Fatalf("krb5.conf: %s", b)
	}
}
