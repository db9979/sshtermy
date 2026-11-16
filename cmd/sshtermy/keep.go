package main

import (
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"

	"github.com/db9979/sshtermy/internal/crash"
	"github.com/db9979/sshtermy/internal/detach"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/store"
	"github.com/db9979/sshtermy/internal/ui"
	"github.com/db9979/sshtermy/internal/update"
	"github.com/db9979/sshtermy/internal/winstate"
)

// Hintergrunddienst (Idee aus herdr): Mit „Hintergrunddienst“ in den
// Einstellungen startet „sshtermy“ im Terminal nicht mehr die Oberfläche
// selbst, sondern einen Prozess ohne Terminal („sshtermy keep-serve“), und
// dockt an ihn an. Schließt man das Terminalfenster oder koppelt mit
// Strg+Q → a ab, laufen Verbindungen, Tunnel und Befehle weiter; „sshtermy“
// hängt sich wieder an. Ohne Einstellung bleibt alles wie bisher; läuft
// schon ein Dienst, hängt sich „sshtermy“ immer an ihn (zwei Oberflächen
// auf demselben Datenordner gäbe es sonst).

const keepProfileEnv = "SSHTERMY_KEEP_PROFILE"

func keepDir(dir string) string { return filepath.Join(dir, "keep") }

// keepWanted: im Terminal und Dienst eingeschaltet oder schon laufend?
func keepWanted(dir string) bool {
	if dir == "" || os.Getenv("SSHTERMY_NO_KEEP") != "" || !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	return winstate.Load(dir).Keep || detach.HubAlive(keepDir(dir))
}

// keepAttach startet bei Bedarf den Dienst und dockt an. ok = false: Dienst
// ließ sich nicht starten, dann normal ohne ihn weiter.
func keepAttach(dir string, flags []string) (code int, ok bool) {
	kd := keepDir(dir)
	if !detach.HubAlive(kd) {
		self, err := os.Executable()
		if err != nil {
			return 0, false
		}
		c := exec.Command(self, append([]string{"keep-serve"}, flags...)...)
		prof := colorprofile.Detect(os.Stdout, os.Environ())
		c.Env = append(os.Environ(), keepProfileEnv+"="+strconv.Itoa(int(prof)))
		detachProcess(c)
		if err := c.Start(); err != nil {
			return 0, false
		}
		go c.Wait() //nolint:errcheck
		if err := detach.WaitHub(kd, 15*time.Second); err != nil {
			return 0, false
		}
	}
	return ui.Attach(kd), true
}

// runKeepServe: der Dienst selbst. Wartet auf das erste Fenster, startet
// dann die Oberfläche mit dessen Größe und reicht sie an jedes weitere
// Fenster durch, bis sshTermy wirklich beendet wird.
func runKeepServe(dir string, noIntro, debug bool) int {
	log.SetOutput(io.Discard)
	kd := keepDir(dir)
	if detach.HubAlive(kd) {
		return 0 // läuft schon
	}
	hub, err := detach.ListenHub(kd)
	if err != nil {
		return 1
	}
	defer hub.Close()
	type first struct {
		c          *detach.Conn
		cols, rows int
		err        error
	}
	ch := make(chan first, 1)
	go func() { c, cols, rows, err := hub.Accept(); ch <- first{c, cols, rows, err} }()
	var f first
	select {
	case f = <-ch:
	case <-time.After(time.Minute):
		return 1 // niemand hat angedockt
	}
	if f.err != nil {
		return 1
	}
	st, err := store.Open()
	if err != nil {
		f.c.Bye(err.Error())
		return 1
	}
	app, err := ui.New(st)
	if err != nil {
		f.c.Bye(err.Error())
		return 1
	}
	if noIntro {
		app.SkipIntro()
	}
	if debug {
		app.ForceDebug()
	}
	scr := detach.NewScreen()
	inR, inW := io.Pipe()
	opts := []tea.ProgramOption{tea.WithInput(inR), tea.WithOutput(scr), tea.WithWindowSize(f.cols, f.rows),
		tea.WithEnvironment(os.Environ()), tea.WithoutSignalHandler()}
	if n, err := strconv.Atoi(os.Getenv(keepProfileEnv)); err == nil {
		opts = append(opts, tea.WithColorProfile(colorprofile.Profile(n)))
	}
	p := tea.NewProgram(app, opts...)
	app.SetKeeper(func(msg string) {
		scr.Bye(msg)
		go p.Send(tea.BlurMsg{}) // niemand schaut zu (aus Update heraus: nicht blockieren)
	})
	serve := func(c *detach.Conn, cols, rows int, again bool) {
		scr.Attach(c, i18n.T("sshTermy wurde in einem anderen Fenster angehängt."))
		// Send wartet, bis die Oberfläche läuft: daher nebenher
		go func() {
			if again {
				p.Send(tea.WindowSizeMsg{Width: cols, Height: rows})
				p.Send(tea.ClearScreen()) // neues Fenster: alles neu zeichnen
			}
			p.Send(tea.FocusMsg{})
		}()
		go func() {
			for {
				ev, err := c.Read()
				if err != nil {
					if scr.Drop(c) {
						p.Send(tea.BlurMsg{})
					}
					return
				}
				if len(ev.Input) > 0 {
					_, _ = inW.Write(ev.Input)
				}
				if ev.Cols > 0 {
					p.Send(tea.WindowSizeMsg{Width: ev.Cols, Height: ev.Rows})
				}
			}
		}()
	}
	serve(f.c, f.cols, f.rows, false)
	go func() {
		for {
			c, cols, rows, err := hub.Accept()
			if err != nil {
				return
			}
			serve(c, cols, rows, true)
		}
	}()
	guard := crash.Start(st.Dir)
	_, err = p.Run()
	bye := ""
	if errors.Is(err, tea.ErrProgramPanic) {
		raw := guard.Panic()
		if r, werr := crash.Write(st.Dir, crash.KindUI, raw, st.Sensitive()); werr == nil {
			bye = i18n.T("sshTermy ist abgestürzt. Bericht: ") + r.Path
		}
	}
	guard.Close()
	if err == nil {
		update.Confirm()
	}
	if app.Restart() {
		bye = i18n.T("Update installiert: bitte sshtermy neu starten.")
	}
	scr.Bye(bye)
	inW.Close()
	return 0
}
