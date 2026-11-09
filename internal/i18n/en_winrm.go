package i18n

// Texte für PowerShell über WinRM (eigene Datei, damit parallele Änderungen
// an en.go nicht kollidieren).
var enWinRM = map[string]string{
	"Angemeldet, aber PowerShell startet drüben nicht": "Signed in, but PowerShell does not start on the server",
	"Anmeldung abgelehnt: Benutzer/Passwort prüfen (Domäne als DOMÄNE\\name oder name@domäne). Lokale Konten brauchen auf dem Server oft LocalAccountTokenFilterPolicy=1.": "Sign-in rejected: check user/password (domain as DOMAIN\\name or name@domain). Local accounts often need LocalAccountTokenFilterPolicy=1 on the server.",
	"Basic-Anmeldung nur über HTTPS (sonst geht das Passwort lesbar übers Netz)":                                                                                           "Basic sign-in only over HTTPS (otherwise the password crosses the network readable)",
	"Befehl abgebrochen · PowerShell neu gestartet (Variablen sind weg, Ordner bleibt)":                                                                                    "Command canceled · PowerShell restarted (variables are gone, folder stays)",
	"HTTP, Nachrichten verschlüsselt": "HTTP, messages encrypted",
	"Passwort wird gefragt":           "password is asked for",
	"Port 5986 mit TLS · aus = 5985, dann verschlüsselt NTLM jede Nachricht": "Port 5986 with TLS · off = 5985, NTLM then encrypts every message",
	"PowerShell (WinRM)":   "PowerShell (WinRM)",
	"PowerShell-Sitzung: ": "PowerShell session: ",
	"Standard 22 · Telnet 23 · WinRM 5985 (HTTP) / 5986 (HTTPS)": "Default 22 · Telnet 23 · WinRM 5985 (HTTP) / 5986 (HTTPS)",
	"Telnet = unverschlüsselt (ältere Switches) · Seriell = USB-Konsolenkabel · Lokal = Shell auf diesem Rechner · PowerShell (WinRM) = Windows-Server wie Enter-PSSession":           "Telnet = unencrypted (older switches) · Serial = USB console cable · Local = shell on this computer · PowerShell (WinRM) = Windows server like Enter-PSSession",
	"Telnet/Konsole: optional, wird eingetippt, sobald das Gerät fragt · WinRM: DOMÄNE\\name oder name@domäne":                                                                        "Telnet/console: optional, typed as soon as the device asks · WinRM: DOMAIN\\name or name@domain",
	"Unerwartete Antwort: passt HTTP/HTTPS zum Port? 5985 = HTTP, 5986 = HTTPS.":                                                                                                      "Unexpected answer: does HTTP/HTTPS match the port? 5985 = HTTP, 5986 = HTTPS.",
	"WinRM antwortet nicht auf Port %d. Auf dem Server (als Administrator): Enable-PSRemoting -Force · Firewall-Regel „Windows-Remoteverwaltung“ prüfen.":                             "WinRM does not answer on port %d. On the server (as administrator): Enable-PSRemoting -Force · check the “Windows Remote Management” firewall rule.",
	"Zertifikat des Servers ist nicht vertrauenswürdig (oft selbst signiert). In der Verbindung „Zertifikat nicht prüfen“ einschalten oder ein Zertifikat der eigenen CA einrichten.": "The server certificate is not trusted (often self-signed). Turn on “Don't check certificate” in the connection or install a certificate from your own CA.",
	"Zertifikat nicht geprüft": "certificate not checked",
	"Zertifikat nicht prüfen":  "Don't check certificate",
	"negotiate = NTLM, auch Domänenkonten (Standard) · basic = nur lokale Konten und nur über HTTPS": "negotiate = NTLM, domain accounts too (default) · basic = local accounts only and only over HTTPS",
	"nur für selbst signierte Zertifikate im eigenen Netz":                                           "only for self-signed certificates in your own network",
}

func init() {
	for k, v := range enWinRM {
		en[k] = v
	}
}
