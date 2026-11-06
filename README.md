# sshTermy

🇬🇧 English · 🇩🇪 [Deutsche Version](README.de.md)

**SSH terminal with an AI copilot** for Windows 11 ARM64, Windows x86 (32/64 bit), macOS and Linux.
*Idea: Dominik Bornhäußer*

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-support-FFDD00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/zquu1xu570)

Like sshTermy? You can buy me a coffee with the button above, in the About dialog (F12 → `b`) or after every update (the page then opens once in your browser by itself).

![Goal mode](docs/bilder/03-ziel-modus.png)

sshTermy is a lean terminal application (TUI, a single file, ~9 MB, no installer, no cgo) with:

- **Connection manager** with an encrypted store (AES-256-GCM, Argon2id, plus DPAPI on Windows)
- **a real SSH terminal** (xterm-256color, truecolor, vim/htop/tmux, scrollback, tabs, jump hosts)
- **Auto-reconnect**: if the line drops (Wi-Fi, VPN, standby), sshTermy reconnects the session by itself, restarts running tunnels and reattaches tmux/screen (can be turned off in the palette)
- **Quick connect**: type `user@host:port` or `ssh -p 2222 user@host` into the palette without saving the connection (agent/default keys, otherwise password)
- **Status line per session**: connection time, latency, character set, warning for root; production hosts (group/tag „Produktion“, „prod“, „live“) get a red line
- **Files (F5)**: SFTP over the same connection, local on the left, server on the right, upload and download files and folders, drag with the mouse to the other side, mark several with Space. Dragging files from Explorer/Finder into the window uploads them, even straight from the terminal
- **Host overview (F6)**: servers show CPU, load, RAM, disk, uptime and pending updates when you connect. Switches (ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS) show firmware, ports and VLANs on request; the show commands run visibly at the prompt, never as a hidden exec session
- **Config backup for switches**: `b` saves `show running-config` encrypted on your computer, `d` shows the diff to the previous state, unchanged states are not stored twice
- **Broadcast** (m in the manager): one command or runbook runs in parallel on a group (e.g. all Aruba switches),
  the results are shown side by side in columns. The danger rules apply here too: red needs ⏎ twice.
  Servers get every step via exec (with exit code), network devices get the commands typed into their shell („-- MORE --“ is paged automatically).
- **Network scan** (f in the manager): search a subnet for SSH devices (only port and SSH banner, no login),
  recognize switches and routers by their banner and create the hits as connections with one key press.
- **Tunnel manager**: `-L`, `-R`, `-D` (SOCKS5) and one-click web services with a local URL; saved per connection, one click turns them on/off, optionally start with the connection; works through jump hosts
- **AI copilot** for any OpenAI-compatible endpoint (`/v1/chat/completions`: OpenAI, vLLM, SGLang, Ollama, LiteLLM, LocalAI)
  - **Command palette** (Ctrl+Space): one input field for everything; text becomes a goal or a question, plus suggestions, tunnels, connections and actions
  - **Ghost text** while typing (Tab accepts)
  - **Follow-up commands** after every Enter (Alt+1…4 inserts)
  - **Goal mode / agent** (F2): set a goal, the AI suggests exactly one command at a time with a reason, you confirm
  - **Runbooks**: save goals with the commands that ran (Ctrl+S) and reuse them; the agent uses them as a template
  - **Endpoint test and model selection** in the AI settings (Ctrl+T tests, Ctrl+L lists the models from `/v1/models`)
  - **Ask AI** (F3) with terminal context, insert commands from the answer with 1…9
  - **System profile per connection**: sshTermy detects system, version and command set (Linux with package manager and
    init system, macOS, Windows, ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS, Junos, RouterOS, Comware …) from the
    SSH banner, an invisible query (servers only) and the terminal it reads along (network devices, no exec).
    The profile goes into every AI prompt; commands the device rejects with „% Invalid input“ are remembered and the
    AI stops suggesting them. Editable in the manager (system, command set, AI notes). In the palette,
    „Rewrite for this system“ rewrites a command into the matching dialect.
  - **Explain errors**: when a command fails, the suggestion line offers „Why? & Fix“ (Alt+0). The AI gets the
    command, exit code and output and answers with the cause and commands you can insert
- **Session log** (manager: `l`): every session is recorded locally and encrypted (commands, output,
  failures), searchable, with an AI summary of one session (`z`) or of all matches (`Z`, e.g. search for a host:
  „What did I do on host X?“) and Markdown export (`x`)
- **Sync between computers**: connections, tunnels, runbooks and settings end-to-end encrypted via a cloud folder
  (OneDrive, Dropbox, iCloud, Syncthing) or directly via your NAS (Synology, QNAP, Nextcloud, any WebDAV server)
- **Import** of existing connections from `~/.ssh/config`, PuTTY (registry or `.reg` export) and MobaXterm
  (`MobaXterm.ini`, `.mxtsessions`), with jump hosts and folders, one key press (i in the manager)
- **Themes** dark, light and high contrast (a in the manager) and **freely assignable keyboard shortcuts** (b in the manager)
- **English and German**: switch the language in the palette („Language“/„Sprache“) or in the About dialog (F12 → `l`);
  the choice is saved and synced. On first start sshTermy follows the system language
- **Versioning and auto-update** from GitHub Releases (signed)

The AI interaction concept (suggestions, goal mode, fixed danger rules) comes from the remote access assistant of
[speech-on-dgx-spark](https://github.com/db9979/speech-on-dgx-spark); the technology was rebuilt from scratch in Go.

---

## Installation

Download the matching file from the [Releases](https://github.com/db9979/sshtermy/releases/latest):

| System | File |
|---|---|
| Windows 11 ARM64 (Surface Pro X/11, Snapdragon X) | `sshtermy_windows_arm64.exe` |
| Windows x86 32 bit | `sshtermy_windows_386.exe` |
| Windows x64 | `sshtermy_windows_amd64.exe` |
| macOS Apple Silicon / Intel | `sshtermy_darwin_arm64` / `sshtermy_darwin_amd64` |
| Linux | `sshtermy_linux_amd64` / `sshtermy_linux_arm64` |

**macOS (and Linux): one line in Terminal** installs the latest version, checks its checksum and creates
`~/Applications/sshTermy.app` (double-click or Spotlight "sshTermy" opens it in Terminal, or iTerm if installed)
plus the command `~/.local/bin/sshtermy`:

```
curl -fsSL https://raw.githubusercontent.com/db9979/sshtermy/main/install.sh | sh
```

A file downloaded with the browser is marked as quarantined by macOS and blocked on double-click ("cannot be
opened"). If you still want to use it: `xattr -d com.apple.quarantine sshtermy_darwin_arm64 && chmod +x
sshtermy_darwin_arm64`, then start it from Terminal with `./sshtermy_darwin_arm64`.

Recommended on Windows: **Windows Terminal** (truecolor, function keys, Unicode).

```
sshtermy              # start
sshtermy --version    # version, commit, build date
sshtermy --update     # update to the newest version
sshtermy --no-intro   # without the start animation (permanently: palette „Start animation off“)
```

## Usage

| Where | Key | Effect |
|---|---|---|
| Manager | ⏎ / n / e / d / / / t / s / y / l | connect / new / edit / delete / search / tunnel / AI settings / sync / session logs |
| | i / a / b | import connections / switch appearance / customize shortcuts |
| | m / f | broadcast to several hosts / network scan |
| Folders | o / r / v / d | create folder („A / B“ nested) / rename / move / dissolve (content stays) |
| | ← → Space / - + | collapse/expand folder / collapse or expand all |
| | Shift+↑↓ (Alt+↑↓), drag with the mouse | change the order, drag into a folder |
| | # , /#tag | filter by tag, search by name, host, folder, tags |
| Broadcast | Space / g / a, Tab, Ctrl+S | select host / folder / all, hosts·source·command, check and confirm |
| | ←→ / ↑↓ / z / c / r / x | host column / scroll / enlarge / copy output / again / cancel |
| Network scan | Ctrl+S, Space / a, ⏎ | start scan, select hits, create as connections |
| Terminal | Tab or → | accept ghost text |
| | Alt+1…4 | insert follow-up command (without Enter; red ones twice) |
| | Alt+0 | after a failure: the AI explains the cause and suggests a fix |
| | F2 | goal mode (agent) |
| | F3 | ask AI |
| | F4 | tunnels on/off (⏎ or click), a start with connection, open a web service in the browser |
| | F5 | files (SFTP): Tab side, c copy (up/down), Space/a mark, drag copies, n folder, d delete, . hidden |
| | F6 | host overview: r/⏎ query, b back up switch config, ↑↓ state, d diff, v view |
| | F7 / F8 | previous / next tab |
| | F10 | back to the manager (sessions keep running) |
| | Ctrl+click / right-click | open URL in the browser / menu for a link, IP or host: connect (directly or via the server), ping from the server or from here, copy |
| | Paste | multi-line shows a preview first (⏎ paste, o without the last Enter, Esc cancel); palette „Paste guard off“ |
| Goal mode | ↑↓ / Ctrl+S / Ctrl+D | choose runbook / save goal as runbook / delete runbook |
| | ⏎ / i / e / s / r / x | run / only insert (dry run) / edit / skip / suggest again / end |
| AI inputs | ⇧⏎ / Ctrl+J | new line in goal, question and palette (long texts wrap automatically, the field grows) |
| Logs | / / z / Z / x / d / a | search / AI summary / summarize all matches / Markdown export / delete / recording on/off |
| AI settings | Ctrl+T / Ctrl+L | test endpoint / choose a model from the endpoint's list |
| Everywhere | Ctrl+Space | command palette: ⏎ start as goal, Tab ask AI, ↑↓ choose suggestion/tunnel/connection |
| | F1 / F12 / Ctrl+Q | help / about + update / quit |
| | F12 → l | switch language (English ↔ German) |

Why function keys instead of `Ctrl+A`? In bash/zsh `Ctrl+A` means „beginning of line“ and it is the prefix of GNU screen.
sshTermy only binds keys that shells and full-screen programs practically never need.

**Customize keys** (b in the manager or palette → „Customize shortcuts“): pick an action, ⏎, press the new key.
Del resets an action, r resets all. Duplicate bindings are marked red. Alt+⏎ is blocked because
Windows Terminal uses it for full screen. Your bindings travel to your other computers with the sync.

**Language:** palette (Ctrl+Space) → „Language“ or F12 → `l` switches between English and German, immediately and
without a restart. The choice is stored in the settings (synced to your other computers) and additionally, unencrypted,
in the file `language` in the config folder so that the unlock screen already knows it. Without that file the system
language decides (`LANG`/`LC_ALL`, on Windows the display language): German on a German system, otherwise English.
The instructions to the AI stay in German internally; with English selected, the AI answers in English.

**Appearance:** a in the manager switches dark → light → high contrast (or palette → „Appearance“). The font size
belongs to the terminal window: Ctrl + plus/minus in Windows Terminal, ⌘ + / − on macOS.

**Import** (i in the manager): sshTermy looks for `~/.ssh/config` (with `Include`, `Host *` defaults and
`ProxyJump`), the PuTTY sessions in the registry and `%APPDATA%\MobaXterm\MobaXterm.ini` by itself. New targets are
preselected, existing ones deselected; ⏎ imports. With f you load a file from somewhere else, e.g. a
PuTTY export (`reg export HKCU\Software\SimonTatham\PuTTY putty.reg`), a portable `MobaXterm.ini` or an
`.mxtsessions` file. Passwords are not imported, sshTermy asks for them on the first connect. Please load PuTTY keys
(`.ppk`) into Pageant or export them to OpenSSH with PuTTYgen.

## Screens

The screenshots show the German interface; the English one looks the same.

| Unlock | Connection manager |
|---|---|
| ![](docs/bilder/01-entsperren.png) | ![](docs/bilder/02-manager.png) |
| **Ghost text** | **Dangerous command** |
| ![](docs/bilder/07-ghost.png) | ![](docs/bilder/04-gefahr.png) |
| **Ask AI** | **Tunnels & web services** |
| ![](docs/bilder/05-frage.png) | ![](docs/bilder/06-tunnel.png) |

**Command palette (Ctrl+Space):** one field for everything; text becomes a goal or a question, below it suggestions, tunnels, connections and actions.

![Command palette](docs/bilder/09-palette.png)

| Files (SFTP, F5) | Runbooks in goal mode |
|---|---|
| ![](docs/bilder/12-dateien.png) | ![](docs/bilder/11-runbooks.png) |
| **Test the AI endpoint, choose a model** | |
| ![](docs/bilder/10-ki-test.png) | |
| **Host overview server (F6)** | **Switch: overview and config backup** |
| ![](docs/bilder/15-host-server.png) | ![](docs/bilder/14-host-switch.png) |

The images are real captures of the app (against a test SSH server and a test LLM from `tools/dev`).

---

## Architecture

```
cmd/sshtermy/          entry point: flags (--version, --update), open store, start TUI
cmd/sshtermy-sign/     create Ed25519 keys, sign releases (CI)
internal/
  domain/              pure data models: Connection, Tunnel, LLMConfig, Settings, Secret
  vault/               encryption: AES-256-GCM, Argon2id, DPAPI (Windows)
  store/               repository on top of the vault: CRUD, paths (%APPDATA%\sshtermy)
  sshx/                SSH: dial (+jump host), auth (password/key/agent), host keys (TOFU),
                       PTY shell, tunnel manager (-L/-R/-D), SOCKS5 server, open browser,
                       commands via exec or typed into the shell (network devices)
  broadcast/           command/runbook in parallel on several hosts, danger check
  netscan/             scan a subnet for SSH, read banners, detect device type
  llm/                 OpenAI-compatible client (/v1/chat/completions, streaming via SSE)
  agent/               AI logic: ghost text, follow-up commands, goal mode, questions; prompts,
                       strict parsing, removal of secrets
  sessionlog/          session logs: encrypted files per session, search, Markdown
  safety/              fixed danger rules (Linux/macOS and Windows/PowerShell)
  termemu/             terminal emulation (charmbracelet/x/vt): grid, scrollback, ghost overlay
  update/              auto-update from GitHub Releases (SHA-256 + Ed25519)
  version/             name, version, credits in exactly one place
  i18n/                interface language: the German texts are the keys, en.go holds the English version
  ui/                  interface (bubbletea v2, lipgloss v2): screens, panels, forms
tools/dev/             test SSH server, test LLM, ANSI→HTML for screenshots
```

Dependencies only point inwards: `ui → agent/sshx/store/update → llm/safety/vault → domain`.
`domain` knows nothing but the standard library.

### Flow

```
Start ─▶ Unlock (DPAPI automatically │ master password │ first setup)
      ─▶ Connection manager
           ├─ n/e  form (alias, host, port, user, auth, folder, tags, jump host picker with search)
           ├─ t    tunnel form (local/remote/dynamic, web service, autostart)
           ├─ s    AI settings (base URL, API key, model, tokens, temperature …)
           └─ ⏎    check host key ─▶ terminal tab
                     ├─ ghost text (pause while typing → suggestion → Tab)
                     ├─ Enter → output settles → follow-up commands as chips
                     ├─ F2 goal → suggest a step → ⏎ → output settles, prompt is back → next step … ✓ done
                     ├─ F3 question (streamed answer, numbered commands)
                     └─ F4 tunnels (status, counters, URL)
```

### Technical decisions

| Topic | Choice | Reason |
|---|---|---|
| Language | Go 1.26 | one static binary per platform, cross-compiling without a toolchain (`GOOS=windows GOARCH=arm64`), no cgo |
| TUI | bubbletea v2 + lipgloss v2 | current Charm stack: real cursor control, layers for overlays, Kitty keyboard, Windows console |
| Terminal emulation | charmbracelet/x/vt | actively maintained, truecolor, alt screen, scrollback, wide characters; encodes keys to match the mode |
| SSH | golang.org/x/crypto/ssh | reference implementation in Go, agent, known_hosts, forwarding |
| Windows agent | named pipe via go-winio | Windows OpenSSH agent (`\\.\pipe\openssh-ssh-agent`), 1Password, KeePassXC |
| Storage | encrypted JSON file | one file, written atomically, metadata (hosts, users) encrypted too |
| Languages | own `i18n` package | German texts in the code are the keys, a test makes sure every text has an English version |

---

## Security

### Encrypted store (`internal/vault`)

```
Master password ──Argon2id (t=3, 64 MiB, p=4)──▶ KEK ──AES-256-GCM──┐
                                                                     ├─▶ DEK (256 bit, random)
Windows login ──DPAPI CryptProtectData───────────────────────────────┘        │
                                                                               ▼
                                         AES-256-GCM(DEK) over the whole store
```

- The **DEK** (data encryption key) from `crypto/rand` encrypts the **whole** store in one piece
  (passwords, private keys, passphrases, API keys and also host names, users, tunnels).
- **AES-256-GCM** with a fresh 96-bit nonce on every save. The header (KDF parameters, wraps) is checked as
  *additional authenticated data*: lowering the KDF parameters or swapping parts makes decryption fail
  (test `TestTamperedHeaderIsRejected`).
- The **master password** is never stored; **Argon2id** (RFC 9106) derives the KEK from it, which wraps the DEK.
- **DPAPI** (Windows only, `CryptProtectData` without `LOCAL_MACHINE`, with its own entropy value) binds a second
  copy of the DEK to your Windows account: after the Windows login sshTermy unlocks without input, but it stays
  unreadable for other accounts and computers. The master password remains as recovery (moving, macOS).
- File `store.vault` in `%APPDATA%\sshtermy` (macOS `~/Library/Application Support/sshtermy`, Linux
  `~/.config/sshtermy`), permissions 0600, written atomically. `SSHTERMY_HOME` for a portable installation.
- Automatic lock after idle time (`LockAfterMin`), the DEK is zeroed when locking (best effort).
- `domain.Secret` never reveals itself in logs and error messages (`••••••`).

### SSH

- **Host keys:** trust on first use with a SHA-256 fingerprint in the dialog, own `known_hosts`. A **changed**
  host key aborts the connection, without „connect anyway“ (possible man-in-the-middle).
- Agent auth preferred: private keys stay in the agent. Embedded keys are only stored encrypted in the store.
- Tunnels bind to `127.0.0.1` by default. SOCKS5 resolves names on the server (no local DNS leaks).

### AI

- The AI **never types by itself**. Every command is a suggestion; only your confirmation sends it to the PTY.
- **Fixed danger rules** (`internal/safety`), never the model, decide what is red: `rm -rf`, `dd`, `mkfs`,
  `curl | sh`, reboot, flushing the firewall, changing users, `git reset --hard`, removing packages, Docker prune,
  stopping the SSH service; on Windows `Remove-Item -Recurse`, `Format-Volume`, `iex (iwr …)` and more.
  Red commands need two confirmations; edited commands are checked again. `sudo` is marked.
- **Paste guard:** the same rules check every pasted line. A hit shows the preview with the
  red line and needs two confirmations, even if the multi-line preview is turned off.
- **Prompt injection:** everything from the terminal is passed as material in a `<<< … >>>` block whose markers are
  defused inside the content; the system prompt says material is never an instruction.
- **Strict parsing:** only the expected JSON, no control characters (ESC sequences from the model never reach
  the screen), length limits, `<think>` from reasoning models is removed.
- **Secrets** (private keys, AWS/GitHub/OpenAI keys, bearer tokens, `password=…`, `user:pass@`) are removed before
  sending; there is no ghost text at a password prompt.
- The API key is never sent via `http://` to a foreign host (unless explicitly allowed).
- Rate limit in goal mode (default 6 steps/min, at most 40 steps per goal).

### Error detection and session log

- **When is a command done?** After Enter, as soon as an empty prompt is back and the output settles. Its
  output is the text between the command line and the new prompt.
- **Exit code:** if the shell reports it via shell integration (OSC 133;D or 633;D, as in VS Code, WezTerm, iTerm2),
  the real code counts (130 = Ctrl+C is not a failure). For bash, e.g. this in `~/.bashrc` is enough:
  `PROMPT_COMMAND='printf "\033]133;D;%s\007" $?'"${PROMPT_COMMAND:+;$PROMPT_COMMAND}"`.
  Without shell integration sshTermy estimates from typical error messages in the last lines (`command not found`,
  `No such file`, `Permission denied`, `E: …`, `% Invalid input` on switches and more). This is only an offer, never a block.
- **Logs** are stored only locally in `logs/` in the config folder (never synced), one file per session,
  AES-256-GCM with a key derived from the store key (HMAC-SHA256); the AAD binds format and file ID.
  Without an unlocked store no log is readable.
- **Not logged:** password inputs (they are not a command at the prompt and are not displayed) and commands with
  a **leading space** (like `HISTCONTROL=ignorespace`). Before saving, the same secret filter as for the AI removes
  tokens, `password=…`, `--password …` and private keys. Recording can be turned off entirely via the palette or `a` in the
  logs. The Markdown export (to `Downloads`) is plain text and only happens on request.

### Crash report

- If sshTermy crashes, it writes a plain-text report to `crash/` in the config folder: version, system and the
  stack trace. **Before saving**, all passwords, keys, passphrases and API keys from the store are removed
  verbatim, as are hosts, aliases, IP addresses, the home folder and typical patterns (`password=…`, `Bearer …`,
  private keys).
- A crash in the interface is caught: the terminal is restored and sshTermy asks whether to report it as a
  GitHub issue. A hard crash (the process ends immediately) is turned into a report after the next unlock and
  shown in a dialog (`g` = report as issue, `o` = open report).
- Nothing is sent automatically. The issue opens in the browser with the report pre-filled; you see everything
  before you submit. The last 10 reports are kept. Try it: `sshtermy -crash-test ui` (or `cmd`, `fatal`).

### Auto-update

- Checks `releases/latest` of `db9979/sshtermy`, HTTPS to github.com only, redirects are checked.
- **SHA-256** from `checksums.txt` and **Ed25519 signature** `checksums.txt.sig` with a built-in public key.
- Replacement: running file → `.old`, new one in its place (works on Windows too), cleaned up on the next start.

---

## Versioning and releases

- Version = `MAJOR.MINOR` from [`VERSION`](VERSION) + number of commits, e.g. `1.0.42`. **Every push to `main`
  counts up automatically** and creates a release (`.github/workflows/release.yml`) with all platforms,
  `checksums.txt` and signature. The app shows the version in the header, in the About dialog (F12) and via `--version`.
- New major version: set `VERSION` to e.g. `1.1`.
- Create the signing key once:
  ```
  go run ./cmd/sshtermy-sign keygen
  ```
  Public key as repository **variable** `UPDATE_PUBKEY`, private key as **secret** `UPDATE_SIGNING_KEY`
  (Settings → Secrets and variables → Actions). Without a key the update only checks the SHA-256 sum.

## Development

```
make test         # go vet + tests
make build        # bin/sshtermy
make dist         # all platforms to dist/ + checksums.txt
make golden       # rewrite the screen tests after an intended UI change
```

**Screen tests:** `internal/ui/golden_test.go` draws every view (manager, forms, dialogs, terminal with panels,
split view …) at a fixed size in German and English, as plain text without colors, and compares it with
`internal/ui/testdata/golden/*.txt`. It also checks that every screen fills the window exactly. After an
intended change run `make golden` and look at the diff of the `.txt` files before committing. On a merge
conflict in these files simply run `make golden` again after the merge. If CI fails, the job artifact
`golden-actual` contains the screens as they look now. New view: add a line to `goldenCases`.

**Interface texts** are written in German in the code and go through `i18n.T("…")` (formats: `i18n.F`, fixed
errors: `i18n.NewError`). Every new text needs an entry in `internal/i18n/en.go`; `go test ./internal/i18n`
reports missing entries and mismatched placeholders.

Test locally without a real server and without a GPU:

```
cd tools/dev/devsshd && go run .          # test SSH server on 127.0.0.1:2222 (any password)
python3 tools/dev/fakellm.py              # test LLM on 127.0.0.1:11434 (/v1/chat/completions)
SSHTERMY_HOME=/tmp/sth go run ./cmd/sshtermy
```

## Next steps (suggestions)

1. Windows resources (icon, version info) and code signing of the `.exe`, macOS notarization.
2. macOS keychain as a second unlock path (like DPAPI on Windows).

## Sync between computers

Press **y** in the manager. The wizard only asks for what is needed and checks every entry live right away
(name, port, certificate, login, folder, write permission, existing sync file). If something fails, the line
below says what to do, e.g. where to enable WebDAV in DSM or QTS.

![Sync wizard](docs/bilder/13-sync.png)

| Location | What you enter | Notes |
|---|---|---|
| **Folder (cloud)** | a folder in OneDrive, Dropbox, iCloud Drive, the Nextcloud client, Syncthing or on a network drive | Cloud folders that are found are suggested. Conflict copies of the cloud service are merged automatically. |
| **NAS / WebDAV: Synology** | name/IP, user, password | Install the „WebDAV Server“ package, enable HTTPS. sshTermy finds port 5006/5005 by itself. Folder e.g. `/home/sshtermy`. |
| **NAS / WebDAV: QNAP** | name/IP, user, password | Control Panel › Network & File Services › enable WebDAV. Ports 8081/443/8080/80 are tried. |
| **NAS / WebDAV: Nextcloud** | address, user, app password | The path `/remote.php/dav/files/<user>/` is built automatically. |
| **Other** | full WebDAV URL | any WebDAV server (Apache, nginx, rclone serve webdav …) |

Self-signed NAS certificates: sshTermy shows the SHA-256 fingerprint and, after your confirmation, remembers exactly
this certificate (pinning). If it changes later, the sync aborts with a warning.

**Security:** only the file `sshtermy-sync.vault` is synced. Before uploading it is encrypted with its own
sync key (AES-256-GCM), which is protected by your sync password (Argon2id). NAS and cloud never see
plain text, not even host names. The local store stays bound to each computer via master password/DPAPI as before;
you enter the sync password only once per computer during setup.

**Merging per entry:** every connection and every tunnel carries a change timestamp. When syncing, the newer change
wins per entry, deleted entries are remembered as tombstones (180 days), and „last used“
does not count as a change. This way changes on two computers are not lost as long as they affect different entries.
Syncing happens at start, a few seconds after every change and every 5 minutes; manually via the palette
(„Sync now“). Computer-specific things like DPAPI and the sync settings themselves are not synced.

To try it without a NAS: `go run ./tools/dev/devdav` (http on port 5005) or `go run ./tools/dev/devdav -tls -addr 127.0.0.1:5006`
(self-signed), user `dominik`, password `nas-pw`.
