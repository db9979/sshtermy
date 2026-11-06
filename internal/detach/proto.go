// Package detach trennt einen Tab in ein eigenes Terminalfenster ab.
//
// Die Sitzung bleibt dabei im laufenden sshTermy: Es öffnet einen lokalen
// Kanal (Unix-Socket bzw. Named Pipe unter Windows), startet ein neues
// Terminalfenster mit „sshtermy attach <ordner>“ und reicht dort Tasten,
// Ausgabe und Fenstergröße roh durch. Schließt man das Fenster, ist der Tab
// wieder im Hauptfenster; die SSH-Verbindung bleibt die ganze Zeit offen.
//
// Der Ordner (nur für den eigenen Benutzer lesbar) enthält die Adresse und
// ein einmaliges Zugangswort, damit kein anderes Programm andocken kann.
package detach

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Rahmen: Typ (1 Byte), Länge (4 Byte, big endian), Nutzdaten.
const (
	frHello  byte = 'H' // Client → sshTermy: Zugangswort
	frData   byte = 'D' // in beide Richtungen: Tasten bzw. Ausgabe
	frResize byte = 'R' // Client → sshTermy: Spalten, Zeilen (je uint16)
	frTitle  byte = 'T' // sshTermy → Client: Fenstertitel
	frBye    byte = 'B' // sshTermy → Client: Ende, Nutzdaten = Meldung
)

const maxFrame = 1 << 20

type frame struct {
	typ byte
	p   []byte
}

func readFrame(r io.Reader) (frame, error) {
	var h [5]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return frame{}, err
	}
	n := binary.BigEndian.Uint32(h[1:])
	if n > maxFrame {
		return frame{}, errors.New("detach: Rahmen zu groß")
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return frame{}, err
	}
	return frame{h[0], p}, nil
}

type frameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (fw *frameWriter) write(typ byte, p []byte) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for {
		chunk := p
		if len(chunk) > maxFrame {
			chunk = chunk[:maxFrame]
		}
		buf := make([]byte, 5+len(chunk))
		buf[0] = typ
		binary.BigEndian.PutUint32(buf[1:], uint32(len(chunk)))
		copy(buf[5:], chunk)
		if _, err := fw.w.Write(buf); err != nil {
			return err
		}
		p = p[len(chunk):]
		if len(p) == 0 {
			return nil
		}
	}
}

func sizePayload(cols, rows int) []byte {
	var b [4]byte
	binary.BigEndian.PutUint16(b[:2], uint16(clamp(cols)))
	binary.BigEndian.PutUint16(b[2:], uint16(clamp(rows)))
	return b[:]
}

func parseSize(p []byte) (cols, rows int, err error) {
	if len(p) != 4 {
		return 0, 0, fmt.Errorf("detach: Größe hat %d Byte", len(p))
	}
	return int(binary.BigEndian.Uint16(p[:2])), int(binary.BigEndian.Uint16(p[2:])), nil
}

func clamp(n int) int { return max(1, min(n, 2000)) }
