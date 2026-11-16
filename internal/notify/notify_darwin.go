package notify

import "os/exec"

// macOS: osascript kann keinen Klick zurückmelden (click bleibt ungenutzt).
func send(title, body, _ string, _ func()) error {
	script := "display notification " + quoteAS(body) + " with title " + quoteAS(title)
	return exec.Command("osascript", "-e", script).Run()
}
