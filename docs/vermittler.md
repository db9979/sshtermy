# Vermittler: Server hinter NAT oder Firewall erreichen

Manche Server sind von außen nicht erreichbar: ein Pi bei Freunden, eine NAS hinter einem Router ohne Portfreigabe,
ein VPS ohne offenen SSH-Port. Mit einem **Vermittler** baut der Server selbst eine Verbindung nach außen auf (der
**Anker**), und sshTermy erreicht sein sshd über diesen Vermittler.

```
sshTermy ──WSS /v1/kanal──► Vermittler ◄──WSS /v1/anker/kanal── Anker ──► sshd (127.0.0.1:22)
```

- **SSH läuft Ende-zu-Ende** zwischen sshTermy und dem Server. Der Vermittler reicht nur verschlüsselte Bytes durch
  und sieht nie ein Passwort, einen Schlüssel oder einen Befehl.
- Am Server meldest du dich ganz normal an (dein Benutzer, dein Schlüssel oder Passwort). Dateien (F5), Tunnel,
  Ziel-Modus und alles andere funktionieren wie bei einer direkten Verbindung.
- Der **Host-Key** kommt vom Anker: Der Anker liest ihn beim Koppeln aus dem eigenen sshd, der Vermittler gibt ihn an
  sshTermy weiter. Passt er, verbindet sshTermy ohne Rückfrage. Zeigt der Server einen anderen Schlüssel, bricht
  sshTermy ab und bietet bewusst kein „trotzdem verbinden“ an.
- Es gibt keinen offenen Port am Server. Der Anker öffnet Kanäle **nur zum eigenen sshd** (Standard 127.0.0.1:22), egal
  was der Vermittler schickt.

Der Vermittler ist freiwillig. Ohne ihn verbindet sshTermy wie bisher direkt oder über einen Jump-Host.

Zwei Arten von Vermittlern sprechen dasselbe Protokoll:

| | eigener Vermittler | Speech on DGX Spark |
|---|---|---|
| Was | `sshtermy vermittler`, dieselbe Programmdatei | Fernzugang im Panel |
| Wo | jeder erreichbare Rechner (VPS, Pi, NAS) | dein Spark |
| Anker | `sshtermy anker` | `spark-anker` |
| Sperre | keine | Seite entsperren mit Code aus der App |

## Eigener Vermittler in drei Schritten

### 1. Vermittler starten (auf einem erreichbaren Rechner)

```
sshtermy vermittler start --url https://vermittler.example.org:8022
```

- Ohne eigenes Zertifikat legt der Vermittler ein selbst signiertes an. sshTermy und der Anker pinnen dessen
  SHA-256 (steht in jeder Kopplungszeile), daher braucht es keine Zertifizierungsstelle.
- Mit Let's Encrypt o. Ä.: `--cert fullchain.pem --key privkey.pem`. Hinter einem Reverse-Proxy (Caddy, nginx):
  `--hinter-proxy --listen 127.0.0.1:8022`, der Proxy macht TLS und reicht WebSockets durch.
- Daten liegen in `/var/lib/sshtermy-vermittler` (als root) bzw. `~/.config/sshtermy/vermittler` (`--dir` ändert das).
  Schlüssel stehen dort nur als SHA-256.

Als Dienst (Linux), z. B. `/etc/systemd/system/sshtermy-vermittler.service`:

```
[Unit]
Description=sshTermy-Vermittler
After=network-online.target

[Service]
ExecStart=/usr/local/bin/sshtermy vermittler start --url https://vermittler.example.org:8022 --dir /var/lib/sshtermy-vermittler
Restart=always
DynamicUser=yes
StateDirectory=sshtermy-vermittler
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes

[Install]
WantedBy=multi-user.target
```

### 2. Server mit Anker koppeln

Auf dem Vermittler eine Zeile für einen Server holen (gilt 10 Minuten und nur einmal):

```
sshtermy vermittler code --anker
```

Auf dem Server, der erreichbar werden soll:

```
sshtermy anker koppeln 'https://vermittler.example.org:8022#termy=CODE&fp=…'
sshtermy anker dienst | sudo tee /etc/systemd/system/sshtermy-anker.service
sudo systemctl enable --now sshtermy-anker
```

`--ziel 127.0.0.1:2222` nimmt ein anderes sshd, `--name` einen anderen Namen. `sshtermy anker zustand` zeigt, mit
welchem Vermittler der Server gekoppelt ist.

### 3. sshTermy koppeln

Auf dem Vermittler eine Zeile für dein Gerät holen:

```
sshtermy vermittler code
```

In sshTermy: **Strg+Leertaste → „Vermittler koppeln / verwalten“ → „Neuen Vermittler koppeln“**, Zeile einfügen,
Strg+S. Danach zeigt **„Systeme über Vermittler“** alle Server mit Online-Punkt. ⏎ auf einem System legt die
Verbindung an (Benutzer und Anmeldung eintragen) oder verbindet, wenn sie schon gespeichert ist. Im
Verbindungsformular wählt das Feld **„Über Vermittler“** das System, Host, Port und Jump-Host fallen dann weg.

Verwalten (Palette → „Vermittler koppeln / verwalten“): **Testen** (erreichbar, Zertifikat, Schlüssel, Anzahl Systeme),
**Entsperren** (nur Spark), **Entfernen** (nur auf diesem Rechner).

Auf dem Vermittler:

```
sshtermy vermittler liste                 # Geräte und Server
sshtermy vermittler entfernen a_1234…     # trennt binnen Sekunden
sshtermy vermittler ordner a_1234… Kunden # Ordner in sshTermy
```

Gekoppelte Vermittler sind **rein lokal**: Jeder Rechner koppelt sich selbst mit eigenem Schlüssel. Der Sync überträgt
die Verbindungen, nicht die Vermittler; auf einem zweiten Rechner einfach dort auch koppeln.

## Protokoll (Version 1)

Für alle, die einen eigenen Vermittler anbieten wollen (z. B. das Spark-Panel). Basis ist eine `https`-Adresse mit
optionalem Pfad, alle Wege hängen daran. Antworten sind JSON, Fehler haben einen HTTP-Status und `{"fehler": "…"}` oder
`{"detail": "…"}`.

**Kopplungszeile**: `https://host[:port][/pfad]#termy=CODE[&fp=SHA256]`, `fp` ist der SHA-256 des TLS-Zertifikats
(DER, hex) und wird gepinnt. Ohne `fp` gilt die normale CA-Prüfung.

Client (sshTermy), Schlüssel im Header `Authorization: Bearer <key>`:

| Weg | Inhalt |
|---|---|
| `POST /v1/koppeln` `{code, name}` | → `{key, name, art}`, Code einmal gültig |
| `GET /v1/systeme` | → `{name, art, gesperrt, entsperrt_min, systeme: [{id, name, os, online, hostkey, ordner, benutzer}]}` |
| `POST /v1/entsperren` `{code}` | → `{bis}` (Unix-Zeit). Nur Vermittler mit Sperre, sonst 404 |
| `GET /v1/kanal?system=ID` (WebSocket) | Binärnachrichten = rohe Bytes zum sshd des Systems |

`hostkey` steht im `authorized_keys`-Format. `benutzer` ist ein Vorschlag für die Anmeldung (Spark: `spark`).
Statuscodes: 401/403 Schlüssel unbekannt, 404 System unbekannt, 409 System offline, 423 gesperrt, 429 zu viele,
503 abgeschaltet oder Notaus, 504 Anker hat keinen Kanal geöffnet.

Anker (eigener Vermittler):

| Weg | Inhalt |
|---|---|
| `POST /v1/anker/koppeln` `{code, name, os, hostkey}` | → `{id, key}` |
| `GET /v1/anker` (WebSocket) | Steuerleitung. Anker → `{"typ":"hallo","os","hostkey"}`, Vermittler → `{"typ":"kanal","id"}` |
| `GET /v1/anker/kanal?id=ID` (WebSocket) | der erbetene Kanal, rohe Bytes |

Ein Host-Key ändert sich beim Vermittler nur durch neues Koppeln, ein abweichendes „hallo“ wird nur protokolliert.

Sitzung live teilen, v1 (nur eigener Vermittler, ältere Versionen):

| Weg | Inhalt |
|---|---|
| `POST /v1/teilen` (Client-Schlüssel) | → `{id}` (32 hex). Höchstens 4 Freigaben je Gerät, 64 insgesamt |
| `GET /v1/teilen/senden?id=ID` (WebSocket, Client-Schlüssel) | Gastgeber schickt Binärrahmen; Vermittler meldet `{"typ":"zuschauer","n"}` |
| `GET /v1/teilen/ansehen?id=ID` (WebSocket, ohne Schlüssel) | Zuschauer bekommt dieselben Binärrahmen, höchstens 10 je Freigabe |

Link zum Zusehen: `https://host[:port][/pfad]#teilen=ID&k=SCHLÜSSEL[&fp=SHA256]`. Jeder Rahmen ist
XChaCha20-Poly1305 mit dem Schlüssel aus dem Link (32 Byte, base64url), Nonce vorne, `ID` als zusätzliche Daten;
entschlüsselt beginnt er mit `d` (Ausgabe), `s` („Spalten Zeilen“) oder `e` (Ende). Der Schlüssel steht hinter `#`
und erreicht den Vermittler nie. Zuschauer schicken nichts; was sie tippen, verwirft sshTermy.

### Teilen v2 (ab dieser Version Standard)

Bedienung: Palette „teilen“ öffnet einen Dialog. Wege: **im Netzwerk** (ohne Vermittler: sshTermy startet einen
eingebauten Teilen-Server auf einem Zufallsport, Zuschauer finden ihn per UDP-Rundruf auf Port 47831), über einen
**gekoppelten Vermittler**, über einen **öffentlichen** Vermittler (gestartet mit `sshtermy relay start --offen`) oder
den **Vermittler-Assistenten** (richtet `sshtermy relay` per SSH/SFTP als systemd-Dienst `sshtermy-vermittler` auf
einem eigenen Server ein, koppelt sshTermy mit Verwaltungsrecht und kann ihn wieder entfernen).

Einladung: Code `PLATZ-wort-wort-wort` (Platz 1–99 auf einem Vermittler, 100–999 im LAN; 256 Wörter je Stelle)
oder Link `sshtermy://zusehen/CODE@host:port?fp=SHA256`, dazu ein QR-Code. Der Code gilt 10 Minuten für bis zu
1/2/5/10 Zuschauer. Aus dem Code entsteht per **CPace** (PAKE über ristretto255) der Schlüssel: wer den Code nicht
kennt, hat genau einen Rateversuch, danach ist die Einladung gesperrt. Beide Seiten zeigen drei Prüfwörter (HKDF aus
dem PAKE-Ergebnis); der Gastgeber lässt erst nach dem Vergleich herein („Anklopfen“, abschaltbar).

| Weg | Inhalt |
|---|---|
| `POST /v1/teilen` `{"v":2,"min":N}` (Client-Schlüssel, im LAN der lokale Schlüssel, bei `--offen` ohne) | → `{id, platz, bis}`. Laufzeit höchstens 12 h (gekoppelt) bzw. 4 h (offen) |
| `GET /v1/teilen/senden?id=ID` (WebSocket) | Gastgeber. Binär: `0x00` + Daten an alle Zugelassenen, `0x01` + 4 Byte Gast-ID + Daten an einen. Text-JSON: `zulassen`/`trennen`/`zu`/`auf` → Vermittler meldet `neu`/`weg`/`zuschauer` |
| `GET /v1/teilen/ansehen?platz=N` (WebSocket, ohne Schlüssel) | Gast; höchstens 3 wartend, wartet höchstens 3 Minuten auf Zulassung |
| `POST /v1/codes` `{"art":…}` (nur Geräte mit Verwaltungsrecht) | → `{zeile}`: neue Kopplungszeile, aus sshTermy heraus. Recht vergibt `sshtermy relay code --verwalten` |

Nachrichten (erstes Byte): `A` PAKE-Start (Gast), `B` PAKE-Antwort + Bestätigung (Gastgeber), `C` Bestätigung + Hallo
(Name des Gasts), `K` Steuernachricht (mit dem PAKE-Schlüssel versiegelt), `S` Rahmen mit dem Teilen-Schlüssel,
`N` Ablehnung. Rahmen wie oben (`d`/`s`/`e`) plus `p` Pause, `q` weiter, `i` Info (Tab, Laufzeit). Der Vermittler
sieht nur Platznummer und Chiffrat.

Schutz beim Gastgeber: Zuschauer sehen erst ab jetzt (kein alter Bildschirm), bei Passwortabfragen pausiert die
Übertragung, bis die Zeile fertig ist, das Teilen endet von selbst, und im Sitzungsprotokoll steht, wer zugelassen oder abgelehnt wurde (`# Zuschauer zugelassen: …`).
Mitschreiben gibt es nur, wenn der Zuschauer die Hand hebt und der Gastgeber es befristet erlaubt; ⏎ eines Zuschauers
braucht bei produktiven Verbindungen die Bestätigung des Gastgebers.

### Zusehen im Browser (ohne sshTermy)

Der eingebaute Teilen-Server im LAN und `sshtermy vermittler` liefern unter `GET /zusehen/` eine Seite aus
(internal/webwatch, xterm.js + `zusehen.wasm`). Link: `https://host[:port][/pfad]/zusehen/#CODE`. Der Code steht hinter
`#` und geht nie an den Server; die Seite spricht dieselbe Anmeldung wie sshTermy (internal/sharewire als WebAssembly,
gebaut mit `make zusehen` aus cmd/zusehen-wasm) über `wss://…/v1/teilen/ansehen`. Im Hallo steht `"browser":true`; der
Gastgeber sieht beim Anklopfen „im Browser“ und lehnt solche Zuschauer ab, wenn *Auch im Browser zusehen* aus ist.
Die Seite hat eine strenge CSP (nur eigene Skripte, nur Verbindungen zum selben Server).

Grenzen: Ein selbst ausgestelltes Zertifikat (LAN, Vermittler ohne `--cert`) zeigt im Browser einmal eine Warnung; den
Fingerabdruck kann der Browser nicht pinnen. Wer im Netz dazwischen sitzt und die Warnung ausnutzt, könnte eine
falsche Seite ausliefern. Deshalb gilt der Browser-Link nur so viel wie die Leitung dorthin: Für heikle Sitzungen den
Link für sshTermy nehmen (Fingerabdruck gepinnt) oder dem Vermittler ein echtes Zertifikat geben. Bei einem fremden,
öffentlichen Vermittler liefert dieser die Seite aus; die Einladung warnt dann.

Speech on DGX Spark kann als Vermittler teilen, sobald es Teilen v2 umsetzt (die Wege oben); bis dahin gilt dort v1.

## Sicherheit

- TLS immer, http nur zwischen Reverse-Proxy und Vermittler auf 127.0.0.1.
- Kopplungscodes: 10 Zeichen, 10 Minuten, einmal, nur als SHA-256 gespeichert, für Gerät und Anker getrennt.
  Höchstens 10 Kopplungsversuche je Adresse in 10 Minuten.
- Schlüssel: 32 Zufallsbytes je Gerät und Server, beim Vermittler nur als SHA-256, in sshTermy im verschlüsselten
  Store. Entfernen wirkt sofort, auch auf offene Kanäle.
- Der Vermittler protokolliert nur feste Wörter und Zahlen (`vermittler: kanal_auf | 2`), nie Namen, Adressen oder
  Inhalte. Er kann keine Befehle ausführen und keine Ziele wählen.
