package syncx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/db9979/sshtermy/internal/i18n"
)

// Live-Test für den Einrichtungsassistenten: jeder Schritt meldet, was
// geklappt hat, und bei einem Fehler einen konkreten Hinweis, was zu tun ist.

type StepState int

const (
	StepOK StepState = iota
	StepWarn
	StepFail
)

type Step struct {
	Title  string
	State  StepState
	Detail string
	Hint   string
}

type Report struct {
	Steps        []Step
	URL          string     // gefundene WebDAV-Ordner-URL
	RemoteRaw    []byte     // vorhandene Sync-Datei (für die Passwortprüfung)
	RemoteExists bool       // Sync-Datei gibt es schon (anderer Rechner)
	Cert         *CertError // selbstsigniertes Zertifikat: kann gepinnt werden
}

func (r *Report) OK() bool {
	for _, s := range r.Steps {
		if s.State == StepFail {
			return false
		}
	}
	return len(r.Steps) > 0
}

func (r *Report) add(title string, st StepState, detail, hint string) {
	r.Steps = append(r.Steps, Step{title, st, detail, hint})
}

func randName() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return ".sshtermy-test-" + hex.EncodeToString(b)
}

// ---------------------------------------------------------------- Ordner

// ExpandHome ersetzt ~ am Anfang durch das Benutzerverzeichnis.
func ExpandHome(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[min(len(p), 2):])
		}
	}
	return p
}

// SuggestFolders: Ordner gängiger Sync-Dienste, die es auf diesem Rechner gibt
// (jeweils mit Unterordner „sshtermy“).
func SuggestFolders() []string {
	home, _ := os.UserHomeDir()
	var cands []string
	for _, env := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
		if v := os.Getenv(env); v != "" {
			cands = append(cands, v)
		}
	}
	if home != "" {
		if runtime.GOOS == "darwin" {
			cands = append(cands, filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs"))
			if ents, err := os.ReadDir(filepath.Join(home, "Library", "CloudStorage")); err == nil {
				for _, e := range ents {
					cands = append(cands, filepath.Join(home, "Library", "CloudStorage", e.Name()))
				}
			}
		}
		for _, n := range []string{"OneDrive", "Dropbox", "Nextcloud", "Sync", "Google Drive", "pCloudDrive", "Seafile"} {
			cands = append(cands, filepath.Join(home, n))
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() && !seen[c] {
			seen[c] = true
			out = append(out, filepath.Join(c, "sshtermy"))
		}
	}
	return out
}

// detectService rät am Pfad, welcher Dienst den Ordner abgleicht.
func detectService(dir string) string {
	l := strings.ToLower(filepath.ToSlash(dir))
	for _, s := range []struct{ key, name string }{
		{"onedrive", "OneDrive"}, {"dropbox", "Dropbox"}, {"com~apple~clouddocs", "iCloud Drive"},
		{"icloud", "iCloud Drive"}, {"google drive", "Google Drive"}, {"googledrive", "Google Drive"},
		{"nextcloud", "Nextcloud"}, {"owncloud", "ownCloud"}, {"pcloud", "pCloud"}, {"seafile", "Seafile"},
		{"synologydrive", "Synology Drive"}, {"synology drive", "Synology Drive"}, {"qsync", "QNAP Qsync"},
	} {
		if strings.Contains(l, s.key) {
			return s.name
		}
	}
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".stfolder")); err == nil {
			return "Syncthing"
		}
		p := filepath.Dir(d)
		if p == d {
			break
		}
		d = p
	}
	if strings.HasPrefix(dir, `\\`) || strings.HasPrefix(l, "/volumes/") || strings.HasPrefix(l, "/mnt/") {
		return i18n.T("Netzlaufwerk")
	}
	return ""
}

// CheckFolder prüft einen Sync-Ordner.
func CheckFolder(ctx context.Context, dir string) *Report {
	r := &Report{}
	dir = ExpandHome(dir)
	if dir == "" {
		r.add(i18n.T("Ordner"), StepFail, i18n.T("kein Ordner angegeben"), i18n.T("Wähle einen Ordner in OneDrive, Dropbox, iCloud Drive, Nextcloud oder Syncthing."))
		return r
	}
	if !filepath.IsAbs(dir) {
		r.add(i18n.T("Ordner"), StepFail, dir, i18n.T("Bitte einen vollständigen Pfad angeben, z. B. ")+examplePath())
		return r
	}
	if st, err := os.Stat(dir); err == nil && !st.IsDir() {
		r.add(i18n.T("Ordner"), StepFail, dir+i18n.T(" ist eine Datei"), i18n.T("Bitte einen Ordner angeben."))
		return r
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			r.add(i18n.T("Ordner"), StepFail, err.Error(), i18n.T("Ordner konnte nicht angelegt werden. Gibt es das übergeordnete Laufwerk, ist der Cloud-Client angemeldet?"))
			return r
		}
		r.add(i18n.T("Ordner"), StepOK, i18n.T("angelegt: ")+dir, "")
	} else if err != nil {
		r.add(i18n.T("Ordner"), StepFail, err.Error(), i18n.T("Kein Zugriff auf den Ordner."))
		return r
	} else {
		r.add(i18n.T("Ordner"), StepOK, dir, "")
	}

	test := filepath.Join(dir, randName())
	data := []byte("sshtermy")
	if err := os.WriteFile(test, data, 0o600); err != nil {
		r.add(i18n.T("Schreiben & Lesen"), StepFail, err.Error(), i18n.T("Keine Schreibrechte in diesem Ordner. Anderen Ordner wählen oder Rechte prüfen."))
		return r
	}
	back, err := os.ReadFile(test)
	_ = os.Remove(test)
	if err != nil || !bytes.Equal(back, data) {
		r.add(i18n.T("Schreiben & Lesen"), StepFail, i18n.T("Testdatei kam anders zurück"), i18n.T("Der Ordner verhält sich ungewöhnlich (Cloud-Platzhalter?). In OneDrive „Immer auf diesem Gerät behalten“ wählen."))
		return r
	}
	r.add(i18n.T("Schreiben & Lesen"), StepOK, i18n.T("Testdatei geschrieben, gelesen, gelöscht"), "")

	if svc := detectService(dir); svc != "" {
		r.add("Sync-Dienst", StepOK, svc+i18n.T(" erkannt"), "")
	} else {
		r.add("Sync-Dienst", StepWarn, i18n.T("kein Sync-Dienst erkannt"),
			i18n.T("Das klappt nur, wenn dieser Ordner auf allen Rechnern abgeglichen wird (OneDrive, Dropbox, iCloud, Syncthing, Netzlaufwerk). Für ein NAS ist „NAS / WebDAV“ meist einfacher."))
	}
	checkSyncFile(ctx, r, Folder{Dir: dir})
	return r
}

func examplePath() string {
	switch runtime.GOOS {
	case "windows":
		return `C:\Users\Name\OneDrive\sshtermy`
	case "darwin":
		return "~/Library/Mobile Documents/com~apple~CloudDocs/sshtermy"
	}
	return "~/Nextcloud/sshtermy"
}

func checkSyncFile(ctx context.Context, r *Report, b Backend) {
	raw, _, err := b.Read(ctx)
	switch {
	case errors.Is(err, ErrNotFound):
		r.add("Sync-Datei", StepOK, i18n.T("noch keine vorhanden: wird mit einem neuen Sync-Passwort angelegt"), "")
	case err != nil:
		r.add("Sync-Datei", StepFail, err.Error(), i18n.T("Die vorhandene Sync-Datei konnte nicht gelesen werden."))
	default:
		if _, err := peek(raw); err != nil {
			r.add("Sync-Datei", StepFail, err.Error(), i18n.T("Im Ordner liegt eine fremde oder beschädigte ")+FileName+i18n.T(". Umbenennen oder anderen Ordner wählen."))
			return
		}
		r.RemoteRaw, r.RemoteExists = raw, true
		r.add("Sync-Datei", StepOK, i18n.T("vorhanden (von einem anderen Rechner)"), i18n.T("Gib unten das Sync-Passwort ein, das du dort vergeben hast."))
	}
}

// ---------------------------------------------------------------- WebDAV / NAS

// Presets für gängige Geräte.
var Presets = []string{"synology", "qnap", "nextcloud", "andere"}

// PresetLabel: Anzeigename.
func PresetLabel(p string) string {
	return map[string]string{"synology": "Synology", "qnap": "QNAP", "nextcloud": "Nextcloud", "andere": i18n.T("Andere")}[p]
}

// DefaultRemote: sinnvoller Zielordner je Gerät.
func DefaultRemote(preset string) string {
	switch preset {
	case "synology", "qnap":
		return "/home/sshtermy"
	case "nextcloud":
		return "sshtermy"
	}
	return "/sshtermy"
}

type candidate struct {
	scheme, host, port string
}

// Candidates: Adressen, die für die Eingabe in Frage kommen (in dieser Reihenfolge).
func candidates(preset, addr string) ([]candidate, string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, "", errors.New(i18n.T("keine Adresse angegeben"))
	}
	scheme, path := "", ""
	if i := strings.Index(addr, "://"); i >= 0 {
		u, err := url.Parse(addr)
		if err != nil || u.Host == "" {
			return nil, "", fmt.Errorf(i18n.T("Adresse %q nicht lesbar"), addr)
		}
		scheme, addr, path = strings.ToLower(u.Scheme), u.Host, u.EscapedPath()
		if scheme != "http" && scheme != "https" {
			return nil, "", fmt.Errorf(i18n.T("nur http:// oder https:// möglich, nicht %s://"), scheme)
		}
	} else if i := strings.IndexAny(addr, "/"); i >= 0 {
		addr, path = addr[:i], addr[i:]
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = strings.Trim(addr, "[]"), ""
	}
	if host == "" {
		return nil, "", errors.New(i18n.T("kein Hostname in der Adresse"))
	}
	var out []candidate
	add := func(s, p string) {
		if scheme == "" || scheme == s {
			out = append(out, candidate{s, host, p})
		}
	}
	switch {
	case port != "":
		add("https", port)
		add("http", port)
	case preset == "synology":
		add("https", "5006")
		add("http", "5005")
		add("https", "443")
	case preset == "qnap":
		add("https", "8081")
		add("https", "443")
		add("http", "8080")
		add("http", "80")
	default:
		add("https", "443")
		add("http", "80")
	}
	return out, path, nil
}

func (c candidate) url(path string) string {
	h := c.host
	if strings.Contains(h, ":") {
		h = "[" + h + "]"
	}
	def := (c.scheme == "https" && c.port == "443") || (c.scheme == "http" && c.port == "80")
	if !def {
		h += ":" + c.port
	}
	return c.scheme + "://" + h + path
}

// FolderPath baut den Pfad des Sync-Ordners auf dem Server.
func folderPath(preset, urlPath, user, remote string) string {
	if urlPath != "" && urlPath != "/" {
		return strings.TrimSuffix(urlPath, "/") + "/"
	}
	if remote == "" {
		remote = DefaultRemote(preset)
	}
	segs := []string{}
	for _, s := range strings.Split(strings.ReplaceAll(remote, `\`, "/"), "/") {
		if s != "" {
			segs = append(segs, url.PathEscape(s))
		}
	}
	p := "/" + strings.Join(segs, "/")
	if preset == "nextcloud" {
		p = "/remote.php/dav/files/" + url.PathEscape(user) + p
	}
	return strings.TrimSuffix(p, "/") + "/"
}

// WebDAVInput: Eingaben aus dem Assistenten.
type WebDAVInput struct {
	Preset, Address, User, Password, Remote, CertPin string
}

func authHint(preset string) string {
	switch preset {
	case "synology":
		return i18n.T("Benutzer oder Passwort falsch, oder WebDAV ist für dieses Konto nicht erlaubt (DSM › Systemsteuerung › Benutzer › Bearbeiten › Anwendungen › WebDAV). Achtung: nach mehreren Fehlversuchen sperrt DSM deine IP (Systemsteuerung › Sicherheit › Schutz › Auto-Block).")
	case "qnap":
		return i18n.T("Benutzer oder Passwort falsch. QTS › Systemsteuerung › Benutzer: Kennwort prüfen; der Benutzer braucht Rechte am Freigabeordner. Achtung: nach mehreren Fehlversuchen sperrt QTS deine IP (Sicherheit › Zugriffsschutz).")
	case "nextcloud":
		return i18n.T("Anmeldung abgelehnt. Mit 2-Faktor-Anmeldung ein App-Passwort anlegen: Nextcloud › Einstellungen › Sicherheit › „Neues App-Passwort erstellen“.")
	}
	return i18n.T("Benutzer oder Passwort falsch.")
}

func portHint(preset string) string {
	switch preset {
	case "synology":
		return i18n.T("Auf der DiskStation: Paket-Zentrum › „WebDAV Server“ installieren und öffnen › HTTPS aktivieren (Port 5006). Firewall in DSM › Sicherheit prüfen.")
	case "qnap":
		return i18n.T("Im QTS: Systemsteuerung › Netzwerk- & Dateidienste › Win/Mac/NFS/WebDAV › Reiter WebDAV › „WebDAV aktivieren“. Anwendungen › Webserver muss laufen (HTTP 80 / HTTPS 8081).")
	case "nextcloud":
		return i18n.T("Läuft Nextcloud unter dieser Adresse? Gib im Zweifel die volle Adresse an, die du im Browser nutzt, z. B. https://cloud.example.de.")
	}
	return i18n.T("WebDAV-Dienst läuft nicht auf diesem Port. Gib die volle URL mit Port an, z. B. https://server:8443/dav/.")
}

func permHint(preset string) string {
	switch preset {
	case "synology":
		return i18n.T("Keine Berechtigung. DSM › Systemsteuerung › Benutzer › Bearbeiten: „Anwendungen › WebDAV“ erlauben und Lese-/Schreibrecht für den freigegebenen Ordner geben.")
	case "qnap":
		return i18n.T("Keine Berechtigung. QTS › Freigabeordner › Rechte bearbeiten: Lese-/Schreibrecht geben. Bei WebDAV „Freigabeordner-Berechtigungen verwenden“ wählen.")
	}
	return i18n.T("Keine Berechtigung für diesen Ordner. Lese-/Schreibrecht am Server vergeben.")
}

func parentHint(preset string) string {
	switch preset {
	case "synology":
		return i18n.T("Der Pfad muss mit einem freigegebenen Ordner beginnen. „/home/…“ geht nur, wenn der Benutzer-Home-Dienst aktiv ist (DSM › Benutzer › Erweitert). Sonst z. B. /Freigabename/sshtermy.")
	case "qnap":
		return i18n.T("Der Pfad muss mit einem Freigabeordner beginnen, z. B. /home/sshtermy oder /Public/sshtermy.")
	case "nextcloud":
		return i18n.T("Ordner relativ zu deinen Dateien angeben, z. B. sshtermy oder Backups/sshtermy.")
	}
	return i18n.T("Den übergeordneten Ordner gibt es nicht. Pfad prüfen.")
}

func netHint(err error, preset string) (detail, hint string) {
	var ne net.Error
	switch {
	case errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "refused"):
		return i18n.T("Verbindung abgelehnt"), portHint(preset)
	case errors.As(err, &ne) && ne.Timeout():
		return i18n.T("keine Antwort (Zeitüberschreitung)"), i18n.T("Firewall, falsches Netz oder Gerät aus? Von unterwegs braucht ein NAS im Heimnetz VPN, Tailscale oder QuickConnect-/myQNAPcloud-Adresse.")
	}
	return err.Error(), portHint(preset)
}

func dial(ctx context.Context, c candidate) error {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(c.host, c.port))
	if err == nil {
		conn.Close()
	}
	return err
}

// CheckWebDAV prüft die NAS-/WebDAV-Einstellungen Schritt für Schritt und
// sucht dabei selbst den richtigen Port und das Protokoll.
func CheckWebDAV(ctx context.Context, in WebDAVInput) *Report {
	r := &Report{}
	cands, urlPath, err := candidates(in.Preset, in.Address)
	if err != nil {
		r.add(i18n.T("Adresse"), StepFail, err.Error(), i18n.T("Name oder IP des NAS eingeben, z. B. diskstation, 192.168.1.20 oder https://nas.local:5006."))
		return r
	}
	host := cands[0].host

	// 1) Name auflösen
	if net.ParseIP(host) == nil {
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ips, err := net.DefaultResolver.LookupHost(rctx, host)
		cancel()
		if err != nil || len(ips) == 0 {
			hint := i18n.T("Name nicht gefunden. Schreibweise prüfen oder die IP-Adresse nehmen")
			switch in.Preset {
			case "synology":
				hint += i18n.T(" (Synology Assistant oder der Router zeigen sie an)")
			case "qnap":
				hint += i18n.T(" (Qfinder Pro oder der Router zeigen sie an)")
			}
			if !strings.Contains(host, ".") {
				hint += i18n.T("; im Heimnetz hilft oft „") + host + i18n.T(".local“ oder „") + host + i18n.T(".fritz.box“")
			} else {
				hint += i18n.T(". Von unterwegs ist das Heimnetz nur per VPN/Tailscale erreichbar")
			}
			hint += "."
			r.add(i18n.T("Name auflösen"), StepFail, host+i18n.T(": nicht gefunden"), hint)
			return r
		}
		r.add(i18n.T("Name auflösen"), StepOK, host+" → "+ips[0], "")
	}

	// 2) Port und Protokoll suchen: erster offener Port, auf dem bei https
	// auch wirklich TLS spricht (sonst nächster Kandidat, z. B. http).
	var found *candidate
	var tlsErr error
	var lastErr error
	var tried []string
	for i := range cands {
		c := cands[i]
		tried = append(tried, c.scheme+":"+c.port)
		if err := dial(ctx, c); err != nil {
			lastErr = err
			continue
		}
		if c.scheme == "https" {
			probe := &WebDAV{CertPin: in.CertPin}
			if err := tlsProbe(ctx, probe, c); err != nil {
				var rh tls.RecordHeaderError
				if errors.As(err, &rh) || strings.Contains(err.Error(), "first record does not look like a TLS handshake") {
					lastErr = fmt.Errorf(i18n.T("Port %s spricht kein https"), c.port)
					continue
				}
				found, tlsErr = &c, err
				break
			}
		}
		found = &c
		break
	}
	if found == nil {
		d, h := netHint(lastErr, in.Preset)
		r.add(i18n.T("Server erreichbar"), StepFail, d+i18n.T(" (versucht: ")+strings.Join(tried, ", ")+")", h)
		return r
	}
	base := found.url(folderPath(in.Preset, urlPath, in.User, in.Remote))
	r.URL = base
	r.add(i18n.T("Server erreichbar"), StepOK, found.scheme+"://"+found.host+":"+found.port, "")

	w := &WebDAV{URL: base, User: in.User, Password: in.Password, CertPin: in.CertPin}

	// 3) TLS
	if found.scheme == "https" {
		if err := tlsErr; err != nil {
			var ce *CertError
			if errors.As(err, &ce) {
				if ce.Pin != "" {
					r.Cert = ce
					r.add(i18n.T("Zertifikat"), StepFail, ce.Reason,
						i18n.T("Typisch bei einem NAS. Fingerprint mit der NAS-Oberfläche vergleichen (DSM › Sicherheit › Zertifikat bzw. QTS › Sicherheit › SSL-Zertifikat) und dann vertrauen:\n")+ce.Fingerprint)
				} else {
					r.add(i18n.T("Zertifikat"), StepFail, ce.Reason, i18n.T("Das Zertifikat des Servers ist nicht mehr das, dem du vertraut hast. Wenn du es selbst erneuert hast: Assistent neu testen und erneut vertrauen. Sonst: Vorsicht, möglicher Angriff."))
				}
				return r
			}
			r.add(i18n.T("Zertifikat"), StepFail, err.Error(), i18n.T("TLS-Verbindung fehlgeschlagen. Ist auf diesem Port wirklich HTTPS aktiv?"))
			return r
		}
		if w.CertPin != "" {
			r.add(i18n.T("Zertifikat"), StepOK, i18n.T("vertrautes eigenes Zertifikat (Fingerprint gespeichert)"), "")
		} else {
			r.add(i18n.T("Zertifikat"), StepOK, i18n.T("gültig"), "")
		}
	} else {
		hint := i18n.T("Die Sync-Datei ist trotzdem Ende-zu-Ende verschlüsselt, aber dein NAS-Passwort geht im Klartext durchs Netz. Besser HTTPS aktivieren")
		if in.Preset == "synology" {
			hint += i18n.T(" (WebDAV Server › HTTPS, Port 5006).")
		} else {
			hint += "."
		}
		r.add(i18n.T("Verschlüsselung"), StepWarn, i18n.T("nur http (unverschlüsselt)"), hint)
	}

	// 4) Anmeldung und Ordner
	if in.User == "" {
		r.add(i18n.T("Anmeldung"), StepFail, i18n.T("kein Benutzer angegeben"), i18n.T("Benutzername und Passwort deines NAS-Kontos eintragen."))
		return r
	}
	code, html, err := w.propfind(ctx, base)
	if err != nil {
		d, h := netHint(err, in.Preset)
		r.add(i18n.T("Anmeldung"), StepFail, d, h)
		return r
	}
	switch {
	case code == 207:
		r.add(i18n.T("Anmeldung & Ordner"), StepOK, i18n.T("angemeldet als ")+in.User+i18n.T(", Ordner vorhanden"), "")
	case code == 401:
		r.add(i18n.T("Anmeldung"), StepFail, i18n.T("abgelehnt (HTTP 401)"), authHint(in.Preset))
		return r
	case code == 403:
		r.add(i18n.T("Anmeldung"), StepFail, i18n.T("verboten (HTTP 403)"), permHint(in.Preset))
		return r
	case code == 404:
		mc, err := w.mkcol(ctx, base)
		switch {
		case err != nil:
			r.add(i18n.T("Ordner anlegen"), StepFail, err.Error(), "")
			return r
		case mc == 201:
			r.add(i18n.T("Anmeldung & Ordner"), StepOK, i18n.T("angemeldet als ")+in.User+i18n.T(", Ordner angelegt"), "")
		case mc == 409:
			r.add(i18n.T("Ordner anlegen"), StepFail, i18n.T("übergeordneter Ordner fehlt (HTTP 409)"), parentHint(in.Preset))
			return r
		case mc == 401:
			r.add(i18n.T("Anmeldung"), StepFail, i18n.T("abgelehnt (HTTP 401)"), authHint(in.Preset))
			return r
		case mc == 403 || mc == 405:
			r.add(i18n.T("Ordner anlegen"), StepFail, fmt.Sprintf(i18n.T("nicht erlaubt (HTTP %d)"), mc), permHint(in.Preset)+i18n.T(" Alternativ den Ordner selbst anlegen."))
			return r
		default:
			r.add(i18n.T("Ordner anlegen"), StepFail, fmt.Sprintf("HTTP %d", mc), parentHint(in.Preset))
			return r
		}
	case html || code == 405 || code == 501 || code == 200:
		r.add("WebDAV", StepFail, fmt.Sprintf(i18n.T("unter dieser Adresse antwortet kein WebDAV (HTTP %d)"), code),
			i18n.T("Vermutlich die Weboberfläche des NAS statt des WebDAV-Dienstes. ")+portHint(in.Preset))
		return r
	default:
		r.add(i18n.T("Anmeldung"), StepFail, fmt.Sprintf(i18n.T("unerwartete Antwort HTTP %d"), code), portHint(in.Preset))
		return r
	}

	// 5) Schreiben, Lesen, Löschen
	test := w.fileURL(randName())
	data := []byte("sshtermy")
	resp, _, err := w.do(ctx, "PUT", test, data, nil)
	if err != nil || (resp.StatusCode != 201 && resp.StatusCode != 204 && resp.StatusCode != 200) {
		var detail string
		hint := permHint(in.Preset)
		if err != nil {
			detail = err.Error()
		} else {
			detail = fmt.Sprintf(i18n.T("Schreiben: HTTP %d"), resp.StatusCode)
			if resp.StatusCode == 507 {
				hint = i18n.T("Der Speicher bzw. das Kontingent auf dem Server ist voll.")
			}
		}
		r.add(i18n.T("Schreiben & Lesen"), StepFail, detail, hint)
		return r
	}
	resp, back, err := w.do(ctx, "GET", test, nil, nil)
	_ = w.del(ctx, test)
	if err != nil || resp.StatusCode != 200 || !bytes.Equal(back, data) {
		r.add(i18n.T("Schreiben & Lesen"), StepFail, i18n.T("Testdatei kam nicht unverändert zurück"), i18n.T("Ein Proxy oder Virenscanner verändert die Daten? Anderen Ordner probieren."))
		return r
	}
	r.add(i18n.T("Schreiben & Lesen"), StepOK, i18n.T("Testdatei geschrieben, gelesen, gelöscht"), "")

	checkSyncFile(ctx, r, w)
	return r
}

func tlsProbe(ctx context.Context, w *WebDAV, c candidate) error {
	cfg := w.http().Transport.(*http.Transport).TLSClientConfig.Clone()
	cfg.ServerName = c.host
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: cfg}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(c.host, c.port))
	if err != nil {
		return classifyNetErr(err)
	}
	return conn.Close()
}
