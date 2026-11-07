package notify

import "os/exec"

func send(title, body string) error {
	script := "display notification " + quoteAS(body) + " with title " + quoteAS(title)
	return exec.Command("osascript", "-e", script).Run()
}
