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

	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/ui"
	"github.com/db9979/sshtermy/internal/update"
	"github.com/db9979/sshtermy/internal/version"
)

func main() {
	// Sprache vor allem anderen: auch Hilfe zu den Schaltern und der
	// Entsperr-Bildschirm sollen sie schon haben.
	dir, _ := store.Dir()
	i18n.Load(dir)

	// Unterbefehle ohne Oberfläche (eigener Vermittler, Anker auf Servern)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "vermittler", "relay":
			os.Exit(runRelay(os.Args[2:]))
		case "anker", "anchor":
			os.Exit(runAnchor(os.Args[2:]))
		case "attach":
			// neues Fenster eines abgetrennten Tabs (⇧F10), siehe internal/detach
			if len(os.Args) != 3 {
				fail(fmt.Errorf("attach <ordner>"))
			}
			os.Exit(ui.Attach(os.Args[2]))
		}
	}

	showVersion := flag.Bool("version", false, i18n.T("Version anzeigen"))
	noIntro := flag.Bool("no-intro", false, i18n.T("ohne Startanimation starten (dauerhaft: Palette „Startanimation aus“)"))
	doUpdate := flag.Bool("update", false, i18n.T("auf die neueste Version aus GitHub aktualisieren und beenden"))
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
			fmt.Println(version.Short(), i18n.T("ist aktuell."))
		default:
			fmt.Println(i18n.T("Aktualisiere auf v") + rel.Version + " …")
			if err := update.Apply(ctx, rel); err != nil {
				fail(err)
			}
			fmt.Println(i18n.T("Fertig. Bitte sshTermy neu starten."))
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
	if *noIntro {
		app.SkipIntro()
	}
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fail(err)
	}
	if app.Restart() {
		// Update installiert: Terminal ist wiederhergestellt, Sitzungen sind
		// zu, Store ist gesperrt. Jetzt die neue Programmdatei starten.
		fmt.Println(i18n.T("Starte ") + version.Name + i18n.T(" neu …"))
		if err := update.Restart(); err != nil {
			fail(fmt.Errorf(i18n.T("Neustart fehlgeschlagen, bitte von Hand starten: %w"), err))
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, version.Name+":", err)
	os.Exit(1)
}
