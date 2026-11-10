package safety

import (
	"testing"

	"github.com/db9979/sshtermy/internal/i18n"
)

func TestReasonsTranslated(t *testing.T) {
	for _, rs := range [][]rule{unixRules, windowsRules, networkRules, sensitiveRules, riskyUnix} {
		for _, r := range rs {
			if !i18n.Has(r.reason) {
				t.Errorf("keine englische Fassung für %q", r.reason)
			}
		}
	}
}
