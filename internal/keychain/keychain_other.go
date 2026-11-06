//go:build !darwin && !windows

package keychain

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Linux & Co.: Secret Service (GNOME Schlüsselbund, KWallet) über das
// Programm secret-tool (Paket libsecret-tools). Das Geheimnis geht über stdin.

type system struct{}

func (system) Available() bool {
	_, err := exec.LookPath("secret-tool")
	return err == nil
}

func (system) Set(account, label, secret string) error {
	cmd := exec.Command("secret-tool", "store", "--label="+label, "service", Service, "account", account)
	cmd.Stdin = strings.NewReader(secret)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain: secret-tool: %s", strings.TrimSpace(out.String()+" "+err.Error()))
	}
	return nil
}

func (system) Get(account string) (string, error) {
	cmd := exec.Command("secret-tool", "lookup", "service", Service, "account", account)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && strings.TrimSpace(errb.String()) == "" {
		return "", ErrNotFound // secret-tool: Exit 1 ohne Meldung = kein Eintrag
	}
	if err != nil {
		return "", fmt.Errorf("keychain: secret-tool: %s", strings.TrimSpace(errb.String()+" "+err.Error()))
	}
	return out.String(), nil
}

func (system) Delete(account string) error {
	cmd := exec.Command("secret-tool", "clear", "service", Service, "account", account)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain: secret-tool: %s", strings.TrimSpace(out.String()+" "+err.Error()))
	}
	return nil
}
