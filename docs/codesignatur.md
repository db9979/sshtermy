# Codesignatur für Windows und macOS (optional, kostet)

Ohne Codesignatur warnt Windows (SmartScreen: „Der Computer wurde durch Windows geschützt“) und
macOS (Gatekeeper: „kann nicht geöffnet werden, da der Entwickler nicht verifiziert werden kann“).
Die Updates selbst sind davon unabhängig mit Ed25519 signiert ([signatur.md](signatur.md)).

Der Release-Workflow (`.github/workflows/release.yml`, Job **sign**) ist vorbereitet: Sobald die
Secrets unten eingetragen sind, werden die Programmdateien signiert, bevor die Prüfsummen entstehen.
Ohne Secrets werden die Schritte übersprungen, alles läuft wie bisher.

## Windows (Authenticode)

Nötig: ein Codesignatur-Zertifikat als `.pfx` (z. B. von Certum, Sectigo, SSL.com; ab ca. 70 € im Jahr
für Open-Source-Entwickler). Zertifikate, die nur auf einem Hardware-Token oder in einem Cloud-HSM
liegen (seit 2023 die Regel bei neuen Zertifikaten), lassen sich so nicht nutzen; dann den
Anbieter nach einem Signierdienst mit Schnittstelle fragen (z. B. Azure Artifact Signing, SSL.com eSigner).

Secrets (Settings → Secrets and variables → Actions):

- `WINDOWS_CERT_PFX`: die `.pfx`-Datei als Base64 (`base64 -w0 zert.pfx`)
- `WINDOWS_CERT_PASSWORD`: Passwort der `.pfx`

Signiert wird mit `osslsigncode`, SHA-256 und Zeitstempel (digicert), danach wird die Signatur geprüft.

## macOS (Developer ID + Notarisierung)

Nötig: Apple Developer Program (99 $ im Jahr), ein Zertifikat „Developer ID Application“ und ein
App-Store-Connect-API-Schlüssel (Rolle „Developer“) für die Notarisierung.

- `APPLE_CERT_P12`: Zertifikat samt privatem Schlüssel als `.p12`, Base64
- `APPLE_CERT_PASSWORD`: Passwort der `.p12`
- `APPLE_API_KEY_P8`: Inhalt der `AuthKey_XXXX.p8` (Text)
- `APPLE_API_KEY_ID`: Key-ID (10 Zeichen)
- `APPLE_API_ISSUER`: Issuer-ID (UUID)

Signiert und notarisiert wird mit [rcodesign](https://github.com/indygreg/apple-platform-rs) unter
Linux (Version und Prüfsumme im Workflow fest), mit gehärteter Laufzeit. Fehlt der API-Schlüssel,
wird nur signiert. Eine einzelne Programmdatei lässt sich nicht „heften“ (stapling); macOS fragt die
Notarisierung beim ersten Start online ab.

## Kontrolle

- Windows: Rechtsklick auf `sshtermy.exe` → Eigenschaften → Digitale Signaturen.
- macOS: `codesign -dv --verbose=2 sshtermy_darwin_arm64` und `spctl -a -vv -t install sshtermy_darwin_arm64`.
