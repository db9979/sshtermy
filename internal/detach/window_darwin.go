//go:build darwin

package detach

import (
	"os"
	"os/exec"
	"strings"
)

// candidates: tmux, dann das Terminal, in dem sshTermy gerade läuft.
// Terminal.app und iTerm2 gehen über AppleScript (openFallback).
func candidates(env func(string) string) []launcher {
	var c []launcher
	if env("TMUX") != "" {
		c = append(c, launcher{"tmux", []string{"new-window", "-n", "{title}", "{shcmd}"}})
	}
	switch env("TERM_PROGRAM") {
	case "WezTerm":
		c = append(c, launcher{"wezterm", []string{"start", "--", "{cmd}"}})
	case "ghostty":
		c = append(c, launcher{"open", []string{"-na", "Ghostty", "--args", "--title={title}", "-e", "{cmd}"}})
	}
	if env("KITTY_WINDOW_ID") != "" {
		c = append(c, launcher{"kitty", []string{"--title", "{title}", "{cmd}"}})
	}
	if env("ALACRITTY_WINDOW_ID") != "" || env("ALACRITTY_SOCKET") != "" {
		c = append(c, launcher{"alacritty", []string{"--title", "{title}", "-e", "{cmd}"}})
	}
	return c
}

func command(path string, args []string) *exec.Cmd { return exec.Command(path, args...) }

func openFallback(title string, argv []string) error {
	cmd := "exec " + shellJoin(argv)
	var script string
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		script = `tell application "iTerm"
	activate
	create window with default profile command "` + asQuote(cmd) + `"
end tell`
	} else {
		script = `tell application "Terminal"
	activate
	set t to do script "` + asQuote("printf '\\033]0;%s\\007' "+shellJoin([]string{title})+"; "+cmd) + `"
end tell`
	}
	c := exec.Command("osascript", "-e", script)
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() //nolint:errcheck
	return nil
}

// asQuote: Text für eine AppleScript-Zeichenkette.
func asQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
