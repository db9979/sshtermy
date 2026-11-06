package keychain

import (
	"errors"
	"testing"
)

func TestParseSecurityPassword(t *testing.T) {
	cases := map[string]string{
		"keychain: \"/Users/x/Library/Keychains/login.keychain-db\"\nclass: \"genp\"\npassword: \"geheim 123\"\n": "geheim 123",
		"password: 0x6162C3A4  \"ab\\303\\244\"\n":                                                                "abä",
		"password: \n": "",
	}
	for in, want := range cases {
		got, err := parseSecurityPassword(in)
		if err != nil || got != want {
			t.Errorf("parse(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := parseSecurityPassword("class: x\n"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ohne password-Zeile: %v", err)
	}
}

func TestMemoryBackend(t *testing.T) {
	old := Use(&Memory{})
	defer Use(old)
	if _, err := Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := Set("a", "Label", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if v, _ := Get("a"); v != "s3cret" {
		t.Fatalf("got %q", v)
	}
	if err := Delete("a"); err != nil {
		t.Fatal(err)
	}
	if err := Delete("a"); err != nil {
		t.Fatal("zweites Löschen darf nicht scheitern:", err)
	}
}
