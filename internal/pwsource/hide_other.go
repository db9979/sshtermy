//go:build !windows

package pwsource

import "os/exec"

func hideWindow(*exec.Cmd) {}
