// Package pwsource holt Passwörter und 2FA-Codes aus einem Passwortmanager,
// statt sie in sshTermy zu speichern. Eine Quelle ist eine Zeile:
//
//	op://Tresor/Eintrag/password   1Password CLI (op read)
//	bw:Eintrag                     Bitwarden CLI (bw get password|totp)
//	keepassxc:/pfad/db.kdbx:Eintrag KeePassXC CLI (fragt das Datenbank-Passwort)
//	pass:pfad/eintrag              pass / gopass (erste Zeile)
//	cmd:befehl …                   beliebiger Befehl, erste Zeile der Ausgabe
//
// Für 2FA-Codes (keyboard-interactive „Verification code“) gilt dieselbe
// Schreibweise; op und bw liefern dann den TOTP-Code des Eintrags,
// keepassxc den TOTP-Wert (-t), pass versucht „pass otp“.
package pwsource

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Kind: was gebraucht wird.
type Kind int

const (
	Password Kind = iota
	OTP
)

// Ask fragt ein Geheimnis interaktiv ab (KeePassXC-Datenbankpasswort).
type Ask func(label string) (string, error)

// Timeout: so lange darf der Passwortmanager brauchen (bw/op können beim
// ersten Mal nach dem Entsperren fragen, das geschieht in deren eigenem Fenster).
var Timeout = 60 * time.Second

// run ist austauschbar für Tests.
var run = func(ctx context.Context, stdin string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin + "\n")
	}
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf(i18n.T("%s ist nicht installiert oder nicht im PATH"), name)
		}
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return "", fmt.Errorf("%s: %s", name, msg)
	}
	return out.String(), nil
}

// Valid prüft die Schreibweise (für das Formular).
func Valid(spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	_, _, err := plan(spec, Password)
	return err
}

// Label: kurze Beschreibung für Hinweise („1Password“, „Bitwarden“ …).
func Label(spec string) string {
	switch {
	case strings.HasPrefix(spec, "op://"):
		return "1Password"
	case strings.HasPrefix(spec, "bw:"):
		return "Bitwarden"
	case strings.HasPrefix(spec, "keepassxc:"):
		return "KeePassXC"
	case strings.HasPrefix(spec, "pass:"):
		return "pass"
	case strings.HasPrefix(spec, "cmd:"):
		return i18n.T("Befehl")
	}
	return ""
}

type step struct {
	name    string
	args    []string
	askDB   string // KeePassXC: Datenbankpasswort abfragen
	firstLn bool
}

func plan(spec string, k Kind) (step, string, error) {
	switch {
	case strings.HasPrefix(spec, "op://"):
		ref := spec
		if k == OTP {
			// op://Tresor/Eintrag/password → op://Tresor/Eintrag/one-time password?attribute=otp
			parts := strings.Split(strings.TrimPrefix(spec, "op://"), "/")
			if len(parts) >= 2 {
				ref = "op://" + parts[0] + "/" + parts[1] + "/one-time password?attribute=otp"
			}
		}
		return step{name: "op", args: []string{"read", "--no-newline", ref}}, ref, nil
	case strings.HasPrefix(spec, "bw:"):
		item := strings.TrimSpace(strings.TrimPrefix(spec, "bw:"))
		if item == "" {
			break
		}
		what := "password"
		if k == OTP {
			what = "totp"
		}
		return step{name: "bw", args: []string{"get", what, item}}, item, nil
	case strings.HasPrefix(spec, "keepassxc:"):
		rest := strings.TrimPrefix(spec, "keepassxc:")
		i := strings.LastIndex(rest, ":")
		// Windows-Laufwerk „C:\…“ hat selbst einen Doppelpunkt
		if i <= 1 {
			break
		}
		db, entry := rest[:i], rest[i+1:]
		if strings.TrimSpace(db) == "" || strings.TrimSpace(entry) == "" {
			break
		}
		args := []string{"show", "-q", "-a", "Password", db, entry}
		if k == OTP {
			args = []string{"show", "-q", "-t", db, entry}
		}
		return step{name: "keepassxc-cli", args: args, askDB: db, firstLn: true}, entry, nil
	case strings.HasPrefix(spec, "pass:"):
		name := strings.TrimSpace(strings.TrimPrefix(spec, "pass:"))
		if name == "" {
			break
		}
		if k == OTP {
			return step{name: "pass", args: []string{"otp", name}, firstLn: true}, name, nil
		}
		return step{name: "pass", args: []string{"show", name}, firstLn: true}, name, nil
	case strings.HasPrefix(spec, "cmd:"):
		line := strings.TrimSpace(strings.TrimPrefix(spec, "cmd:"))
		args := splitArgs(line)
		if len(args) == 0 {
			break
		}
		return step{name: args[0], args: args[1:], firstLn: true}, line, nil
	}
	return step{}, "", errors.New(i18n.T("Quelle unbekannt: op://…, bw:Eintrag, keepassxc:Datei:Eintrag, pass:Name oder cmd:Befehl"))
}

// Get holt das Geheimnis.
func Get(ctx context.Context, spec string, k Kind, ask Ask) (string, error) {
	spec = strings.TrimSpace(spec)
	st, _, err := plan(spec, k)
	if err != nil {
		return "", err
	}
	stdin := ""
	if st.askDB != "" {
		if ask == nil {
			return "", errors.New(i18n.T("KeePassXC braucht das Datenbank-Passwort"))
		}
		if stdin, err = ask(i18n.T("KeePassXC-Datenbank ") + st.askDB); err != nil {
			return "", err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	out, err := run(ctx, stdin, st.name, st.args...)
	if err != nil {
		return "", err
	}
	if st.firstLn {
		out, _, _ = strings.Cut(out, "\n")
	}
	out = strings.TrimRight(out, "\r\n")
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf(i18n.T("%s lieferte nichts"), Label(spec))
	}
	return out, nil
}

// IsOTPQuestion: fragt der Server nach einem Einmalcode?
func IsOTPQuestion(q string) bool {
	q = strings.ToLower(q)
	for _, w := range []string{"verification code", "one-time", "one time", "otp", "token", "2fa", "two-factor", "authenticator", "code:", "bestätigungscode", "einmal"} {
		if strings.Contains(q, w) {
			return true
		}
	}
	return false
}

func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	quote, have := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quote, have = !quote, true
		case (r == ' ' || r == '\t') && !quote:
			if have {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if have {
		out = append(out, cur.String())
	}
	return out
}
