// Package importer liest bestehende Verbindungen aus anderen Programmen:
// OpenSSH (~/.ssh/config), PuTTY (Registry oder .reg-Export) und MobaXterm
// (MobaXterm.ini oder .mxtsessions-Export). Passwörter werden nie
// übernommen (PuTTY und MobaXterm speichern sie verschlüsselt bzw. gar
// nicht); sshTermy fragt beim ersten Verbinden danach.
package importer

import (
	"bytes"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
)

// Source: woher ein Eintrag stammt.
type Source string

const (
	SrcSSHConfig Source = "ssh-config"
	SrcPuTTY     Source = "putty"
	SrcMoba      Source = "mobaxterm"
)

// Label: Anzeigename der Quelle.
func (s Source) Label() string {
	switch s {
	case SrcSSHConfig:
		return "~/.ssh/config"
	case SrcPuTTY:
		return "PuTTY"
	case SrcMoba:
		return "MobaXterm"
	}
	return string(s)
}

// Entry ist eine gefundene Verbindung. Jump ist der Alias bzw. die Adresse
// des Sprung-Hosts (ProxyJump), aufgelöst wird er erst beim Übernehmen.
type Entry struct {
	Conn   domain.Connection
	Source Source
	Jump   string
	Notes  []string // Hinweise, z. B. „PPK-Key: in Pageant laden“
}

// Result: Funde einer Quelle.
type Result struct {
	Source  Source
	Path    string // Datei oder Registry-Pfad
	Entries []Entry
	Err     error // nil, wenn gelesen (auch wenn leer)
}

// ErrNotFound: die Quelle gibt es auf diesem Rechner nicht.
var ErrNotFound = i18n.NewError("nicht gefunden")

// Scan durchsucht alle bekannten Orte dieses Rechners.
func Scan() []Result {
	var out []Result
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".ssh", "config")
	out = append(out, fromFile(SrcSSHConfig, p))
	out = append(out, scanPuTTY())
	out = append(out, scanMoba())
	return out
}

// File liest eine vom Nutzer angegebene Datei und erkennt das Format.
func File(path string) Result {
	path = expandHome(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return Result{Path: path, Err: err}
	}
	txt := decodeText(b)
	switch {
	case strings.Contains(txt, `\SimonTatham\PuTTY\Sessions\`):
		e, err := ParsePuTTYReg(txt)
		return Result{Source: SrcPuTTY, Path: path, Entries: e, Err: err}
	case strings.Contains(strings.ToLower(txt), "[bookmarks") || strings.HasSuffix(strings.ToLower(path), ".mxtsessions"):
		e, err := ParseMoba(txt)
		return Result{Source: SrcMoba, Path: path, Entries: e, Err: err}
	}
	return fromFile(SrcSSHConfig, path)
}

func fromFile(src Source, path string) Result {
	r := Result{Source: src, Path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		r.Err = ErrNotFound
		return r
	} else if err != nil {
		r.Err = err
		return r
	}
	txt := decodeText(b)
	switch src {
	case SrcSSHConfig:
		r.Entries, r.Err = ParseSSHConfig(txt, path)
	case SrcPuTTY:
		r.Entries, r.Err = ParsePuTTYReg(txt)
	case SrcMoba:
		r.Entries, r.Err = ParseMoba(txt)
	}
	return r
}

// scanMoba: installierte Version (%APPDATA%) und übliche Orte der
// portablen Version.
func scanMoba() Result {
	var cand []string
	if ad := os.Getenv("APPDATA"); ad != "" {
		cand = append(cand, filepath.Join(ad, "MobaXterm", "MobaXterm.ini"))
	}
	home, _ := os.UserHomeDir()
	cand = append(cand,
		filepath.Join(home, "Documents", "MobaXterm", "MobaXterm.ini"),
		filepath.Join(home, "Documents", "MobaXterm", "MobaXterm.mxtsessions"),
		filepath.Join(home, "Desktop", "MobaXterm.ini"),
		filepath.Join(home, "Downloads", "MobaXterm.ini"))
	for _, p := range cand {
		if _, err := os.Stat(p); err == nil {
			return fromFile(SrcMoba, p)
		}
	}
	return Result{Source: SrcMoba, Path: cand[0], Err: ErrNotFound}
}

// decodeText: .reg-Exporte sind UTF-16 (mit BOM), MobaXterm schreibt
// Windows-1252. Beides wird zu UTF-8.
func decodeText(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xff, 0xfe}):
		return utf16le(b[2:])
	case bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}):
		b = b[3:]
	}
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c) // Latin-1 ≈ Windows-1252 für Umlaute
	}
	return string(r)
}

func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

// localUser: wie OpenSSH: ohne User gilt der lokale Anmeldename.
func localUser() string {
	if u, err := user.Current(); err == nil {
		n := u.Username
		if runtime.GOOS == "windows" {
			if i := strings.LastIndex(n, `\`); i >= 0 {
				n = n[i+1:]
			}
		}
		return n
	}
	if n := os.Getenv("USER"); n != "" {
		return n
	}
	return os.Getenv("USERNAME")
}

// fillUser setzt einen fehlenden Benutzer und vermerkt das.
func fillUser(e *Entry) {
	if e.Conn.Username == "" {
		e.Conn.Username = localUser()
		e.Notes = append(e.Notes, i18n.T("kein Benutzer angegeben, lokaler Name „")+e.Conn.Username+i18n.T("“ eingesetzt"))
	}
}

// keyAuth: Schlüsseldatei übernehmen. PuTTY-Keys (.ppk) liest sshTermy
// direkt (sshx/ppk.go), ohne Pageant.
func keyAuth(e *Entry, path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	e.Conn.Auth, e.Conn.KeyPath = domain.AuthKeyFile, path
}

// Dedupe markiert Funde, die es schon gibt (gleiches Ziel oder gleicher
// Alias), und doppelte Funde aus mehreren Quellen.
func IsKnown(c domain.Connection, existing []domain.Connection) bool {
	for _, o := range existing {
		if strings.EqualFold(o.Alias, c.Alias) {
			return true
		}
		if strings.EqualFold(o.Host, c.Host) && port(o.Port) == port(c.Port) && o.Username == c.Username {
			return true
		}
	}
	return false
}

func port(p int) int {
	if p == 0 {
		return 22
	}
	return p
}
