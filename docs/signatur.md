# Updates signieren (einmalig einrichten)

sshTermy aktualisiert sich selbst aus den GitHub-Releases. Ohne Signatur prüft die App nur die
Prüfsummen aus `checksums.txt`. Die liegen aber im selben Release: Wer das GitHub-Konto oder den
Release-Workflow übernimmt, könnte also eine veränderte Version samt passender Prüfsumme ausliefern.
Mit Signatur lehnt die App jedes Update ab, das nicht mit dem privaten Schlüssel signiert wurde,
und dieser Schlüssel liegt nur als Secret im Repository.

## Schritte (ca. 5 Minuten)

1. Schlüsselpaar erzeugen (auf einem eigenen Rechner, braucht Go und git). Der Befehl muss im
   Repository-Ordner laufen, sonst meldet Go „go.mod file not found“. Ohne vorhandenen Klon:

   ```
   git clone --depth 1 https://github.com/db9979/sshtermy /tmp/sshtermy
   cd /tmp/sshtermy && go run ./cmd/sshtermy-sign keygen
   ```

   Ausgegeben werden zwei Zeilen: der **öffentliche** Schlüssel (`UPDATE_PUBKEY`) und der
   **private** Schlüssel (`UPDATE_SIGNING_KEY`).

2. Auf GitHub: **Settings → Secrets and variables → Actions**
   - Reiter **Secrets** → *New repository secret*: Name `UPDATE_SIGNING_KEY`, Wert = privater Schlüssel.
   - Reiter **Variables** → *New repository variable*: Name `UPDATE_PUBKEY`, Wert = öffentlicher Schlüssel.

   Zuerst das Secret, dann die Variable anlegen. Der Release-Workflow bricht ab, wenn nur die
   Variable da ist, weil die neue App sonst jedes spätere Update ablehnen würde.

3. Den privaten Schlüssel zusätzlich sicher aufbewahren (Passwortmanager). Geht er verloren,
   braucht es einen neuen Schlüssel, und alle Nutzer müssen einmal von Hand aktualisieren.

4. Fertig. Ab dem nächsten Release liegt `checksums.txt.sig` im Release, und jede ab dann
   gebaute App prüft sie vor jedem Update. Ältere Versionen ohne eingebauten Schlüssel
   aktualisieren sich weiter wie bisher und haben danach die Prüfung.

## Was signiert wird (seit November 2026)

Der Release-Workflow hat drei Jobs: **build** (Tests, Bauen, ohne Secrets), **sign** (nur hier liegt
`UPDATE_SIGNING_KEY`; kein Checkout, kein `go`, nur das in *build* gebaute `sshtermy-sign`) und
**publish** (Herkunftsnachweis, Release, Homebrew).

Die erste Zeile von `checksums.txt` nennt die Version, z. B. `# sshtermy v1.0.380`. Sie ist mit
signiert, und die App installiert nur, wenn sie zur angekündigten Version passt. So lässt sich ein
älteres, ebenfalls signiertes Release nicht als neues ausgeben. Ältere Apps überlesen die Zeile.
Übergang: Releases von vor dieser Änderung haben die Zeile nicht und werden von neuen Apps nicht
mehr installiert; beim Kanal „stabil“ gibt es deshalb nach dem Umstieg ein paar Tage lang kein Update.

Zusätzlich hat jedes Release einen Herkunftsnachweis von GitHub (Sigstore) und eine
Bibliotheksliste (SBOM). Prüfen:

```
gh attestation verify sshtermy_linux_amd64 --repo db9979/sshtermy
```

Empfehlung: unter **Settings → General → Releases** „Release immutability“ einschalten. Dann lässt
sich ein veröffentlichtes Release (Dateien und Tag) nachträglich nicht mehr verändern.

## Kontrolle

- Im Release `v…` gibt es die Datei `checksums.txt.sig`.
- Im Lauf des Workflows **release** steht keine Warnung „Release unsigniert“ mehr.

## Wenn der Schritt „Signieren“ fehlschlägt

Die Meldung im Lauf nennt die Ursache, ohne den Schlüssel zu zeigen:

- **64 Hex-Zeichen – das ist der öffentliche Schlüssel**: Im Secret steht die erste statt der
  zweiten Zeile von `keygen`. Secret mit dem privaten Schlüssel (128 Zeichen) überschreiben.
- **Hex-Folge mit … Zeichen, erwartet 128**: Beim Kopieren fehlt ein Stück. Zeilenumbrüche,
  Leerzeichen und Anführungszeichen stören nicht, sie werden ignoriert.
- **passt nicht zu UPDATE_PUBKEY**: Secret und Variable stammen aus zwei verschiedenen
  `keygen`-Läufen. Einmal neu erzeugen und beide Werte ersetzen.

Ein Secret lässt sich auf GitHub nicht anzeigen, nur überschreiben (*Update*).
