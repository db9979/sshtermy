package detach

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Client ist die Fensterseite eines abgetrennten Tabs: für das eigene
// Fenster (internal/gui), das statt eines Terminals andockt.
type Client struct {
	nc   net.Conn
	fw   *frameWriter
	once sync.Once
}

// Dial dockt an den Tab an (Ordner aus „sshtermy attach <ordner>“).
func Dial(dir string, cols, rows int) (*Client, error) {
	network, addr, err := readAddr(dir)
	if err != nil {
		return nil, err
	}
	tok, err := os.ReadFile(filepath.Join(dir, tokenFile))
	if err != nil {
		return nil, err
	}
	nc, err := dial(network, addr)
	if err != nil {
		return nil, err
	}
	c := &Client{nc: nc, fw: &frameWriter{w: nc}}
	if err := c.fw.write(frHello, bytes.TrimSpace(tok)); err != nil {
		nc.Close()
		return nil, err
	}
	_ = c.Resize(cols, rows)
	return c, nil
}

// Write schickt Tasten (bzw. Antworten des Emulators) an den Tab.
func (c *Client) Write(p []byte) (int, error) {
	if err := c.fw.write(frData, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Resize meldet die neue Fenstergröße.
func (c *Client) Resize(cols, rows int) error { return c.fw.write(frResize, sizePayload(cols, rows)) }

// Message ist eine Nachricht von sshTermy an das Fenster.
type Message struct {
	Output []byte // Ausgabe des Tabs
	Title  string // neuer Titel
	Bye    bool   // sshTermy holt den Tab zurück
	Text   string // Meldung zum Abschied
}

// Next liefert die nächste Nachricht; io.EOF, wenn sshTermy weg ist.
func (c *Client) Next() (Message, error) {
	for {
		f, err := readFrame(c.nc)
		if err != nil {
			if isClosed(err) {
				return Message{}, io.EOF
			}
			return Message{}, err
		}
		switch f.typ {
		case frData:
			return Message{Output: f.p}, nil
		case frTitle:
			return Message{Title: string(f.p)}, nil
		case frBye:
			return Message{Bye: true, Text: string(f.p)}, nil
		}
	}
}

// Close trennt; der Tab kehrt ins Hauptfenster zurück.
func (c *Client) Close() { c.once.Do(func() { c.nc.Close() }) }
