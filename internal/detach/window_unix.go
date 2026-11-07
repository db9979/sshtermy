//go:build !windows && !darwin

package detach

import "os/exec"

// candidates: tmux zuerst (dann wird es ein tmux-Fenster), sonst das erste
// grafische Terminal, das sich findet.
func candidates(env func(string) string) []launcher {
	var c []launcher
	if env("TMUX") != "" {
		c = append(c, launcher{"tmux", []string{"new-window", "-n", "{title}", "{shcmd}"}})
	}
	if env("DISPLAY") == "" && env("WAYLAND_DISPLAY") == "" {
		return c
	}
	if t := env("TERMINAL"); t != "" {
		c = append(c, launcher{t, []string{"-e", "{cmd}"}})
	}
	return append(c,
		launcher{"x-terminal-emulator", []string{"-T", "{title}", "-e", "{cmd}"}},
		launcher{"gnome-terminal", []string{"--title={title}", "--", "{cmd}"}},
		launcher{"konsole", []string{"-e", "{cmd}"}},
		launcher{"xfce4-terminal", []string{"--title={title}", "-x", "{cmd}"}},
		launcher{"kitty", []string{"--title", "{title}", "{cmd}"}},
		launcher{"alacritty", []string{"--title", "{title}", "-e", "{cmd}"}},
		launcher{"wezterm", []string{"start", "--", "{cmd}"}},
		launcher{"ghostty", []string{"--title={title}", "-e", "{cmd}"}},
		launcher{"foot", []string{"-T", "{title}", "{cmd}"}},
		launcher{"tilix", []string{"-t", "{title}", "-e", "{shcmd}"}},
		launcher{"xterm", []string{"-T", "{title}", "-e", "{cmd}"}},
	)
}

func command(path string, args []string) *exec.Cmd { return exec.Command(path, args...) }

func openFallback(string, []string) error { return errNoFallback }
