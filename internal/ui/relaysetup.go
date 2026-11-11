package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/db9979/sshtermy/internal/domain"
	"github.com/db9979/sshtermy/internal/i18n"
	"github.com/db9979/sshtermy/internal/relay"
	"github.com/db9979/sshtermy/internal/update"
)

// Vermittler-Assistent: richtet auf einem eigenen Linux-Server (offener Tab)
// einen `sshtermy vermittler` als systemd-Dienst ein und koppelt sich selbst
// – statt sieben Schritten auf drei Rechnern. Jeder ändernde Schritt zeigt
// seine Befehle und braucht ein eigenes „j“ (alle Verbindungen gelten als
// produktiv). Der Rückweg steht am Vermittler (Verwalten › Vom Server entfernen).

const (
	setupUnit = "sshtermy-vermittler"
	setupDir  = "/var/lib/sshtermy-vermittler"
	setupBin  = "/usr/local/bin/sshtermy"
	setupUser = "sshtermy-vermittler"
)

// setupPlan: was der Assistent über den Server weiß.
type setupPlan struct {
	sid     int
	base    string // https://host:port
	port    string
	sudo    string // "" (root) oder "sudo -n "
	arch    string // amd64, arm64, arm, riscv64
	hasBin  bool
	running bool
	bin     bool // Programmdatei vom Assistenten
}

func (a *App) setupCandidates() []*session {
	var out []*session
	for _, x := range a.sessions {
		if x.state == stConnected && x.client != nil && !x.netDev() && x.conn.Protocol != domain.ProtoWatch {
			out = append(out, x)
		}
	}
	return out
}

func setupLabel(x *session) string {
	return x.tabName() + "  (" + x.conn.Username + "@" + x.conn.Host + ")"
}

func (a *App) openRelaySetup(cur *session) tea.Cmd {
	cands := a.setupCandidates()
	if len(cands) == 0 {
		a.modal = &modal{kind: mInfo, title: i18n.T("Vermittler einrichten"), body: i18n.T("Der Assistent richtet den Vermittler auf einem deiner Linux-Server ein, der von außen erreichbar ist (VPS, Pi mit Portfreigabe).\n\nÖffne zuerst eine SSH-Verbindung zu diesem Server, dann noch einmal „teilen“ › „Eigenen Vermittler einrichten“.")}
		return nil
	}
	var labels []string
	sel := ""
	for _, x := range cands {
		labels = append(labels, setupLabel(x))
		if x == cur {
			sel = setupLabel(x)
		}
	}
	a.shareForm.ways = map[string]string{}
	for _, x := range cands {
		a.shareForm.ways[setupLabel(x)] = fmt.Sprint(x.id)
	}
	f := &form{title: i18n.T("Vermittler einrichten · Assistent")}
	f.choice("srv", i18n.T("Auf Server"), labels, sel, i18n.T("offener SSH-Tab, Linux mit systemd; von außen erreichbar"))
	f.text("url", i18n.T("Adresse von außen"), "", i18n.T("leer = https://<Server-Adresse>:8022 · Name oder IP, wie andere den Server erreichen"))
	f.section("info", i18n.T("Danach: jeder Schritt zeigt seine Befehle und braucht dein „j“. Rückweg: Vermittler verwalten › Vom Server entfernen"), nil)
	a.form, a.formID, a.scr = f, "", scrRelaySetup
	return f.setFocus(0)
}

func (a *App) submitRelaySetup() (tea.Cmd, error) {
	f := a.form
	var x *session
	for _, c := range a.setupCandidates() {
		if setupLabel(c) == f.sel("srv") {
			x = c
		}
	}
	if x == nil {
		return nil, errors.New(i18n.T("Server nicht mehr verbunden"))
	}
	u := strings.TrimSpace(f.str("url"))
	if u == "" {
		u = "https://" + x.conn.Host + ":8022"
	}
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	base, err := relay.NormBase(u)
	if err != nil {
		return nil, err
	}
	pu, _ := url.Parse(base)
	port := pu.Port()
	if port == "" {
		port = "443"
	}
	a.returnToTerminal = true
	p := &setupPlan{sid: x.id, base: base, port: port}
	return a.setupDetect(p), nil
}

func (a *App) setupSession(p *setupPlan) (*session, error) {
	x := a.byID(p.sid)
	if x == nil || x.client == nil || x.state != stConnected {
		return nil, errors.New(i18n.T("Server nicht mehr verbunden"))
	}
	return x, nil
}

// setupRun: Skript auf dem Server, Erfolg nur mit __OK__ am Ende.
func setupRun(x *session, script string, d time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	out, err := x.client.RunScript(ctx, "set -e\n"+script+"\necho __OK__\n")
	if err == nil && !strings.Contains(out, "__OK__") {
		err = errors.New(strings.TrimSpace(lastLines(out, 6)))
		if err.Error() == "" {
			err = errors.New(i18n.T("Befehl fehlgeschlagen"))
		}
	}
	return strings.ReplaceAll(out, "__OK__", ""), err
}

// Schritt 1 (liest): System, Rechte, vorhandene Installation.
func (a *App) setupDetect(p *setupPlan) tea.Cmd {
	x, err := a.setupSession(p)
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.modal = busy(i18n.T("Vermittler einrichten"), i18n.T("Schritt 1/5: Server prüfen (liest nur) …"))
	return func() tea.Msg {
		out, err := setupRun(x, `uname -sm
echo "uid=$(id -u)"
command -v systemctl >/dev/null && echo systemd=ja || echo systemd=nein
if sudo -n true 2>/dev/null; then echo sudo=ja; else echo sudo=nein; fi
[ -x `+setupBin+` ] && echo bin=ja || echo bin=nein
systemctl is-active `+setupUnit+` 2>/dev/null || true`, 20*time.Second)
		return relayMsg{func(a *App) tea.Cmd {
			a.modal = nil
			if err != nil {
				a.flashErr(i18n.T("Server prüfen: ") + err.Error())
				return nil
			}
			return a.setupAfterDetect(p, out)
		}}
	}
}

func (a *App) setupAfterDetect(p *setupPlan, out string) tea.Cmd {
	if !strings.Contains(out, "Linux") || !strings.Contains(out, "systemd=ja") {
		a.modal = &modal{kind: mInfo, title: i18n.T("Vermittler einrichten"), body: i18n.T("Der Assistent kann nur Linux-Server mit systemd einrichten.\n\nAndere Systeme: sshtermy dort von Hand starten (siehe Handbuch › Vermittler) und dann die Kopplungszeile einfügen.")}
		return nil
	}
	switch {
	case strings.Contains(out, "x86_64"):
		p.arch = "amd64"
	case strings.Contains(out, "aarch64"), strings.Contains(out, "arm64"):
		p.arch = "arm64"
	case strings.Contains(out, "armv7"), strings.Contains(out, "armv6"):
		p.arch = "arm"
	case strings.Contains(out, "riscv64"):
		p.arch = "riscv64"
	default:
		a.flashErr(i18n.T("Unbekannte Prozessorarchitektur: ") + strings.TrimSpace(strings.SplitN(out, "\n", 2)[0]))
		return nil
	}
	p.hasBin = strings.Contains(out, "bin=ja")
	p.running = strings.Contains(out, "\nactive")
	if !strings.Contains(out, "uid=0") {
		if !strings.Contains(out, "sudo=ja") {
			return a.setupManual(p)
		}
		p.sudo = "sudo -n "
	}
	return a.setupAskBin(p)
}

// setupManual: sudo will ein Passwort – Befehle zum Einfügen in den Tab.
func (a *App) setupManual(p *setupPlan) tea.Cmd {
	script := "sudo sh -c '" + strings.ReplaceAll(setupServiceScript(p, ""), "'", `'"'"'`) + "'\n" +
		"sudo -u " + setupUser + " " + setupBin + " vermittler code --verwalten --dir " + setupDir + "\n"
	lines := []string{
		i18n.T("sudo fragt auf diesem Server nach einem Passwort – das gibt sshTermy nicht im Hintergrund ein."),
		i18n.T("So geht es trotzdem schnell:"),
		"",
		i18n.T("1. c kopiert die Befehle unten. Im Tab des Servers einfügen und ausführen (sudo-Passwort)."),
		fmt.Sprintf(i18n.T("   Liegt %s dort noch nicht, vorher installieren (Paketquellen, siehe Webseite)."), setupBin),
		i18n.T("2. Die ausgegebene Zeile https://…#termy=… kopieren."),
		i18n.T("3. Palette › „Vermittler koppeln / verwalten“ › „Neuen Vermittler koppeln“ › einfügen."),
		"",
	}
	lines = append(lines, strings.Split(script, "\n")...)
	m := textModal(i18n.T("Vermittler einrichten · Befehle zum Einfügen"), lines)
	m.copy = script
	a.modal = m
	return nil
}

func setupUnitText(p *setupPlan) string {
	return `[Unit]
Description=sshTermy-Vermittler
After=network-online.target
Wants=network-online.target

[Service]
User=` + setupUser + `
ExecStart=` + setupBin + ` vermittler start --url ` + p.base + ` --listen :` + p.port + ` --dir ` + setupDir + `
Restart=always
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=` + setupDir + `

[Install]
WantedBy=multi-user.target
`
}

// setupServiceScript: Benutzer, Datenordner, Dienst (Schritt 3).
func setupServiceScript(p *setupPlan, sudo string) string {
	return `id ` + setupUser + ` >/dev/null 2>&1 || ` + sudo + `useradd --system --home-dir ` + setupDir + ` --shell /usr/sbin/nologin ` + setupUser + `
` + sudo + `install -d -o ` + setupUser + ` -m 0700 ` + setupDir + `
` + sudo + `tee /etc/systemd/system/` + setupUnit + `.service >/dev/null <<'UNIT'
` + setupUnitText(p) + `UNIT
` + sudo + `systemctl daemon-reload
` + sudo + `systemctl enable ` + setupUnit + `
` + sudo + `systemctl restart ` + setupUnit
}

func (a *App) setupConfirm(p *setupPlan, title, cmds string, run func() tea.Cmd) {
	x, _ := a.setupSession(p)
	alias := ""
	if x != nil {
		alias = x.conn.Alias
	}
	body := cmds + "\n\n" + fmt.Sprintf(i18n.T("▲ ändert %s (gilt als produktiv) · j = ausführen   n = abbrechen"), alias)
	a.modal = confirmModal(title, body, run)
}

// Schritt 2 (ändert): Programmdatei auf den Server.
func (a *App) setupAskBin(p *setupPlan) tea.Cmd {
	if p.hasBin {
		return a.setupAskService(p)
	}
	how := i18n.T("lädt die passende Linux-Datei von GitHub (Prüfsumme und Signatur geprüft) und kopiert sie per SFTP")
	if runtime.GOOS == "linux" && runtime.GOARCH == p.arch {
		how = i18n.T("kopiert diese sshTermy-Programmdatei per SFTP (gleiches System)")
	}
	a.setupConfirm(p, i18n.T("Schritt 2/5: sshtermy auf den Server"),
		how+"\n\n  "+p.sudo+"install -m 0755 /tmp/sshtermy.neu "+setupBin, func() tea.Cmd {
			x, err := a.setupSession(p)
			if err != nil {
				a.flashErr(err.Error())
				return nil
			}
			a.modal = busy(i18n.T("Vermittler einrichten"), i18n.T("Schritt 2/5: Programmdatei übertragen …"))
			return func() tea.Msg {
				err := setupUpload(x, p)
				return relayMsg{func(a *App) tea.Cmd {
					a.modal = nil
					if err != nil {
						a.flashErr(i18n.T("Programmdatei: ") + err.Error())
						return nil
					}
					p.bin = true
					return a.setupAskService(p)
				}}
			}
		})
	return nil
}

func setupUpload(x *session, p *setupPlan) error {
	var bin []byte
	if runtime.GOOS == "linux" && runtime.GOARCH == p.arch {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if bin, err = os.ReadFile(exe); err != nil {
			return err
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		rel, err := update.Check(ctx)
		if err != nil {
			return err
		}
		if bin, err = update.FetchAsset(ctx, rel, "sshtermy_linux_"+p.arch); err != nil {
			return err
		}
	}
	fs, err := x.client.OpenSFTP()
	if err != nil {
		return err
	}
	defer fs.Close()
	tmp := "/tmp/sshtermy.neu"
	w, err := fs.Create(tmp, 0o700)
	if err != nil {
		return err
	}
	if _, err := w.Write(bin); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	_, err = setupRun(x, p.sudo+"install -m 0755 "+tmp+" "+setupBin+"\nrm -f "+tmp+"\n"+setupBin+" version >/dev/null", time.Minute)
	return err
}

// Schritt 3 (ändert): Benutzer, Datenordner, systemd-Dienst.
func (a *App) setupAskService(p *setupPlan) tea.Cmd {
	script := setupServiceScript(p, p.sudo)
	a.setupConfirm(p, i18n.T("Schritt 3/5: Dienst anlegen und starten"), script, func() tea.Cmd {
		x, err := a.setupSession(p)
		if err != nil {
			a.flashErr(err.Error())
			return nil
		}
		a.modal = busy(i18n.T("Vermittler einrichten"), i18n.T("Schritt 3/5: Dienst starten …"))
		return func() tea.Msg {
			_, err := setupRun(x, script+"\nsleep 2\n"+p.sudo+"systemctl is-active "+setupUnit, time.Minute)
			return relayMsg{func(a *App) tea.Cmd {
				a.modal = nil
				if err != nil {
					a.flashErr(i18n.T("Dienst: ") + err.Error())
					return nil
				}
				return a.setupPair(p)
			}}
		}
	})
	return nil
}

var pairLineRE = regexp.MustCompile(`https://\S+#termy=\S+`)

// Schritte 4 und 5 (lesen): Code holen, von hier aus koppeln (prüft zugleich,
// ob der Vermittler von außen erreichbar ist).
func (a *App) setupPair(p *setupPlan) tea.Cmd {
	x, err := a.setupSession(p)
	if err != nil {
		a.flashErr(err.Error())
		return nil
	}
	a.modal = busy(i18n.T("Vermittler einrichten"), i18n.T("Schritt 4/5: Kopplungscode holen · Schritt 5/5: von hier aus koppeln …"))
	device, _ := os.Hostname()
	return func() tea.Msg {
		code := setupBin + " vermittler code --verwalten --dir " + setupDir
		cmd := p.sudo + "-u " + setupUser + " " + code
		if p.sudo == "" {
			cmd = "su -s /bin/sh " + setupUser + " -c '" + code + "'"
		}
		out, err := setupRun(x, cmd, 30*time.Second)
		line := pairLineRE.FindString(out)
		var pr relay.PairResult
		var cl *relay.Client
		var pl relay.PairLine
		if err == nil && line == "" {
			err = errors.New(i18n.T("keine Kopplungszeile erhalten"))
		}
		if err == nil {
			pl, err = relay.ParseLine(line)
		}
		if err == nil {
			cl, err = relay.NewClient(pl.Base, "", pl.FP)
		}
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			pr, err = cl.Pair(ctx, pl.Code, device)
			cancel()
			if err != nil {
				err = fmt.Errorf(i18n.T("%w\n\nLäuft der Dienst, ist aber von hier nicht erreichbar? Port %s in der Firewall öffnen (z. B. sudo ufw allow %s/tcp) und Kopplungszeile von Hand einfügen:\n%s"), err, p.port, p.port, line)
			}
		}
		return relayMsg{func(a *App) tea.Cmd {
			a.modal = nil
			if err != nil {
				if line != "" {
					go func() { _ = clipboard.WriteAll(line) }()
				}
				a.modal = &modal{kind: mInfo, title: i18n.T("Vermittler einrichten · Koppeln"), body: err.Error()}
				return nil
			}
			r, err := a.st.UpsertRelay(domain.Relay{Name: orDefault(pr.Name, cl.Base), Kind: relay.KindSSHTermy, Base: cl.Base, FP: pl.FP,
				Key: domain.Secret(pr.Key), Device: device, Admin: pr.Admin,
				Setup: &domain.RelaySetup{ConnID: x.conn.ID, Bin: p.bin, Unit: setupUnit}})
			if err != nil {
				a.flashErr(err.Error())
				return nil
			}
			_ = a.st.UpdateSettings(func(s *domain.Settings) { s.Share.Way = "relay:" + r.ID })
			a.flashOK(fmt.Sprintf(i18n.T("Vermittler „%s“ läuft auf %s und ist gekoppelt"), r.Name, x.conn.Alias))
			if c := a.byID(a.shareForm.sid); c != nil && c.state == stConnected && c.share.Load() == nil {
				return a.openShareForm(c)
			}
			return nil
		}}
	}
}

// relayUninstall: Rückweg des Assistenten (Dienst, Benutzer, Daten, Datei).
func (a *App) relayUninstall(r domain.Relay) tea.Cmd {
	var x *session
	for _, c := range a.setupCandidates() {
		if r.Setup != nil && c.conn.ID == r.Setup.ConnID {
			x = c
		}
	}
	if x == nil {
		a.flashErr(i18n.T("Erst eine SSH-Verbindung zum Server des Vermittlers öffnen"))
		return nil
	}
	sudo := "sudo -n "
	if x.conn.Username == "root" {
		sudo = ""
	}
	script := sudo + "systemctl disable --now " + setupUnit + " || true\n" +
		sudo + "rm -f /etc/systemd/system/" + setupUnit + ".service\n" +
		sudo + "systemctl daemon-reload\n" +
		sudo + "userdel " + setupUser + " || true\n" +
		sudo + "rm -rf " + setupDir
	if r.Setup.Bin {
		script += "\n" + sudo + "rm -f " + setupBin
	}
	m := confirmModal(i18n.T("Vermittler vom Server entfernen?"), fmt.Sprintf(i18n.T("Auf %s:\n\n%s\n\nAlle Kopplungen und Freigaben über diesen Vermittler enden."), x.conn.Alias, script), func() tea.Cmd {
		a.modal = busy(i18n.T("Vermittler"), i18n.T("Entferne vom Server …"))
		return func() tea.Msg {
			_, err := setupRun(x, script, time.Minute)
			return relayMsg{func(a *App) tea.Cmd {
				a.modal = nil
				if err != nil {
					a.flashErr(i18n.T("Entfernen: ") + err.Error())
					return nil
				}
				_ = a.st.DeleteRelay(r.ID)
				a.flash(i18n.T("Vermittler vom Server entfernt: ") + r.Name)
				return nil
			}}
		}
	})
	m.twice = true
	a.modal = m
	return nil
}

// relayNewLine: Kopplungszeile aus sshTermy heraus (Gerät mit Verwaltungsrecht).
func (a *App) relayNewLine(r domain.Relay, kind string) tea.Cmd {
	a.modal = busy(i18n.T("Vermittler"), i18n.T("Hole Kopplungszeile …"))
	return func() tea.Msg {
		cl, _, err := a.relayClient(r.ID)
		line := ""
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			line, err = cl.NewPairLine(ctx, kind)
			cancel()
		}
		return relayMsg{func(a *App) tea.Cmd {
			a.modal = nil
			if err != nil {
				a.flashErr(err.Error())
				return nil
			}
			go func() { _ = clipboard.WriteAll(line) }()
			var lines []string
			if kind == relay.CodeAnchor {
				lines = []string{i18n.T("Auf dem Server, der erreichbar werden soll (gilt 10 Minuten, einmal):"), "",
					"  sshtermy anker koppeln '" + line + "'", "",
					i18n.T("Danach: sshtermy anker dienst | sudo tee /etc/systemd/system/sshtermy-anker.service"),
					i18n.T("        sudo systemctl enable --now sshtermy-anker")}
			} else {
				lines = []string{i18n.T("Auf dem anderen Rechner in sshTermy: Palette › „Vermittler koppeln“ › einfügen (gilt 10 Minuten, einmal):"), "", "  " + line}
			}
			lines = append(lines, "", i18n.T("(in der Zwischenablage)"))
			m := textModal(i18n.T("Kopplungszeile · ")+r.Name, lines)
			m.copy = line
			a.modal = m
			return nil
		}}
	}
}
