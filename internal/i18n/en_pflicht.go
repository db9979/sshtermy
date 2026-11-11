package i18n

// Pflichtpunkte 2026-11-11: Schutz je Verbindung, Syslog-Absender,
// unsignierte Builds, Kerberos bei WinRM.
func init() {
	for k, v := range map[string]string{
		"Schutz":     "Protection",
		"Schutz aus": "Protection off",
		"Standard an: riskante Befehle fragen vor ⏎, der Agent fragt bei Änderungen · aus nur für Test- und Laborsysteme": "On by default: risky commands ask before ⏎, the agent asks before changes · turn off only for test and lab systems",
		"Update ohne Signaturprüfung?":              "Update without signature check?",
		"j = trotzdem installieren · n = abbrechen": "y = install anyway · n = cancel",
		"▲ Dieser Build prüft die Signatur von Updates nicht (selbst gebaut, ohne UPDATE_PUBKEY).\n":                                                              "▲ This build does not check the signature of updates (self-built, without UPDATE_PUBKEY).\n",
		"  Geprüft wird nur die Prüfsumme; wer das GitHub-Konto übernimmt, könnte ein Update unterschieben.\n":                                                    "  Only the checksum is checked; whoever takes over the GitHub account could slip in an update.\n",
		"  Sicherer: die offizielle Version installieren (github.com/db9979/sshtermy/releases).":                                                                  "  Safer: install the official version (github.com/db9979/sshtermy/releases).",
		"Warnung: Dieser Build prüft die Signatur von Updates nicht (selbst gebaut, ohne UPDATE_PUBKEY). Nur die Prüfsumme wird geprüft.":                         "Warning: this build does not check the signature of updates (self-built, without UPDATE_PUBKEY). Only the checksum is checked.",
		"Syslog auch von unbekannten Absendern annehmen":                                                                                                          "Accept syslog from unknown senders too",
		"Syslog nur noch von gespeicherten Verbindungen.":                                                                                                         "Syslog only from saved connections now.",
		"Syslog von %s verworfen: gehört zu keiner Verbindung. Verbindung anlegen oder in den Einstellungen „Syslog auch von unbekannten Absendern“ einschalten.": "Syslog from %s dropped: it belongs to no connection. Add a connection or turn on “Accept syslog from unknown senders too” in the settings.",
		"Syslog von allen Absendern: auch Geräte ohne Verbindung landen in der Logwache (Absender lassen sich fälschen).":                                         "Syslog from all senders: devices without a connection reach the log watch too (senders can be spoofed).",
		"unbekannte Absender verworfen":  "unknown senders dropped",
		"unbekannter Absender verworfen": "unknown sender dropped",
		"Kerberos braucht den Rechnernamen (z. B. server.firma.local), keine IP-Adresse":                                                                     "Kerberos needs the computer name (e.g. server.corp.local), not an IP address",
		"Kerberos: Der Domänencontroller kennt HTTP/<Rechnername> nicht. Den vollen Rechnernamen aus dem AD eintragen (kein Alias/CNAME).":                   "Kerberos: the domain controller does not know HTTP/<computer name>. Enter the full computer name from AD (no alias/CNAME).",
		"Kerberos: Domäne unbekannt, Benutzer als name@firma.local angeben":                                                                                  "Kerberos: domain unknown, enter the user as name@corp.local",
		"Kerberos: kein Domänencontroller erreichbar. DNS der Domäne nötig (SRV _kerberos._tcp) oder eine krb5.conf (KRB5_CONFIG).":                          "Kerberos: no domain controller reachable. Needs the domain's DNS (SRV _kerberos._tcp) or a krb5.conf (KRB5_CONFIG).",
		"negotiate = NTLM, auch Domänenkonten (Standard) · kerberos = Domäne ohne NTLM, Rechnername statt IP · basic = nur lokale Konten und nur über HTTPS": "negotiate = NTLM, domain accounts too (default) · kerberos = domain without NTLM, computer name instead of IP · basic = local accounts only and only over HTTPS",
		"„Schutz“ aus: nur Test-/Laborsysteme":                      "“Protection” off: test/lab systems only",
		"Schutz an: Datei anlegen fragt nach":                       "Protection on: creating a file asks first",
		"Schutz an: Änderungen fragen nach":                         "Protection on: changes ask first",
		"▲ Schutz an: jede Änderung fragt nach (Formular › Schutz)": "▲ Protection on: every change asks first (form › Protection)",
	} {
		en[k] = v
	}
}
