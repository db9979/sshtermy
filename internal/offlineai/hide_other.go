//go:build !windows

package offlineai

import "os/exec"

func hideWindow(*exec.Cmd) {}
