//go:build !windows

package wol

import "os/exec"

func hideWindow(*exec.Cmd) {}
