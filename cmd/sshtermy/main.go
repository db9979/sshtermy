// sshTermy: SSH-Terminal mit KI-Copilot für Windows (ARM64/x86), macOS und Linux.
// Idee: Dominik Bornhäußer.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/ui"
	"github.com/db9979/sshtermy/internal/update"
	"github.com/db9979/sshtermy/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "Version anzeigen")
	doUpdate := flag.Bool("update", false, "auf die neueste Version aus GitHub aktualisieren und beenden")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Long())
		return
	}
	update.Cleanup()
	if *doUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		rel, err := update.Check(ctx)
		switch {
		case err != nil:
			fail(err)
		case rel == nil:
			fmt.Println(version.Short(), "ist aktuell.")
		default:
			fmt.Println("Aktualisiere auf v" + rel.Version + " …")
			if err := update.Apply(ctx, rel); err != nil {
				fail(err)
			}
			fmt.Println("Fertig. Bitte sshTermy neu starten.")
		}
		return
	}

	st, err := store.Open()
	if err != nil {
		fail(err)
	}
	app, err := ui.New(st)
	if err != nil {
		fail(err)
	}
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fail(err)
	}
	if app.Restart() {
		// Update installiert: Terminal ist wiederhergestellt, Sitzungen sind
		// zu, Store ist gesperrt. Jetzt die neue Programmdatei starten.
		fmt.Println("Starte " + version.Name + " neu …")
		if err := update.Restart(); err != nil {
			fail(fmt.Errorf("Neustart fehlgeschlagen, bitte von Hand starten: %w", err))
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, version.Name+":", err)
	os.Exit(1)
}
