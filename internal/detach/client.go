package detach

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/x/term"
)

// Texte des Fensters; sshTermy setzt sie übersetzt (i18n liegt eine Ebene höher).
var (
	MsgConnectFailed = "Keine Verbindung zu sshTermy: %v"
	MsgLost          = "Die Verbindung zu sshTermy ist weg (Hauptfenster geschlossen?)."
	MsgPressKey      = "Taste drücken zum Schließen …"
)

// Attach läuft im neuen Fenster: dockt an den Tab an und reicht alles durch,
// bis sshTermy das Fenster entlässt oder der Benutzer es schließt.
func Attach(dir string) int {
	network, addr, err := readAddr(dir)
	var tok []byte
	if err == nil {
		tok, err = os.ReadFile(filepath.Join(dir, tokenFile))
	}
	if err != nil {
		return fail(fmt.Sprintf(MsgConnectFailed, err))
	}
	nc, err := dial(network, addr)
	if err != nil {
		return fail(fmt.Sprintf(MsgConnectFailed, err))
	}
	defer nc.Close()
	fw := &frameWriter{w: nc}
	in, out := os.Stdin.Fd(), os.Stdout.Fd()
	cols, rows, err := term.GetSize(out)
	if err != nil || cols <= 0 {
		cols, rows = 80, 24
	}
	if err := fw.write(frHello, bytes.TrimSpace(tok)); err != nil {
		return fail(fmt.Sprintf(MsgConnectFailed, err))
	}
	_ = fw.write(frResize, sizePayload(cols, rows))

	restoreOut := enableVT(out)
	if st, err := term.MakeRaw(in); err == nil {
		defer term.Restore(in, st)
	}
	defer restoreOut()

	// Tasten → sshTermy
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if fw.write(frData, buf[:n]) != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// Fenstergröße beobachten
	stop := make(chan struct{})
	defer close(stop)
	go watchSize(out, stop, func(c, r int) { _ = fw.write(frResize, sizePayload(c, r)) })

	for {
		f, err := readFrame(nc)
		if err != nil {
			if err == io.EOF || isClosed(err) {
				resetScreen()
				fmt.Print("\r\n" + MsgLost + "\r\n")
				waitKey()
				return 1
			}
			resetScreen()
			fmt.Printf("\r\n%v\r\n", err)
			waitKey()
			return 1
		}
		switch f.typ {
		case frData:
			os.Stdout.Write(f.p)
		case frTitle:
			fmt.Printf("\x1b]0;%s\x07", f.p)
		case frBye:
			resetScreen()
			if len(f.p) > 0 {
				fmt.Print("\r\n" + string(f.p) + "\r\n")
				time.Sleep(1200 * time.Millisecond)
			}
			return 0
		}
	}
}

// resetScreen: Programme wie vim hinterlassen sonst Alt-Bildschirm und Maus an.
func resetScreen() {
	fmt.Print("\x1b[0m\x1b[?1049l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[?25h")
}

func waitKey() {
	fmt.Print(MsgPressKey)
	done := make(chan struct{})
	go func() {
		b := make([]byte, 1)
		_, _ = os.Stdin.Read(b)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
}

func fail(msg string) int {
	fmt.Fprintln(os.Stderr, msg)
	waitKey()
	return 1
}

func isClosed(err error) bool {
	return err != nil && (bytes.Contains([]byte(err.Error()), []byte("closed")) || bytes.Contains([]byte(err.Error()), []byte("reset")))
}
