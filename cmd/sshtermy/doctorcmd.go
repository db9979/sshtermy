package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/db9979/sshtermy/internal/doctor"
	"github.com/db9979/sshtermy/internal/i18n"
)

// runDoctor: „sshtermy doctor“ prüft die Umgebung ohne Oberfläche und ohne
// Master-Passwort. 0 = in Ordnung (auch mit Hinweisen), 1 = Probleme.
func runDoctor(dir string) int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fmt.Println(i18n.T("sshTermy-Selbstprüfung …"))
	rs := doctor.Run(ctx, doctor.Env{Dir: dir, Extra: []doctor.Check{checkTerminal}})
	fmt.Print("\n" + doctor.Format(rs))
	if doctor.Worst(rs) == doctor.Fail {
		return 1
	}
	return 0
}

// checkTerminal: Farben und Größe des Terminals, in dem sshtermy läuft.
func checkTerminal(context.Context) doctor.Result {
	name := i18n.T("Terminal")
	t, ct := os.Getenv("TERM"), os.Getenv("COLORTERM")
	if !term.IsTerminal(os.Stdout.Fd()) {
		return doctor.Result{Name: name, Status: doctor.Info, Detail: i18n.T("Ausgabe ist kein Terminal (umgeleitet)")}
	}
	w, h, _ := term.GetSize(os.Stdout.Fd())
	d := fmt.Sprintf("%dx%d, TERM=%s", w, h, t)
	if ct != "" {
		d += ", COLORTERM=" + ct
	}
	if w > 0 && (w < 80 || h < 20) {
		return doctor.Result{Name: name, Status: doctor.Warn, Detail: d + i18n.T(" (kleiner als 80×20: manches wird gekürzt)")}
	}
	return doctor.Result{Name: name, Status: doctor.OK, Detail: d}
}
