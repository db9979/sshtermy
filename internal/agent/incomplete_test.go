package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestIncomplete(t *testing.T) {
	complete := []string{
		"ls -la",
		"echo 'a b' \"c d\"",
		"echo \"it's\"",
		"echo $'it\\'s'",
		"grep -E 'a|b' file | wc -l",
		"cd /etc && ls",
		"sleep 10 &",
		"echo $(date +%F) ${HOME} $((1+2))",
		"echo \"$(echo \"inner\")\"",
		"for f in a b; do echo $f; done",
		"if [ -f x ]; then echo ja; else echo nein; fi",
		"case $x in a) echo a;; *) echo b;; esac",
		"{ echo a; echo b; } > out",
		`find . -name '*.go' -exec grep -l foo {} \;`,
		"echo {a,b}.txt",
		"cat <<< 'hallo'",
		"echo a # kommentar mit ' und |",
		"echo done fi esac",
		"awk '{print $1}' f",
		"diff <(ls a) <(ls b)",
		"echo `date`",
		"cmd 2>&1 | tee log",
		"cat > /tmp/x <<'EOF'\nzeile\nEOF",
		"cat <<-EOF\n\tzeile\n\tEOF",
		"echo a\\\nb",
		"echo ${x:-\"a b\"}",
		"echo $#",
	}
	for _, c := range complete {
		if r := Incomplete(c); r != "" {
			t.Errorf("Incomplete(%q) = %q, erwartet vollständig", c, r)
		}
	}
	incomplete := []string{
		"echo 'hallo",
		"echo \"hallo",
		"echo \"$(date\"",
		"cat > /etc/x.conf << 'EOF'",
		"cat <<EOF\nzeile",
		"ls |",
		"ls | ",
		"cd /tmp &&",
		"false ||",
		"echo a \\",
		"echo $(date",
		"echo ${HOME",
		"( cd /tmp; ls",
		"{ echo a; echo b;",
		"if true; then echo a",
		"for i in 1 2; do echo $i",
		"while true; do sleep 1; done; if x; then y",
		"case $x in a) echo",
		"echo `date",
		"sed -i 's/a/b/ f",
	}
	for _, c := range incomplete {
		if r := Incomplete(c); r == "" {
			t.Errorf("Incomplete(%q) = vollständig, erwartet Grund", c)
		}
	}
}

// Ein Ziel-/Agent-Schritt, der die Shell am „>“ ließe, geht mit gezielter
// Nachfrage zurück ans Modell; auf Switches gelten andere Regeln.
func TestParseStepIncomplete(t *testing.T) {
	out := `{"befehl": "echo 'listen 80; > /etc/nginx/conf.d/a.conf", "grund": "Port setzen"}`
	_, err := parseStep(out, "unix")
	var se *StepError
	if !errors.As(err, &se) || !strings.Contains(se.fix, "Fortsetzungs-Prompt") {
		t.Fatalf("unvollständiger Befehl nicht abgewiesen: %v", err)
	}
	if fix := retryFor("x", err, false); !strings.Contains(fix, "offenes Anführungszeichen '") {
		t.Fatalf("Nachfrage nennt den Grund nicht: %q", fix)
	}
	if _, err := parseStep(`{"befehl": "description Dominik's port", "grund": "x"}`, "network"); err != nil {
		t.Fatalf("Switch-Befehl abgewiesen: %v", err)
	}
}
