package agent

import (
	"strings"
	"testing"

	"github.com/db9979/sshtermy/internal/i18n"
)

func TestFailReasonsTranslated(t *testing.T) {
	for _, p := range failPatterns {
		if !i18n.Has(p.reason) {
			t.Errorf("keine englische Fassung für %q", p.reason)
		}
	}
}

func TestSysLanguage(t *testing.T) {
	defer i18n.Set(i18n.DE)
	if sys("x") != "x" {
		t.Fatal("Deutsch: Prompt unverändert")
	}
	i18n.Set(i18n.EN)
	if !strings.Contains(sys("x"), "Englisch") {
		t.Fatal("Englisch: Sprachhinweis fehlt")
	}
}
