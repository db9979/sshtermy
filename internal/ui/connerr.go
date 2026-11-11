package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/db9979/sshtermy/internal/audit"
	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/sshx"
)

// Verbindungsfehler in Klartext: statt „ssh: handshake failed: …“ steht im
// Tab, was los ist, was man tun kann und welche Taste dabei hilft. Der
// Originaltext bleibt gedämpft darunter (für Fehlerberichte und Profis).
// Dieselbe Erklärung nutzt „Testen“ im Verbindungsformular.

type connProblem struct {
	kind  string // auth, hostkey, dns, timeout, refused, unreach, reset, algo, passphrase, serial, cert, cancel, other
	title string
	hint  string
	keys  [][2]string // zusätzliche Tasten (vor ⏎/x)
}

func explainConnErr(err error, c domain.Connection) connProblem {
	if err == nil {
		return connProblem{kind: "other", title: i18n.T("getrennt")}
	}
	m := strings.ToLower(err.Error())
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(m, s) {
				return true
			}
		}
		return false
	}
	target := c.Host
	if c.Port > 0 && !c.Protocol.NoHost() {
		target = c.Host + ":" + strconv.Itoa(c.Port)
	}
	edit := [2]string{"e", i18n.T("Verbindung bearbeiten")}
	var hc *sshx.HostKeyChangedError
	var km *sshx.KeyMissingError
	switch {
	case errors.Is(err, context.Canceled):
		return connProblem{kind: "cancel", title: i18n.T("Abgebrochen"), hint: i18n.T("Die Rückfrage wurde abgebrochen.")}
	case errors.As(err, &hc) || errors.Is(err, sshx.ErrHostKeyChanged):
		p := connProblem{kind: "hostkey", title: i18n.T("Der Server zeigt einen anderen Host-Key als beim letzten Mal"),
			hint: i18n.T("Nach Neuinstallation, Werksreset oder Gerätetausch ist das normal. Sonst sitzt vielleicht jemand dazwischen (Man-in-the-Middle).")}
		if hc != nil {
			p.keys = [][2]string{{"k", i18n.T("Schlüssel vergleichen und übernehmen")}}
		}
		return p
	case errors.As(err, &km):
		return connProblem{kind: "keyfile", title: i18n.T("Schlüsseldatei fehlt auf diesem Rechner"),
			hint: i18n.T("Der Pfad stammt vielleicht von einem anderen Rechner. Datei wählen oder Schlüssel einbetten."), keys: [][2]string{edit}}
	case isAuthFailure(err) || has("permission denied (publickey", "access denied", "401"):
		u := c.Username
		if u == "" {
			u = "?"
		}
		return connProblem{kind: "auth", title: i18n.T("Anmeldung abgelehnt"),
			hint: i18n.F("Benutzer „%s“ und Passwort bzw. Schlüssel passen nicht zusammen. ⏎ fragt neu nach, falls nichts gespeichert ist.", u), keys: [][2]string{edit}}
	case has("passphrase", "decrypt", "x509: decryption"):
		return connProblem{kind: "passphrase", title: i18n.T("Passphrase des Schlüssels falsch"),
			hint: i18n.T("Der Schlüssel ist verschlüsselt und ließ sich nicht öffnen."), keys: [][2]string{edit}}
	case has("host-key von", "host key from", "abgelehnt") && has("host-key", "host key"):
		return connProblem{kind: "hostkey-rejected", title: i18n.T("Host-Key nicht bestätigt"),
			hint: i18n.T("Ohne Bestätigung des Fingerprints verbindet sshTermy nicht. ⏎ fragt noch einmal.")}
	case c.Protocol == domain.ProtoSerial && has("permission denied", "operation not permitted"):
		return connProblem{kind: "serial", title: i18n.T("Kein Zugriff auf den seriellen Port"),
			hint: i18n.T("Linux: Benutzer in die Gruppe „dialout“ aufnehmen (sudo usermod -aG dialout $USER), dann neu anmelden.")}
	case c.Protocol == domain.ProtoSerial && has("busy", "access is denied", "zugriff verweigert", "in use"):
		return connProblem{kind: "serial", title: i18n.T("Serieller Port ist belegt"),
			hint: i18n.T("Ein anderes Programm (PuTTY, screen, minicom) hat den Port noch offen.")}
	case c.Protocol == domain.ProtoSerial && has("no such file", "not found", "cannot find", "kein kabel", "no cable", "nicht gefunden"):
		return connProblem{kind: "serial", title: i18n.T("Konsolenkabel nicht gefunden"),
			hint: i18n.T("Kabel eingesteckt? Unter Windows ggf. Treiber (CH340, FTDI, PL2303) installieren."), keys: [][2]string{edit}}
	case has("no such host", "server misbehaving", "nodename nor servname", "name resolution", "name or service not known", "getaddrinfow", "lookup "):
		return connProblem{kind: "dns", title: i18n.F("Name „%s“ nicht gefunden", c.Host),
			hint: i18n.T("Tippfehler? Interne Namen gehen oft nur im VPN oder Firmennetz. Sonst die IP-Adresse eintragen."), keys: [][2]string{edit}}
	case has("connection refused", "actively refused", "verbindung verweigert", "aktiv verweigert"):
		return connProblem{kind: "refused", title: i18n.F("%s nimmt keine Verbindung an", target),
			hint: i18n.T("Das Gerät antwortet, aber auf diesem Port läuft kein Dienst. Dienst aus, anderer Port oder falsche Verbindungsart (SSH/Telnet)?"), keys: [][2]string{edit}}
	case has("no route to host", "network is unreachable", "host is unreachable", "netzwerk ist nicht erreichbar", "unreachable network", "unreachable host"):
		return connProblem{kind: "unreach", title: i18n.T("Netz nicht erreichbar"),
			hint: i18n.T("Kein Weg zu diesem Netz: VPN aus, falsches WLAN oder Kabel ab?"), keys: [][2]string{edit}}
	case has("i/o timeout", "deadline exceeded", "timed out", "timeout", "zeitüberschreitung"):
		h := i18n.T("Gerät aus, Firewall blockt, oder nur über VPN bzw. Jump-Host erreichbar.")
		if c.JumpHostID != "" {
			h = i18n.T("Gerät aus, Firewall blockt, oder der Jump-Host erreicht es nicht.")
		}
		return connProblem{kind: "timeout", title: i18n.F("Keine Antwort von %s", target), hint: h, keys: [][2]string{edit}}
	case has("no common algorithm", "no common", "kex", "unsupported"):
		return connProblem{kind: "algo", title: i18n.T("Kein gemeinsames Verschlüsselungsverfahren"),
			hint: i18n.T("Meist ein sehr altes Gerät. Firmware aktualisieren oder über Telnet bzw. Konsolenkabel verbinden.")}
	case has("x509", "certificate", "zertifikat"):
		return connProblem{kind: "cert", title: i18n.T("Zertifikat des Servers wird nicht anerkannt"),
			hint: i18n.T("Bei selbst signiertem Zertifikat im eigenen Netz: im Formular „Zertifikat nicht prüfen“ einschalten."), keys: [][2]string{edit}}
	case has("connection reset", "forcibly closed", "wsaeconnreset", "eof", "broken pipe", "closed by remote"):
		return connProblem{kind: "reset", title: i18n.T("Der Server hat die Verbindung sofort beendet"),
			hint: i18n.T("Oft: zu viele Fehlversuche (fail2ban), SSH nur von bestimmten Adressen erlaubt, oder Gerät ausgelastet. Kurz warten und neu versuchen.")}
	}
	return connProblem{kind: "other", title: i18n.T("Verbindung fehlgeschlagen")}
}

// closedView: Inhalt eines getrennten Tabs (Fehlerkarte oder „getrennt“).
func (a *App) closedView(s *session, cols, rows int) string {
	st := a.s
	keys := [][2]string{{"⏎", i18n.T("neu verbinden")}}
	var lines []string
	w := max(20, min(cols-4, 76))
	wrap := func(style lipgloss.Style, t string) {
		for _, l := range strings.Split(ansi.Wrap(t, w, " "), "\n") {
			lines = append(lines, style.Render(l))
		}
	}
	if s.errv != nil {
		p := explainConnErr(s.errv, s.conn)
		wrap(st.Danger, "× "+p.title)
		if p.hint != "" {
			lines = append(lines, "")
			wrap(st.Text, p.hint)
		}
		if raw := strings.TrimSpace(s.errv.Error()); raw != "" && !strings.EqualFold(raw, p.title) {
			lines = append(lines, "")
			wrap(st.Muted, raw)
		}
		keys = append(p.keys, keys...)
		if canWake(s.conn) {
			keys = append(keys, [2]string{"w", i18n.T("aufwecken")})
		}
	} else {
		wrap(st.Danger, "× "+orDefault(s.err, i18n.T("getrennt")))
	}
	keys = append(keys, [2]string{"x", i18n.T("Tab schließen")}, [2]string{a.keys.label(actManager), i18n.T("Manager")})
	lines = append(lines, "", a.hints(st.Muted, keys...))
	return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
}

// closedKey: zusätzliche Tasten im getrennten Tab (e, k). true = verbraucht.
func (a *App) closedKey(s *session, key string) (tea.Cmd, bool) {
	if s.state != stClosed {
		return nil, false
	}
	switch key {
	case "w":
		if s.errv == nil || !canWake(s.conn) {
			return nil, false
		}
		return a.wake(s.conn), true
	case "e":
		if s.conn.Protocol == domain.ProtoWatch {
			return nil, false
		}
		c, ok := a.st.Connection(s.conn.ID)
		if !ok {
			return nil, false
		}
		cmd := a.openConnForm(c)
		a.returnToTerminal = true
		return cmd, true
	case "k":
		var hc *sshx.HostKeyChangedError
		if errors.As(s.errv, &hc) {
			a.modal = a.hostKeyChangedModal(s, hc)
			return nil, true
		}
	}
	return nil, false
}

// hostKeyChangedModal: alter und neuer Fingerprint nebeneinander; erst „j“
// übernimmt den neuen Schlüssel (zweite Bestätigung nach „k“).
func (a *App) hostKeyChangedModal(s *session, hc *sshx.HostKeyChangedError) *modal {
	var b strings.Builder
	b.WriteString(i18n.T("Server: ") + hc.Host + "\n\n")
	b.WriteString(i18n.T("Bisher gespeichert:") + "\n")
	for _, o := range hc.Old {
		b.WriteString("  " + o + "\n")
	}
	b.WriteString("\n" + i18n.T("Jetzt gemeldet:") + "\n  " + hc.Key.Type() + " " + hc.Fingerprint() + "\n\n")
	b.WriteString(i18n.T("Nur übernehmen, wenn du weißt, warum sich der Schlüssel geändert hat. Im Zweifel den Fingerprint am Gerät prüfen (ssh-keygen -lf /etc/ssh/ssh_host_*_key.pub bzw. „show crypto host-public-key“).") + "\n\n")
	b.WriteString(i18n.T("Neuen Schlüssel übernehmen und verbinden?"))
	return confirmModal(i18n.T("Host-Key hat sich geändert"), b.String(), func() tea.Cmd {
		n, err := a.hostKeys.Replace(hc.Host, hc.Remote, hc.Key)
		if err != nil {
			a.flashErr(i18n.T("Host-Key nicht gespeichert: ") + err.Error())
			return nil
		}
		a.auditSess(s.id, audit.EvSettings, fmt.Sprintf("Host-Key ersetzt: %s %s (%d alt)", hc.Host, hc.Fingerprint(), n))
		a.flash(i18n.T("Neuer Host-Key gespeichert"))
		if fresh, ok := a.st.Connection(s.conn.ID); ok {
			s.conn = fresh
		}
		s.state, s.err, s.errv = stConnecting, "", nil
		return a.connectCmd(s)
	})
}
