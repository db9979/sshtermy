package pwsource

import (
	"context"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	var got []string
	var gotIn string
	old := run
	t.Cleanup(func() { run = old })
	run = func(_ context.Context, stdin, name string, args ...string) (string, error) {
		got = append([]string{name}, args...)
		gotIn = stdin
		return "geheim\nzweite Zeile\n", nil
	}
	cases := []struct {
		spec string
		k    Kind
		want string
		out  string
	}{
		{"op://Server/web-01/password", Password, "op read --no-newline op://Server/web-01/password", "geheim\nzweite Zeile"},
		{"op://Server/web-01/password", OTP, "op read --no-newline op://Server/web-01/one-time password?attribute=otp", "geheim\nzweite Zeile"},
		{"bw:web-01", Password, "bw get password web-01", "geheim\nzweite Zeile"},
		{"bw:web-01", OTP, "bw get totp web-01", "geheim\nzweite Zeile"},
		{"pass:server/web", Password, "pass show server/web", "geheim"},
		{`cmd:"C:\Tools\pw.exe" web`, Password, `C:\Tools\pw.exe web`, "geheim"},
		{`keepassxc:C:\db.kdbx:Server/web`, Password, `keepassxc-cli show -q -a Password C:\db.kdbx Server/web`, "geheim"},
	}
	for _, c := range cases {
		out, err := Get(context.Background(), c.spec, c.k, func(string) (string, error) { return "dbpw", nil })
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if strings.Join(got, " ") != c.want || out != c.out {
			t.Errorf("%s: %q → %q", c.spec, strings.Join(got, " "), out)
		}
	}
	if gotIn != "dbpw" {
		t.Errorf("KeePassXC bekommt das DB-Passwort auf stdin: %q", gotIn)
	}
	for _, bad := range []string{"bw:", "foo", "keepassxc:db", "cmd:  "} {
		if Valid(bad) == nil {
			t.Errorf("%q müsste ungültig sein", bad)
		}
	}
	if Valid("") != nil {
		t.Error("leer ist erlaubt")
	}
}

func TestIsOTPQuestion(t *testing.T) {
	for _, q := range []string{"Verification code: ", "OTP:", "Enter your Duo token"} {
		if !IsOTPQuestion(q) {
			t.Errorf("%q", q)
		}
	}
	if IsOTPQuestion("Password: ") {
		t.Error("Passwort ist kein Code")
	}
}
