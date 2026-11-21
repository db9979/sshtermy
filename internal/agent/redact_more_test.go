package agent

import (
	"strings"
	"testing"
)

// Geheimnisse, die der Filter früher durchließ (Code-Review 2026-11-21).
func TestRedactMore(t *testing.T) {
	for _, s := range []string{
		"MYSQL_ROOT_PASSWORD=hunter2", "DB_PASS=hunter2", "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG",
		"mysql -u root -phunter2 db", "mysqldump -phunter2 db", "curl -u admin:hunter2 http://x",
		"username admin password 0 hunter2", "enable secret 5 $1$abcd$efghijklmnop", "snmp-server community hunter2 RO",
		"password manager user-name admin plaintext hunter2", " password 7 hunter2",
		"root:$6$abc$Xyzhashhashhash/hash.:19000:0:99999:7:::", "u:$y$j9T$abcdefghijkl$mnopqrstuvwxyz0123:1::",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
	} {
		if got := Redact(s); strings.Contains(got, "hunter2") || strings.Contains(got, "Xyzhash") || strings.Contains(got, "mnopq") ||
			strings.Contains(got, "efghij") || strings.Contains(got, "wJalr") || strings.Contains(got, "abcdefghijklmnop") {
			t.Errorf("%q → %q", s, got)
		}
	}
	// Alltagstext bleibt
	for _, s := range []string{"Tests PASSED: 5", "ls -la /etc", "echo $1 $2", "tokens: 12", "mysql -u root -p", "password changed for root", "show running-config"} {
		if got := Redact(s); got != s {
			t.Errorf("%q verändert: %q", s, got)
		}
	}
}
