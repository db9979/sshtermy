# sshTermy

🇬🇧 [English version](README.md) · 🇩🇪 Deutsch

**SSH-Terminal mit KI-Copilot** für Windows 11 ARM64, Windows x86 (32/64 Bit), macOS und Linux.
*Idee: Dominik Bornhäußer*

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-unterst%C3%BCtzen-FFDD00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/zquu1xu570)

Gefällt dir sshTermy? Über den Button oben, im Über-Dialog (F12 → `b`) oder nach jedem Update (die Seite öffnet sich dann einmal von selbst im Browser) kannst du einen Kaffee spendieren.

![Ziel-Modus](docs/bilder/03-ziel-modus.png)

sshTermy ist eine schlanke Terminal-Anwendung (TUI, eine einzige Datei, ~9 MB, kein Installer, kein cgo) mit:

- **Verbindungs-Manager** mit verschlüsseltem Store (AES-256-GCM, Argon2id, unter Windows zusätzlich DPAPI)
- **echtem SSH-Terminal** (xterm-256color, Truecolor, vim/htop/tmux, Scrollback, Tabs, Jump-Host)
- **Auto-Reconnect**: reißt die Leitung ab (WLAN, VPN, Standby), verbindet sshTermy die Sitzung von selbst neu, startet laufende Tunnel wieder und hängt tmux/screen wieder an (abschaltbar über die Palette)
- **Feste Sitzung** (Idee: Dominik Bornhäußer): pro Verbindung beim Verbinden immer in dieselbe tmux-/screen-Sitzung, auch von einem anderen Rechner aus. Laufen schon andere Sitzungen, fragt sshTermy, welche. Gleicher Sitzungsname in zwei Tabs oder auf zwei Rechnern = gespiegelter Bildschirm; „nur zuschauen“ hängt tmux schreibgeschützt an. Die Palette öffnet einen zweiten Tab zum Zuschauen oder kopiert einen `ssh … tmux attach -r`-Befehl für Kollegen. Nicht bei Switches, Telnet und Konsole.
- **Schnellverbinden**: `user@host:port` oder `ssh -p 2222 user@host` in die Palette tippen, ohne die Verbindung anzulegen (Agent/Standard-Keys, sonst Passwort)
- **Statuszeile pro Sitzung**: Verbindungsdauer, Latenz, Zeichensatz, Warnung bei root; Produktiv-Hosts (Gruppe/Tag „Produktion“, „prod“, „live“) bekommen eine rote Zeile
- **Dateien (F5)**: SFTP über dieselbe Verbindung, links lokal, rechts Server, Dateien und Ordner hoch- und herunterladen, mit der Maus auf die andere Seite ziehen, mehrere mit der Leertaste markieren. Dateien aus Explorer/Finder ins Fenster ziehen lädt sie hoch, auch direkt im Terminal
- **Host-Übersicht (F6)**: Server zeigen beim Verbinden CPU, Last, RAM, Platte, Laufzeit und offene Updates. Switches (ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS) zeigen auf Abruf Firmware, Ports und VLANs; die show-Befehle laufen sichtbar am Prompt, nie als versteckte Exec-Sitzung
- **Config-Sicherung für Switches**: `b` sichert `show running-config` verschlüsselt auf dem Rechner, `d` zeigt den Diff zum vorherigen Stand, unveränderte Stände werden nicht doppelt gespeichert
- **Broadcast** (m im Manager): ein Befehl oder Runbook läuft parallel auf einer Gruppe (z. B. alle Aruba-Switches),
  die Ergebnisse stehen in Spalten nebeneinander. Gefahrenregeln gelten auch hier: Rot braucht zweimal ⏎.
  Server bekommen jeden Schritt per Exec (mit Exit-Code), Netzwerkgeräte die Befehle in die Shell getippt („-- MORE --“ wird weitergeblättert).
- **Runbooks mit Variablen:** `{{vlan}}` oder `{{vlan=20}}` in einem Schritt wird vor dem Start abgefragt, `{{host}}`,
  `{{alias}}`, `{{user}}`, `{{port}}` setzt sshTermy pro Host ein. Im Broadcast lädt `e` ein Runbook zum Bearbeiten,
  `Strg+R` speichert, `Strg+D` löscht. Vor dem Start prüft die KI mit `k` gegen das Systemprofil jedes Hosts, ob die
  Schritte dort laufen, und schlägt Entsprechungen vor (`a` übernimmt sie). Gefährlich oder nicht entscheiden weiter die festen Regeln.
- **Gefahren-Check:** Auf Produktiv-Hosts (Ordner, Tag oder Alias mit „prod“, Tag „live“) fragt sshTermy bei riskanten
  Befehlen vor dem Enter nach (`rm -rf`, `reboot`, auf Switches `reload`, `erase`, `write erase`, `delete`, `no vlan` …).
  Ausführen nur mit `j`, Esc lässt die Zeile stehen. Palette „Gefahren-Check auf allen Hosts“ weitet das auf alle aus.
- **Aufzeichnung exportieren:** Palette „Aufnahme starten“ nimmt den Tab als asciinema-Datei (`.cast`, mit Farben und
  vim/htop) auf, abspielbar mit `asciinema play`. Protokolle (`l`, dann `x`) gehen als Markdown, Text oder nachgebaute
  Aufnahme raus; „Terminal-Verlauf als Text exportieren“ speichert den Verlauf. Alles landet im Download-Ordner.
- **Netzwerk-Scan** (f im Manager): ein Subnetz nach SSH-Geräten durchsuchen (nur Port und SSH-Kennung, ohne Anmeldung),
  Switches und Router am Banner erkennen und die Funde mit einem Druck als Verbindungen anlegen.
- **Tunnel-Manager**: `-L`, `-R`, `-D` (SOCKS5) und One-Click-Web-Services mit lokaler URL; pro Verbindung gespeichert, ein Klick schaltet an/aus, auf Wunsch startet ein Tunnel mit der Verbindung; läuft auch über Jump-Hosts
- **Vermittler für Server hinter NAT/Firewall**: der Server baut selbst die Verbindung zu einem Vermittler auf (`sshtermy anker`), sshTermy erreicht sein sshd darüber, ohne offenen Port. SSH läuft Ende-zu-Ende, Dateien und Tunnel gehen wie gewohnt. Den Vermittler bringt sshTermy selbst mit (`sshtermy vermittler`), alternativ dient Speech on DGX Spark als Vermittler ([Anleitung](docs/vermittler.md))
- **KI-Copilot** für jeden OpenAI-kompatiblen Endpunkt (`/v1/chat/completions`: OpenAI, vLLM, SGLang, Ollama, LiteLLM, LocalAI)
  - **Befehlspalette** (Strg+Leertaste): ein Eingabefeld für alles, Text wird Ziel oder Frage, dazu Vorschläge, Tunnel, Verbindungen und Aktionen
  - **Ghost-Text** beim Tippen (Tab übernimmt)
  - **Folgebefehle** nach jedem Enter (Alt+1…4 fügt ein)
  - **Ziel-Modus / Agent** (F2): Ziel setzen, die KI schlägt Schritt für Schritt genau einen Befehl mit Begründung vor, du bestätigst
  - **Runbooks**: Ziele mit den ausgeführten Befehlen speichern (Strg+S) und wiederverwenden, der Agent nimmt sie als Vorlage
  - **Endpunkt-Test und Modellauswahl** in den KI-Einstellungen (Strg+T testet, Strg+L listet die Modelle von `/v1/models`)
  - **KI fragen** (F3) mit Terminal-Kontext, Befehle aus der Antwort als Karten mit Syntaxfarben: einfügen (⏎/1…9), ausführen (a), kopieren (c), Erklärung einklappbar (z)
  - **Systemprofil pro Verbindung**: sshTermy erkennt System, Version und Befehlssatz (Linux mit Paketmanager und
    Init-System, macOS, Windows, ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS, Junos, RouterOS, Comware …) aus
    SSH-Kennung, unsichtbarer Abfrage (nur Server) und dem mitgelesenen Terminal (Netzwerkgeräte, ohne Exec).
    Das Profil geht in jeden KI-Prompt; Befehle, die das Gerät mit „% Invalid input“ ablehnt, merkt es sich und die
    KI schlägt sie nicht mehr vor. Im Manager editierbar (System, Befehlssatz, KI-Notizen). In der Palette schreibt
    „Für dieses System umschreiben“ einen Befehl in den passenden Dialekt um.
  - **Fehler erklären**: schlägt ein Befehl fehl, bietet die Vorschlagszeile „Warum? & Fix“ an (Alt+0). Die KI bekommt
    Befehl, Exit-Code und Ausgabe und antwortet mit Ursache und einfügbaren Befehlen
- **Sitzungsprotokoll** (Manager: `l`): jede Sitzung wird lokal und verschlüsselt mitgeschrieben (Befehle, Ausgabe,
  Fehlschläge), durchsuchbar, mit KI-Zusammenfassung einer Sitzung (`z`) oder aller Treffer (`Z`, z. B. nach einem Host
  suchen: „Was habe ich auf Host X gemacht?“) und Markdown-Export (`x`)
- **Sync zwischen Rechnern**: Verbindungen, Tunnel, Runbooks und Einstellungen Ende-zu-Ende verschlüsselt über einen Cloud-Ordner
  (OneDrive, Dropbox, iCloud, Syncthing) oder direkt über dein NAS (Synology, QNAP, Nextcloud, jeder WebDAV-Server)
- **Import** bestehender Verbindungen aus `~/.ssh/config`, PuTTY (Registry oder `.reg`-Export) und MobaXterm
  (`MobaXterm.ini`, `.mxtsessions`), mit Jump-Hosts und Ordnern, ein Tastendruck (i im Manager)
- **Themes** Dunkel, Hell und Hochkontrast (a im Manager) und **frei belegbare Tastenkürzel** (b im Manager)
- **Deutsch und Englisch**: Sprache in der Palette („Sprache“/„Language“) oder im Über-Dialog (F12 → `l`) umschalten;
  die Wahl wird gespeichert und mit dem Sync übernommen. Beim ersten Start richtet sich sshTermy nach der Systemsprache
- **Versionierung und Auto-Update** aus GitHub Releases (signiert)

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
`~/Applications/sshTermy.app` an (Doppelklick oder Spotlight „sshTermy“ öffnet sie im Terminal, bzw. in iTerm,
falls installiert), dazu den Befehl `~/.local/bin/sshtermy`:

```
curl -fsSL https://raw.githubusercontent.com/db9979/sshtermy/main/install.sh | sh
```

Eine im Browser geladene Datei markiert macOS als Quarantäne und blockt sie beim Doppelklick („kann nicht
geöffnet werden“). Wer sie trotzdem nutzen will: `xattr -d com.apple.quarantine sshtermy_darwin_arm64 && chmod +x
sshtermy_darwin_arm64`, dann im Terminal mit `./sshtermy_darwin_arm64` starten.

Empfohlen unter Windows: **Windows Terminal** (Truecolor, F-Tasten, Unicode).

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
sshtermy --no-intro   # ohne Startanimation (dauerhaft: Palette „Startanimation aus“)
```

## Bedienung

| Wo | Taste | Wirkung |
|---|---|---|
| Manager | ⏎ / n / e / d / / / t / s / y / l | verbinden / neu / bearbeiten / löschen / suchen / Tunnel / KI-Einstellungen / Sync / Sitzungsprotokolle |
| | i / a / b | Verbindungen importieren / Aussehen wechseln / Tastenkürzel anpassen |
| | m / f | Broadcast an mehrere Hosts / Netzwerk-Scan |
| Ordner | o / r / v / d | Ordner anlegen („A / B“ verschachtelt) / umbenennen / verschieben / auflösen (Inhalt bleibt) |
| | ← → Space / - + | Ordner zu-/aufklappen / alle zu bzw. auf |
| | Shift+↑↓ (Alt+↑↓), Maus ziehen | Reihenfolge ändern, in einen Ordner ziehen |
| | # , /#tag | nach Tag filtern, Suche über Name, Host, Ordner, Tags |
| Broadcast | Space / g / a, Tab, Strg+S | Host / Ordner / alle wählen, Hosts·Quelle·Befehl, prüfen und bestätigen |
| | ←→ / ↑↓ / z / c / r / x | Host-Spalte / blättern / groß / Ausgabe kopieren / nochmal / abbrechen |
| Netzwerk-Scan | Strg+S, Space / a, ⏎ | Scan starten, Funde wählen, als Verbindungen anlegen |
| Terminal | F2 | Ziel-Modus (Agent) |
| | F3 | KI fragen |
| | F4 | Tunnel an/aus (⏎ oder Klick), a mit Verbindung starten, Web-Service im Browser öffnen |
| | F5 | Dateien (SFTP): Tab Seite, c kopieren (hoch/runter), Leertaste/a markieren, Ziehen kopiert, n Ordner, d löschen, . versteckte |
| | F6 | Host-Übersicht: r/⏎ abfragen, b Switch-Config sichern, ↑↓ Stand, d Diff, v ansehen |
| | F7 / F8 | vorheriger / nächster Tab |
| | F9 / ⇧F1…F8 | Schnellbefehle: Liste / direkt ausführen |
| | F10 | zurück zum Manager (Sitzungen laufen weiter) |
| | ⇧F9 | Tastenzeile erklärt die Tasten des Bereichs |
| | ⇧F10 | Tab in ein eigenes Terminalfenster abtrennen (Windows Terminal/Konsole, Terminal.app/iTerm2, Linux-Terminals, tmux); die Sitzung bleibt offen, Fenster schließen oder ⏎ holt sie zurück. Eigenes Terminal: `SSHTERMY_TERMINAL="kitty {cmd}"` |
| | Strg+F / Strg+T | im Verlauf suchen / Ansicht teilen (2 / 4) |
| | Tab oder → | Ghost-Text übernehmen |
| | Alt+1…4 | Folgebefehl einfügen (ohne Enter; rote zweimal) |
| | Alt+0 | nach einem Fehlschlag: KI erklärt Ursache und schlägt einen Fix vor |
| | Strg+Klick / Rechtsklick | URL im Browser öffnen / Menü zu Link, IP oder Host: verbinden (direkt oder über den Server), Ping vom Server oder von hier, kopieren |
| | Einfügen | mehrzeilig erst Vorschau (⏎ einfügen, o ohne letztes Enter, Esc abbrechen); Palette „Einfügeschutz aus“ |
| Ziel-Modus | ↑↓ / Strg+S / Strg+D | Runbook wählen / Ziel als Runbook speichern / Runbook löschen |
| | ⏎ / i / e / s / r / x | ausführen / nur einfügen (Dry-Run) / bearbeiten / überspringen / neu / beenden |
| KI-Eingaben | ⇧⏎ / Strg+J | neue Zeile in Ziel, Frage und Palette (lange Texte brechen automatisch um, das Feld wächst mit) |
| Protokolle | / / z / Z / x / d / a | suchen / KI-Zusammenfassung / alle Treffer zusammenfassen / Markdown-Export / löschen / Mitschreiben an/aus |
| KI-Einstellungen | Strg+T / Strg+L | Endpunkt testen / Modell aus der Liste des Endpunkts wählen |
| Überall | Strg+Leertaste | Befehlspalette: ⏎ als Ziel starten, Tab KI fragen, ↑↓ Vorschlag/Tunnel/Verbindung wählen |
| | F1 / F12 / Strg+Q | Hilfe / Über + Update / beenden |

Warum F-Tasten statt `Strg+A`? `Strg+A` ist in bash/zsh „Zeilenanfang“ und das Präfix von GNU screen. sshTermy
belegt nur Tasten, die Shells und Vollbildprogramme praktisch nie brauchen.

**Tasten anpassen** (b im Manager oder Palette → „Tastenkürzel anpassen“): Aktion wählen, ⏎, neue Taste drücken.
Entf setzt eine Aktion zurück, r alle. Doppelte Belegungen werden rot markiert. Alt+⏎ ist gesperrt, weil
Windows Terminal damit auf Vollbild schaltet. Die Belegung wandert mit dem Sync auf deine anderen Rechner.

**Sprache:** Palette (Strg+Leertaste) → „Sprache“ oder F12 → `l` schaltet zwischen Deutsch und Englisch um, sofort
und ohne Neustart. Die Wahl steht in den Einstellungen (geht mit dem Sync auf andere Rechner) und zusätzlich
unverschlüsselt in der Datei `language` im Konfigurationsordner, damit schon der Entsperr-Bildschirm sie kennt.
Ohne diese Datei entscheidet die Systemsprache (`LANG`/`LC_ALL`, unter Windows die Anzeigesprache): Deutsch bei
einem deutschen System, sonst Englisch. Die Hinweise an die KI bleiben deutsch; bei Englisch antwortet sie englisch.

**Aussehen:** a im Manager wechselt Dunkel → Hell → Hochkontrast (oder Palette → „Aussehen“). Die Schriftgröße
gehört dem Terminalfenster: Strg + Plus/Minus in Windows Terminal, ⌘ + / − unter macOS.

**Import** (i im Manager): sshTermy sucht selbst nach `~/.ssh/config` (mit `Include`, `Host *`-Vorgaben und
`ProxyJump`), den PuTTY-Sitzungen in der Registry und `%APPDATA%\MobaXterm\MobaXterm.ini`. Neue Ziele sind
vorausgewählt, schon vorhandene abgewählt; ⏎ übernimmt. Mit f lädst du eine Datei von woanders, z. B. einen
PuTTY-Export (`reg export HKCU\Software\SimonTatham\PuTTY putty.reg`), eine portable `MobaXterm.ini` oder eine
`.mxtsessions`-Datei. Passwörter werden nicht übernommen, sshTermy fragt beim ersten Verbinden. PuTTY-Keys
(`.ppk`, Version 2 und 3, auch mit Passphrase) liest sshTermy direkt, Pageant ist nicht nötig.

**Telnet und serielle Konsole:** Im Verbindungsformular wählt „Verbindung über“ zwischen SSH, Telnet und
„Seriell (Konsolenkabel)“, z. B. für einen Aruba-Switch am USB-Konsolenkabel. Schnittstelle „automatisch“ nimmt das
einzige eingesteckte USB-Konsolenkabel, so klappt eine synchronisierte Verbindung auch auf dem anderen Rechner (COM5
unter Windows, `/dev/cu.usbserial-…` unter macOS, `/dev/ttyUSB0` unter Linux). Standard 9600 Baud 8N1. Sind Benutzer
und Passwort hinterlegt, tippt sshTermy sie ein, sobald das Gerät fragt (und beantwortet „Press any key to
continue“). In der Palette verbindet `telnet 10.0.0.5`, `COM3` oder `konsole /dev/ttyUSB0 115200` sofort;
Palette → „Break senden“ schickt ein serielles Break. Host-Übersicht (F6), Config-Sicherung, Schnellbefehle und KI
laufen wie bei SSH; Dateien, Tunnel und Broadcast brauchen SSH. Linux: Benutzer muss in der Gruppe `dialout` sein.
Telnet ist unverschlüsselt, nur in vertrauenswürdigen Netzen verwenden.

## Screens

| Entsperren | Verbindungs-Manager |
|---|---|
| ![](docs/bilder/01-entsperren.png) | ![](docs/bilder/02-manager.png) |
| **Ghost-Text** | **Gefährlicher Befehl** |
| ![](docs/bilder/07-ghost.png) | ![](docs/bilder/04-gefahr.png) |
| **KI fragen** | **Tunnel & Web-Services** |
| ![](docs/bilder/05-frage.png) | ![](docs/bilder/06-tunnel.png) |

**Befehlspalette (Strg+Leertaste):** ein Feld für alles, Text wird zum Ziel oder zur Frage, darunter Vorschläge, Tunnel, Verbindungen und Aktionen.
Schalter, Aussehen, Sprache und Einrichtung liegen im Untermenü **⚙ Einstellungen ›** (rechts steht „an“/„aus“, die Palette bleibt beim Umschalten offen), Tastenkürzel, Tour, Neuigkeiten und Update unter **? Hilfe & Info ›**. Wer tippt, findet auch die Punkte aus den Untermenüs direkt; Esc oder ← führt zurück.

![Befehlspalette](docs/bilder/09-palette.png)

| Dateien (SFTP, F5) | Runbooks im Ziel-Modus |
|---|---|
| ![](docs/bilder/12-dateien.png) | ![](docs/bilder/11-runbooks.png) |
| **KI-Endpunkt testen, Modell wählen** | |
| ![](docs/bilder/10-ki-test.png) | |
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
  für andere Konten und Rechner unlesbar. Das Master-Passwort bleibt als Wiederherstellung (Umzug, macOS).
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
- **Feste Gefahrenregeln** (`internal/safety`), nie das Modell, entscheiden über Rot: `rm -rf`, `dd`, `mkfs`,
  `curl | sh`, Neustart, Firewall leeren, Benutzer ändern, `git reset --hard`, Pakete entfernen, Docker prune,
  SSH-Dienst stoppen; für Windows `Remove-Item -Recurse`, `Format-Volume`, `iex (iwr …)` u. a.
  Rote Befehle brauchen zwei Bestätigungen; bearbeitete Befehle werden neu geprüft. `sudo` wird markiert.
- **Einfügeschutz:** Dieselben Regeln prüfen jede eingefügte Zeile. Ein Treffer zeigt die Vorschau mit der
  roten Zeile und braucht zwei Bestätigungen, auch wenn die Mehrzeilen-Vorschau abgeschaltet ist.
- **Prompt Injection:** alles aus dem Terminal steht als Material in einem Block `<<< … >>>`, dessen Marker im
  Inhalt entschärft werden; der System-Prompt sagt, dass Material nie eine Anweisung ist.
- **Striktes Parsen:** nur das erwartete JSON, keine Steuerzeichen (ESC-Sequenzen aus dem Modell kommen nie auf
  den Bildschirm), Längenlimits, `<think>` von Reasoning-Modellen wird entfernt.
- **Geheimnisse** (Private Keys, AWS/GitHub/OpenAI-Keys, Bearer-Token, `password=…`, `user:pass@`) werden vor dem
  Senden entfernt; an einer Passwortabfrage gibt es keinen Ghost-Text.
- API-Key geht nie per `http://` an einen fremden Host (außer bewusst erlaubt).
- Ratenlimit im Ziel-Modus (Standard 6 Schritte/min, max. 40 Schritte pro Ziel).

### KI-Anbieter und Ersatz

- **Anbieter** in den KI-Einstellungen (`s`): Spark bzw. jeder OpenAI-kompatible Server (vLLM, SGLang, LiteLLM,
  LM Studio), Ollama, OpenAI oder Claude (Anthropic Messages-API). ←/→ wechselt und belegt Adresse und Modell vor;
  Strg+L listet die Modelle des Anbieters, Strg+T testet.
- **Ersatz-Anbieter** (Einstellungen → „Ersatz-Anbieter“ oder `r` im KI-Status): bis zu drei, die der Reihe nach
  einspringen, wenn der Endpunkt nicht antwortet. Ein ausgefallener Endpunkt wird 5 Minuten übersprungen oder bis
  die Hintergrundprüfung ihn wieder erreicht. Der Status-Punkt wird gelb und zeigt „Ersatz: Claude“, solange ein
  Ersatz antwortet.
- Cloud-Dienste brauchen immer einen API-Key; er liegt verschlüsselt wie die Passwörter.

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

### Auto-Update

- Prüft `releases/latest` von `db9979/sshtermy`, nur HTTPS zu github.com, Weiterleitungen geprüft.
- **SHA-256** aus `checksums.txt` und **Ed25519-Signatur** `checksums.txt.sig` mit eingebautem Public Key.
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
make build        # bin/sshtermy
make dist         # alle Plattformen nach dist/ + checksums.txt
```

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
(„Jetzt synchronisieren“). Nicht synchronisiert werden rechnerspezifische Dinge wie DPAPI und die Sync-Einstellungen selbst.

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
eigene Passwörter, Keys und Benutzernamen bleiben erhalten.

### Team-Ordner: automatisch aktuell

Leg die Datei beim Teilen in einen Ordner, den alle erreichen (Netzlaufwerk, OneDrive- oder Teams-Ordner) oder gib
eine https-/WebDAV-Adresse an (`https://benutzer:passwort@nas:5006/team/acme.sshteam`), und schalte „Aktuell halten“
ein. Dann schreibt sshTermy die Datei neu, sobald sich an den Verbindungen etwas ändert (Prüfung jede Minute). Wer aus
einem Ordner im Manager geteilt hat, teilt den ganzen Ordner: Neue Verbindungen darin gehen automatisch mit.

Kollegen lassen beim Übernehmen „Abonnieren“ an. sshTermy liest die Datei dann alle 5 Minuten und übernimmt
Änderungen: neue Verbindungen kommen dazu, geänderte werden aktualisiert, beim Absender gelöschte verschwinden auch
hier. Eigene Passwörter, Keys und Benutzernamen bleiben. Strg+Leertaste → „Team-Ordner“ zeigt den Stand, gleicht
sofort ab oder beendet ein Abo (die Verbindungen bleiben dann). Team-Ordner sind rein lokal und gehen nicht in den Sync.

## Lizenz

sshTermy steht unter der [MIT-Lizenz](LICENSE): Du darfst es benutzen, kopieren, ändern und weitergeben, auch
kommerziell, solange der Urheberhinweis und der Lizenztext dabei bleiben. Idee: Dominik Bornhäußer.
