package sshx

import "strings"

// Kerberos (gssapi-with-mic, RFC 4462)
//
// Windows: SSPI mit dem Ticket der Windows-Anmeldung (Domänen-Konto, kein
// kinit nötig). Linux/macOS: das Ticket aus dem Credential-Cache von kinit
// (KRB5CCNAME bzw. /tmp/krb5cc_<uid>), KDC aus /etc/krb5.conf oder per DNS.
// sshTermy bittet nicht um Weitergabe des Tickets an den Server
// (keine Delegation, wie GSSAPIDelegateCredentials no).

// KerberosSPN: Dienstname des Servers, wie OpenSSH ihn nutzt (host/<name>).
// Der Name muss der sein, unter dem der Server im KDC steht (keine
// IP-Adresse).
func KerberosSPN(host string) string {
	return "host/" + strings.TrimSuffix(strings.ToLower(host), ".")
}
