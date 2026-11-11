package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/version"
)

// Sicherheitsnetz für Updates. Jeder Merge wird sofort ein Release; eine
// kaputte Programmdatei würde sonst jede Installation lahmlegen.
//
//  1. Selbsttest vor dem Tausch: die heruntergeladene Datei muss mit
//     "-version" starten und genau die angekündigte Version nennen. Das
//     bindet zugleich die Versionsnummer an die signierte Datei: ein altes,
//     gültig signiertes Release lässt sich nicht als neues ausgeben.
//  2. Rückfall nach dem Tausch: die alte Datei bleibt liegen, bis die neue
//     einmal sauber gelaufen ist (Confirm). Startet sie zweimal, ohne das zu
//     erreichen, stellt der dritte Start die alte wieder her (Guard) und
//     merkt sich die Version, damit sie nicht gleich wieder angeboten wird.

// maxUnconfirmed: so viele Starts ohne Confirm sind erlaubt.
const maxUnconfirmed = 2

type pendingInfo struct {
	Version string `json:"version"` // die neue, noch unbestätigte Version
	From    string `json:"from"`    // die Version davor
	Old     string `json:"old"`     // Pfad der alten Programmdatei
	Starts  int    `json:"starts"`  // Starts der neuen ohne Confirm
}

type skipInfo struct {
	Version string `json:"version"` // zurückgenommene Version
	Told    bool   `json:"told"`    // dem Nutzer schon gemeldet
}

func pendingPath(exe string) string { return exe + ".pending" }
func skipPath(exe string) string    { return exe + ".skip" }

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

func writeJSON(path string, v any) {
	if b, err := json.Marshal(v); err == nil {
		_ = os.WriteFile(path, b, 0o600)
	}
}

// newPath: Name der heruntergeladenen Datei neben der Programmdatei. Unter
// Windows muss sie auf .exe enden, sonst lässt sie sich nicht starten.
func newPath(exe string) string {
	if runtime.GOOS == "windows" {
		return exe + ".new.exe"
	}
	return exe + ".new"
}

// selfTest startet die neue Programmdatei mit -version. Variable für Tests.
var selfTest = func(path, want string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-version")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf(i18n.T("update: die neue Programmdatei startet nicht (%v), Update verworfen"), err)
	}
	if got := reportedVersion(string(out)); got != want {
		return fmt.Errorf(i18n.T("update: die neue Programmdatei meldet Version %s statt %s, Update verworfen"), got, want)
	}
	return nil
}

// reportedVersion liest "1.0.42" aus "sshTermy v1.0.42 (…)".
func reportedVersion(out string) string {
	for _, f := range strings.Fields(out) {
		if v := strings.TrimPrefix(f, "v"); v != f {
			if _, ok := parse(v); ok {
				return v
			}
		}
	}
	return "?"
}

// Guard gehört an den Anfang des Programmstarts (nach -version). Zählt die
// Starts einer frisch installierten Version und stellt die alte Datei
// wieder her, wenn die neue wiederholt nicht bis Confirm kommt. Ohne
// offenes Update räumt es nur auf.
func Guard() {
	exe, err := Executable()
	if err != nil {
		return
	}
	var p pendingInfo
	if !readJSON(pendingPath(exe), &p) {
		Cleanup()
		return
	}
	if p.Version != version.Version {
		// Es läuft etwas anderes als das unbestätigte Update (von Hand
		// ersetzt, Paketquelle): der Merker gilt nicht mehr.
		_ = os.Remove(pendingPath(exe))
		Cleanup()
		return
	}
	p.Starts++
	if p.Starts <= maxUnconfirmed {
		writeJSON(pendingPath(exe), p)
		return
	}
	if err := rollback(exe, p); err != nil {
		_ = os.Remove(pendingPath(exe)) // nicht bei jedem Start wieder versuchen
		return
	}
	fmt.Fprintf(os.Stderr, i18n.T("sshTermy v%s ist zweimal nicht sauber gestartet: zurück zu v%s.")+"\n", p.Version, p.From)
	_ = restart()
}

// restart: Variable für Tests.
var restart = Restart

func rollback(exe string, p pendingInfo) error {
	if p.Old == "" {
		return errors.New("keine alte Programmdatei")
	}
	if _, err := os.Stat(p.Old); err != nil {
		return err
	}
	bad := fmt.Sprintf("%s.%d.old", exe, time.Now().UnixNano()) // räumt Cleanup später weg
	if err := os.Rename(exe, bad); err != nil {
		return err
	}
	if err := os.Rename(p.Old, exe); err != nil {
		_ = os.Rename(bad, exe)
		return err
	}
	_ = os.Remove(pendingPath(exe))
	writeJSON(skipPath(exe), skipInfo{Version: p.Version})
	return nil
}

// Confirm: die laufende Version funktioniert (Oberfläche läuft seit einer
// Weile oder wurde sauber beendet). Die alte Programmdatei darf weg.
func Confirm() {
	exe, err := Executable()
	if err != nil {
		return
	}
	if _, err := os.Stat(pendingPath(exe)); err != nil {
		return
	}
	_ = os.Remove(pendingPath(exe))
	Cleanup()
}

// skipped: Version, die nach einem Rückfall nicht wieder angeboten wird.
func skipped() string {
	exe, err := Executable()
	if err != nil {
		return ""
	}
	var s skipInfo
	if !readJSON(skipPath(exe), &s) {
		return ""
	}
	return s.Version
}

// TakeRollbackNote: Wurde ein Update zurückgenommen und das noch nicht
// gemeldet, liefert es dessen Version (einmal).
func TakeRollbackNote() string {
	exe, err := Executable()
	if err != nil {
		return ""
	}
	var s skipInfo
	if !readJSON(skipPath(exe), &s) || s.Told {
		return ""
	}
	s.Told = true
	writeJSON(skipPath(exe), s)
	return s.Version
}
