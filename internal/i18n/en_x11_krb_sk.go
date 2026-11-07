package i18n

// X11-Weiterleitung, Kerberos/GSSAPI und FIDO2-Sicherheitsschlüssel
// (sshx/x11.go, sshx/gssapi*.go, sshx/sk.go). Eigene Datei gegen Merge-Konflikte.
func init() {
	for k, v := range enX11KrbSK {
		en[k] = v
	}
}

var enX11KrbSK = map[string]string{
	"Kanal schon belegt": "channel already in use",
	"Kerberos-Cache %q wird nicht unterstützt, bitte kinit -c FILE:/tmp/krb5cc_%d": "Kerberos cache %q is not supported, please run kinit -c FILE:/tmp/krb5cc_%d",
	"Kerberos: ": "Kerberos: ",
	"Passphrase des Sicherheitsschlüssels falsch":      "wrong security key passphrase",
	"Sicherheitsschlüssel berühren …":                  "Touch your security key …",
	"Sicherheitsschlüssel nach ssh-add nicht im Agent": "security key not in the agent after ssh-add",
	"X11 weiterleiten":                                 "Forward X11",
	"agent = ssh-agent / Windows OpenSSH-Agent (empfohlen) · kerberos = Ticket der Firmen-Domäne (Windows-Anmeldung oder kinit)": "agent = ssh-agent / Windows OpenSSH agent (recommended) · kerberos = company domain ticket (Windows sign-in or kinit)",
	"kein Kerberos-Ticket (erst kinit): %w":                                                                         "no Kerberos ticket (run kinit first): %w",
	"kein lokaler X-Server (DISPLAY leer)":                                                                          "no local X server (DISPLAY is empty)",
	"ssh-add (OpenSSH) nicht gefunden, es wird für Sicherheitsschlüssel gebraucht":                                  "ssh-add (OpenSSH) not found, it is needed for security keys",
	"ssh-agent (OpenSSH) nicht gefunden, er wird für Sicherheitsschlüssel gebraucht":                                "ssh-agent (OpenSSH) not found, it is needed for security keys",
	"ssh-agent startet nicht":                                                                                       "ssh-agent does not start",
	"vom Server abgelehnt (X11Forwarding no oder xauth fehlt dort?)":                                                "rejected by the server (X11Forwarding no, or xauth missing there?)",
	"wie ssh -X: grafische Programme vom Server hier zeigen · braucht lokal einen X-Server (XQuartz, VcXsrv, WSLg)": "like ssh -X: show graphical programs from the server here · needs a local X server (XQuartz, VcXsrv, WSLg)",
}
