package psremote

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/masterzen/winrm"
)

// Kerberos für Server, die NTLM abgeschaltet haben (nur Kerberos).
// gokrb5 (reines Go) holt das Ticket selbst: mit Benutzer und Passwort oder
// – auf Linux/macOS – aus dem Cache von kinit. Die Windows-Anmeldung (SSPI)
// erreicht diese Bibliothek nicht; unter Windows fragt sshTermy deshalb
// nach dem Passwort.
//
// Realm: aus name@realm, sonst aus dem Rechnernamen (server.corp.example →
// CORP.EXAMPLE), sonst default_realm aus krb5.conf. Die KDCs findet gokrb5
// über DNS (SRV _kerberos._tcp), wenn keine krb5.conf da ist. Über HTTP
// verschlüsselt Kerberos jede Nachricht, über HTTPS das TLS.

// AuthKerberos: Kerberos (Domäne, z. B. wenn NTLM gesperrt ist).
const AuthKerberos = "kerberos"

// ErrKerberosIP: Kerberos braucht den Rechnernamen, mit IP gibt es kein Ticket.
var ErrKerberosIP = errors.New("Kerberos braucht den Rechnernamen (z. B. server.firma.local), keine IP-Adresse")

// ErrKerberosSPN: der Domänencontroller kennt den Dienst HTTP/<host> nicht.
var ErrKerberosSPN = errors.New("Kerberos: Dienst unbekannt")

// ErrKerberosKDC: kein Domänencontroller erreichbar.
var ErrKerberosKDC = errors.New("Kerberos: Domänencontroller nicht erreichbar")

// ErrKerberosRealm: Realm nicht bestimmbar.
var ErrKerberosRealm = errors.New("Kerberos: Domäne unbekannt, Benutzer als name@firma.local angeben")

// krbUser: Benutzer und Realm aus „name@realm“, „DOMÄNE\name“ oder „name“.
func krbUser(user, host, defRealm string) (name, realm string) {
	name = strings.TrimSpace(user)
	if i := strings.LastIndex(name, "@"); i > 0 {
		return name[:i], strings.ToUpper(name[i+1:])
	}
	if i := strings.Index(name, `\`); i >= 0 {
		name = name[i+1:] // NetBIOS-Name ist kein Realm
	}
	if net.ParseIP(host) == nil {
		if i := strings.Index(host, "."); i > 0 && i < len(host)-1 {
			return name, strings.ToUpper(host[i+1:])
		}
	}
	return name, strings.ToUpper(defRealm)
}

// krbConfPath: vorhandene krb5.conf oder eine erzeugte mit DNS-Suche.
func krbConfPath(realm string) (string, error) {
	for _, p := range []string{os.Getenv("KRB5_CONFIG"), "/etc/krb5.conf"} {
		if p == "" || runtime.GOOS == "windows" && p == "/etc/krb5.conf" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "sshtermy")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "krb5-"+strings.ToLower(realm)+".conf")
	conf := "[libdefaults]\n default_realm = " + realm + "\n dns_lookup_kdc = true\n dns_lookup_realm = true\n udp_preference_limit = 1\n"
	return p, os.WriteFile(p, []byte(conf), 0o600)
}

// defaultRealm: default_realm aus einer vorhandenen krb5.conf.
func defaultRealm() string {
	for _, p := range []string{os.Getenv("KRB5_CONFIG"), "/etc/krb5.conf"} {
		b, err := os.ReadFile(p)
		if p == "" || err != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
			if ok && strings.TrimSpace(k) == "default_realm" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// ccacheFile: Ticket-Cache von kinit (nur Datei-Caches, nicht unter Windows).
func ccacheFile() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	p := os.Getenv("KRB5CCNAME")
	if p == "" {
		p = "/tmp/krb5cc_" + strconv.Itoa(os.Getuid())
	}
	p = strings.TrimPrefix(p, "FILE:")
	if strings.Contains(p, ":") && !strings.HasPrefix(p, "/") {
		return "" // KCM:, KEYRING: … gehen ohne C-Bibliothek nicht
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// HasKerberosTicket: liegt ein Ticket von kinit bereit? Dann braucht
// Kerberos kein Passwort.
func HasKerberosTicket() bool { return ccacheFile() != "" }

func kerberosTransport(cfg Config) (winrm.Transporter, error) {
	if net.ParseIP(strings.Trim(cfg.Host, "[]")) != nil {
		return nil, ErrKerberosIP
	}
	name, realm := krbUser(cfg.User, cfg.Host, defaultRealm())
	if realm == "" {
		return nil, ErrKerberosRealm
	}
	conf, err := krbConfPath(realm)
	if err != nil {
		return nil, fmt.Errorf("Kerberos: %w", err)
	}
	s := &winrm.Settings{WinRMUsername: name, WinRMPassword: cfg.Password, WinRMHost: cfg.Host, WinRMPort: cfg.Port,
		WinRMProto: map[bool]string{false: "http", true: "https"}[cfg.HTTPS], WinRMInsecure: cfg.Insecure,
		WinRMMessageEncryption: !cfg.HTTPS, KrbRealm: realm, KrbConfig: conf, KrbSpn: "HTTP/" + strings.ToLower(cfg.Host)}
	if cfg.Password == "" {
		s.KrbCCache = ccacheFile()
	}
	return winrm.NewClientKerberos(s), nil
}
