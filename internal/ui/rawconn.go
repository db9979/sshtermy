package ui

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/db9979/sshtermy/internal/autologin"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/serialx"
	"github.com/db9979/sshtermy/internal/telnet"
	"github.com/db9979/sshtermy/internal/termemu"
)

// Telnet und serielle Konsole: dieselbe Sitzung wie bei SSH (Emulator, KI,
// Schnellbefehle, Host-Übersicht über die sichtbare Shell), nur ohne
// SSH-Client. Exec, SFTP und Tunnel gibt es dort nicht; alles, was s.client
// braucht, prüft vorher auf nil.

// termStream: was eine Sitzung von ihrer Leitung braucht.
type termStream interface {
	Write(p []byte) (int, error)
	Resize(cols, rows int) error
	Close() error
}

// netDev: Gegenstelle ist ein Switch/Router. Telnet und Konsole gelten als
// Netzwerkgerät, solange die Verbindung nicht ausdrücklich „Server“ ist.
func (s *session) netDev() bool {
	if s.client != nil {
		return s.client.NetDevice
	}
	return s.conn.Protocol.Raw() && s.conn.Device != domain.DeviceServer
}

// serialPorts ist austauschbar, damit Tests ohne Kabel auskommen.
type serialxPort = serialx.Port

var defaultSerialPorts = serialx.List
var serialPorts = defaultSerialPorts

func (a *App) connectRawCmd(s *session, cols, rows int, keepTerm bool) tea.Cmd {
	ev := a.events
	c := s.conn
	return func() tea.Msg {
		if !keepTerm || s.term == nil {
			s.term = termemu.New(cols, rows, shellWriter{s})
		}
		fail := func(err error) tea.Msg {
			if !keepTerm {
				s.term.Close()
				s.term = nil
			}
			return connectErrMsg{s.id, err}
		}

		// Anmeldung: Benutzer/Passwort tippen, sobald das Gerät fragt.
		ready := make(chan struct{})
		var st termStream
		login := autologin.New(c.Username, c.Password.Reveal(), func(txt string) {
			go func() {
				<-ready
				_, _ = st.Write([]byte(txt))
			}()
		})
		out := io.MultiWriter(outWriter{s, ev}, login)

		var done <-chan error
		note := ""
		switch c.Protocol {
		case domain.ProtoTelnet:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			t, err := telnet.Dial(ctx, c.Addr(), cols, rows, out)
			if err != nil {
				return fail(err)
			}
			st, done = t, t.Done
		case domain.ProtoSerial:
			name, guessed, err := serialx.Resolve(c.SerialPort, serialPorts())
			if err != nil {
				return fail(serialError(err))
			}
			p, err := serialx.Open(name, c.SerialBaud(), c.SerialFrame(), out)
			if err != nil {
				return fail(serialError(err))
			}
			st, done = p, p.Done
			if guessed {
				note = i18n.F("Konsolenkabel an %s gefunden (gespeichert: %s)", name, c.SerialPort)
			}
		default:
			return fail(errors.New("unknown protocol " + string(c.Protocol)))
		}
		close(ready)
		if c.Protocol == domain.ProtoSerial {
			// Konsolen zeigen oft erst nach einem Tastendruck etwas an.
			_, _ = st.Write([]byte("\r"))
		}
		go func() {
			err := <-done
			ev <- shellClosedMsg{s.id, st, err, err != nil}
		}()
		return connectedMsg{sid: s.id, shell: st, note: note}
	}
}

// serialError übersetzt die Fehler der Schnittstelle.
func serialError(err error) error {
	var amb *serialx.AmbiguousError
	switch {
	case errors.As(err, &amb):
		names := make([]string, len(amb.Ports))
		for i, p := range amb.Ports {
			names[i] = p.Name
		}
		return errors.New(i18n.F("Mehrere Konsolenkabel gefunden (%s): bitte in der Verbindung eins auswählen", strings.Join(names, ", ")))
	case errors.Is(err, serialx.ErrNoPort):
		return errors.New(i18n.T("Kein Konsolenkabel gefunden. Ist der USB-Adapter eingesteckt und der Treiber installiert?"))
	case errors.Is(err, serialx.ErrBusy):
		return errors.New(err.Error() + " · " + i18n.T("Die Schnittstelle benutzt gerade ein anderes Programm (PuTTY, screen, minicom …)."))
	case errors.Is(err, serialx.ErrPermission):
		return errors.New(err.Error() + " · " + i18n.T("Linux: Benutzer in die Gruppe „dialout“ aufnehmen (sudo usermod -aG dialout $USER), dann neu anmelden."))
	case errors.Is(err, serialx.ErrNotFound):
		return errors.New(err.Error() + " · " + i18n.T("Kabel eingesteckt? Sonst in der Verbindung eine andere Schnittstelle wählen."))
	}
	return err
}

// sendBreak: Break-Signal auf der seriellen Leitung.
func (a *App) sendBreak(s *session) tea.Cmd {
	p, ok := s.shell.(*serialx.Stream)
	if !ok {
		a.flash(i18n.T("Break gibt es nur an der seriellen Konsole"))
		return nil
	}
	if err := p.Break(); err != nil {
		a.flash("Break: " + err.Error())
		return nil
	}
	a.flash(i18n.T("Break gesendet"))
	return nil
}

// protoBadge: Plakette in der Sitzungsleiste für Telnet und Konsole.
func (a *App) protoBadge(s *session) string {
	st := a.s
	switch s.conn.Protocol {
	case domain.ProtoTelnet:
		return st.Badge.Foreground(st.T.Warn).Render(i18n.T("telnet · unverschlüsselt"))
	case domain.ProtoSerial:
		name := s.conn.SerialPort
		if p, ok := s.shell.(*serialx.Stream); ok {
			name = p.Name
		}
		if name == "" {
			name = "USB"
		}
		name = strings.TrimPrefix(name, "/dev/")
		return st.Badge.Foreground(st.T.Accent).Render("≈ " + name + " " + strconv.Itoa(s.conn.SerialBaud()))
	}
	return ""
}

// shortTarget: kurze Zielangabe für Listen („admin@10.0.0.1“, „telnet
// sw-core“, „≈ COM3“).
func shortTarget(c domain.Connection) string {
	t := c.Host
	if c.Username != "" {
		t = c.Username + "@" + c.Host
	}
	if c.ViaRelay() {
		return "↔ " + t
	}
	switch c.Protocol {
	case domain.ProtoTelnet:
		return "telnet " + t
	case domain.ProtoSerial:
		p := strings.TrimPrefix(c.SerialPort, "/dev/")
		if p == "" {
			p = "USB"
		}
		return "≈ " + p
	}
	return t
}

// ---------------------------------------------------------------- Formular

// protoLabels: Auswahl im Verbindungsformular.
var protoLabels = []string{"SSH", "Telnet", "Seriell (Konsolenkabel)"}

func protoLabel(p domain.Protocol) string {
	switch p {
	case domain.ProtoTelnet:
		return protoLabels[1]
	case domain.ProtoSerial:
		return protoLabels[2]
	}
	return protoLabels[0]
}

func protoFromLabel(l string) domain.Protocol {
	switch l {
	case protoLabels[1]:
		return domain.ProtoTelnet
	case protoLabels[2]:
		return domain.ProtoSerial
	}
	return domain.ProtoSSH
}

const serialAuto = "automatisch"

// serialPortOptions: „automatisch“, die gefundenen Schnittstellen und die
// gespeicherte (auch wenn das Kabel gerade nicht steckt).
func serialPortOptions(cur string) (opts []string, curOpt string) {
	opts = []string{serialAuto}
	curOpt = serialAuto
	found := false
	for _, p := range serialPorts() {
		opts = append(opts, p.Name)
		if cur != "" && strings.EqualFold(p.Name, cur) {
			found, curOpt = true, p.Name
		}
	}
	if cur != "" && !found {
		opts = append(opts, cur)
		curOpt = cur
	}
	return opts, curOpt
}

// serialPortHelp: was hinter den Namen steckt (Treiber/Chip).
func serialPortHelp() string {
	ps := serialPorts()
	if len(ps) == 0 {
		return i18n.T("kein Kabel gefunden · automatisch = das einzige USB-Konsolenkabel")
	}
	var parts []string
	for _, p := range ps {
		if p.Desc != "" {
			parts = append(parts, strings.TrimPrefix(p.Name, "/dev/")+" = "+p.Desc)
		}
	}
	h := i18n.T("automatisch = das einzige USB-Konsolenkabel (auch auf anderen Rechnern)")
	if len(parts) > 0 {
		h += " · " + strings.Join(parts, ", ")
	}
	return h
}

var baudOptions = []string{"9600", "19200", "38400", "57600", "115200", "1200", "2400", "4800"}
var frameOptions = []string{"8N1", "8E1", "8O1", "7E1", "7O1", "8N2"}

func baudOpts(cur int) []string {
	c := strconv.Itoa(cur)
	for _, b := range baudOptions {
		if b == c {
			return baudOptions
		}
	}
	return append(append([]string(nil), baudOptions...), c)
}

func frameOpts(cur string) []string {
	for _, f := range frameOptions {
		if f == cur {
			return frameOptions
		}
	}
	return append(append([]string(nil), frameOptions...), cur)
}

// ---------------------------------------------------------------- Schnellverbinden

// parseQuickRaw erkennt in der Palette „telnet host [port]“, „telnet://host:port“,
// „konsole [COM3] [115200]“ sowie einen nackten Schnittstellennamen
// („COM3“, „/dev/ttyUSB0“).
func parseQuickRaw(q string) (c domain.Connection, ok bool) {
	f := strings.Fields(strings.TrimSpace(q))
	if len(f) == 0 {
		return c, false
	}
	w := strings.ToLower(f[0])
	switch {
	case w == "telnet" && (len(f) == 2 || len(f) == 3):
		port := 23
		if len(f) == 3 {
			p, err := strconv.Atoi(f[2])
			if err != nil || p <= 0 || p > 65535 {
				return c, false
			}
			port = p
		}
		return quickTelnet(f[1], port)
	case strings.HasPrefix(w, "telnet://") && len(f) == 1:
		t := f[0][len("telnet://"):]
		port := 23
		if h, ps, found := strings.Cut(t, ":"); found && !strings.Contains(ps, ":") {
			p, err := strconv.Atoi(strings.TrimSuffix(ps, "/"))
			if err != nil || p <= 0 || p > 65535 {
				return c, false
			}
			t, port = h, p
		}
		return quickTelnet(strings.TrimSuffix(t, "/"), port)
	case (w == "konsole" || w == "console" || w == "seriell" || w == "serial") && len(f) <= 3:
		c = domain.Connection{Protocol: domain.ProtoSerial}
		for _, a := range f[1:] {
			if b, err := strconv.Atoi(a); err == nil {
				c.Baud = b
			} else {
				c.SerialPort = a
			}
		}
	case isPortName(f[0]) && len(f) <= 2:
		c = domain.Connection{Protocol: domain.ProtoSerial, SerialPort: f[0]}
		if len(f) == 2 {
			b, err := strconv.Atoi(f[1])
			if err != nil {
				return c, false
			}
			c.Baud = b
		}
	default:
		return c, false
	}
	if c.Baud < 0 || c.Baud > 4000000 {
		return c, false
	}
	name := c.SerialPort
	if name == "" {
		name = "USB"
	}
	c.Alias = i18n.T("Konsole") + " " + strings.TrimPrefix(name, "/dev/")
	c.ID = "quick:serial:" + name
	return c, true
}

func quickTelnet(target string, port int) (domain.Connection, bool) {
	userName := ""
	if at := strings.LastIndex(target, "@"); at >= 0 {
		userName, target = target[:at], target[at+1:]
	}
	if target == "" || !validHost(target) {
		return domain.Connection{}, false
	}
	alias := "telnet " + target
	if port != 23 {
		alias += ":" + strconv.Itoa(port)
	}
	return domain.Connection{ID: "quick:" + alias, Alias: alias, Host: target, Port: port, Username: userName,
		Protocol: domain.ProtoTelnet, Auth: domain.AuthPassword}, true
}

// isPortName: COM3, /dev/ttyUSB0, /dev/cu.usbserial-1410 …
func isPortName(s string) bool {
	u := strings.ToUpper(s)
	if strings.HasPrefix(u, "COM") && len(u) > 3 {
		_, err := strconv.Atoi(u[3:])
		return err == nil
	}
	return strings.HasPrefix(s, "/dev/tty") || strings.HasPrefix(s, "/dev/cu.")
}

// quickRawItems: Palette-Einträge für Telnet/Konsole.
func (a *App) quickRawItems(c domain.Connection) []palItem {
	hint := "Telnet"
	if c.Protocol == domain.ProtoSerial {
		hint = i18n.T("Konsolenkabel")
	}
	return []palItem{
		{group: pgConnect, icon: ic("connect"), label: i18n.T("Schnell verbinden: ") + c.Alias, hint: hint,
			run: func() tea.Cmd { return a.openOrConnect(c) }},
		{group: pgConnect, icon: ic("new"), label: i18n.T("Als Verbindung speichern: ") + c.Alias, hint: i18n.T("anlegen"),
			run: func() tea.Cmd {
				n := c
				n.ID = ""
				if n.Protocol == domain.ProtoTelnet {
					n.Alias = n.Host
				}
				return a.openConnForm(n)
			}},
	}
}

func (s *session) isSerial() bool { return s.conn.Protocol == domain.ProtoSerial }

// remoteString: Systembeschreibung für die Gefahren-Einstufung.
func (s *session) remoteString() string {
	if s.client != nil {
		return s.client.Remote.String()
	}
	return s.profileSummary()
}

// syncProtoPort: Wechsel SSH ↔ Telnet im Formular passt den Standard-Port an,
// solange dort noch der Standard des anderen Protokolls steht.
func syncProtoPort(f *form) {
	if f == nil {
		return
	}
	pc := f.get("proto")
	now := f.sel("proto")
	if pc.value == "" {
		pc.value = protoLabels[0]
	}
	if pc.value == now {
		return // nur beim Wechsel des Protokolls
	}
	pc.value = now
	pf := f.get("port")
	cur := strings.TrimSpace(pf.in.Value())
	switch protoFromLabel(now) {
	case domain.ProtoTelnet:
		if cur == "22" {
			pf.in.SetValue("23")
		}
	case domain.ProtoSSH:
		if cur == "23" {
			pf.in.SetValue("22")
		}
	}
}
