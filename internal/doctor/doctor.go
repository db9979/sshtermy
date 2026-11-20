// Package doctor ist die Selbstprüfung (sshtermy doctor, Palette › Hilfe ›
// Fehlersuche › Selbstprüfung): Datenordner und Rechte, Tresor,
// Schlüsselbund, SSH-Agent, Update-Signatur, Internet und Uhrzeit,
// Absturzberichte. Das Ergebnis ist reiner Text ohne Geheimnisse und Hosts,
// damit man es in einen Fehlerbericht kopieren kann.
package doctor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/keychain"
	"github.com/db9979/sshtermy/internal/sshx"
	"github.com/db9979/sshtermy/internal/update"
	"github.com/db9979/sshtermy/internal/vault"
	"github.com/db9979/sshtermy/internal/version"
)

// Status einer Prüfung.
type Status int

const (
	OK   Status = iota
	Info        // nur zur Kenntnis (z. B. Version)
	Warn        // geht, aber etwas fehlt
	Fail        // so funktioniert etwas nicht
)

// Mark: Zeichen vor der Zeile (nur ASCII/Cascadia-sicher).
func (s Status) Mark() string {
	switch s {
	case OK:
		return "✓"
	case Warn:
		return "!"
	case Fail:
		return "✗"
	}
	return "·"
}

// Result ist eine Zeile der Selbstprüfung.
type Result struct {
	Name   string
	Status Status
	Detail string
}

// Check ist eine einzelne Prüfung.
type Check func(ctx context.Context) Result

// Env: Umgebung der Prüfungen. Leere Felder = Standard.
type Env struct {
	Dir   string       // Datenordner
	HTTP  *http.Client // für Internet/Uhrzeit (Tests setzen einen eigenen)
	Probe string       // Adresse für Internet/Uhrzeit (Standard: GitHub-API)
	Now   func() time.Time
	Extra []Check // z. B. KI-Endpunkt (Oberfläche) oder Terminal (Kommandozeile)
}

// Run führt alle Prüfungen aus (Reihenfolge fest, jede mit eigener Frist).
func Run(ctx context.Context, env Env) []Result {
	if env.HTTP == nil {
		env.HTTP = &http.Client{Timeout: 8 * time.Second}
	}
	if env.Probe == "" {
		env.Probe = "https://api.github.com"
	}
	if env.Now == nil {
		env.Now = time.Now
	}
	checks := []Check{
		func(context.Context) Result { return checkVersion() },
		func(context.Context) Result { return checkDir(env.Dir) },
		func(context.Context) Result { return checkVault(env.Dir) },
		func(context.Context) Result { return checkKeychain() },
		func(context.Context) Result { return checkAgent() },
		func(context.Context) Result { return checkSigned() },
		func(ctx context.Context) Result { return checkNet(ctx, env) },
		func(context.Context) Result { return checkCrashes(env.Dir) },
	}
	checks = append(checks, env.Extra...)
	out := make([]Result, 0, len(checks))
	for _, c := range checks {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		out = append(out, c(cctx))
		cancel()
	}
	return out
}

// Worst: schlechtester Status (für den Rückgabewert der Kommandozeile).
func Worst(rs []Result) Status {
	w := OK
	for _, r := range rs {
		if r.Status != Info && r.Status > w {
			w = r.Status
		}
	}
	return w
}

// Format: Text zum Anzeigen und Kopieren.
func Format(rs []Result) string {
	n := 0
	for _, r := range rs {
		n = max(n, len([]rune(r.Name)))
	}
	var b strings.Builder
	for _, r := range rs {
		pad := strings.Repeat(" ", n-len([]rune(r.Name)))
		fmt.Fprintf(&b, "%s %s%s  %s\n", r.Status.Mark(), r.Name, pad, r.Detail)
	}
	switch Worst(rs) {
	case OK:
		b.WriteString("\n" + i18n.T("Alles in Ordnung.") + "\n")
	case Warn:
		b.WriteString("\n" + i18n.T("Läuft, aber nicht alles ist eingerichtet (Zeilen mit !).") + "\n")
	default:
		b.WriteString("\n" + i18n.T("Es gibt Probleme (Zeilen mit ✗). Text kopieren und dem Fehlerbericht beilegen.") + "\n")
	}
	return b.String()
}

func checkVersion() Result {
	return Result{i18n.T("Version"), Info, fmt.Sprintf("%s · %s/%s · %s", version.Short(), runtime.GOOS, runtime.GOARCH, runtime.Version())}
}

func checkDir(dir string) Result {
	name := i18n.T("Datenordner")
	if dir == "" {
		return Result{name, Fail, i18n.T("unbekannt (kein Benutzerordner?)")}
	}
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return Result{name, Info, i18n.T("noch nicht angelegt (erster Start)")}
	}
	if err != nil {
		return Result{name, Fail, err.Error()}
	}
	if !fi.IsDir() {
		return Result{name, Fail, i18n.T("ist eine Datei, kein Ordner")}
	}
	probe := filepath.Join(dir, ".doctor-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return Result{name, Fail, i18n.T("nicht beschreibbar: ") + err.Error()}
	}
	_ = os.Remove(probe)
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		return Result{name, Warn, fmt.Sprintf(i18n.T("für andere lesbar (%o), besser: chmod 700"), fi.Mode().Perm())}
	}
	return Result{name, OK, i18n.T("beschreibbar")}
}

func checkVault(dir string) Result {
	name := i18n.T("Tresor")
	p := filepath.Join(dir, "store.vault")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return Result{name, Info, i18n.T("noch nicht angelegt (erster Start)")}
	}
	v, err := vault.Open(p)
	if err != nil {
		return Result{name, Fail, err.Error()}
	}
	d := i18n.T("lesbar, Master-Passwort")
	if v.HasDPAPI() {
		d += i18n.T(" + Windows-Konto")
	}
	return Result{name, OK, d}
}

func checkKeychain() Result {
	name := i18n.T("Schlüsselbund")
	if keychain.Available() {
		return Result{name, OK, i18n.T("verfügbar (Passwörter können dort liegen)")}
	}
	return Result{name, Warn, i18n.T("nicht verfügbar: Passwörter liegen nur im Tresor")}
}

func checkAgent() Result {
	name := i18n.T("SSH-Agent")
	if err := sshx.AgentReachable(); err != nil {
		return Result{name, Info, i18n.T("nicht erreichbar (nur nötig für Schlüssel im Agenten): ") + err.Error()}
	}
	return Result{name, OK, i18n.T("erreichbar")}
}

func checkSigned() Result {
	name := i18n.T("Update-Signatur")
	if update.Signed() {
		return Result{name, OK, i18n.T("Updates werden vor dem Einspielen geprüft")}
	}
	return Result{name, Warn, i18n.T("eigener Build ohne Schlüssel: Updates werden nicht geprüft")}
}

// checkNet: Internet (für Updates) und Uhrzeit (falsche Uhr ⇒ Zertifikate,
// Einmal-Codes und SSH-Zertifikate scheitern scheinbar grundlos).
func checkNet(ctx context.Context, env Env) Result {
	name := i18n.T("Internet und Uhr")
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, env.Probe, nil)
	if err != nil {
		return Result{name, Warn, err.Error()}
	}
	req.Header.Set("User-Agent", version.Short())
	resp, err := env.HTTP.Do(req)
	if err != nil {
		return Result{name, Warn, i18n.T("GitHub nicht erreichbar, Updates gehen nicht: ") + err.Error()}
	}
	resp.Body.Close()
	srv, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return Result{name, OK, i18n.T("GitHub erreichbar")}
	}
	diff := env.Now().Sub(srv)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Minute {
		return Result{name, Warn, fmt.Sprintf(i18n.T("Uhr geht %s falsch: Anmeldung mit Einmal-Codes oder Zertifikaten kann scheitern"), diff.Round(time.Second))}
	}
	return Result{name, OK, i18n.T("GitHub erreichbar, Uhr stimmt")}
}

func checkCrashes(dir string) Result {
	name := i18n.T("Absturzberichte")
	es, _ := os.ReadDir(crash.Dir(dir))
	n := 0
	for _, e := range es {
		if strings.HasPrefix(e.Name(), "sshtermy-crash-") {
			n++
		}
	}
	if n == 0 {
		return Result{name, OK, i18n.T("keine")}
	}
	return Result{name, Info, fmt.Sprintf(i18n.T("%d im Ordner %s"), n, tilde(crash.Dir(dir)))}
}

// tilde: Home-Ordner als ~ (der Text landet ggf. in einem Fehlerbericht).
func tilde(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" && strings.HasPrefix(p, h) {
		return "~" + p[len(h):]
	}
	return p
}
