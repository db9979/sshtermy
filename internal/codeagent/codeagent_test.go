package codeagent

import "testing"

func TestFromCommand(t *testing.T) {
	for cmd, want := range map[string]string{
		"claude":                           "Claude Code",
		"claude --resume":                  "Claude Code",
		"npx -y @anthropic-ai/claude-code": "Claude Code",
		"codex exec 'fix tests'":           "Codex",
		"npx @openai/codex":                "Codex",
		"FOO=1 gemini -p hallo":            "Gemini CLI",
		"/usr/local/bin/opencode":          "OpenCode",
		"sudo -E claude":                   "Claude Code",
		"cursor-agent":                     "Cursor Agent",
		"agent":                            "",
		"ls -l":                            "",
		"vim claude.md":                    "",
		"":                                 "",
	} {
		if got := FromCommand(cmd); got != want {
			t.Errorf("FromCommand(%q) = %q, will %q", cmd, got, want)
		}
	}
}

func TestDetectClaude(t *testing.T) {
	working := "● Ich lese die Dateien.\n\n✻ Thinking… (12s · ↑ 1.2k tokens · esc to interrupt)\n\n╭──────╮\n│ >    │\n╰──────╯\n  ? for shortcuts"
	if st, seen := Detect("Claude Code", working, "✳ Fix tests"); st != Working || !seen {
		t.Fatalf("arbeitet: %v %v", st, seen)
	}
	blocked := " Bash command\n\n   rm -rf build\n   Remove build output\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. Yes, and don't ask again\n   3. No, and tell Claude what to do differently (esc)"
	if st, _ := Detect("Claude Code", blocked, "Claude Code"); st != Blocked {
		t.Fatalf("Freigabe: %v", st)
	}
	idle := "● Fertig, alle Tests laufen.\n\n╭──────╮\n│ >    │\n╰──────╯\n  ? for shortcuts"
	if st, seen := Detect("Claude Code", idle, "Claude Code"); st != Idle || !seen {
		t.Fatalf("bereit: %v %v", st, seen)
	}
	// zurück an der Shell: nichts mehr zu sehen
	if _, seen := Detect("Claude Code", "user@web:~$ ", "user@web: ~"); seen {
		t.Fatal("Shell als Agent erkannt")
	}
}

func TestDetectCodexGemini(t *testing.T) {
	if st, _ := Detect("Codex", "• Working (5s • esc to interrupt)\n\n› \n  ⏎ send   ⌃J newline", "codex"); st != Working {
		t.Fatalf("Codex arbeitet: %v", st)
	}
	if st, _ := Detect("Codex", "Would you like to run the following command?\n\n  $ npm test\n\n› 1. Yes, proceed\n  2. No", ""); st != Blocked {
		t.Fatalf("Codex Freigabe: %v", st)
	}
	if st, _ := Detect("Gemini CLI", "⠏ Reading files (esc to cancel, 3s)", ""); st != Working {
		t.Fatalf("Gemini arbeitet: %v", st)
	}
	if st, _ := Detect("Gemini CLI", "Allow execution of: 'ls'?\n● 1. Yes, allow once", ""); st != Blocked {
		t.Fatalf("Gemini Freigabe: %v", st)
	}
}

func TestFromScreen(t *testing.T) {
	if FromScreen("✻ Welcome to Claude Code!", "") != "Claude Code" {
		t.Fatal("Claude-Banner")
	}
	if FromScreen("$ ls\nfoo bar", "bash") != "" {
		t.Fatal("Shell als Agent")
	}
}
