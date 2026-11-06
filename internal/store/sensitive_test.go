package store

import (
	"slices"
	"testing"

	"github.com/db9979/sshtermy/internal/domain"
)

func TestSensitive(t *testing.T) {
	s := openTest(t)
	if _, err := s.UpsertConnection(domain.Connection{Alias: "db-01", Host: "10.0.1.5", Username: "pi", Auth: domain.AuthPassword,
		Password: "pw-geheim", KeyPassphrase: "pass-geheim"}); err != nil {
		t.Fatal(err)
	}
	llm := s.LLM()
	llm.BaseURL, llm.APIKey = "http://spark.lan:8000/v1", "sk-geheim"
	if err := s.SetLLM(llm); err != nil {
		t.Fatal(err)
	}
	got := s.Sensitive()
	for _, want := range []string{"pw-geheim", "pass-geheim", "sk-geheim", "10.0.1.5", "db-01", "spark.lan"} {
		if !slices.Contains(got, want) {
			t.Errorf("%q fehlt in %v", want, got)
		}
	}
	if slices.Contains(got, "pi") {
		t.Error("kurzer Benutzername würde den Stack zerreißen")
	}
	s.Lock()
	if s.Sensitive() != nil {
		t.Error("gesperrt: nil erwartet")
	}
}
