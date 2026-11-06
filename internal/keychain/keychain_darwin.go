package keychain

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// macOS: /usr/bin/security. Das Geheimnis geht nie über die Befehlszeile
// (dort sähe es jeder per ps), sondern hex-kodiert über stdin an
// „security -i“.

const securityBin = "/usr/bin/security"

type system struct{}

func (system) Available() bool {
	_, err := os.Stat(securityBin)
	return err == nil
}

func (system) Set(account, label, secret string) error {
	cmd := exec.Command(securityBin, "-i")
	line := fmt.Sprintf("add-generic-password -U -s %s -a %s -l %s -X %s\n",
		quote(Service), quote(account), quote(label), hex.EncodeToString([]byte(secret)))
	cmd.Stdin = strings.NewReader(line)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain: %s", strings.TrimSpace(out.String()))
	}
	if s := strings.TrimSpace(out.String()); strings.Contains(strings.ToLower(s), "error") {
		return fmt.Errorf("keychain: %s", s)
	}
	return nil
}

func (system) Get(account string) (string, error) {
	// -g schreibt das Passwort auf stderr: als "password: "…"" oder bei
	// nicht druckbaren Zeichen als "password: 0x… "…"".
	cmd := exec.Command(securityBin, "find-generic-password", "-s", Service, "-a", account, "-g")
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = nil, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 44 {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keychain: %s", strings.TrimSpace(stderr.String()))
	}
	return parseSecurityPassword(stderr.String())
}

func (system) Delete(account string) error {
	cmd := exec.Command(securityBin, "delete-generic-password", "-s", Service, "-a", account)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 44 {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("keychain: %s", strings.TrimSpace(out.String()))
	}
	return nil
}

// quote: Argument für den Befehlsinterpreter von security -i (wie eine
// Shell in einfachen Anführungszeichen). Anführungszeichen und Backslashes
// fliegen raus: Dienst und Konto enthalten keine, der Name ist nur Anzeige.
func quote(s string) string {
	return "'" + strings.NewReplacer("'", "", `"`, "", `\`, "", "\n", " ").Replace(s) + "'"
}
