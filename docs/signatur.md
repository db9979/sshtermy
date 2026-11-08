# Updates signieren (einmalig einrichten)

sshTermy aktualisiert sich selbst aus den GitHub-Releases. Ohne Signatur prüft die App nur die
Prüfsummen aus `checksums.txt`. Die liegen aber im selben Release: Wer das GitHub-Konto oder den
Release-Workflow übernimmt, könnte also eine veränderte Version samt passender Prüfsumme ausliefern.
Mit Signatur lehnt die App jedes Update ab, das nicht mit dem privaten Schlüssel signiert wurde,
und dieser Schlüssel liegt nur als Secret im Repository.

## Schritte (ca. 5 Minuten)

1. Schlüsselpaar erzeugen (auf einem eigenen Rechner, im Repository-Ordner):

   ```
   go run ./cmd/sshtermy-sign keygen
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

## Kontrolle

- Im Release `v…` gibt es die Datei `checksums.txt.sig`.
- Im Lauf des Workflows **release** steht keine Warnung „Release unsigniert“ mehr.
