# <img src="assets/icon/sshtermy-128.png" alt="sshTermy-Icon" width="48" height="48" align="top"> sshTermy

🇬🇧 [English version](README.md) · 🇩🇪 Deutsch

**SSH-Terminal mit KI-Copilot** für Windows 11 ARM64, Windows x86 (32/64 Bit), macOS und Linux.
*Idee: Dominik Bornhäußer*

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-unterst%C3%BCtzen-FFDD00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/zquu1xu570)

Gefällt dir sshTermy? Über den Button oben, im Über-Dialog (F12 → `b`) oder nach jedem Update (die Seite öffnet sich dann einmal von selbst im Browser) kannst du einen Kaffee spendieren.

![Ziel-Modus](docs/bilder/03-ziel-modus.png)

sshTermy ist eine schlanke Terminal-Anwendung (TUI, eine einzige Datei, ~9 MB, kein Installer, kein cgo) mit:

- **Verbindungs-Manager** mit verschlüsseltem Store (AES-256-GCM, Argon2id, unter Windows zusätzlich DPAPI)
- **echtem SSH-Terminal** (xterm-256color, Truecolor, vim/htop/tmux, Scrollback, Tabs, Jump-Host)
- **Auto-Reconnect**: reißt die Leitung ab (WLAN, VPN, Standby), verbindet sshTermy die Sitzung von selbst neu, startet laufende Tunnel wieder und hängt tmux/screen wieder an (abschaltbar über die Palette). Nach **Ruhezustand oder Netzwechsel** (anderes WLAN, VPN an/aus) prüft sshTermy jede Leitung binnen Sekunden, statt auf den Keepalive-Ablauf zu warten; solange gar kein Netz da ist, wartet es, ohne Versuche zu verbrauchen
- **Feste Sitzung** (Idee: Dominik Bornhäußer): pro Verbindung beim Verbinden immer in dieselbe tmux-/screen-Sitzung, auch von einem anderen Rechner aus. Laufen schon andere Sitzungen, fragt sshTermy, welche. Gleicher Sitzungsname in zwei Tabs oder auf zwei Rechnern = gespiegelter Bildschirm; „nur zuschauen“ hängt tmux schreibgeschützt an. Die Palette öffnet einen zweiten Tab zum Zuschauen oder kopiert einen `ssh … tmux attach -r`-Befehl für Kollegen. Nicht bei Switches, Telnet und Konsole.
- **Eigenes Fenster** (ohne Terminal): Doppelklick auf `sshtermy.exe` im Explorer, `sshTermy.app` auf dem Mac oder `sshtermy -window`, und sshTermy öffnet ein eigenes Fenster mit eingebauter Cascadia Mono samt Nerd-Font-Symbolen und Emoji. Bilder vom Server erscheinen pixelgenau, ins Fenster gezogene Dateien werden zum Hochladen angeboten, ⇧F10 holt einen Tab in ein zweites eigenes Fenster. Alle Tasten kommen an (Alt+⏎, ⇧F1 … ⇧F12, Strg+Leertaste), Strg+⇧V oder ⇧Einfg fügen ein (Windows auch Strg+V, Mac ⌘V), Strg + / Strg − / Strg 0 oder Strg + Mausrad ändern die Schriftgröße (wird gemerkt), F11 schaltet Vollbild um. Das Fenster startet standardmäßig im **Vollbild**; Palette › Einstellungen › „Fenster beim Start“ stellt auf maximiert oder „wie zuletzt“ um (Größe und Position werden gemerkt). Im Terminal gestartet läuft sshTermy wie bisher im Terminal; `-tui` erzwingt das
- **Lokale Shell** (Alt+L): deine eigene PowerShell (Windows, über ConPTY), zsh oder bash (macOS/Linux) als Tab, mit KI-Hilfe, Ziel-/Agent-/Plan-Modus, geteilter Ansicht, Tabs und Sitzungsprotokoll. Gefahren-Check und Agent-Stufen gelten auch auf dem eigenen Rechner; der Agent legt seine Skripte in `~/.sshtermy-agent` ab. Oder als Verbindung speichern („Verbindung über: Lokal“) mit eigener Shell (`pwsh`, `cmd`, `wsl`, `bash -l` …) und Startordner
- **PowerShell-Fernzugriff (WinRM)**: auf Windows-Server einloggen wie mit `Enter-PSSession`, ohne SSH auf dem Server („Verbindungsart: PowerShell“, Port 5985/5986). NTLM auch für Domänenkonten, Kerberos für Server ohne NTLM, Nachrichten über HTTP verschlüsselt, oder HTTPS; eigene Eingabezeile mit Verlauf, Tab-Ergänzung vom Server und mehrzeiliger Eingabe. Die KI schlägt PowerShell-Befehle vor, der Gefahren-Check nimmt die Windows-Regeln. Palette: `Enter-PSSession srv01` oder `winrm admin@srv01`
- **Fehlerverzeihend im Alltag**: Strg+Q und das Schließen des Fensters fragen nach, wenn noch Verbindungen offen sind; Verbindung, Ordner oder Runbook löschen geht sofort, ein Hinweis bietet 10 Sekunden lang **Rückgängig** an (Strg+Z im Manager oder Klick); **Strg+T im Verbindungsformular testet** Erreichbarkeit, Host-Key und Anmeldung vor dem Speichern; fehlgeschlagene Verbindungen sagen **in Klartext**, was los ist und was hilft („Anmeldung abgelehnt“, „Keine Antwort von sw1:22 … VPN oder Jump-Host?“, `e` bearbeitet die Verbindung), der Originalfehler steht grau darunter. Zeigt ein Server einen **neuen Host-Key** (Neuinstallation, Werksreset), vergleicht `k` alten und neuen Fingerprint und übernimmt den neuen nach zweiter Bestätigung
- **Einfacher Modus** (Palette › Einstellungen › Allgemein): blendet Profi-Funktionen (Broadcast, Netzwerk, Team, Agent, Aufzeichnung, MCP, Audit …) aus Palette-Übersicht, Einstellungen und Tastenzeile aus; die Suche in der Palette findet weiter alles
- **Schnellverbinden**: `user@host:port` oder `ssh -p 2222 user@host` in die Palette tippen, ohne die Verbindung anzulegen (Agent/Standard-Keys, sonst Passwort)
- **Mehrere Verbindungen auf einmal ändern**: Im Manager markiert die Leertaste eine Verbindung, `*` alle sichtbaren (mit Suche oder `#tag` also alle Treffer). Danach gelten `e` (Sammeländerung: Tags hinzufügen oder entfernen, Benutzer, Schutz, Farbe), `v` (verschieben) und `d` (löschen, mit Rückgängig) für alle markierten; Esc hebt die Auswahl auf.
- **Notiz je Verbindung**: ein freies Feld nur für dich (Ansprechpartner, Wartungsfenster, Standort). Steht im Manager rechts, die Suche findet es, an die KI geht es nie. Die „KI-Notizen“ bleiben das Feld für Hinweise an die KI.
- **Startbefehl je Verbindung** (Formular › Erweitert): wird nach dem Anmelden einmal sichtbar eingetippt, z. B. `cd /srv/app` oder `sudo -i`. Gefährliche Befehle lehnt das Formular ab; aus einem Team-Paket wird ein Startbefehl nie übernommen.
- **Wake-on-LAN**: Ein ausgeschalteter Rechner im eigenen Netz lässt sich aus dem Manager wecken (Klick auf „Wake-on-LAN senden“, Palette „aufwecken“ oder `w` im getrennten Tab). Die MAC-Adresse merkt sich sshTermy beim ersten Verbinden von selbst oder du trägst sie unter Erweitert ein.
- **Platzhalter in Schnellbefehlen**: `systemctl restart {{dienst}}` fragt beim Ausführen nach dem Wert, `{{dienst:nginx}}` schlägt einen vor; der letzte Wert steht beim nächsten Mal schon da. Macht erst der eingesetzte Wert den Befehl gefährlich, fragt sshTermy nach.
- **Suche über alle Tabs** (Alt+S): ein Suchbegriff, die Treffer aus dem Verlauf aller offenen Tabs in einer Liste; ⏎ springt in den Tab an genau diese Zeile.
- **Tabs mit der Maus ziehen**: Ein Tab lässt sich in der Tab-Leiste an eine andere Stelle ziehen.
- **Fertig im Hintergrund**: Ein Befehl, der 10 s oder länger in einem verdeckten Tab lief, markiert den Tab (✓ / ×) und meldet sich mit einer Einblendung; ist das sshTermy-Fenster im Hintergrund, kommt zusätzlich eine Desktop-Benachrichtigung (Windows, macOS, Linux). Die Terminal-Glocke markiert den Tab ebenso. Beides ist in der Palette abschaltbar
- **Agent-Weiterleitung** pro Verbindung (wie `ssh -A`): `git pull`, `ssh` und `scp` auf dem Server nutzen deine lokalen Keys aus ssh-agent, Pageant oder dem Windows-OpenSSH-Agent, die Keys verlassen deinen Rechner nie
- **X11-Weiterleitung** pro Verbindung (wie `ssh -X`): Grafische Programme vom Server öffnen sich hier; braucht lokal einen X-Server (XQuartz, VcXsrv, WSLg), das X-Cookie verlässt deinen Rechner nie
- **FIDO2-Sicherheitsschlüssel** (`id_ed25519_sk`, `id_ecdsa_sk`, z. B. YubiKey) werden direkt genutzt; sshTermy sagt dir, wann du den Schlüssel berühren musst (braucht OpenSSH ssh-agent/ssh-add)
- **Passwortmanager statt gespeicherter Passwörter**: Im Verbindungsformular „Passwort aus“ und (unter Erweitert)
  „2FA-Code aus“: `op://Tresor/Eintrag/password` (1Password CLI), `bw:Eintrag` (Bitwarden CLI), `keepassxc:Datei.kdbx:Eintrag`
  (KeePassXC, fragt das Datenbank-Passwort), `pass:Name` oder `cmd:Befehl`. Fragt ein Server nach einem Einmalcode
  („Verification code“), holt sshTermy den TOTP-Code aus derselben Quelle, auch nach einer Anmeldung mit Key.
  Gespeichert wird nur der Verweis, nie das Passwort
- **Kerberos/GSSAPI-Anmeldung** (Anmeldung `kerberos`): Nutzt das Ticket der Firmen-Domäne aus der Windows-Anmeldung oder `kinit`, ohne Passwort; ohne Ticket fragt sshTermy wie gewohnt nach dem Passwort
- **Bilder im Terminal**: Sixel (`img2sixel`, `chafa -f sixel`, gnuplot), das kitty-Grafikprotokoll (`kitty +kitten icat`, `viu`, `timg`) und iTerm2-Bilder (`imgcat`) erscheinen als farbige Halbblöcke direkt im Text, in jedem Terminal mit 24-Bit-Farbe, im Verlauf und in der geteilten Ansicht. Zwei Pixel pro Zelle: Diagramme und Fotos sind erkennbar, nicht pixelgenau. In kitty, Ghostty und im eigenen Fenster von sshTermy erscheinen die Bilder stattdessen pixelgenau (kitty-Grafik mit Unicode-Platzhaltern, sie wandern also weiter mit dem Text; aus mit `SSHTERMY_NO_PIXELS=1`)
- **Zwischenablage vom Server (OSC 52)**: Was du in tmux oder vim auf dem Server kopierst, landet in deiner lokalen Zwischenablage. Lesen kann der Server sie nie
- **Geteilte Ansicht** (Strg+T): bis zu sechs Sitzungen gleichzeitig in sieben Aufteilungen, Größe per Maus oder Alt+⇧+Pfeil, Zoom auf einen Bereich (Alt+Z), Bereiche per Titel tauschen, andere Bereiche auf Wunsch abdunkeln. Die Aufteilung übersteht Updates; **Arbeitsplätze** (w im Manager) speichern Aufteilung und Verbindungen zum Wiederöffnen mit einem Klick, „Zuletzt“ merkt sich sshTermy beim Beenden selbst, `g` auf einem Ordner öffnet alle Verbindungen darin als Raster
- **Eingabe an alle geteilten Bereiche** (Alt+I in der geteilten Ansicht): Was du tippst, geht live an jeden sichtbaren Host, z. B. dieselbe Änderung auf vier Switches. Enter geht nur durch, wenn die Zeile auf keinem Host gefährlich ist
- **SSH-Zertifikate**: Von einer Firmen-CA signierte Keys (`id_ed25519-cert.pub` neben dem Key oder ein eigener Pfad in der Verbindung) werden automatisch genutzt; ein abgelaufenes Zertifikat meldet sshTermy mit Datum
- **Dateien direkt im Terminal (trzsz)**: `tsz datei` auf dem Server lädt in deinen Downloads-Ordner (sshTermy fragt vorher und überschreibt nie eine vorhandene Datei), `trz` lädt hoch; klappt auch nach `sudo su`, über mehrere Sprünge und auf Geräten ohne SFTP. ZMODEM (`sz`/`rz`) wird erkannt und mit Hinweis abgebrochen, statt zu hängen
- **Bildschirmsperre**: Palette „Jetzt sperren“ oder automatisch bei Leerlauf (5/15/30/60 min). Offene Sitzungen laufen hinter der Sperre weiter, nur das Master-Passwort entsperrt. **Master-Passwort ändern** (Palette → Einstellungen › Sicherheit) schaltet auch die Windows-Anmeldung (DPAPI) nachträglich ein oder aus
- **Statuszeile pro Sitzung**: Verbindungsdauer, Latenz, Zeichensatz, Warnung bei root; Produktiv-Hosts (Gruppe/Tag „Produktion“, „prod“, „live“) bekommen eine rote Zeile
- **Dateien (F5)**: SFTP über dieselbe Verbindung, links lokal, rechts Server, Dateien und Ordner hoch- und herunterladen, mit der Maus auf die andere Seite ziehen, mehrere mit der Leertaste markieren. Dateien aus Explorer/Finder ins Fenster ziehen lädt sie hoch, auch direkt im Terminal. Beim Kopieren zeigt ein Balken Prozent, MB/s, Restzeit und „Datei 2 von 5“, dazu eine kleine Tempo-Kurve
- **Dateien mit der Maus**: Doppelklick auf eine Datei zeigt sie direkt in sshTermy (Zeilennummern, Farben, Rad blättert, Ziehen markiert und kopiert, `/` sucht, `f` liest mit wie `tail -f`, `w` bricht um; große Dateien lädt er am Anfang und am Ende). Bilder, PDFs und Office-Dateien öffnen sich mit dem Standardprogramm deines Rechners (Server-Dateien werden vorher heruntergeladen). Ein Einfachklick zeigt den Anfang einer Textdatei unter der Liste (`v` schaltet das aus). Rechtsklick öffnet ein Menü: ansehen, bearbeiten, mit Standardprogramm öffnen, kopieren, herunter-/hochladen nach …, umbenennen (`F2`), Rechte ändern, Pfad kopieren (`y`), im Terminal hierhin `cd`, löschen. Ein Klick auf einen Teil des Pfads (/ › etc › nginx) springt dorthin, Knöpfe für ↑ ~ neu laden, neuer Ordner und versteckte Dateien, ein Klick auf einen Spaltenkopf sortiert (`s`), Strg+Klick markiert einzeln, Umschalt+Klick einen Bereich, die Zurück-Taste der Maus geht eine Ebene hoch
- **Server-Datei im eigenen Editor**: `e` in F5 (Server-Seite) oder Palette „Server-Datei im eigenen Editor öffnen“ mit Pfad öffnet die Datei in VS Code, Notepad++, TextEdit … auf deinem Rechner; jedes Speichern lädt sie über dieselbe Verbindung zurück (Besitzer und Rechte bleiben). Editor einstellen: Palette → Einstellungen › „Editor für Server-Dateien“ (z. B. `code --new-window`)
- **Farbregeln für die Ausgabe**: Text ohne eigene Farbe wird eingefärbt, Fehler rot, Warnungen gelb, ok/up grün, IP-Adressen blau, MAC-Adressen lila, Switch-Syslog (`%LINK-3-UPDOWN`) nach Schwere; nie in vim/htop. Eigene Regeln „farbe muster“ (Farben deutsch oder englisch; Palette → „Farbregeln für die Ausgabe“); mit `!` davor meldet sich sshTermy, wenn das Muster in einem verdeckten Tab auftaucht (Tab bekommt ein `!`, Einblendung, auf Wunsch Desktop-Meldung)
- **Host-Übersicht (F6)**: Server zeigen beim Verbinden CPU, Last, RAM, alle Platten (jedes echte Dateisystem; tmpfs, overlay, snap und Ähnliches bleiben weg), Laufzeit und offene Updates. Der Grenzwert für Platten (`g`) gilt für jede Platte einzeln.
  Bei Linux-Servern laufen, solange F6 offen ist, Live-Graphen aus Braille-Punkten für CPU, RAM, Netz und Platte
  (alle 2 s, `t` wechselt 5 min / 1 h, `z` zeigt sie groß über dem Terminal mit den Top-Prozessen; ein kleines,
  nur lesendes Skript in einer eigenen Exec-Sitzung, Panel zu = Skript beendet). Switches, Router und Firewalls (ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS/NX-OS/ASA/Small Business, Arista, Juniper, HPE Comware, Huawei, Dell, Extreme, Ruckus, Allied Telesis, Netgear, Zyxel, TP-Link, Lancom, MikroTik, FortiGate, Palo Alto, Sophos, Check Point, SonicWall, WatchGuard, pfSense/OPNsense, VyOS/EdgeOS, UniFi u. a.; siehe *Befehlssätze*) zeigen auf Abruf Firmware, Ports und VLANs; die show-Befehle laufen sichtbar am Prompt, nie als versteckte Exec-Sitzung
- **Container** (`c` in F6 oder Palette „docker“): Docker- und Podman-Container und Kubernetes-Pods des Servers in einer Liste. ⏎ öffnet eine Shell im Container, Strg+L verfolgt die Logs, Strg+K lässt die KI die letzten 400 Log-Zeilen auswerten. Neustart und Start/Stopp werden nur am Prompt eingefügt, ⏎ führt sie aus. Braucht docker root und geht sudo ohne Passwort, nimmt sshTermy `sudo -n`
- **Log-Blick (Alt+E oder `l` in F6)**: liest die Logs des Systems, auf dem du gerade bist, nur lesend, und zeigt Auffälligkeiten statt Zeilen. Linux/BSD: journalctl (oder Syslog-Dateien), fehlgeschlagene Units und Paket-Updates; Windows (PowerShell/WinRM): Ereignisprotokolle System/Application, Fehlanmeldungen 4625/4740 und gestoppte automatische Dienste; Switches und Firewalls: die Log-Befehle sichtbar am Prompt getippt (ProCurve/CX, Cisco, Junos, RouterOS, FortiOS, UniFi) und eine Port-Leiste. Gleiche Zeilen werden lokal ohne KI zu Mustern zusammengefasst (Zahlen, IPs, MACs ersetzt), bekannte Muster bekommen einen Klartext-Titel, einen Hinweis und lesende Prüfbefehle (`1`…`4` fügen sie ein). `n` *Was ist neu?* vergleicht mit deinem letzten Blick (neu, deutlich häufiger, verschwunden; die Grundlinie liegt verschlüsselt nur auf diesem Rechner), `x`/`X` blendet Rauschen für den Host oder den ganzen Ordner aus (wird gesynct). `t` Zeitleiste: Fehler und Warnungen pro Zeitabschnitt mit Marken für Neustarts, Paket-Updates, Konfigurationsänderungen und deine eigenen Befehle; ←→ wählt einen Abschnitt, `k` fragt die KI, was dort passierte. `k` lässt die KI ein Muster erklären, `b` schreibt einen KI-Lagebericht nur aus den verdichteten Mustern (Ursache, Zusammenhang, was prüfen). `z` Zeitraum 1 h … 7 Tage, `w` Meldungen der Logwache, `c` kopiert als Markdown. Bei Switches und Firewalls richtet `s` Syslog an sshTermy ein: Es zeigt die Befehle des Herstellers mit deiner eigenen IP und tippt sie nach Bestätigung sichtbar ein (am Gerät wird nichts gespeichert, `write memory` bleibt bei dir). Von sshTermy empfangenes Syslog fließt in den Log-Blick des Switches ein und reicht so weiter zurück als der Puffer im Gerät.
- **Logwache** (Einstellungen › Protokolle & Logs › *Logwache*: Fehler oder Fehler + Warnungen): Solange ein Tab offen ist, folgt sshTermy im Hintergrund seinen Logs (Linux `journalctl -f`, Windows über WinRM jede Minute) und meldet nur, was gegenüber dem letzten Log-Blick neu oder kritisch ist, jedes Muster höchstens einmal pro Stunde: Toast, ▲ am Tab, Desktop-Benachrichtigung, wenn sshTermy im Hintergrund ist. Ausgeblendetes Rauschen bleibt still. *Syslog-Empfang* (UDP 5514, 1514 oder 514) nimmt Syslog von Switches und Firewalls an und ordnet es über IP oder Hostnamen der passenden Verbindung zu. Absender, die zu keiner gespeicherten Verbindung
passen, werden verworfen (ein Toast nennt die IP einmal), weil sich UDP-Absender fälschen lassen; Einstellungen › Protokolle & Logs ›
*Syslog auch von unbekannten Absendern annehmen* lässt sie zu. Der KI-Lagebericht wertet Syslog-Zeilen als Hinweis,
nicht als Beweis. Die Liste: Palette *Logwache: Meldungen* oder `w` im Log-Blick; ⏎ springt zum Muster im Log-Blick des Hosts, `x` blendet es als Rauschen aus. Fehlanmeldungen (SSH, Windows 4625) zählen ab 20 in 10 Minuten als Fehler; Link-down-Meldungen behalten die Schwere des Geräts (Cisco `LINK-3` = Fehler).
- **Ordner-Scan** (Palette *Ordner-Scan*): der Log-Blick für alle Hosts eines Ordners auf einmal, nur lesend und ohne Rückfragen (6 gleichzeitig; Server über SSH, Windows über WinRM, Switches über ihre Shell). Eine Zeile pro Host mit kritisch/Fehler/Warnungen, neuen Mustern und dem wichtigsten Muster, dazu *Gemeinsam*: Muster, die auf mehreren Hosts auftauchen. `c` kopiert das Ergebnis als Markdown; die Grundlinie für *Was ist neu?* wird pro Host gespeichert. Als Zeitplan (Art *Ordner-Scan (Logs)*) läuft er z. B. jede Nacht; das Ergebnis steht im Zeitplan und unter *Ordner-Scan: letztes Ergebnis*.
- **Config-Sicherung für Switches**: `b` sichert die laufende Konfiguration (`show running-config`; Junos `show configuration | display set`, RouterOS `/export`, FortiOS `show full-configuration`, UniFi `cat /tmp/system.cfg`) verschlüsselt auf dem Rechner, `d` zeigt den Diff zum vorherigen Stand, unveränderte Stände werden nicht doppelt gespeichert.
  Nächtlich geht es per Zeitplan (`z`, Art „Config-Sicherung“) für einen ganzen Ordner. **Regeln für Switch-Configs**
  (Palette → Einstellungen): `+^ntp server` muss vorkommen, `-telnet-server` darf nicht; jede Sicherung wird geprüft,
  Verstöße stehen im F6-Panel, als Einblendung und im Ergebnis des Zeitplans
- **Gerät finden**: MAC (jede Schreibweise) oder IP in die Palette tippen, sshTermy fragt alle Switches nach MAC-,
  ARP- und LLDP-Tabellen und zeigt Switch, Port und VLAN, an dem das Gerät hängt. Uplinks erkennt es an den
  LLDP-Nachbarn und lässt sie hinten anstehen
- **Mehrere Netze**: Liegen die Switches in verschiedenen Ordnern (je Kunde oder Standort), fragen Gerät finden
  und Topologie erst, welcher Ordner abgefragt wird, oder alle; die letzte Wahl ist vorausgewählt
- **Netzwerk-Topologie** (Palette „topo“): aus LLDP/CDP-Nachbarn aller Switches entsteht ein Bild mit Kästen und Linien (wer hängt an
  welchem Port von wem, APs und Telefone gedimmt im Switch-Kasten, Ringverbindungen darunter; passt es nicht, ein Baum), dazu ein Mermaid-Diagramm in Downloads für Doku, Wiki oder GitHub
- **Broadcast** (m im Manager): ein Befehl oder Runbook läuft parallel auf einer Gruppe (z. B. alle Aruba-Switches),
  jeder Host bekommt eine eigene Kachel (grün umrandet wenn fertig, rot bei Fehler, pulsierend solange er läuft; `z` vergrößert eine). Gefahrenregeln gelten auch hier: Rot braucht zweimal ⏎.
  Ändernde Schritte laufen erst auf einem Host als Probelauf, dann auf dem Rest; nach 3 gescheiterten Hosts startet kein weiterer (`p` schaltet den Probelauf ab).
  Server bekommen jeden Schritt per Exec (mit Exit-Code), Netzwerkgeräte die Befehle in die Shell getippt („-- MORE --“ wird weitergeblättert).
- **Zeitpläne** (z im Manager): Ein Broadcast oder Runbook läuft zu einer Uhrzeit auf einem Ordner oder einzelnen
  Verbindungen, täglich, Mo–Fr, am Wochenende oder an einem Wochentag, z. B. „3:00 Mo–Fr `show running-config` auf
  allen Switches“. Läuft, solange sshTermy offen ist (auch gesperrt), verpasste lesende Läufe holt es innerhalb von 12 Stunden nach, ändernde nur innerhalb von 10 Minuten (eine Wartung für 3:00 läuft nie mittags).
  Hosts, die später im Ziel-Ordner auftauchen (etwa aus einem Team-Ordner), laufen erst nach `Strg+A` in der Liste mit; ein Runbook, das geändert aus einem Team-Ordner kam, wartet, bis du es einmal im Broadcast gestartet hast.
  Ohne Aufsicht gibt es keine Rückfragen: unbekannte Host-Keys und fehlende Passwörter lassen den Host scheitern,
  Gefährliches und Runbooks mit Variablen lassen sich gar nicht einplanen. Das Ergebnis je Host steht in der Liste
  (⏎), dazu eine Einblendung bzw. Desktop-Meldung. Zeitpläne gehen mit dem Sync, laufen aber nur auf einem Rechner
  („Läuft auf“ im Formular, Standard: der Rechner, der ihn angelegt hat), damit nicht zwei Rechner denselben Lauf starten
- **Runbooks mit Variablen:** `{{vlan}}` oder `{{vlan=20}}` in einem Schritt wird vor dem Start abgefragt, `{{host}}`,
  `{{alias}}`, `{{user}}`, `{{port}}` setzt sshTermy pro Host ein. Im Broadcast lädt `e` ein Runbook zum Bearbeiten,
  `Strg+R` speichert, `Strg+D` löscht. Vor dem Start prüft die KI mit `k` gegen das Systemprofil jedes Hosts, ob die
  Schritte dort laufen, und schlägt Entsprechungen vor (`a` übernimmt sie). Gefährlich oder nicht entscheiden weiter die festen Regeln.
- **Gefahren-Check:** Jede Verbindung gilt als Produktivsystem, deshalb fragt sshTermy bei riskanten
  Befehlen vor dem Enter nach (`rm -rf`, `reboot`, auf Switches `reload`, `erase`, `write erase`, `delete`, `no vlan` …).
  Ausführen nur mit `j` oder ⏎ gedrückt halten (ein Balken füllt sich, ein kurzes ⏎ reicht nicht), Esc lässt die Zeile stehen.
  Auch der Agent fragt dort bei jeder Änderung. Abschalten geht nur einzeln mit dem Schalter „Schutz“ im
  Verbindungsformular (für Test- und Laborsysteme; die Statuszeile zeigt dann „Schutz aus“). „prod“/„live“ in Ordner,
  Tag oder Alias färbt die Statuszeile weiterhin rot.
- **Aufzeichnung exportieren:** Palette „Aufnahme starten“ nimmt den Tab als asciinema-Datei (`.cast`, mit Farben und
  vim/htop) auf, abspielbar mit `asciinema play`. Protokolle (`l`, dann `x`) gehen als Markdown, Text oder nachgebaute
  Aufnahme raus; „Terminal-Verlauf als Text exportieren“ speichert den Verlauf. Alles landet im Download-Ordner.
- **Netzwerk-Scan** (f im Manager): ein Subnetz nach SSH-Geräten durchsuchen (nur Port und SSH-Kennung, ohne Anmeldung),
  Switches und Router am Banner erkennen und die Funde mit einem Druck als Verbindungen anlegen.
- **Tunnel-Manager**: `-L`, `-R`, `-D` (SOCKS5) und One-Click-Web-Services mit lokaler URL; pro Verbindung gespeichert, ein Klick schaltet an/aus, auf Wunsch startet ein Tunnel mit der Verbindung; läuft auch über Jump-Hosts
- **Vermittler für Server hinter NAT/Firewall**: der Server baut selbst die Verbindung zu einem Vermittler auf (`sshtermy anker`), sshTermy erreicht sein sshd darüber, ohne offenen Port. SSH läuft Ende-zu-Ende, Dateien und Tunnel gehen wie gewohnt. Den Vermittler bringt sshTermy selbst mit (`sshtermy vermittler`), alternativ dient Speech on DGX Spark als Vermittler ([Anleitung](docs/vermittler.md))
- **Sitzung live teilen** (Palette „teilen“): ein kurzer Dialog, nichts einzurichten. Im selben Netz teilt sshTermy direkt ohne Vermittler, sonst über den eigenen oder einen öffentlichen Vermittler (`sshtermy relay start --offen`). Der Zuschauer bekommt einen kurzen Code wie `7-taube-kompass-ofen` (oder Link/QR-Code) und tippt ihn in seine Palette. Der Code ist nach einem Rateversuch verbraucht (PAKE: der Vermittler erfährt den Schlüssel nie), der Zuschauer klopft an und du lässt ihn nach Vergleich von drei Prüfwörtern herein. Solange geteilt wird, zeigt der Tab einen roten Punkt und „● LIVE · 1 Zuschauer · 28 min“; das Teilen endet von selbst (15 min … 4 h), Zuschauer sehen erst ab jetzt, Passwortabfragen pausieren die Übertragung, das Sitzungsprotokoll hält fest, wer zugesehen hat; ein Klick auf den LIVE-Chip verwaltet die Zuschauer. Zuschauer sehen nur zu; Mitschreiben nur auf Bitte („Hand heben“), ⏎ bei produktiven Verbindungen braucht dein OK. Noch kein Vermittler? Der Vermittler-Assistent richtet ihn per SSH auf einem deiner Server ein (und entfernt ihn wieder); Kopplungscodes kommen direkt aus sshTermy
- **KI-Copilot** für jeden OpenAI-kompatiblen Endpunkt (`/v1/chat/completions`: OpenAI, vLLM, SGLang, Ollama, LiteLLM, LocalAI)
  - **Befehlspalette** (Strg+Leertaste): ein Eingabefeld für alles, Text wird Ziel oder Frage; ohne Text nur Häufiges, Verbindungen und Ordner je Bereich (KI, Sitzung, Suchen & Logs, Tabs & Ansicht, Netzwerk, Team …), Tippen findet alles direkt; Rechtsklick im Manager öffnet ein Menü zur Verbindung oder zum Ordner
  - **Ghost-Text** beim Tippen (Tab übernimmt)
  - **Folgebefehle** nach jedem Enter (Alt+1…4 fügt ein)
  - **Ziel-Modus / Agent** (F2): Ziel setzen, die KI schlägt Schritt für Schritt genau einen Befehl mit Begründung vor, du bestätigst. Text im Panel (Ziel, Schritte, Ausgaben, Fehlermeldungen) lässt sich mit der Maus markieren und wird gleich kopiert.
    Mehrzeilige Inhalte (Konfigurationsabschnitte, Skripte) kommen als Datei, die sshTermy nach ⏎ per SFTP in `~/.sshtermy-agent/<lauf>/`
    anlegt, nie als Heredoc im Befehl; Konfigurationsdateien werden gesichert und ergänzt statt ersetzt
    (`docker cp` in einen Container gilt als Gefahr). Ist die Antwort des Modells unbrauchbar, fragt sshTermy einmal nach und nennt den Fehler.
  - **Agent-Modus** (F2, Tab wählt den Modus): Der Agent arbeitet selbstständig bis zum Ziel. Er führt Befehle aus,
    sieht Ausgabe und Exit-Code, legt Skripte per SFTP in `~/.sshtermy-agent/<lauf>/` an und führt sie aus (erst Probelauf).
    Stufen: *Vorsichtig* (Standard; nur Lesendes läuft allein), *Normal* (alles außer Gefahr, sudo und Code in der Zeile wie `python -c`, `bash -c`, `eval`; als root angemeldet oder unter Windows fragt jede Änderung, weil es dort kein sudo zum Anhalten gibt), *Voll* (auch sudo).
    Gefahrenbefehle, auch jede Zeile eines Skripts, warten immer auf dich; auf Produktiv-Hosts und Netzwerkgeräten fragt
    jede Änderung nach. Grenzen: 25 Schritte, 15 Minuten, 3 Fehler in Folge. Esc oder Tippen im Terminal hält ihn an,
    `?` gibt ihm eine Anweisung. Am Ende: die vollständige Zusammenfassung (blättern mit Bild↑/Bild↓ oder Mausrad, `c`
    kopiert sie, `l` zeigt sie groß), geänderte Dateien, `s` speichert den Lauf als Runbook samt
    Skripten, `r` löscht die Skripte auf dem Host. Jeder Schritt steht als „Agent“ im Sitzungsprotokoll.
  - **Plan-Modus** (F2, Tab bis „Erst planen“): Die KI sieht sich erst nur mit lesenden Befehlen um und schreibt dann
    einen vollständigen Plan (Schritte, Skripte, Risiken, Prüfung). Schritte streichen (Leertaste), ändern (`e`), neu
    planen lassen (`?`), Ausführung wählen (Tab: Schritt für Schritt oder Agent-Stufe) und mit ⏎ freigeben. Der Agent
    hält sich dann an den Plan: ein ändernder Befehl oder eine Datei, die nicht im Plan steht, hält ihn an.
    Strg+S speichert den Plan als Runbook.
    Ziel-, Agent- und Plan-Modus zeigen den Fortschritt als Zeitleiste: erledigte Schritte mit ✓ und Dauer samt letzter
    Ausgabezeile, der laufende Schritt mit Lichtlauf, geplante Schritte als ○.
  - **KI-Begleiter** (Alt+K wählt die Stufe, Alt+J öffnet seine Leiste): Die KI liest mit und meldet sich von selbst.
    Feste Regeln entscheiden, *wann*; die KI nur, *was* sie sagt (sie darf auch still bleiben). *Leise*: eine Zeile unter
    dem Terminal nur, wenn ein Befehl fehlschlägt, eine getippte Zeile vor ⏎ riskant ist (`chmod 777`, `rm -rf`,
    `ufw disable` …; Alt+1 ersetzt die Zeile durch die sicherere Variante) oder sich derselbe Befehl wiederholt.
    *Gesprächig*: ein Kommentar zu jedem Befehl (gebündelt, höchstens alle 20 s, ersetzt die Folgebefehle). *Coach*:
    erkennt dazu, worauf du hinarbeitest („HTTPS für shop.example.de einrichten“), und bietet an, den Rest an den
    Plan-Modus (`p`) oder den Agenten „Vorsichtig“ (`g`) zu geben; beim Schließen des Tabs gibt es einen Rückblick, der
    Tatsachen ins Systemprofil übernimmt und die Sitzung als Runbook speichert. Grenzen: Anfragen pro Stunde und ein
    geschätztes Tagesbudget in Tokens. Nichts geht hinaus bei Passwortabfragen, in Vollbild-Programmen (vim, less, top),
    bei privaten Befehlen (Leerzeichen am Anfang) oder solange Ziel/Agent/Plan läuft; Tag `begleiter-aus` schaltet ihn
    pro Verbindung ab. *Begleiter nur mit eigener KI* beschränkt ihn auf eigene Endpunkte, *Filter aus bei eigener KI*
    lässt das Entfernen von Geheimnissen weg, aber nur, solange jede Anfrage an eine eigene KI geht.
  - **Eigene KI-Server** (Einstellungen › KI): Hosts wie `llm.firma.de`, die außerhalb des eigenen Netzes stehen (per
    API erreichbar), zählen dann wie lokale Endpunkte für „nur eigene KI“ und den Filter-Schalter des Begleiters.
  - **Runbooks**: Ziele mit den ausgeführten Befehlen speichern (Strg+S) und wiederverwenden, der Agent nimmt sie als Vorlage
  - **Endpunkt-Test und Modellauswahl** in den KI-Einstellungen (Strg+T testet, Strg+L listet die Modelle von `/v1/models`)
  - **KI fragen** (F3) mit Terminal-Kontext, Befehle aus der Antwort als Karten mit Syntaxfarben: einfügen (⏎/1…9), ausführen (a), kopieren (c), Erklärung einklappbar (z); schlägt ein Befehl aus einer Karte fehl, zeigt die Karte den Fehler und einen korrigierten Befehl (vom Begleiter oder eigens erfragt), u übernimmt ihn, gleiche Fehler in anderen Karten werden mitkorrigiert und die Korrektur fürs Gerät gemerkt; die KI kennt das heutige Datum und schreibt Datum/Uhrzeit in Befehlen im Format des Programms (ISO statt „10.11.“); Esc oder Strg+C bricht eine laufende Antwort ab, n stellt eine neue Frage, f eine Folgefrage mit dem bisherigen Verlauf
  - **Systemprofil pro Verbindung**: sshTermy erkennt System, Version und Befehlssatz (Linux mit Paketmanager und
    Init-System, macOS, Windows, ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS, Junos, RouterOS, Comware, UniFi-Switches …) aus
    SSH-Kennung, unsichtbarer Abfrage (nur Server) und dem mitgelesenen Terminal (Netzwerkgeräte, ohne Exec).
    Das Profil geht in jeden KI-Prompt; Befehle, die das Gerät mit „% Invalid input“ ablehnt, merkt es sich und die
    KI schlägt sie nicht mehr vor. Im Manager editierbar (System, Befehlssatz, KI-Notizen). In der Palette schreibt
    „Für dieses System umschreiben“ einen Befehl in den passenden Dialekt um.
  - **Befehlssätze je Hersteller**: Was sshTermy an einem Gerät tippt (Blättern aus, Version, CPU/RAM, Ports, VLANs,
    Konfiguration, Logs, Syslog einrichten), wie es die Ausgabe liest und was es der KI sagt, steht je Hersteller in
    einer YAML-Datei, mit Varianten für ältere oder neuere Firmware. 48 eingebaut: Switches (Cisco, Arista, Aruba, HPE
    Comware, Huawei, Dell, Extreme, Ruckus, Allied Telesis, Netgear, Zyxel, TP-Link, Lancom …), Firewalls (FortiGate,
    Palo Alto, Sophos, Check Point, SonicWall, WatchGuard, Cisco ASA, pfSense, OPNsense, VyOS, EdgeOS), WLAN-Controller
    (Aruba, Cisco WLC) und Appliances (Proxmox, ESXi, Synology, QNAP, TrueNAS, iLO, iDRAC). Palette › Einstellungen › Netzwerkgeräte ›
    *Befehlssätze*: `n` neuer Satz auf Basis eines eingebauten, `e`/`o` im Formular oder in der Datei bearbeiten, `t` am
    offenen Gerät testen, `k` KI-Entwurf aus dem Terminal, `l` die am Gerät korrigierten Befehle übernehmen, `x`/`i`
    Export/Import; Sätze geteilter Verbindungen gehen mit der Team-Freigabe mit, eigene Sätze auch mit dem Sync (am anderen Rechner mit `f` übernehmen). Sicherheit: eigene Sätze tippen nur
    Lesebefehle, können Gefahrenregeln nur ergänzen (eingebaute nie abschwächen), erkennen ein Gerät erst, wenn kein
    eingebauter Satz passt, Syslog-Schritte werden jedes Mal gezeigt und geprüft, und Team-Sätze bleiben aus, bis du sie
    freigibst (nach jeder Änderung erneut, mit Diff).
  - **Fehler erklären**: schlägt ein Befehl fehl, bietet die Vorschlagszeile „Warum? & Fix“ an (Alt+0). Die KI bekommt
    Befehl, Exit-Code und Ausgabe und antwortet mit Ursache und einfügbaren Befehlen
  - **Log-Diagnose**: Palette „KI: Log/Ausgabe auswerten“ (oder Text markieren, dann Palette): Die KI liest die Markierung
    bzw. die letzten 400 Zeilen (journalctl, `show logging`, dmesg …) und antwortet mit Lage, Auffälligem samt Zeitpunkt,
    wahrscheinlicher Ursache und lesenden Prüfbefehlen als Karten
- **Sitzungsprotokoll** (Manager: `l`): jede Sitzung wird lokal und verschlüsselt mitgeschrieben (Befehle, Ausgabe,
  Fehlschläge), durchsuchbar, mit KI-Zusammenfassung einer Sitzung (`z`) oder aller Treffer (`Z`, z. B. nach einem Host
  suchen: „Was habe ich auf Host X gemacht?“) und Markdown-Export (`x`). Ein Tag oder Host zeigt einen Aktivitätskalender (ein Kästchen
  je Tag, heller = mehr Befehle, Klick springt zum Tag) und zu welcher Tageszeit gearbeitet wurde. **Änderungsbericht** (`b`): Die KI schreibt aus
  einem Tag, einem Host oder einer Sitzung, was auf welchem Host geändert wurde (mit Befehlen, Fehlschlägen, Offenem),
  fertig für Ticket oder Doku; er liegt sofort in der Zwischenablage und als Markdown-Datei in Downloads
- **Sync zwischen Rechnern**: Verbindungen, Tunnel, Runbooks und Einstellungen Ende-zu-Ende verschlüsselt über einen Cloud-Ordner
  (OneDrive, Dropbox, iCloud, Syncthing) oder direkt über dein NAS (Synology, QNAP, Nextcloud, jeder WebDAV-Server)
- **Import** bestehender Verbindungen aus `~/.ssh/config`, PuTTY (Registry oder `.reg`-Export), MobaXterm
  (`MobaXterm.ini`, `.mxtsessions`), Ansible-Inventaren (INI oder YAML) und CSV (auch der Geräte-Export von NetBox), mit Jump-Hosts und Ordnern, ein Tastendruck (i im Manager). `e` dort exportiert alle Verbindungen als CSV (ohne Passwörter)
- **Gesamtsicherung / Umzug auf einen neuen Rechner** (Palette › Verbindungen & Daten oder `sshtermy backup`): der
  ganze Datenordner (Verbindungen, Schlüssel, Protokolle, Switch-Sicherungen, known_hosts, Einstellungen) als eine
  mit dem Master-Passwort verschlüsselte Datei (Argon2id + AES-256-GCM). Einspielen ersetzt nach Rückfrage und
  Neustart alles; der bisherige Stand bleibt daneben als `….vorher-<Datum>`
- **26 Themes** mit Live-Vorschau (a im Manager): dunkle wie Nacht, Polarnacht, Tiefsee, Graphit, Vampir, Kirschblüte, Abendrot, Neon, Tusche, Wald, Kreide, Lagune, Solar, Lagerfeuer, Bernstein und Matrix, helle wie Hell, Papier, Frost, Sand, Morgenrot und Minze, dazu Hochkontrast und Farbenblind (dunkel und hell), **eigene Farben** und einheitliche Menüsymbole (Text-Tags oder Nerd Font) und **frei belegbare Tastenkürzel** (b im Manager)
- **Deutsch und Englisch**: Sprache in der Palette („Sprache“/„Language“) oder im Über-Dialog (F12 → `l`) umschalten;
  die Wahl wird gespeichert und mit dem Sync übernommen. Beim ersten Start richtet sich sshTermy nach der Systemsprache
- **Fensterstil** (Palette „Fensterstil“): Dialoge, Palette und Auswahl werfen einen Schatten, dahinter wird es etwas
  dunkler, und Kopfzeile, Logo und Statusleiste tragen einen sanften Farbverlauf (die Statusleiste in der Akzentfarbe
  der Verbindung); wahlweise mit runden Ecken oder ganz flach wie früher (ohne Schatten und Verläufe)
- **Barrierearm** (Einstellungen › Aussehen): keine Animationen, Formen statt nur Farben (× für „nicht erreichbar“);
  dazu die **Farbenblind-Themes** (Blau/Gelb/Orange statt Grün/Rot, Okabe-Ito-Farben)
- **Lebendige Oberfläche** (Palette „Bewegung“: voll, dezent, aus): Spinner im Tab beim Verbinden, Tabs leuchten bei
  Erfolg grün und bei Fehlern rot auf, ein pulsierender Punkt zeigt neue Ausgabe in verdeckten Tabs, Maus auf einem Tab
  zeigt eine Karte mit Ziel, Dauer und Latenzverlauf. Im Manager „atmen“ die Online-Punkte, ein Ausfall blinkt und meldet
  sich (geprüft werden die im Manager sichtbaren Verbindungen, ein Ordner sofort beim Aufklappen; Einstellung
  „Online-Punkt“: aus, sichtbare, alle), die Details zeigen den Verlauf der Erreichbarkeit und bei verbundenen Servern CPU/RAM/Platte als Balken.
  Beim Verbinden zeigt eine Karte die Schritte (Verbindung, Host-Schlüssel, Anmelden, Shell) mit Zeiten und einem
  Fortschrittsbalken mit Lichtlauf, danach blendet die Shell von oben ein. Die KI-Antwort schimmert „denkt nach …“ und
  tippt sich gleichmäßig, Panels blenden ein. In der Befehlspalette leuchten die getroffenen Buchstaben, die Zeilen
  blenden nacheinander ein und die Auswahl gleitet weich. Die Statuszeile zeigt die Latenz als kleine Kurve (rot und
  „▲ zäh“ bei schlechter Leitung). In der geteilten Ansicht leuchtet die Trennlinie neben dem aktiven Bereich, beim
  Wechsel und beim Zoom (Alt+Z) gleitet ein Rahmen mit.
  Gezeichnet wird nur, solange sich etwas bewegt; Barrierearm schaltet alles ab
- **MCP-Server** (`sshtermy mcp`): Claude und andere KI-Clients können Verbindungen auflisten und lesende Befehle ausführen, alles andere erst nach deiner Bestätigung in sshTermy
- **Audit-Protokoll** (Einstellungen › Protokolle & Logs): wer hat wann auf welchem Host was getan: Verbindungen, Befehle
  (auch aus Agent-Modus und Broadcast), Dateiübertragungen, Config-Sicherungen, geänderte Verbindungen,
  Master-Passwort, Gesamtsicherungen und KI-Anfragen (nur Host/Modell). Jede Zeile ist verkettet und mit einem
  Schlüssel aus dem Store signiert; geänderte, gelöschte oder umsortierte Zeilen fallen auf. „Audit-Protokoll prüfen
  und speichern“ legt Prüfergebnis und lesbare Fassung in Downloads. Befehle mit führendem Leerzeichen erscheinen nur
  als „(privat)“
- **Debug-Protokoll für Fehlerberichte** (Einstellungen oder `sshtermy --debug`): Verbindungsversuche, Abbrüche,
  Updates, KI-Anfragen und Sync als Ereignisse, nie Passwörter oder Bildschirminhalte. Hilfe › „Debug-Protokoll
  bereinigt speichern“ legt eine Kopie ohne Hosts, IPs und Benutzernamen in Downloads, fertig für ein GitHub-Issue.
  Sehr langsame Bilder (über 50 ms) stehen dort auch
- **Adaptive Bildrate**: Flutet ein Server die Ausgabe (`cat` auf eine große Datei), zeichnet sshTermy nur so oft, wie ein
  Bild es zulässt, statt für jedes Stück; Tasten und Maus zeichnen immer sofort. Unveränderter Terminalinhalt wird nicht
  für jedes Bild neu aufgebaut. Palette „Leistungsanzeige“ bewertet die Zeichenzeit in Worten (flüssig/gut/etwas
  träge/ruckelt), zeigt je Bild einen Balken, Bilder pro Sekunde (1 im Leerlauf ist normal: gezeichnet wird nur bei
  Änderung), die mögliche Bildrate und die Ausgaberate; im Terminal steht sie links neben der F6-Leiste
- **Versionierung und Auto-Update** aus GitHub Releases (signiert). Vor dem Austausch wird die neue Programmdatei einmal gestartet und muss die angekündigte Version nennen; startet sie danach zweimal nicht sauber, stellt sshTermy die alte Version von selbst wieder her **Update-Kanal** (Einstellungen › Sicherheit › „Updates anbieten“): „sofort“ zeigt jede neue Version, „stabil“ erst Versionen, die seit einem Tag oder einer Woche veröffentlicht sind. `sshtermy -update` nimmt immer die neueste.
- **Zwei sshTermy gleichzeitig** (zweites Fenster, zweiter Start): Jedes bemerkt, wenn das andere gespeichert hat, und lädt Verbindungen und Einstellungen neu, statt sie zu überschreiben. Schlägt das Speichern fehl (Platte voll, Ordner schreibgeschützt), erscheint eine Meldung

Das Bedienkonzept der KI (Vorschläge, Ziel-Modus, feste Gefahrenregeln) stammt aus dem Fernzugang-Assistenten von
[speech-on-dgx-spark](https://github.com/db9979/speech-on-dgx-spark); die Technik ist hier neu in Go gebaut.

---

## Installation

📖 Anleitung mit Animationen: **https://db9979.github.io/sshtermy/de/**

**Paketquellen** (die kümmern sich auch um Updates):

```
winget install db9979.sshTermy                                   # Windows (sobald im winget-Katalog freigegeben)
brew tap db9979/sshtermy https://github.com/db9979/sshtermy      # macOS und Linux
brew install sshtermy
echo "deb [trusted=yes] https://db9979.github.io/sshtermy/apt ./" | sudo tee /etc/apt/sources.list.d/sshtermy.list
sudo apt update && sudo apt install sshtermy                     # Debian, Ubuntu, Raspberry Pi OS (amd64, arm64, armhf, riscv64)
```

Über Homebrew oder apt installiert, tauscht sshTermy sich nicht selbst aus, sondern zeigt den Update-Befehl.

Oder lade die passende Datei aus den [Releases](https://github.com/db9979/sshtermy/releases/latest):

| System | Datei |
|---|---|
| Windows 11 ARM64 (Surface Pro X/11, Snapdragon X) | `sshtermy_windows_arm64.exe` |
| Windows x86 32 Bit | `sshtermy_windows_386.exe` |
| Windows x64 | `sshtermy_windows_amd64.exe` |
| macOS Apple Silicon / Intel | `sshtermy_darwin_arm64` / `sshtermy_darwin_amd64` |
| Linux | `sshtermy_linux_amd64` / `sshtermy_linux_arm64` / `sshtermy_linux_arm` (32 Bit ARMv7) / `sshtermy_linux_riscv64` |

**macOS (und Linux): eine Zeile im Terminal** installiert die neueste Version, prüft die Prüfsumme und legt
`~/Applications/sshTermy.app` an (Doppelklick oder Spotlight „sshTermy“ öffnet sshTermy im eigenen Fenster, ganz
ohne Terminal; eine `.app` von einem älteren Installer öffnet das Terminal, dann die Zeile einfach noch mal
ausführen), dazu den Befehl `~/.local/bin/sshtermy`:

```
curl -fsSL https://raw.githubusercontent.com/db9979/sshtermy/main/install.sh | sh
```

Eine im Browser geladene Datei markiert macOS als Quarantäne und blockt sie beim Doppelklick („kann nicht
geöffnet werden“). Wer sie trotzdem nutzen will: `xattr -d com.apple.quarantine sshtermy_darwin_arm64 && chmod +x
sshtermy_darwin_arm64`, dann im Terminal mit `./sshtermy_darwin_arm64` starten.

Windows: Doppelklick auf die `.exe` öffnet sshTermy im eigenen Fenster. Aus PowerShell oder cmd gestartet bleibt es
im Terminal; dort ist **Windows Terminal** empfohlen (Truecolor, F-Tasten, Unicode).

**ClockworkPi uConsole** (und ähnliche Handhelds): apt und der Einzeiler wählen für jeden Kern das passende Programm
(CM4/CM5 und A-06 mit 64-Bit-System: arm64, 32-Bit-Raspberry-Pi-OS: armhf, R-01: riscv64). Der 5"-Bildschirm reicht:
Mit gut lesbarer Schrift passen etwa 80×22 bis 106×30 Zeichen, sshTermy ist ab 80×24 ausgelegt; darunter lassen
Kopfzeile und Hilfe Details weg, statt Text abzuschneiden. Die Tastatur hat keine F-Reihe:
**Fn+1 … Fn+0 = F1 … F10, Fn+- = F11, Fn+= = F12**. Für ⇧F1 … ⇧F10 *zuerst* Shift halten, dann Fn und die Ziffer.
Alles steht auch in der Befehlspalette (**Strg+Leertaste**), mit dem Trackball lassen sich Tasten und Hinweise
anklicken, und im Manager mit `b` jede Taste umlegen.

```
sshtermy              # starten
sshtermy --version    # Version, Commit, Build-Datum
sshtermy --update     # auf die neueste Version aktualisieren
sshtermy -window      # eigenes Fenster statt Terminal (automatisch, wenn ohne Terminal gestartet)
sshtermy -tui         # immer im Terminal, nie ein eigenes Fenster
sshtermy --no-intro   # ohne Startanimation (dauerhaft: Einstellungen › Bewegung „aus“)
sshtermy --debug      # Debug-Protokoll für diesen Start (debug.log im Konfigurationsordner)
sshtermy backup [datei]   # Gesamtsicherung aller Daten (fragt das Master-Passwort)
sshtermy restore <datei>  # Gesamtsicherung einspielen (ersetzt alles, alter Ordner bleibt)
sshtermy mcp              # MCP-Server für KI-Clients (stdin/stdout), siehe „sshTermy als MCP-Server“
```

## Bedienung

📖 **Handbuch & FAQ** (Einrichtung, KI-Datenschutz, Netzwerkgeräte, Umzug, MCP): [db9979.github.io/sshtermy/de/handbuch.html](https://db9979.github.io/sshtermy/de/handbuch.html) · Palette › Hilfe › „Handbuch & FAQ“

🖨 **Kurzreferenz zum Ausdrucken** (A4, 2 Seiten): [Deutsch (PDF)](https://db9979.github.io/sshtermy/handout/sshtermy-kurzreferenz-de.pdf) · [English (PDF)](https://db9979.github.io/sshtermy/handout/sshtermy-quickref-en.pdf)

| Wo | Taste | Wirkung |
|---|---|---|
| Manager | ⏎ / n / e / d / / / t / s / y / l | verbinden / neu / bearbeiten / löschen / suchen / Tunnel / KI-Einstellungen / Sync / Sitzungsprotokolle |
| | Leertaste / * | Verbindung markieren / alle sichtbaren markieren; danach wirken e (Sammeländerung), v und d auf alle markierten, Esc hebt auf |
| | Rechtsklick | Menü zu Verbindung oder Ordner: verbinden, bearbeiten, verschieben, Tunnel, SSH-Key, Schnellbefehle, Wake-on-LAN, markieren, löschen; bei Ordnern auf-/zuklappen, neue Verbindung hier, als Raster öffnen |
| | Strg+Z | letztes Löschen rückgängig (Verbindung, Ordner, Runbook; 10 s, oder Klick auf den Hinweis) |
| Verbindungsformular | Strg+T / Strg+S | Verbindung testen (erreichbar, Host-Key, Anmeldung) / speichern |
| Getrennter Tab | ⏎ / e / k / x / w | neu verbinden / Verbindung bearbeiten / geänderten Host-Key vergleichen und übernehmen / Tab schließen / aufwecken (Wake-on-LAN) |
| | i / a / b | Verbindungen importieren / Farbschema wählen (26 Themes, Vorschau) / Tastenkürzel anpassen |
| | c / q | SSH-Schlüssel erzeugen oder übertragen / Schnellbefehle bearbeiten |
| | m / f / z | Broadcast an mehrere Hosts / Netzwerk-Scan / Zeitpläne (Strg+N neu, Strg+E bearbeiten, Strg+R jetzt, Strg+O an/aus, ⏎ letztes Ergebnis) |
| | w / g | Arbeitsplätze (gespeicherte Aufteilungen) / Ordner als Raster öffnen |
| Ordner | o / r / v / d | Ordner anlegen („A / B“ verschachtelt) / bearbeiten (Name, KI-Hausregeln) / verschieben / auflösen (Inhalt bleibt) |
| | ← → Space / - + | Ordner zu-/aufklappen / alle zu bzw. auf |
| | Shift+↑↓ (Alt+↑↓), Maus ziehen | Reihenfolge ändern, in einen Ordner ziehen |
| | # , /#tag | nach Tag filtern, Suche über Name, Host, Ordner, Tags |
| Broadcast | Space / g / a, Tab, Strg+S, p | Host / Ordner / alle wählen, Hosts·Quelle·Befehl, prüfen und bestätigen, Probelauf an/aus |
| | ←→ / ↑↓ / z / c / r / x | Host-Spalte / blättern / groß / Ausgabe kopieren / nochmal / abbrechen |
| Netzwerk-Scan | Strg+S, Space / a, ⏎ | Scan starten, Funde wählen, als Verbindungen anlegen |
| Terminal | F2 | Ziel-Modus (Agent) |
| | F3 | KI fragen (Esc bricht die Antwort ab, n neue Frage, f Folgefrage, u Fix nach Fehler übernehmen) |
| | F4 | Tunnel an/aus (⏎ oder Klick), a mit Verbindung starten, Web-Service im Browser öffnen |
| | F5 | Dateien (SFTP): Tab Seite, ⏎/Doppelklick öffnen bzw. ansehen, c kopieren (hoch/runter), e eigener Editor, o Standardprogramm, Leertaste/a markieren, Strg/Umschalt+Klick markieren, Ziehen kopiert, Rechtsklick Menü, F2 umbenennen, y Pfad kopieren, s sortieren, v Vorschau, n Ordner, d löschen, . versteckte; im Betrachter / suchen, f mitlesen, w umbrechen |
| | F6 | Host-Übersicht: r/⏎ abfragen, t Zeitraum der Graphen 5 min/1 h, z groß, l Log-Blick, c Container, b Switch-Config sichern, ↑↓ Stand, d Diff, v ansehen |
| | F7 / F8 | vorheriger / nächster Tab |
| | Alt+T | alle Tabs (Liste: springen, schließen, umbenennen, duplizieren, verschieben) |
| | Alt+S | in allen offenen Tabs suchen, ⏎ springt zur Trefferzeile |
| | Alt+N / Alt+W | neuer Tab / Tab schließen (auch `+` und `×` in der Tab-Leiste, Rechtsklick auf einen Tab öffnet sein Menü, Maus darauf zeigt Ziel, Dauer und Latenz) |
| | Alt+L | lokale Shell in neuem Tab (auch im Manager; Palette: `lokal`, `pwsh`, `cmd`, `wsl`, `zsh` …) |
| | F9 / ⇧F1…F8 | Schnellbefehle: Liste / direkt ausführen (`{{name}}` im Befehl fragt nach einem Wert) |
| | F10 | zurück zum Manager (Sitzungen laufen weiter) |
| | ⇧F9 | Tastenzeile erklärt die Tasten des Bereichs |
| | ⇧F10 | Tab in ein eigenes Terminalfenster abtrennen (Windows Terminal/Konsole, Terminal.app/iTerm2, Linux-Terminals, tmux; im eigenen Fenster von sshTermy ein zweites eigenes Fenster); die Sitzung bleibt offen, Fenster schließen oder ⏎ holt sie zurück. Eigenes Terminal: `SSHTERMY_TERMINAL="kitty {cmd}"` |
| | Strg+F | im Verlauf suchen (Strg+E nur Fehlerzeilen, Tab Fehlerliste) |
| | Strg+T | Aufteilung wählen: eine, zwei nebeneinander/untereinander, groß + zwei, drei, vier, sechs (Tasten 1…7); im Fenster ⇧+Pfeil Größe, `=` gleich groß, `z` Zoom, `d` andere abdunkeln, `w` Arbeitsplätze |
| | Alt+I | geteilte Ansicht: Eingabe gleichzeitig an alle sichtbaren Bereiche |
| | Strg+Pfeil / Alt+⇧+Pfeil / Alt+Z | geteilte Ansicht: Bereich wechseln / Trennlinie verschieben / aktiven Bereich zoomen. Maus: Trennlinie ziehen (Doppelklick = gleich groß), Titel auf anderen Bereich ziehen = tauschen, Doppelklick auf Titel = Zoom |
| | Tab oder → | Ghost-Text übernehmen |
| | Alt+1…4 | Folgebefehl einfügen (ohne Enter; rote zweimal) |
| | Alt+0 | nach einem Fehlschlag: KI erklärt Ursache und schlägt einen Fix vor |
| | Alt+E | Log-Blick: Auffälligkeiten in den Logs (⏎ Zeilen, 1…4 prüfen, k KI erklärt, b KI-Lagebericht, x/X ausblenden, n Was ist neu?, t Zeitleiste, z Zeitraum, w Logwache, s Syslog am Switch einrichten) |
| | Alt+K / Alt+J | KI-Begleiter: Stufe Aus · Leise · Gesprächig · Coach / Begleiter-Leiste |
| Begleiter-Leiste | ↑↓ / ⏎ / a / c | Vorschlag wählen / einfügen / ausführen (gefährliche zweimal) / kopieren |
| | m / l / s / ? / r | Thema stumm / Tatsache merken / Stufe / zuletzt gesendet / Rückblick |
| | p / g / n / x | erkannte Absicht: Plan / Agent übernimmt / selbst machen / nicht mehr fragen |
| | Strg+Klick / Rechtsklick | URL im Browser öffnen / Menü zu Link, IP oder Host: verbinden (direkt oder über den Server), Ping vom Server oder von hier, kopieren |
| | Einfügen | mehrzeilig erst Vorschau (⏎ einfügen, o ohne letztes Enter, Esc abbrechen); Palette „Einfügeschutz aus“ |
| Ziel-Modus | ↑↓ / Strg+S / Strg+D | Runbook wählen / Ziel als Runbook speichern / Runbook löschen |
| Ziel-Modus, Datei-Schritt | ⏎ / v / s | Datei auf dem Host anlegen / ansehen / überspringen; am Ende löscht `r` den Arbeitsordner |
| | ⏎ / i / e / s / r / x | ausführen / nur einfügen (Dry-Run) / bearbeiten / überspringen / neu / beenden |
| | Tab | Schritt für Schritt → Agent *Vorsichtig* → *Normal* → *Voll* → *Erst planen* |
| Agent-Modus | Esc / ? / v / x | anhalten / Anweisung geben / Skript ansehen / beenden; am Ende `s` als Runbook speichern, `r` Skripte auf dem Host löschen, `c` Ergebnis kopieren, `l` ganz lesen |
| Ziel-/Agent-/Plan-Panel, KI fragen | Bild↑ / Bild↓, Mausrad | langen Inhalt blättern (Zusammenfassung, Plan, Antwort) |
| | Ziehen / Doppelklick / Dreifachklick, Rechtsklick | Text im Panel markieren (Ziel, Schritte, Ausgaben, Fehlermeldungen) und gleich kopieren (Zwischenablage + OSC 52): Wort / Zeile; Rechtsklick kopiert die Markierung erneut |
| Plan-Modus | ↑↓ / Leertaste / e / v / ? | Schritt wählen / streichen oder zurückholen / ändern / Skript ansehen / neu planen |
| | Tab / ⏎ / x | Ausführung (Schritt für Schritt oder Agent-Stufe) / freigeben und ausführen / verwerfen; Strg+S speichert den Plan als Runbook |
| KI-Eingaben | ⇧⏎ / Strg+J | neue Zeile in Ziel, Frage und Palette (lange Texte brechen automatisch um, das Feld wächst mit) |
| Protokolle | / / Tab / f / s | suchen (auch in der Ausgabe) / nach Tagen oder nach Host / Filter heute · 7 Tage · Fehler / Hosts sortieren |
| | z / Z / b / x / d / a | KI-Zusammenfassung / alle Treffer zusammenfassen / Änderungsbericht / Export (md, txt, cast) / löschen / Mitschreiben an/aus |
| KI-Einstellungen | Strg+T / Strg+L | Endpunkt testen / Modell aus der Liste des Endpunkts wählen |
| Überall | Strg+Leertaste | Befehlspalette: ⏎ als Ziel starten, Tab KI fragen, ↑↓ Vorschlag/Tunnel/Verbindung wählen; `log` = KI wertet Ausgabe aus, `farb` = Farbregeln |
| | F1 / F12 / Strg+Q | Hilfe / Über + Update / beenden (fragt bei offenen Verbindungen; Strg+Q nochmal = beenden) |
| | F12 → l / F12 → w | Sprache wechseln (Deutsch ↔ Englisch) / Was ist neu |
| Eigenes Fenster | Strg+⇧V · ⇧Einfg (Windows auch Strg+V, Mac ⌘V) / Strg + · Strg − · Strg 0 (Mac: ⌘) | einfügen / Schrift größer · kleiner · Standard (auch Strg + Mausrad, wird gemerkt); F11 Vollbild; auf dem Mac wirkt ⌘ als Alt, ⌥ schreibt weiter @ [ ] { } \| ~ |

Warum F-Tasten statt `Strg+A`? `Strg+A` ist in bash/zsh „Zeilenanfang“ und das Präfix von GNU screen. sshTermy
belegt nur Tasten, die Shells und Vollbildprogramme praktisch nie brauchen.

**Tasten anpassen** (b im Manager oder Palette → „Tastenkürzel anpassen“): Aktion wählen, ⏎, neue Taste drücken.
Entf setzt eine Aktion zurück, r alle. Doppelte Belegungen werden rot markiert. Alt+⏎ ist gesperrt, weil
Windows Terminal damit auf Vollbild schaltet. Die Belegung wandert mit dem Sync auf deine anderen Rechner. `l`
schaltet auf „nur dieser Rechner“: dann gelten neue Tasten nur hier (markiert mit ◆) und gehen vor der Belegung aus
dem Sync, z. B. auf der uConsole ohne F-Reihe oder im Mac-Terminal ohne Alt; r entfernt sie in diesem Modus wieder.

**Sprache:** Palette (Strg+Leertaste) → „Sprache“ oder F12 → `l` schaltet zwischen Deutsch und Englisch um, sofort
und ohne Neustart. Die Wahl steht in den Einstellungen (geht mit dem Sync auf andere Rechner) und zusätzlich
unverschlüsselt in der Datei `language` im Konfigurationsordner, damit schon der Entsperr-Bildschirm sie kennt.
Ohne diese Datei entscheidet die Systemsprache (`LANG`/`LC_ALL`, unter Windows die Anzeigesprache): Deutsch bei
einem deutschen System, sonst Englisch. Die Hinweise an die KI bleiben deutsch; bei Englisch antwortet sie englisch.

**Aussehen:** a im Manager wechselt Dunkel → Hell → Hochkontrast (dunkel/hell) → Farbenblind (dunkel/hell) → Matrix (grün) → Bernstein (Retro) → Neon (oder
Palette → Einstellungen › „Aussehen“). **Farben anpassen** im selben Untermenü: Vorlage wählen, dann jede Farbe mit
←/→ (Farbton), +/− (Helligkeit) oder als Hex-Wert ändern; die Oberfläche zeigt die Änderung sofort, ein Kontrast-Check
warnt vor schlecht lesbaren Farben, Strg+S speichert (geht mit dem Sync mit).

**Symbole:** Einstellungen › „Symbole: …“ bietet vier Stile (gehen mit dem Sync mit). Drei davon laufen mit jeder
Terminalschrift, weil sie nur Zeichen nutzen, die Cascadia Mono, Menlo und Co. selbst haben:
**Text-Tags** (Standard) wie `[+]` `[k]` `[/]`, im Manager mit Prompt-Zeichen (`$` Linux, `%` macOS, `>` Windows,
`#` Switch); **Unicode-Formen** wie `+ ¤ ◎ ◈ ¶`; **Minimal** mit `▸` für Aktionen, `»` für Untermenüs und `◆` für
die KI. Der vierte Stil **Nerd Font** zeigt echte Bildsymbole, braucht aber eine Nerd Font im Terminal (z. B.
„Cascadia Mono NF“). Vorher zeigt sshTermy eine Probezeile: Erscheinen dort Kästchen oder „?“, fehlt die Schrift
(Mac: `brew install --cask font-caskaydia-mono-nerd-font`, dann im Terminalprofil als Schrift wählen).

Die Schriftgröße gehört dem Terminalfenster: Strg + Plus/Minus in Windows Terminal, ⌘ + / − unter macOS.

**Import** (i im Manager): sshTermy sucht selbst nach `~/.ssh/config` (mit `Include`, `Host *`-Vorgaben und
`ProxyJump`), den PuTTY-Sitzungen in der Registry und `%APPDATA%\MobaXterm\MobaXterm.ini`. Neue Ziele sind
vorausgewählt, schon vorhandene abgewählt; ⏎ übernimmt. Mit f lädst du eine Datei von woanders, z. B. einen
PuTTY-Export (`reg export HKCU\Software\SimonTatham\PuTTY putty.reg`), eine portable `MobaXterm.ini` oder eine
`.mxtsessions`-Datei. Passwörter werden nicht übernommen, sshTermy fragt beim ersten Verbinden. PuTTY-Keys
(`.ppk`, Version 2 und 3, auch mit Passphrase) liest sshTermy direkt, Pageant ist nicht nötig.

**Telnet und serielle Konsole:** Im Verbindungsformular wählt „Verbindungsart“ zwischen SSH, Telnet und
„Seriell (Konsolenkabel)“, z. B. für einen Aruba-Switch am USB-Konsolenkabel. Schnittstelle „automatisch“ nimmt das
einzige eingesteckte USB-Konsolenkabel, so klappt eine synchronisierte Verbindung auch auf dem anderen Rechner (COM5
unter Windows, `/dev/cu.usbserial-…` unter macOS, `/dev/ttyUSB0` unter Linux). Mit „Nur dieser Rechner“ gilt eine
fest gewählte Schnittstelle nur hier, die anderen Rechner behalten ihre. Standard 9600 Baud 8N1. Sind Benutzer
und Passwort hinterlegt, tippt sshTermy sie ein, sobald das Gerät fragt (und beantwortet „Press any key to
continue“). In der Palette verbindet `telnet 10.0.0.5`, `COM3` oder `konsole /dev/ttyUSB0 115200` sofort;
Palette → „Break senden“ schickt ein serielles Break. Host-Übersicht (F6), Config-Sicherung, Schnellbefehle und KI
laufen wie bei SSH; Dateien, Tunnel und Broadcast brauchen SSH. Linux: Benutzer muss in der Gruppe `dialout` sein.
Telnet ist unverschlüsselt, nur in vertrauenswürdigen Netzen verwenden.

**Lokale Shell:** Alt+L (oder der erste Eintrag unter Alt+N, oder Palette → „Lokale Shell öffnen“) öffnet die Shell
des eigenen Rechners in einem Tab: unter Windows PowerShell 7 (`pwsh`), sonst Windows PowerShell oder cmd (ConPTY, ab
Windows 10 1809), unter macOS die Login-Shell, unter Linux `$SHELL`. `pwsh`, `cmd`, `wsl -d Ubuntu` oder `zsh` in der
Palette startet genau diese. Als gespeicherte Verbindung („Verbindungsart: Lokal“) lassen sich Shell
und Startordner einstellen; leer nimmt jeder Rechner seinen eigenen Standard, ein gesyncter Eintrag passt also unter
Windows wie auf dem Mac. Mit „Nur dieser Rechner“ gilt eine Shell wie `pwsh` nur auf diesem Rechner, die anderen
behalten ihre. Die KI weiß, ob sie mit PowerShell, cmd oder einer Unix-Shell spricht; Gefahren-Check und
Agent-Stufen arbeiten wie auf einem Server. Dateien (F5), Tunnel und Broadcast brauchen SSH.

**PowerShell-Fernzugriff (WinRM):** für Windows-Server ohne SSH. Im Verbindungsformular „Verbindungsart: PowerShell“
wählen, Host, Benutzer (`DOMÄNE\name` oder `name@domäne`) und Passwort (oder Schlüsselbund/Passwortmanager) eintragen.
Standard ist Port 5985 (HTTP); NTLM verschlüsselt dann jede Nachricht. Mit „HTTPS“ nimmt sshTermy 5986; „Zertifikat
nicht prüfen“ ist nur für selbst signierte Zertifikate im eigenen Netz gedacht. „basic“ geht nur über HTTPS. In der
Palette verbindet `Enter-PSSession srv01`, `Enter-PSSession -ComputerName srv01 -Credential FIRMA\admin -UseSSL`,
`etsn srv01` oder `winrm admin@srv01` sofort. Die Sitzung fühlt sich an wie `Enter-PSSession`: Prompt
`[SRV01]: PS C:\…>`, ↑/↓ Verlauf, Tab/⇧Tab schaltet durch die Ergänzungen vom Server, `function … {` geht in der
nächsten Zeile weiter (`>>`), `Read-Host` und Bestätigungsfragen bekommen die Antwort. Variablen und der aktuelle
Ordner bleiben erhalten. Strg+C bricht einen laufenden Befehl ab, indem sshTermy PowerShell im selben Ordner neu
startet (Variablen sind dann weg). Ohne Konsole auf dem Server laufen Vollbild-Programme (`edit`, `vim`) nicht. Auf dem
Server (als Administrator): `Enable-PSRemoting -Force`. Reines Go (WinRS); Negotiate/NTLM klappt auch mit
Domänenkonten. Für Server, die nur Kerberos erlauben, „kerberos“ wählen: den Rechnernamen eintragen (z. B.
`srv01.firma.local`, keine IP) und den Benutzer als `name@firma.local`; den Domänencontroller findet sshTermy über DNS
(oder `krb5.conf`/`KRB5_CONFIG`). Unter Linux/macOS reicht ein Ticket von `kinit`, sonst fragt sshTermy nach dem
Passwort (die Windows-Anmeldung selbst lässt sich dafür nicht nutzen). Alternativ geht Windows-OpenSSH über normales SSH (PowerShell als
`DefaultShell`), dann auch mit Dateien und Tunneln.

## Screens

| Entsperren | Verbindungs-Manager |
|---|---|
| ![](docs/bilder/01-entsperren.png) | ![](docs/bilder/02-manager.png) |
| **Ghost-Text** | **Gefährlicher Befehl** |
| ![](docs/bilder/07-ghost.png) | ![](docs/bilder/04-gefahr.png) |
| **KI fragen** | **Tunnel & Web-Services** |
| ![](docs/bilder/05-frage.png) | ![](docs/bilder/06-tunnel.png) |

**Befehlspalette (Strg+Leertaste):** ein Feld für alles, Text wird zum Ziel oder zur Frage. Ohne Text bleibt sie kurz: oben **Häufig** (im Terminal Ziel-Modus, Host-Übersicht, Suche, Aufteilung, neuer Tab), dann die zuletzt benutzten Verbindungen und darunter je Bereich ein Ordner: **Schnellbefehle & Tunnel ›, KI › (Ziel, Agent, KI fragen, Vorgang, Begleiter), Diese Sitzung › (Host, Dateien, Container, Aufnahme), Suchen & Logs › (Verlauf, alle Tabs, Fehlerzeilen, Log-Blick, Protokolle, Ordner-Scan, Logwache), Tabs & Ansicht › (Tabs, Aufteilung ‹ Wert ›, Zoom, Arbeitsplätze), Mehrere Hosts & Netzwerk ›, Team & Teilen ›, Verbindungen & Daten › (neu, SSH-Key, Import, Export, Gesamtsicherung)** (rechts die Zahl der Einträge). Tasten, die nur im Manager wirken, stehen im Terminal nicht dabei. ⏎ oder → öffnet einen Ordner, Esc oder ← führt zurück. Wer tippt, bekommt alle Treffer aus allen Ordnern direkt.
Schalter liegen im Untermenü **[:] Einstellungen ›** in vier Gruppen: Allgemein, Aussehen, Sicherheit, Einrichten (rechts steht „an“/„aus“, die Palette bleibt beim Umschalten offen). Tastenkürzel, Tour, Neuigkeiten, Update und „Updates anbieten“ stehen unter **[?] Hilfe & Info ›**, Debug-Protokoll und Leistungsanzeige dort unter **Fehlersuche ›**. Einträge mit mehreren Werten zeigen sie als ‹ Wert ›: ⏎, → oder Klick schaltet weiter, ← zurück. KI, Protokolle & Logs (Sitzungsprotokoll, Logwache, Syslog, Audit), Netzwerkgeräte (Befehlssätze, Config-Regeln) und Vermittler haben eigene Unterordner. Bewegung „aus“ schaltet auch die Startanimation ab. Wer tippt, findet auch die Punkte aus den Untermenüs direkt, sogar Einzelwerte wie „Matrix“; Esc oder ← führt zurück.

![Befehlspalette](docs/bilder/09-palette.png)

| Dateien (SFTP, F5) | Runbooks im Ziel-Modus |
|---|---|
| ![](docs/bilder/12-dateien.png) | ![](docs/bilder/11-runbooks.png) |
| **KI-Endpunkt testen, Modell wählen** | **Agent-Modus: arbeitet selbstständig, legt Skripte an** |
| ![](docs/bilder/10-ki-test.png) | ![](docs/bilder/16-agent.png) |
| **Geteilte Ansicht: groß + zwei** | **Tab-Liste (Alt+T)** |
| ![](docs/bilder/17-geteilt.png) | ![](docs/bilder/18-tabs.png) |
| **Sitzungsprotokolle nach Tagen** | |
| ![](docs/bilder/19-protokolle.png) | |
| **Host-Übersicht Server (F6)** | **Switch: Übersicht und Config-Sicherung** |
| ![](docs/bilder/15-host-server.png) | ![](docs/bilder/14-host-switch.png) |

Die Bilder sind echte Aufnahmen der App (gegen einen Test-SSH-Server und ein Test-LLM aus `tools/dev`).

---

## Architektur

```
cmd/sshtermy/          Einstieg: Flags (--version, --update), Store öffnen, TUI starten
cmd/sshtermy-sign/     Ed25519-Schlüssel erzeugen, Releases signieren (CI)
internal/
  domain/              reine Datenmodelle: Connection, Tunnel, LLMConfig, Settings, Secret
  vault/               Verschlüsselung: AES-256-GCM, Argon2id, DPAPI (Windows)
  store/               Repository über dem Vault: CRUD, Pfade (%APPDATA%\sshtermy)
  sshx/                SSH: Dial (+Jump-Host), Auth (Passwort/Key/Agent), Host-Keys (TOFU),
                       PTY-Shell, Tunnel-Manager (-L/-R/-D), SOCKS5-Server, Browser öffnen,
                       Befehle per Exec oder in die Shell getippt (Netzwerkgeräte)
  broadcast/           Befehl/Runbook parallel auf mehreren Hosts, Gefahrenprüfung
  netscan/             Subnetz nach SSH durchsuchen, Banner lesen, Gerätetyp erkennen
  llm/                 OpenAI-kompatibler Client (/v1/chat/completions, Streaming per SSE)
  agent/               KI-Logik: Ghost-Text, Folgebefehle, Ziel-Modus, Frage; Prompts,
                       striktes Parsen, Entfernen von Geheimnissen
  sessionlog/          Sitzungsprotokolle: verschlüsselte Dateien je Sitzung, Suche, Markdown
  safety/              feste Gefahrenregeln (Linux/macOS und Windows/PowerShell)
  termemu/             Terminal-Emulation (charmbracelet/x/vt): Raster, Scrollback, Ghost-Overlay
  update/              Auto-Update aus GitHub Releases (SHA-256 + Ed25519)
  version/             Name, Version, Credits an genau einer Stelle
  i18n/                Sprache der Oberfläche: deutsche Texte sind die Schlüssel, en.go die englische Fassung
  ui/                  Oberfläche (bubbletea v2, lipgloss v2): Screens, Panels, Formulare
tools/dev/             Test-SSH-Server, Test-LLM, ANSI→HTML für Screenshots
```

Abhängigkeiten zeigen nur nach innen: `ui → agent/sshx/store/update → llm/safety/vault → domain`.
`domain` kennt nichts außer der Standardbibliothek.

### Ablauf

```
Start ─▶ Entsperren (DPAPI automatisch │ Master-Passwort │ Ersteinrichtung)
      ─▶ Verbindungs-Manager
           ├─ n/e  Formular (Alias, Host, Port, Benutzer, Auth, Ordner, Tags, Jump-Host-Auswahl mit Suche)
           ├─ t    Tunnel-Formular (local/remote/dynamic, Web-Service, Autostart)
           ├─ s    KI-Einstellungen (Base-URL, API-Key, Modell, Tokens, Temperatur …)
           └─ ⏎    Host-Key prüfen ─▶ Terminal-Tab
                     ├─ Ghost-Text (Pause beim Tippen → Vorschlag → Tab)
                     ├─ Enter → Ausgabe ruht → Folgebefehle als Chips
                     ├─ F2 Ziel → Schritt vorschlagen → ⏎ → Ausgabe ruht, Prompt da → nächster Schritt … ✓ fertig
                     ├─ F3 Frage (gestreamte Antwort, Befehle nummeriert)
                     └─ F4 Tunnel (Status, Zähler, URL)
```

### Technik-Entscheidungen

| Thema | Wahl | Grund |
|---|---|---|
| Sprache | Go 1.26 | Ein statisches Binary pro Plattform, Cross-Compile ohne Toolchain (`GOOS=windows GOARCH=arm64`), kein cgo |
| TUI | bubbletea v2 + lipgloss v2 | aktueller Charm-Stack: echte Cursor-Steuerung, Ebenen für Overlays, Kitty-Keyboard, Windows-Konsole |
| Terminal-Emulation | charmbracelet/x/vt | aktiv gepflegt, Truecolor, Alt-Screen, Scrollback, Breitzeichen; kodiert Tasten passend zum Modus |
| SSH | golang.org/x/crypto/ssh | Referenz-Implementierung in Go, Agent, known_hosts, Forwarding |
| Windows-Agent | Named Pipe via go-winio | Windows-OpenSSH-Agent (`\\.\pipe\openssh-ssh-agent`), 1Password, KeePassXC |
| Speicher | verschlüsselte JSON-Datei | eine Datei, atomar geschrieben, auch Metadaten (Hosts, Benutzer) verschlüsselt |

---

## Sicherheit

### Verschlüsselter Store (`internal/vault`)

```
Master-Passwort ──Argon2id (t=3, 64 MiB, p=4)──▶ KEK ──AES-256-GCM──┐
                                                                     ├─▶ DEK (256 Bit, zufällig)
Windows-Anmeldung ──DPAPI CryptProtectData───────────────────────────┘        │
                                                                               ▼
                                         AES-256-GCM(DEK) über den ganzen Store
```

- **DEK** (Data Encryption Key) aus `crypto/rand` verschlüsselt den **gesamten** Store in einem Stück
  (Passwörter, Private Keys, Passphrasen, API-Keys und auch Hostnamen, Benutzer, Tunnel).
- **AES-256-GCM** mit frischer 96-Bit-Nonce bei jedem Speichern. Der Header (KDF-Parameter, Wraps) ist als
  *Additional Authenticated Data* mitgeprüft: ein Herabsetzen der KDF-Parameter oder Vertauschen von Teilen
  lässt das Entschlüsseln scheitern (Test `TestTamperedHeaderIsRejected`).
- **Master-Passwort** wird nie gespeichert; **Argon2id** (RFC 9106) leitet daraus den KEK ab, der den DEK einwickelt.
- **DPAPI** (nur Windows, `CryptProtectData` ohne `LOCAL_MACHINE`, mit eigenem Entropy-Wert) bindet eine zweite
  Kopie des DEK an dein Windows-Konto: nach der Windows-Anmeldung entsperrt sich sshTermy ohne Eingabe, ist aber
  für andere Konten und Rechner unlesbar. Das Master-Passwort bleibt als Wiederherstellung (Umzug, macOS). Ein- und ausschalten jederzeit unter „Master-Passwort ändern“.
- Datei `store.vault` unter `%APPDATA%\sshtermy` (macOS `~/Library/Application Support/sshtermy`, Linux
  `~/.config/sshtermy`), Rechte 0600, atomar geschrieben. `SSHTERMY_HOME` für eine portable Installation.
- Automatische Sperre nach Leerlauf (`LockAfterMin`), DEK wird beim Sperren genullt (best effort).
- `domain.Secret` gibt sich in Logs und Fehlermeldungen nie preis (`••••••`).

### SSH

- **Host-Keys:** Trust On First Use mit SHA-256-Fingerprint im Dialog, eigene `known_hosts`. Ein **geänderter**
  Host-Key bricht die Verbindung ab, ohne „trotzdem verbinden“ (möglicher Man-in-the-Middle).
- Agent-Auth bevorzugt: Private Keys bleiben im Agent. Eingebettete Keys liegen nur verschlüsselt im Store.
- Tunnel binden standardmäßig auf `127.0.0.1`. SOCKS5 löst Namen auf dem Server auf (keine lokalen DNS-Leaks).
- **SSH-Schlüssel (`c` im Manager):** erzeugt ed25519- oder RSA-4096-Keys (wie `ssh-keygen`, wahlweise mit
  Passphrase, als Datei in `~/.ssh` oder eingebettet), trägt sie wie `ssh-copy-id` in `~/.ssh/authorized_keys`
  ein und stellt die Verbindung erst nach einer geglückten Probe-Anmeldung mit dem Key um.
- **Schlüsselbund:** Passwort oder Key-Passphrase können statt im Store im macOS-Schlüsselbund, in der
  Windows-Anmeldeinformationsverwaltung oder im Secret Service (Linux, `secret-tool`) liegen. Jeder Rechner
  hat seinen eigenen Eintrag; fehlt er nach dem Sync, fragt sshTermy einmal und merkt sich die Antwort dort.

### KI

- Die KI **tippt nie selbst**. Jeder Befehl ist ein Vorschlag; erst deine Bestätigung sendet ihn ans PTY.
- **Feste Gefahrenregeln** (`internal/safety`), nie das Modell, entscheiden über Rot: `rm -rf`, `find -delete`, `dd`, `mkfs`,
  `curl | sh`, `… | base64 -d | sh`, Neustart, Firewall leeren, Benutzer ändern, `git reset --hard`, Pakete entfernen, Docker prune,
  SSH-Dienst stoppen, Systemdateien löschen, Netzwerk abschalten, `iptables -P … DROP`, `kubectl delete`, `DROP DATABASE`, `zfs destroy`, `rsync --delete`;
  für Windows `Remove-Item -Recurse`, `Format-Volume`, `iex (iwr …)`, WinRM stoppen, `Disable-NetAdapter` u. a.
  Jeder Befehl wird gegen alle Regelgruppen geprüft (Unix, Windows, Netzwerkgeräte), Mehrzeiliges Zeile für Zeile: ein Switch, der für einen Server gehalten wurde, versteckt kein `erase startup-config`.
  Rote Befehle brauchen zwei Bestätigungen; bearbeitete Befehle werden neu geprüft. `sudo` wird markiert, auch hinter `&&`, `;` oder `|`.
- **Geheimnisse lesen fragt auch:** `/etc/shadow`, private Schlüssel, `.env`, `kubectl get secret`, `docker inspect`, `printenv` oder `show running-config` am Switch ändern nichts, ihre Ausgabe ginge aber an die KI. Agent und MCP fragen deshalb immer; der Plan-Modus lässt sie beim Erkunden aus.
- **Dialoge, die von selbst aufgehen** (Host-Key, Passwort, MCP-Anfrage, Dateien vom Server), nehmen eine Sekunde lang keine Tasten an: ein `j` aus `journalctl`, das du gerade tippst, bestätigt nichts. Ist schon ein Dialog offen, warten sie, statt ihn zu ersetzen.
- **Einfügeschutz:** Dieselben Regeln prüfen jede eingefügte Zeile. Ein Treffer zeigt die Vorschau mit der
  roten Zeile und braucht zwei Bestätigungen, auch wenn die Mehrzeilen-Vorschau abgeschaltet ist.
- **Prompt Injection:** alles aus dem Terminal steht als Material in einem Block `<<< … >>>`, dessen Marker im
  Inhalt entschärft werden; der System-Prompt sagt, dass Material nie eine Anweisung ist.
- **Striktes Parsen:** nur das erwartete JSON, keine Steuerzeichen (ESC-Sequenzen aus dem Modell kommen nie auf
  den Bildschirm), Längenlimits, `<think>` von Reasoning-Modellen wird entfernt.
- **Geheimnisse** (Private Keys, AWS/GitHub/OpenAI-Keys, Bearer-Token, `password=…`, `user:pass@`) werden vor dem
  Senden entfernt; an einer Passwortabfrage gibt es keinen Ghost-Text.
- API-Key geht nie per `http://` an einen fremden Host (außer bewusst erlaubt).
- **KI-Datenschutz** (Einstellungen › KI): „alle Anbieter“, „nur lokale KI“ (nur Endpunkte im eigenen Netz:
  localhost, private IPs, Namen auf `.local`/`.lan`/`.internal`; Cloud-Ersatz wird übersprungen) oder „KI aus“.
  Pro Verbindung wirken die Tags `ki-lokal` bzw. `ki-aus` (auch `ai-local`, `no-ai`) genauso; die strengere
  Einstellung gilt. Im selben Ordner zeigt „Letzte KI-Anfrage ansehen“ genau, was zuletzt an welchen Host und
  welches Modell ging.
- Ratenlimit im Ziel-Modus (Standard 6 Schritte/min, max. 40 Schritte pro Ziel).

### sshTermy als MCP-Server

KI-Clients wie Claude Desktop oder Claude Code können deine gespeicherten Verbindungen über das Model Context
Protocol nutzen. Einschalten unter Einstellungen › KI › „MCP-Zugriff für KI-Clients“: **nur lesen**
(Verbindungen und offene Tabs auflisten, einen Tab lesen, lesende Befehle wie `df -h` oder `show interfaces`) oder
**mit Rückfrage** (jeder andere Befehl läuft erst, wenn du in sshTermy `j` drückst; gefährliche brauchen `j` zweimal). Befehle mit `sudo` oder solche, die Geheimnisse lesen (`/etc/shadow`, Schlüssel, `.env`, `show running-config` …), fragen immer, auch wenn sie nur lesen.
sshTermy muss laufen und entsperrt sein; `sshtermy mcp` ist nur die Brücke und hält keine Daten. Verbindungen mit Tag
`ki-aus`/`ki-lokal` oder KI-Datenschutz „KI aus“ tauchen nie auf. Geheimnisse in Ausgaben werden entfernt, jeder
Aufruf steht im Audit-Protokoll.

```
claude mcp add sshtermy -- sshtermy mcp
```

Claude Desktop (`claude_desktop_config.json`): `{"mcpServers": {"sshtermy": {"command": "sshtermy", "args": ["mcp"]}}}`

### KI-Anbieter und Ersatz

- **Anbieter** in den KI-Einstellungen (`s`): Spark bzw. jeder OpenAI-kompatible Server (vLLM, SGLang, LiteLLM,
  LM Studio), Ollama, OpenAI oder Claude (Anthropic Messages-API). ←/→ wechselt und belegt Adresse und Modell vor;
  Strg+L listet die Modelle des Anbieters, Strg+T testet.
- **Ersatz-Anbieter** (Einstellungen → „Ersatz-Anbieter“ oder `r` im KI-Status): bis zu drei, die der Reihe nach
  einspringen, wenn der Endpunkt nicht antwortet. Ein ausgefallener Endpunkt wird 5 Minuten übersprungen oder bis
  die Hintergrundprüfung ihn wieder erreicht. Der Status-Punkt wird gelb und zeigt „Ersatz: Claude“, solange ein
  Ersatz antwortet.
- Cloud-Dienste brauchen immer einen API-Key; er liegt verschlüsselt wie die Passwörter.

### KI schneller und gezielter

- **Jede Funktion hat ihre eigene Rolle:** Ghost-Text, Folgebefehle, Ziel-Modus, Agent, Plan, Begleiter, Erklären,
  Logdiagnose, Bericht und „an alle anpassen“ schicken je einen eigenen Systemprompt mit eigenem Token-Limit und eigener Temperatur.
- **Hausregeln** (KI-Einstellungen → „Hausregeln“, pro Ordner mit `e` im Manager → „KI-Hausregeln“): Freitext, der
  bei jeder KI-Aufgabe mitgeht, z. B. „Debian 12, nie reboot vorschlagen“. Ordner-Regeln gelten auch für Unterordner.
  Sicherheitsregeln heben sie nie auf.
- **Schnelles Modell** (KI-Einstellungen): auf Wunsch ein zweites, kleines Modell am selben Endpunkt für Ghost-Text,
  Folgebefehle und Begleiter; Ziel-Modus, Agent, Plan und Diagnose bleiben beim Hauptmodell. Fällt das schnelle Modell
  aus, antwortet das Hauptmodell, und das schnelle wird 5 Minuten übersprungen.
- **Schnellere Antworten:** Der feste Teil der Anfrage (Systemprofil, letzte Befehle, Bildschirm) steht vorn, das gerade
  Getippte am Ende, so nutzen vLLM, SGLang und Ollama ihren Prefix-Cache. Der Ghost-Text schickt nur die letzten 25
  Bildschirmzeilen, Folgebefehle 40. Solange der angezeigte Vorschlag noch passt, gibt es keine neue Anfrage, und dieselbe
  Eingabe innerhalb von 3 Minuten kommt aus dem Speicher. Bei kurzen Antworten werden Denkmodelle gebeten, nicht lange
  nachzudenken (Qwen, gpt-oss, OpenAI o/gpt-5, Ollama-Denkmodelle).
- **Warm halten:** Solange Tabs offen sind, hält eine Anfrage mit einem Token alle 4 Minuten das eigene Modell geladen
  (Ollama entlädt nach 5 Minuten). Cloud-Dienste werden nicht angepingt.
- **KI-Status** (Klick auf den Punkt) zeigt die Antwortzeit je Aufgabe (Mittel und zuletzt), so sieht man, was bremst.

### Fehler erkennen und Sitzungsprotokoll

- **Wann ist ein Befehl fertig?** Nach Enter, sobald wieder ein leerer Prompt dasteht und die Ausgabe ruht. Seine
  Ausgabe ist der Text zwischen Befehlszeile und neuem Prompt.
- **Exit-Code:** Meldet die Shell ihn per Shell-Integration (OSC 133;D oder 633;D, wie bei VS Code, WezTerm, iTerm2),
  zählt der echte Code (130 = Strg+C gilt nicht als Fehler). Für bash reicht z. B. in der `~/.bashrc`:
  `PROMPT_COMMAND='printf "\033]133;D;%s\007" $?'"${PROMPT_COMMAND:+;$PROMPT_COMMAND}"`.
  Ohne Shell-Integration schätzt sshTermy anhand typischer Fehlermeldungen in den letzten Zeilen (`command not found`,
  `No such file`, `Permission denied`, `E: …`, `% Invalid input` bei Switches u. a.). Das ist nur ein Angebot, nie eine Sperre.
- **Protokolle** liegen in `logs/` im Konfigurationsordner (im Sync nur mit eingeschaltetem Archiv, siehe unten), eine Datei je Sitzung,
  AES-256-GCM mit einem aus dem Store-Schlüssel abgeleiteten Schlüssel (HMAC-SHA256); die AAD bindet Format und Datei-ID.
  Ohne entsperrten Store ist kein Protokoll lesbar.
- **Nicht protokolliert:** Passwort-Eingaben (sie sind kein Befehl am Prompt und werden nicht angezeigt) und Befehle mit
  einem **Leerzeichen am Anfang** (wie `HISTCONTROL=ignorespace`). Vor dem Speichern entfernt dieselbe Geheimnis-Filterung wie
  für die KI Tokens, `password=…`, `--password …` und private Schlüssel. Mitschreiben lässt sich über die Palette oder `a` in den
  Protokollen ganz abschalten. Der Markdown-Export (in `Downloads`) ist Klartext und passiert nur auf Wunsch.

### Absturzbericht

- Stürzt sshTermy ab, legt es im Konfigurationsordner unter `crash/` einen Bericht als Text ab: Version, System und
  die Stelle im Programm (Stack). **Vor dem Speichern** werden alle Passwörter, Keys, Passphrasen und API-Keys aus
  dem Store wörtlich entfernt, dazu Hosts, Aliase, IP-Adressen, der Home-Ordner und typische Muster
  (`password=…`, `Bearer …`, Private Keys).
- Ein Absturz in der Oberfläche wird abgefangen: Das Terminal wird wiederhergestellt und sshTermy fragt, ob der
  Bericht als GitHub-Issue gemeldet werden soll. Bei einem harten Absturz (Prozess endet sofort) entsteht der
  Bericht nach dem nächsten Entsperren und erscheint in einem Dialog (`g` = als Issue melden, `o` = Bericht öffnen).
- Nichts wird automatisch verschickt. Das Issue öffnet sich im Browser mit dem Bericht; du siehst alles vor dem
  Absenden. Die letzten 10 Berichte bleiben liegen. Ausprobieren: `sshtermy -crash-test ui` (oder `cmd`, `fatal`).

### Auto-Update

- Prüft `releases/latest` von `db9979/sshtermy`, nur HTTPS zu github.com, Weiterleitungen geprüft.
- **SHA-256** aus `checksums.txt` und **Ed25519-Signatur** `checksums.txt.sig` mit eingebautem Public Key.
- Ein Build ohne diesen Schlüssel (selbst gebaut ohne `UPDATE_PUBKEY`) sagt das im Über-Dialog (F12) und fragt vor
  dem Installieren eines Updates nach; `--update` gibt eine Warnung aus.
- Austausch: laufende Datei → `.old`, neue an ihren Platz (geht auch unter Windows), beim nächsten Start aufgeräumt.

---

## Versionierung und Releases

- Version = `MAJOR.MINOR` aus [`VERSION`](VERSION) + Anzahl der Commits, z. B. `1.0.42`. **Jeder Push auf `main`
  zählt automatisch hoch** und erzeugt ein Release (`.github/workflows/release.yml`) mit allen Plattformen,
  `checksums.txt` und Signatur. Die App zeigt die Version in der Kopfzeile, im Über-Dialog (F12) und per `--version`.
- Neue Hauptversion: `VERSION` auf z. B. `1.1` setzen.
- Signierschlüssel einmalig anlegen:
  ```
  go run ./cmd/sshtermy-sign keygen
  ```
  Public Key als Repository-**Variable** `UPDATE_PUBKEY`, Private Key als **Secret** `UPDATE_SIGNING_KEY`
  (Settings → Secrets and variables → Actions). Ohne Schlüssel prüft das Update nur die SHA-256-Summe.

## Entwickeln

```
make test         # go vet + Tests
make bench        # Leistung: Terminal-Durchsatz, Speicher pro Tab, Manager mit 500 Verbindungen
make loadtest     # Lasttest: 1000 Hosts (Store, Manager, Palette, Broadcast, Sync, MCP)
make build        # bin/sshtermy
make dist         # alle Plattformen nach dist/ + checksums.txt
make fuzz         # Fuzz-Tests für Parser von Daten fremder Geräte (FUZZTIME=5m für länger)
make vuln         # bekannte Sicherheitslücken in Go und den Bibliotheken (govulncheck)
make golden       # Bildschirm-Tests nach einer gewollten Änderung neu schreiben
make handout      # Kurzreferenz zum Ausdrucken site/handout/*.pdf (Node + Playwright; Inhalt in tools/handout/content.mjs)
                  # TestHandoutKeysCurrent schlägt fehl, wenn sich Tasten oder Hilfe ändern, bis das neu gelaufen ist
```

**Bildschirm-Tests:** `internal/ui/golden_test.go` zeichnet jede Ansicht (Manager, Formulare, Dialoge, Terminal mit
Panels, geteilte Ansicht …) in fester Größe auf Deutsch und Englisch als Text ohne Farben und vergleicht sie mit
`internal/ui/testdata/golden/*.txt`. Außerdem muss jedes Bild das Fenster genau füllen. Nach einer gewollten
Änderung `make golden` ausführen und den Diff der `.txt`-Dateien ansehen, bevor du committest. Bei einem
Merge-Konflikt in diesen Dateien nach dem Merge einfach `make golden` neu laufen lassen. Schlägt die CI fehl, steht
im Artefakt `golden-actual` des Laufs, wie die Bildschirme jetzt aussehen. Neue Ansicht: eine Zeile in `goldenCases`.

**CI** testet unter Linux, Windows und macOS, dazu Datenrennen-Prüfung, `govulncheck` und ein kurzer Fuzz-Lauf.
Dependabot schlägt wöchentlich neue Bibliotheksversionen vor. Updates signieren (einmalig einrichten): [docs/signatur.md](docs/signatur.md).

**Texte in der Oberfläche** stehen deutsch im Code und laufen durch `i18n.T("…")` (Formate: `i18n.F`, feste
Fehler: `i18n.NewError`). Jeder neue Text braucht einen Eintrag in `internal/i18n/en.go`; `go test ./internal/i18n`
meldet fehlende Einträge und falsche Platzhalter.

Lokal testen ohne echten Server und ohne GPU:

```
cd tools/dev/devsshd && go run .          # Test-SSH-Server auf 127.0.0.1:2222 (jedes Passwort)
python3 tools/dev/fakellm.py              # Test-LLM auf 127.0.0.1:11434 (/v1/chat/completions)
SSHTERMY_HOME=/tmp/sth go run ./cmd/sshtermy
```

Der Terminal-Emulator liegt als angepasste Kopie in `third_party/vt` (schnellerer Bildlauf, kompakter Verlauf),
siehe `third_party/vt/README.sshtermy.md`.

## Nächste Schritte (Vorschläge)

1. Windows-Ressourcen (Icon, Versionsinfo) und Code-Signing der `.exe`, macOS Notarisierung.
2. macOS-Schlüsselbund als zweiter Entsperr-Weg (wie DPAPI unter Windows).

## Vermittler: Server hinter NAT/Firewall

Kurz (ausführlich in [docs/vermittler.md](docs/vermittler.md)):

```
# 1. auf einem erreichbaren Rechner (VPS, Pi, NAS)
sshtermy vermittler start --url https://vermittler.example.org:8022
sshtermy vermittler code --anker        # Zeile für einen Server
sshtermy vermittler code                # Zeile für sshTermy

# 2. auf dem Server, der erreichbar werden soll
sshtermy anker koppeln 'https://vermittler.example.org:8022#termy=…&fp=…'
sshtermy anker dienst | sudo tee /etc/systemd/system/sshtermy-anker.service
sudo systemctl enable --now sshtermy-anker
```

3. In sshTermy: Strg+Leertaste → „Vermittler koppeln / verwalten“, Zeile einfügen. „Systeme über Vermittler“ zeigt
   alle Server mit Online-Punkt, ⏎ legt die Verbindung an oder verbindet.

Der Vermittler reicht nur verschlüsselte Bytes durch. Den Host-Key meldet der Anker; passt er nicht, bricht sshTermy
ab. Ohne Vermittler ändert sich nichts.

## Sync zwischen Rechnern

Im Manager **y** drücken. Der Assistent fragt nur das Nötigste und prüft jede Angabe sofort live
(Name, Port, Zertifikat, Anmeldung, Ordner, Schreibrecht, vorhandene Sync-Datei). Bei Fehlern steht direkt
darunter, was zu tun ist, z. B. wo man in DSM oder QTS WebDAV einschaltet.

![Sync-Assistent](docs/bilder/13-sync.png)

| Speicherort | Was du angibst | Hinweise |
|---|---|---|
| **Ordner (Cloud)** | einen Ordner in OneDrive, Dropbox, iCloud Drive, Nextcloud-Client, Syncthing oder auf einem Netzlaufwerk | Gefundene Cloud-Ordner werden vorgeschlagen. Konfliktkopien des Cloud-Dienstes werden automatisch eingearbeitet. |
| **NAS / WebDAV: Synology** | Name/IP, Benutzer, Passwort | Paket „WebDAV Server“ installieren, HTTPS aktivieren. sshTermy findet Port 5006/5005 selbst. Ordner z. B. `/home/sshtermy`. |
| **NAS / WebDAV: QNAP** | Name/IP, Benutzer, Passwort | Systemsteuerung › Netzwerk- & Dateidienste › WebDAV aktivieren. Ports 8081/443/8080/80 werden durchprobiert. |
| **NAS / WebDAV: Nextcloud** | Adresse, Benutzer, App-Passwort | Pfad `/remote.php/dav/files/<Benutzer>/` wird automatisch gebaut. |
| **Andere** | volle WebDAV-URL | jeder WebDAV-Server (Apache, nginx, rclone serve webdav …) |

Selbstsignierte NAS-Zertifikate: sshTermy zeigt den SHA-256-Fingerprint und merkt sich nach deiner Bestätigung genau
dieses Zertifikat (Pinning). Ändert es sich später, bricht der Sync mit einer Warnung ab.

**Sicherheit:** Synchronisiert wird nur die Datei `sshtermy-sync.vault`. Sie wird vor dem Hochladen mit einem eigenen
Sync-Schlüssel verschlüsselt (AES-256-GCM), der mit deinem Sync-Passwort (Argon2id) gesichert ist. NAS und Cloud sehen
keinen Klartext, auch keine Hostnamen. Der lokale Store bleibt wie bisher per Master-Passwort/DPAPI an den jeweiligen
Rechner gebunden; das Sync-Passwort gibst du pro Rechner nur einmal bei der Einrichtung ein.

**Zusammenführung pro Eintrag:** Jede Verbindung und jeder Tunnel trägt einen Änderungszeitstempel. Beim Abgleich gewinnt
pro Eintrag die neuere Änderung, gelöschte Einträge werden als Grabstein gemerkt (180 Tage), und „zuletzt benutzt“
zählt nicht als Änderung. So gehen Änderungen auf zwei Rechnern nicht verloren, solange sie verschiedene Einträge
betreffen. Abgeglichen wird beim Start, wenige Sekunden nach jeder Änderung und alle 5 Minuten; manuell über die Palette
(„Jetzt synchronisieren“). Alle Einstellungen gehen mit: Aussehen, Farben, Symbole, Sprache, Tasten, KI, Sperre,
Meldungen, Grenzwerte der Überwachung, Gefahren-Check, Einfüge-Vorschau, Protokolle, Startanimation usw. Eigene Befehlssätze gehen ebenfalls
mit; auf einem anderen Rechner wirkt ein neuer oder geänderter Satz erst, wenn du ihn dort übernimmst (Einstellungen › Netzwerkgeräte ›
Befehlssätze, `f`, mit Diff), und ein anderswo gelöschter Satz wird nach `befehlssaetze/.geloescht` gelegt.
Nur rechnerspezifische Dinge bleiben lokal: DPAPI, die Sync-Einstellungen selbst, Schlüsseldatei-Pfade, gekoppelte
Vermittler, Team-Ordner-Pfade, der Schalter „Maus“ (hängt vom Terminalprogramm ab), Audit-/Debug-Protokoll,
MCP-Server, Editor, die Schriftgröße im eigenen Fenster und alles, was auf „nur dieser Rechner“ steht (Tasten, Shell,
Schnittstelle). Passwörter im Schlüsselbund des Betriebssystems bleiben auf jedem Rechner; nur der Schalter wandert
mit, jeder Rechner fragt einmal. Nerd-Font-Symbole gehen ebenfalls mit, schalten sich auf einem anderen Rechner aber
erst nach der Probe ein, weil die Schrift dort fehlen kann. Zeitpläne gehen pro Eintrag mit und laufen nur auf ihrem
Rechner. Mit „Befehlsverlauf“ (Standard an) zählen die Favoriten-Vorschläge (F9) die Befehle aller Rechner; jeder
Rechner schreibt nur seine eigene Zählung. Unter dem Formular zeigt der Assistent die Rechner im Sync mit ihrem
letzten Abgleich und was mitgeht bzw. hier bleibt.

**Archiv (zuschaltbar):** Mit dem Schalter „Archiv“ im Sync-Assistenten (y) wandern auch die Config-Sicherungen der
Switches (F6) und die Sitzungsprotokolle auf die anderen Rechner. Sie liegen im Unterordner `sshtermy-archiv/` neben der
Sync-Datei, eine verschlüsselte Datei je Sicherung bzw. Protokoll (Schlüssel aus dem Sync-Schlüssel abgeleitet; der
Dateiname verrät nur Art und Tag, keine Hostnamen). Jede Datei wird nur einmal geschrieben, deshalb gibt es keine
Konflikte. „Aufbewahren“ (30, 90 oder 365 Tage) legt fest, wie lange Einträge im Archiv bleiben; lokale Kopien bleiben
erhalten. Ein gelöschtes Protokoll verschwindet auch auf den anderen Rechnern. Laufende Sitzungen kommen erst nach dem
Ende ins Archiv; in der Protokollansicht steht, von welchem Rechner ein Protokoll stammt.

Zum Ausprobieren ohne NAS: `go run ./tools/dev/devdav` (http auf Port 5005) oder `go run ./tools/dev/devdav -tls -addr 127.0.0.1:5006`
(selbstsigniert), Benutzer `dominik`, Passwort `nas-pw`.

## Mit dem Team teilen

Strg+Leertaste → „Mit dem Team teilen“: Verbindungen wählen (Leertaste, `g` Ordner, `a` alle), dann Passwort und
Optionen. sshTermy schreibt eine verschlüsselte `.sshteam`-Datei (Argon2id + AES-256-GCM, ohne Passwort ist nichts
lesbar, auch nicht der Absender). Jump-Hosts kommen automatisch mit; Tunnel, Runbooks und Schnellbefehle nach Wahl.
**Nie enthalten:** Passwörter, eingebettete Keys, Passphrasen, Key-Pfade außerhalb von `~/`, Benutzernamen (nach
Wahl), gelernte abgelehnte Vorschläge, KI- und Sync-Einstellungen. Bleibt das Passwort leer, erzeugt sshTermy eines.

Der Kollege nimmt Strg+Leertaste → „Team-Paket übernehmen“: Datei, Passwort, Zielordner (Standard „Team“). Eine
Vorschau zeigt neue und aktualisierte Einträge. Wer dasselbe Paket später erneut übernimmt, bekommt die Änderungen;
eigene Passwörter, Keys und Benutzernamen bleiben erhalten. Zur Sicherheit kommen neue Einträge ohne Agent- und
X11-Weiterleitung, ohne Befehl für die lokale Shell und ohne `cmd:`-Passwortquelle an; das schaltet man bei Bedarf selbst ein. Ändert das Team die Adresse einer Verbindung, für die hier Passwort, Key oder Agent-Weiterleitung gespeichert sind, fragt sshTermy vor der Übernahme.

### Team-Ordner: automatisch aktuell

Leg die Datei beim Teilen in einen Ordner, den alle erreichen (Netzlaufwerk, OneDrive- oder Teams-Ordner) oder gib
eine https-/WebDAV-Adresse an (`https://benutzer:passwort@nas:5006/team/acme.sshteam`), und schalte „Aktuell halten“
ein. Dann schreibt sshTermy die Datei neu, sobald sich an den Verbindungen etwas ändert (Prüfung jede Minute). Wer aus
einem Ordner im Manager geteilt hat, teilt den ganzen Ordner: Neue Verbindungen darin gehen automatisch mit.

Kollegen lassen beim Übernehmen „Abonnieren“ an. sshTermy liest die Datei dann alle 5 Minuten und übernimmt
Änderungen: neue Verbindungen kommen dazu, geänderte werden aktualisiert, beim Absender gelöschte verschwinden auch
hier. Eigene Passwörter, Keys und Benutzernamen bleiben. Ein Paket nimmt nie Schutz weg: ein Tag „prod“ oder der
Gerätetyp „Netzwerkgerät“ bleibt, auch wenn der Absender ihn entfernt hat. Geänderte Schnellbefehle zeigen beim ersten
Druck nur den Befehl und laufen erst beim zweiten; neue oder geänderte Runbooks warten, bevor ein Zeitplan sie
ausführt; Tunnel aus dem Team starten nie von selbst. Strg+Leertaste → „Team-Ordner“ zeigt den Stand, gleicht
sofort ab oder beendet ein Abo (die Verbindungen bleiben dann). Team-Ordner sind rein lokal und gehen nicht in den Sync.

## Lizenz

sshTermy steht unter der [MIT-Lizenz](LICENSE): Du darfst es benutzen, kopieren, ändern und weitergeben, auch
kommerziell, solange der Urheberhinweis und der Lizenztext dabei bleiben. Idee: Dominik Bornhäußer.
