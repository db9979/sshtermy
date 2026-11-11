# <img src="assets/icon/sshtermy-128.png" alt="sshTermy icon" width="48" height="48" align="top"> sshTermy

🇬🇧 English · 🇩🇪 [Deutsche Version](README.de.md)

**SSH terminal with an AI copilot** for Windows 11 ARM64, Windows x86 (32/64 bit), macOS and Linux.
*Idea: Dominik Bornhäußer*

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-support-FFDD00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/zquu1xu570)

Like sshTermy? You can buy me a coffee with the button above, in the About dialog (F12 → `b`) or after every update (the page then opens once in your browser by itself).

![Goal mode](docs/bilder/03-ziel-modus.png)

sshTermy is a lean terminal application (TUI, a single file, ~9 MB, no installer, no cgo) with:

- **Connection manager** with an encrypted store (AES-256-GCM, Argon2id, plus DPAPI on Windows)
- **a real SSH terminal** (xterm-256color, truecolor, vim/htop/tmux, scrollback, tabs, jump hosts)
- **Auto-reconnect**: if the line drops (Wi-Fi, VPN, standby), sshTermy reconnects the session by itself, restarts running tunnels and reattaches tmux/screen (can be turned off in the palette). After **sleep or a network change** (other Wi-Fi, VPN on/off) every line is checked within seconds instead of waiting for the keepalive timeout; while there is no network at all, sshTermy waits without using up its attempts
- **Persistent session** (idea: Dominik Bornhäußer): per connection, always land in the same tmux/screen session on connect, even from another computer. Other sessions already running? sshTermy asks which one. Same session name in two tabs or on two computers = mirrored screen; "view only" attaches tmux read-only. The palette opens a second watching tab or copies an `ssh … tmux attach -r` command for colleagues. Not on switches, Telnet or console.
- **Own window** (no terminal needed): double-click `sshtermy.exe` in Explorer, open `sshTermy.app` on the Mac or run `sshtermy -window`, and sshTermy opens its own window with Cascadia Mono, Nerd Font symbols and emoji built in. Images from the server show pixel-perfect, files dragged onto the window are offered for upload, ⇧F10 moves a tab into a second window of its own. All keys arrive (Alt+⏎, ⇧F1 … ⇧F12, Ctrl+Space), Ctrl+⇧V or ⇧Insert paste (Windows also Ctrl+V, Mac ⌘V), Ctrl + / Ctrl − / Ctrl 0 or Ctrl + mouse wheel change the font size (remembered), F11 toggles full screen. The window starts in **full screen** by default; palette › Settings › “Window at startup” switches to maximized or “as last time” (size and position are remembered). Started from a terminal, sshTermy runs in that terminal as before; `-tui` forces it
- **Local shell** (Alt+L): your own PowerShell (Windows, via ConPTY), zsh or bash (macOS/Linux) as a tab, with AI help, goal/agent/plan mode, split view, tabs and session log. Danger rules and agent levels apply on your own computer too; the agent drops its scripts in `~/.sshtermy-agent`. Or save it as a connection (“Connect via: Local”) with its own shell (`pwsh`, `cmd`, `wsl`, `bash -l` …) and start folder
- **Remote PowerShell (WinRM)**: log into Windows servers like `Enter-PSSession`, without SSH on the server (“Connection type: PowerShell”, ports 5985/5986). NTLM also for domain accounts, Kerberos for servers without NTLM, messages encrypted over HTTP, or HTTPS; own input line with history, Tab completion via the server and multi-line input. The AI suggests PowerShell commands, danger rules for Windows apply. Palette: `Enter-PSSession srv01` or `winrm admin@srv01`
- **Forgiving everyday use**: Ctrl+Q and closing the window ask first when connections are still open; deleting a connection, folder or runbook happens right away and a toast offers **undo** for 10 s (Ctrl+Z in the manager or click); **Ctrl+T in the connection form tests** reachability, host key and login before you save; failed connections say in **plain language** what's wrong and what helps (“Login rejected”, “No answer from sw1:22 … VPN or jump host?”, `e` edits the connection), the original error stays below in gray. If a server shows a **new host key** (reinstall, factory reset), `k` compares old and new fingerprint and accepts the new one after a second confirmation
- **Simple mode** (palette › Settings › General): hides pro features (broadcast, network, team, agent, recording, MCP, audit …) from the palette overview, settings and key bar; searching in the palette still finds everything
- **Quick connect**: type `user@host:port` or `ssh -p 2222 user@host` into the palette without saving the connection (agent/default keys, otherwise password)
- **Change several connections at once**: in the manager Space marks a connection, `*` all visible ones (with a search or `#tag`, all hits). After that `e` (bulk change: add or remove tags, user, protection, color), `v` (move) and `d` (delete, with undo) apply to all marked ones; Esc clears the selection.
- **Note per connection**: a free field just for you (contact person, maintenance window, location). It shows in the manager on the right, the search finds it, and it is never sent to the AI. “AI notes” remain the field for hints to the AI.
- **Start command per connection** (form › Advanced): typed visibly once after login, e.g. `cd /srv/app` or `sudo -i`. The form rejects dangerous commands; a start command is never taken over from a team package.
- **Wake-on-LAN**: a powered-off machine in your own network can be woken from the manager (click “Send Wake-on-LAN”, palette “wake”, or `w` in a disconnected tab). sshTermy remembers the MAC address on the first connect, or you enter it under Advanced.
- **Placeholders in quick commands**: `systemctl restart {{service}}` asks for the value when run, `{{service:nginx}}` suggests one; the last value is prefilled next time. If only the inserted value makes the command dangerous, sshTermy asks first.
- **Search across all tabs** (Alt+S): one search term, the hits from the scrollback of all open tabs in one list; ⏎ jumps to the tab at exactly that line.
- **Drag tabs with the mouse**: a tab can be dragged to another position in the tab bar.
- **Finished in the background**: a command that ran 10 s or longer in a hidden tab marks the tab (✓ / ×) and shows a toast; if the sshTermy window is in the background you also get a desktop notification (Windows, macOS, Linux). The terminal bell marks the tab too. Both can be switched off in the palette
- **Agent forwarding** per connection (like `ssh -A`): `git pull`, `ssh` and `scp` on the server use your local keys from ssh-agent, Pageant or the Windows OpenSSH agent, the keys never leave your computer
- **X11 forwarding** per connection (like `ssh -X`): graphical programs from the server open here; needs a local X server (XQuartz, VcXsrv, WSLg), the X cookie never leaves your computer
- **FIDO2 security keys** (`id_ed25519_sk`, `id_ecdsa_sk`, e.g. YubiKey) are used directly; sshTermy asks you to touch the key (needs OpenSSH ssh-agent/ssh-add)
- **Password manager instead of stored passwords**: in the connection form “Password from” and (under Advanced)
  “2FA code from”: `op://Vault/Item/password` (1Password CLI), `bw:Item` (Bitwarden CLI), `keepassxc:file.kdbx:Item`
  (KeePassXC, asks for the database password), `pass:name` or `cmd:command`. When a server asks for a one-time code
  (“Verification code”), sshTermy takes the TOTP code from the same source, also after a key login.
  Only the reference is stored, never the password
- **Kerberos/GSSAPI login** (login `kerberos`): uses the company domain ticket from the Windows sign-in or `kinit`, no password needed; without a ticket it falls back to the password
- **Images in the terminal**: Sixel (`img2sixel`, `chafa -f sixel`, gnuplot), the kitty graphics protocol (`kitty +kitten icat`, `viu`, `timg`) and iTerm2 inline images (`imgcat`) show up as colored half blocks right in the text, in every terminal with 24-bit color, in the scrollback and in split view. Two pixels per cell, so plots and photos are recognizable, not pixel-perfect. In kitty, Ghostty and sshTermy's own window the images show pixel-perfect instead (kitty graphics with Unicode placeholders, so they still scroll with the text; off with `SSHTERMY_NO_PIXELS=1`)
- **Clipboard from the server (OSC 52)**: what you copy in tmux or vim on the server lands in your local clipboard. The server can never read it
- **Split view** (Ctrl+T): up to six sessions at once in seven layouts, resize with the mouse or Alt+⇧+arrow, zoom one pane (Alt+Z), swap panes by dragging their title, optionally dim the other panes. The layout survives updates; **workspaces** (w in the manager) save layout and connections to reopen in one go, “Last used” is remembered on quit, `g` on a folder opens all its connections as a grid
- **Type into all split panes** (Alt+I in split view): what you type goes live to every visible host, e.g. the same change on four switches. Enter only goes through if the line is harmless on every host
- **SSH certificates**: keys signed by a company CA (`id_ed25519-cert.pub` next to the key, or any path in the connection) are used automatically; an expired certificate is reported with its date
- **Files right in the terminal (trzsz)**: run `tsz file` on the server to download into your Downloads folder (sshTermy asks first and never overwrites an existing file), or `trz` to upload; works through `sudo su`, several hops and devices without SFTP. ZMODEM (`sz`/`rz`) is detected and cancelled with a hint instead of hanging
- **Screen lock**: palette “Lock now” or automatically when idle (5/15/30/60 min). Open sessions keep running behind the lock, only the master password unlocks the screen. **Change master password** (palette → Settings › Security) also switches the Windows sign-in (DPAPI) on or off later
- **Status line per session**: connection time, latency, character set, warning for root; production hosts (group/tag “Produktion”, “prod”, “live”) get a red line
- **Files (F5)**: SFTP over the same connection, local on the left, server on the right, upload and download files and folders, drag with the mouse to the other side, mark several with Space. Dragging files from Explorer/Finder into the window uploads them, even straight from the terminal. While copying, a bar shows percent, MB/s, time left and “file 2 of 5”, with a small speed curve
- **Files with the mouse**: double-click a file to view it right in sshTermy (line numbers, colors, wheel scrolls, drag selects and copies, `/` searches, `f` follows like `tail -f`, `w` wraps; big files load beginning and end). Images, PDFs and Office files open with the default app of your computer (server files are downloaded first). A single click shows the start of a text file below the list (`v` turns that off). Right-click opens a menu: view, edit, open with default app, copy, download/upload to …, rename (`F2`), change permissions, copy path (`y`), `cd` here in the terminal, delete. Click a part of the path (/ › etc › nginx) to jump there, buttons for ↑ ~ reload, new folder and hidden files, click a column header to sort (`s`), Ctrl+click marks one by one, Shift+click a range, the mouse back button goes up a level
- **Server file in your own editor**: `e` in F5 (server side) or the palette entry “Open server file in your own editor” with a path opens the file in VS Code, Notepad++, TextEdit … on your computer; every save uploads it back over the same connection (owner and permissions stay). Choose the editor: palette → Settings › “Editor for server files” (e.g. `code --new-window`)
- **Color rules for output**: text without its own color gets colored, errors red, warnings yellow, ok/up green, IP addresses blue, MAC addresses purple, switch syslog (`%LINK-3-UPDOWN`) by severity; never in vim/htop. Own rules “color pattern” (colors in English or German; palette → “Color rules for output”); with `!` in front sshTermy notifies you when the pattern shows up in a hidden tab (the tab gets a `!`, a toast, a desktop notification if you like)
- **Host overview (F6)**: servers show CPU, load, RAM, every disk (all real file systems; tmpfs, overlay, snap and the like are left out), uptime and pending updates when you connect. The disk warning limit (`g`) applies to each disk separately.
  On Linux servers, while F6 is open, live Braille graphs show CPU, RAM, network and disk (every 2 s, `t` switches
  5 min / 1 h, `z` shows them large over the terminal with the top processes; a small read-only script in its own
  exec session, closing the panel ends it). Switches, routers and firewalls (ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS/NX-OS/ASA/Small Business, Arista, Juniper, HPE Comware, Huawei, Dell, Extreme, Ruckus, Allied Telesis, Netgear, Zyxel, TP-Link, Lancom, MikroTik, FortiGate, Palo Alto, Sophos, Check Point, SonicWall, WatchGuard, pfSense/OPNsense, VyOS/EdgeOS, UniFi and more; see *Command sets*) show firmware, ports and VLANs on request; the show commands run visibly at the prompt, never as a hidden exec session
- **Containers** (`c` in F6 or palette “docker”): Docker and Podman containers and Kubernetes pods of the server in one list. ⏎ opens a shell in the container, Ctrl+L follows the logs, Ctrl+K lets the AI analyze the last 400 log lines. Restart and start/stop are only inserted at the prompt, ⏎ runs them. Uses `sudo -n` when docker needs root and sudo works without a password
- **Log view (Alt+E, or `l` in F6)**: reads the logs of the system you are on, read-only, and shows anomalies instead of lines. Linux/BSD: journalctl (or syslog files), failed units and package updates; Windows (PowerShell/WinRM): event logs System/Application, failed sign-ins 4625/4740 and stopped automatic services; switches and firewalls: the log commands typed visibly at the prompt (ProCurve/CX, Cisco, Junos, RouterOS, FortiOS, UniFi) plus a port bar. Same lines are grouped into patterns locally, without AI (numbers, IPs, MACs replaced), known patterns get a plain title, hint and read-only check commands (`1`…`4` insert them). `n` *What's new?* compares with your last look (new, much more frequent, gone; the baseline stays encrypted on this computer), `x`/`X` hides noise for the host or the whole folder (synced). `t` timeline: errors and warnings per time slot with marks for reboots, package updates, config changes and your own commands; ←→ picks a slot, `k` asks the AI what happened there. `k` lets the AI explain a pattern, `b` writes an AI status report from the condensed patterns only (cause, connections, what to check). `z` period 1 h … 7 days, `w` log watch alerts, `c` copies as Markdown. On switches and firewalls `s` sets up syslog to sshTermy: it shows the vendor's commands with your own IP and types them visibly after you confirm (nothing is saved on the device; `write memory` stays with you). Syslog received by sshTermy is merged into the switch's log view, so it reaches further back than the device's own buffer.
- **Log watch** (Settings › Logs & audit › *Log watch*: errors or errors + warnings): while a tab is open, sshTermy follows its logs in the background (Linux `journalctl -f`, Windows over WinRM every minute) and reports only what is new compared with the last log view or critical, each pattern at most once an hour: toast, ▲ on the tab, desktop notification when sshTermy is in the background. Hidden noise stays quiet. *Syslog receiver* (UDP 5514, 1514 or 514) takes syslog from switches and firewalls and assigns it to the matching connection by IP or host name. Senders that match no saved connection are dropped
(a toast names the IP once), because UDP senders can be spoofed; Settings › Logs & audit › *Accept syslog from unknown senders too*
lets them in. The AI status report treats syslog lines as hints, not as proof. The list: palette *Log watch: alerts* or `w` in the log view; ⏎ jumps to the pattern in the host's log view, `x` hides it as noise. Failed sign-ins (SSH, Windows 4625) count as an error from 20 in 10 minutes; link-down messages keep the device's severity (Cisco `LINK-3` = error).
- **Folder scan** (palette *Folder scan*): the log view for every host in a folder at once, read-only and without prompts (6 at a time; servers over SSH, Windows over WinRM, switches via their shell). One line per host with critical/errors/warnings, new patterns and the top pattern, plus *Shared*: patterns that show up on several hosts. `c` copies the result as Markdown; the baseline for *What's new?* is saved per host. As a schedule (kind *Folder scan (logs)*) it runs e.g. every night; the result is in the schedule and under *Folder scan: last result*.
- **Config backup for switches**: `b` saves the running configuration (`show running-config`; Junos `show configuration | display set`, RouterOS `/export`, FortiOS `show full-configuration`, UniFi `cat /tmp/system.cfg`) encrypted on your computer, `d` shows the diff to the previous state, unchanged states are not stored twice.
  Nightly runs go through a schedule (`z`, type “Config backup”) for a whole folder. **Switch config rules**
  (palette → Settings): `+^ntp server` must appear, `-telnet-server` must not; every backup is checked,
  violations show in the F6 panel, as a toast and in the schedule result
- **Find device**: type a MAC (any notation) or IP into the palette, sshTermy asks all switches for their MAC,
  ARP and LLDP tables and shows the switch, port and VLAN the device is plugged into. Uplinks are recognized
  by their LLDP neighbors and listed last
- **Several networks**: if your switches sit in different folders (one per customer or site), find device and
  topology first ask which folder to query, or all of them; the last choice is preselected
- **Network topology** (palette “topo”): the LLDP/CDP neighbors of all switches become a picture with boxes and lines (who is connected to
  which port of whom, APs and phones listed dimmed inside the switch box, ring links below; a plain tree if it does not fit), plus a Mermaid diagram in Downloads for docs, wiki or GitHub
- **Broadcast** (m in the manager): one command or runbook runs in parallel on a group (e.g. all Aruba switches),
  every host gets its own tile (framed green when done, red on failure, pulsing while running; `z` enlarges one). The danger rules apply here too: red needs ⏎ twice.
  Steps that change something first run on one host as a trial run, then on the rest; after 3 failed hosts no further host starts (`p` turns the trial run off).
  Servers get every step via exec (with exit code), network devices get the commands typed into their shell (“-- MORE --” is paged automatically).
- **Schedules** (z in the manager): a broadcast or runbook runs at a set time on a folder or single connections,
  daily, Mon–Fri, on weekends or on one weekday, e.g. “3:00 Mon–Fri `show running-config` on all switches”.
  Runs while sshTermy is open (also when locked), missed read-only runs are caught up within 12 hours; steps that change something only within 10 minutes (a 3:00 maintenance never runs at noon).
  New hosts that appear in the target folder later (e.g. from a team folder) only join after `Ctrl+A` in the list, and a runbook that came changed from a team folder waits until you have started it once in the broadcast.
  Unattended there are no questions: unknown host keys and missing passwords make that host fail,
  dangerous steps and runbooks with variables cannot be scheduled at all. The result per host is in the list
  (⏎), plus a toast or desktop notification. Schedules travel with the sync but run only on one computer
  (“Runs on” in the form, default: the computer that created it), so two computers never start the same run
- **Runbooks with variables:** `{{vlan}}` or `{{vlan=20}}` in a step is asked before the run, `{{host}}`, `{{alias}}`,
  `{{user}}`, `{{port}}` are filled in per host. In the broadcast `e` loads a runbook for editing, `Ctrl+R` saves,
  `Ctrl+D` deletes. Before the run `k` lets the AI check the steps against each host's system profile and suggest
  equivalents (`a` applies them). Whether something is dangerous is still decided by the fixed rules.
- **Danger check:** every connection counts as a production system, so sshTermy asks before risky
  commands run (`rm -rf`, `reboot`, on switches `reload`, `erase`, `write erase`, `delete`, `no vlan` …). Only `y` or holding ⏎
  runs them (a bar fills up, a short ⏎ is not enough), Esc leaves the line in place. The agent asks before every change
  there too. Only the switch “Protection” in the connection form turns this off, one connection at a time (for test and
  lab systems; the status bar then shows “Protection off”). “prod”/“live” in folder, tag or alias still colors the status bar red.
- **Export recordings:** palette “Start recording” records the tab as an asciinema file (`.cast`, with colors and
  vim/htop), playable with `asciinema play`. Session logs (`l`, then `x`) export as Markdown, text or a rebuilt
  recording; “Export terminal history as text” saves the scrollback. Everything goes to your Downloads folder.
- **Network scan** (f in the manager): search a subnet for SSH devices (only port and SSH banner, no login),
  recognize switches and routers by their banner and create the hits as connections with one key press.
- **Tunnel manager**: `-L`, `-R`, `-D` (SOCKS5) and one-click web services with a local URL; saved per connection, one click turns them on/off, optionally start with the connection; works through jump hosts
- **Relay for servers behind NAT/firewall**: the server itself connects out to a relay (`sshtermy anker`), sshTermy reaches its sshd through it without an open port. SSH runs end to end, files and tunnels work as usual. sshTermy brings its own relay (`sshtermy relay`, also `sshtermy vermittler`); Speech on DGX Spark can serve as a relay too ([guide, German](docs/vermittler.md))
- **Share a session live** (palette “share”): a short dialog, no setup. In the same network sshTermy shares directly without a relay; otherwise through your own relay or a public one (`sshtermy relay start --offen`). The viewer gets a short code like `7-taube-kompass-ofen` (or a link/QR code; `m` in the invitation opens a ready-made email in your mail app) and types it into their palette. The code is spent after one guess (PAKE: the relay never learns the key), the viewer knocks and you let them in after comparing three check words. While sharing, the tab shows a red dot and “● LIVE · 1 viewer · 28 min”; sharing ends by itself (15 min … 4 h), viewers only see from now on, password prompts pause the stream, and the session log records who watched; a click on the LIVE chip manages viewers. Viewers only watch; typing only on request (“raise hand”), Enter on production connections needs your OK. No relay yet? The relay assistant installs one on a server of yours over SSH (and removes it again); pairing codes come straight from sshTermy
- **AI copilot** for any OpenAI-compatible endpoint (`/v1/chat/completions`: OpenAI, vLLM, SGLang, Ollama, LiteLLM, LocalAI)
  - **Command palette** (Ctrl+Space): one input field for everything; text becomes a goal or a question; without text only frequent actions, connections and one folder per area (AI, session, search & logs, tabs & view, network, team …), typing finds everything directly; right-click in the manager opens a menu for the connection or folder
  - **Ghost text** while typing (Tab accepts)
  - **Follow-up commands** after every Enter (Alt+1…4 inserts)
  - **Goal mode / agent** (F2): set a goal, the AI suggests exactly one command at a time with a reason, you confirm. Text in the panel (goal, steps, output, error messages) can be selected with the mouse and is copied right away.
    Multi-line content (config sections, scripts) comes as a file that sshTermy creates via SFTP in `~/.sshtermy-agent/<run>/`
    after ⏎, never as a heredoc in the command; config files are backed up and extended, not replaced
    (`docker cp` into a container counts as dangerous). If the model's answer is unusable, sshTermy asks once more and names the problem.
  - **Agent mode** (F2, Tab picks the mode): the agent works on its own until the goal is reached. It runs commands,
    sees their output and exit code, writes scripts via SFTP to `~/.sshtermy-agent/<run>/` and runs them (dry run first).
    Levels: *Careful* (default; only read-only commands run on their own), *Normal* (everything except danger, sudo and inline code such as `python -c`, `bash -c`, `eval`; signed in as root or on Windows every change asks, because there is no sudo to stop at),
    *Full* (sudo too). Dangerous commands, including every line of a script, always wait for you; on production hosts
    and network devices every change asks. Limits: 25 steps, 15 minutes, 3 failures in a row. Esc or typing in the
    terminal pauses it, `?` gives it an instruction. At the end: the full summary (scroll with PgUp/PgDn or the mouse wheel,
    `c` copies it, `l` opens it in full), changed files, `s` saves the run as a runbook
    including its scripts, `r` deletes the scripts on the host. Every step is marked "Agent" in the session log.
  - **Plan mode** (F2, Tab until "Plan first"): the AI first looks around with read-only commands only, then writes a
    complete plan (steps, scripts, risks, checks). Remove steps (Space), change them (`e`), have it re-planned (`?`),
    pick how it runs (Tab: step by step or agent level) and approve with ⏎. The agent then sticks to the approved plan:
    a changing command or file that is not in the plan pauses it. Ctrl+S saves the plan as a runbook.
  - **AI companion** (Alt+K picks the level, Alt+J opens its sidebar): the AI reads along and speaks up on its own.
    Fixed rules decide *when*, the AI only decides *what* to say (and may stay quiet). *Quiet*: a line under the terminal
    only when a command fails, a typed line is risky before you press ⏎ (`chmod 777`, `rm -rf`, `ufw disable` …; Alt+1
    replaces the line with the safer variant) or the same command repeats. *Chatty*: a comment on every command
    (bundled, at most every 20 s, replaces the follow-up commands). *Coach*: also spots what you are working towards
    (“set up HTTPS for shop.example.de”) and offers to hand the rest to plan mode (`p`) or the agent “Careful” (`g`); when
    you close the tab it shows a recap, saves facts to the system profile and the session as a runbook.
    Limits: requests per hour and an estimated daily token budget. Nothing is sent during password prompts, in full-screen
    programs (vim, less, top), for private commands (leading space) or while goal/agent/plan runs; tag `begleiter-aus`
    (`companion-off`) turns it off per connection. *Companion only with your own AI* restricts it to your own endpoints,
    *Filters off with your own AI* skips secret removal, but only while every request goes to your own AI.
  - **Your own AI servers** (settings › AI): register hosts such as `llm.example.com` that run outside your network
    (reached via API); they count like local endpoints for “own AI only” and the companion's filter switch.
    Goal, agent and plan mode show their progress as a timeline: done steps with ✓ and duration plus the last output
    line, the running step with a moving bar, planned steps as ○.
  - **Runbooks**: save goals with the commands that ran (Ctrl+S) and reuse them; the agent uses them as a template
  - **Endpoint test and model selection** in the AI settings (Ctrl+T tests, Ctrl+L lists the models from `/v1/models`)
  - **Ask AI** (F3) with terminal context, commands from the answer shown as highlighted cards: insert (⏎/1…9), run (a), copy (c), collapsible explanation (z); if a card command fails, the card shows the error and a corrected command (from the companion or asked for separately), u puts it into the card, the same mistake in other cards is fixed too and the correction is remembered for the device; the AI knows today's date and writes dates/times in commands in the program's format (ISO, not "10.11."); Esc or Ctrl+C stops a running answer, n asks a new question, f a follow-up that keeps the conversation
  - **System profile per connection**: sshTermy detects system, version and command set (Linux with package manager and
    init system, macOS, Windows, ArubaOS-Switch/ProCurve, ArubaOS-CX, Cisco IOS, Junos, RouterOS, Comware, UniFi switches …) from the
    SSH banner, an invisible query (servers only) and the terminal it reads along (network devices, no exec).
    The profile goes into every AI prompt; commands the device rejects with “% Invalid input” are remembered and the
    AI stops suggesting them. Editable in the manager (system, command set, AI notes). In the palette,
    “Rewrite for this system” rewrites a command into the matching dialect.
  - **Command sets per vendor**: what sshTermy types on a device (paging off, version, CPU/RAM, ports, VLANs, config, logs,
    syslog setup), how it reads the output and what it tells the AI lives in one YAML file per vendor, with variants for
    older or newer firmware. 48 built in: switches (Cisco, Arista, Aruba, HPE Comware, Huawei, Dell, Extreme, Ruckus,
    Allied Telesis, Netgear, Zyxel, TP-Link, Lancom …), firewalls (FortiGate, Palo Alto, Sophos, Check Point, SonicWall,
    WatchGuard, Cisco ASA, pfSense, OPNsense, VyOS, EdgeOS), WLAN controllers (Aruba, Cisco WLC) and appliances
    (Proxmox, ESXi, Synology, QNAP, TrueNAS, iLO, iDRAC). Palette › Settings › Network devices › *Command sets*: `n` new set based on a
    built-in one, `e`/`o` edit in the form or the file, `t` test on the open device, `k` AI draft from the terminal,
    `l` take over the commands you corrected on a device, `x`/`i` export/import; sets used by shared connections travel
    with the team share, your own sets also with the sync (taken over on the other computer with `f`). Safety: own sets only type read commands, can only add danger rules (never weaken the built-in
    ones), only detect a device when no built-in set matches, syslog steps are shown and danger-checked every time,
    and team sets stay inactive until you approve them (and again after every change, with a diff).
  - **Explain errors**: when a command fails, the suggestion line offers “Why? & Fix” (Alt+0). The AI gets the
    command, exit code and output and answers with the cause and commands you can insert
  - **Log diagnosis**: palette “AI: analyze log/output” (or select text first): the AI reads the selection or the last
    400 lines (journalctl, `show logging`, dmesg …) and answers with the situation, what stands out with timestamps,
    the likely cause and read-only commands to check, as cards
- **Session log** (manager: `l`): every session is recorded locally and encrypted (commands, output,
  failures), searchable, with an AI summary of one session (`z`) or of all matches (`Z`, e.g. search for a host:
  “What did I do on host X?”) and Markdown export (`x`). A day or host shows an activity calendar (one square per day,
  brighter = more commands, click jumps to that day) and the time of day you worked. **Change report** (`b`): the AI writes, for a day, a host or
  one session, what was changed on which host (with commands, failures, open items), ready for a ticket or docs; it
  lands in the clipboard right away and as a Markdown file in Downloads
- **Sync between computers**: connections, tunnels, runbooks and settings end-to-end encrypted via a cloud folder
  (OneDrive, Dropbox, iCloud, Syncthing) or directly via your NAS (Synology, QNAP, Nextcloud, any WebDAV server)
- **Import** of existing connections from `~/.ssh/config`, PuTTY (registry or `.reg` export), MobaXterm
  (`MobaXterm.ini`, `.mxtsessions`), Ansible inventories (INI or YAML) and CSV (also the NetBox device export), with jump hosts and folders, one key press (i in the manager). `e` there exports all connections as CSV (without passwords)
- **Full backup / move to a new computer** (palette › Connections & data or `sshtermy backup`): the whole data folder
  (connections, keys, logs, switch backups, known_hosts, settings) as one file encrypted with the master password
  (Argon2id + AES-256-GCM). Restoring replaces everything after a confirmation and a restart; the previous state stays
  next to it as `….vorher-<date>`
- **26 themes** with live preview (a in the manager): dark ones like Night, Polar night, Deep sea, Graphite, Vampire, Cherry blossom, Sunset, Neon, Ink, Forest, Chalk, Lagoon, Solar, Campfire, Amber and Matrix, light ones like Light, Paper, Frost, Sand, Dawn and Mint, plus high contrast and color-blind (dark and light), **custom colors** and consistent menu icons (text tags or Nerd Font) and **freely assignable keyboard shortcuts** (b in the manager)
- **English and German**: switch the language in the palette (“Language”/“Sprache”) or in the About dialog (F12 → `l`);
  the choice is saved and synced. On first start sshTermy follows the system language
- **Window style** (palette “Window style”): dialogs, palette and pickers cast a shadow and the background dims a
  little, and the header, logo and status bar carry a soft color gradient (the status bar in the connection's accent
  color); optionally with round corners, or flat as before (no shadows, no gradients)
- **Accessible mode** (Settings › Appearance): no animations, shapes instead of color only (× for “unreachable”);
  together with the **color-blind themes** (blue/yellow/orange instead of green/red, Okabe-Ito colors)
- **Lively interface** (palette “Motion”: full, subtle, off): a spinner in the tab while connecting, tabs flash green
  on success and red on errors, a pulsing dot shows new output in hidden tabs, hovering a tab shows a card with target,
  duration and latency history. In the manager the online dots “breathe”, an outage blinks and is reported (checked are
  the connections visible in the manager, a folder is checked as soon as you expand it; settings “Online dot”: off,
  visible, all), and the details show the reachability history and CPU/RAM/disk bars for connected servers. While connecting, a card shows
  the steps (connection, host key, login, shell) with timings and a progress bar with a light sweep, then the shell
  fades in from the top. The AI answer shimmers “thinking …” and types out smoothly, panels fade in. In the command
  palette the matched letters light up, rows fade in one after another and the selection glides. The status line shows
  the latency as a small curve (red and “▲ sluggish” on a bad line). In split view the divider next to the active pane
  lights up, and a frame glides along when you switch panes or zoom (Alt+Z). Redraws only happen while
  something moves; accessible mode turns it all off
- **MCP server** (`sshtermy mcp`): Claude and other AI clients can list your connections and run read-only commands, anything else only after you confirm in sshTermy
- **Audit log** (Settings › Logs & audit): who did what and when, on which host: connections, commands (also from the
  agent mode and broadcast), file transfers, config backups, changed connections, master password, full backups and
  AI requests (host/model only). Each line is chained and signed with a key from the store, so changes, deleted or
  reordered lines are detected. “Verify and save audit log” writes the check result and a readable copy to Downloads.
  Commands with a leading space appear only as “(privat)”
- **Debug log for bug reports** (Settings or `sshtermy --debug`): connection attempts, drops, updates, AI requests and
  sync as events, never passwords or screen content. Help › “Save cleaned debug log” writes a copy without hosts, IPs and
  user names to Downloads, ready to attach to a GitHub issue. Very slow frames (over 50 ms) are logged there too
- **Adaptive frame rate**: when a server floods output (`cat` on a big file), sshTermy draws only as often as a frame
  allows instead of for every chunk; keys and mouse always draw at once. Unchanged terminal content is not rebuilt for
  every frame. Palette “Performance display” rates the draw time in words (smooth/good/a bit slow/choppy), shows a bar per
  frame, frames per second (1 at rest is normal: it only draws on change), the possible frame rate and the output rate;
  in the terminal it sits left of the F6 sidebar
- **Versioning and auto-update** from GitHub Releases (signed). Before the swap the new program file is started once and must report the announced version; if it then fails to start cleanly twice, sshTermy restores the previous version by itself **Update channel** (Settings › Security › “Offer updates”): “immediately” shows every new version, “stable” only versions that have been published for a day or a week. `sshtermy -update` always takes the newest.
- **Two sshTermy at once** (second window, second start): each one notices when the other has saved and reloads connections and settings instead of overwriting them. If saving fails (disk full, read-only folder), a message appears

The AI interaction concept (suggestions, goal mode, fixed danger rules) comes from the remote access assistant of
[speech-on-dgx-spark](https://github.com/db9979/speech-on-dgx-spark); the technology was rebuilt from scratch in Go.

---

## Installation

📖 Guide with animations: **https://db9979.github.io/sshtermy/**

**Package managers** (they also take care of updates):

```
winget install db9979.sshTermy                                   # Windows (once approved in the winget catalog)
brew tap db9979/sshtermy https://github.com/db9979/sshtermy      # macOS and Linux
brew install sshtermy
echo "deb [trusted=yes] https://db9979.github.io/sshtermy/apt ./" | sudo tee /etc/apt/sources.list.d/sshtermy.list
sudo apt update && sudo apt install sshtermy                     # Debian, Ubuntu, Raspberry Pi OS (amd64, arm64, armhf, riscv64)
```

Installed via Homebrew or apt, sshTermy does not replace itself but shows the update command instead.

Or download the matching file from the [Releases](https://github.com/db9979/sshtermy/releases/latest):

| System | File |
|---|---|
| Windows 11 ARM64 (Surface Pro X/11, Snapdragon X) | `sshtermy_windows_arm64.exe` |
| Windows x86 32 bit | `sshtermy_windows_386.exe` |
| Windows x64 | `sshtermy_windows_amd64.exe` |
| macOS Apple Silicon / Intel | `sshtermy_darwin_arm64` / `sshtermy_darwin_amd64` |
| Linux | `sshtermy_linux_amd64` / `sshtermy_linux_arm64` / `sshtermy_linux_arm` (32 bit ARMv7) / `sshtermy_linux_riscv64` |

**macOS (and Linux): one line in Terminal** installs the latest version, checks its checksum and creates
`~/Applications/sshTermy.app` (double-click or Spotlight "sshTermy" opens sshTermy in its own window, no Terminal
needed; an `.app` from an older installer opens Terminal, run the line again to switch) plus the command `~/.local/bin/sshtermy`:

```
curl -fsSL https://raw.githubusercontent.com/db9979/sshtermy/main/install.sh | sh
```

A file downloaded with the browser is marked as quarantined by macOS and blocked on double-click ("cannot be
opened"). If you still want to use it: `xattr -d com.apple.quarantine sshtermy_darwin_arm64 && chmod +x
sshtermy_darwin_arm64`, then start it from Terminal with `./sshtermy_darwin_arm64`.

Windows: double-click the `.exe` and sshTermy opens in its own window. Started from PowerShell or cmd it stays in
that terminal; there **Windows Terminal** is recommended (truecolor, function keys, Unicode).

**ClockworkPi uConsole** (and similar handhelds): apt or the one-line installer pick the right build for every core
(CM4/CM5 and A-06 with a 64 bit system: arm64, 32 bit Raspberry Pi OS: armhf, R-01: riscv64). The 5" screen is big
enough: with a readable font you get about 80×22 to 106×30 characters, and sshTermy is laid out for 80×24 and up;
below that, the header and help drop details instead of cutting text. The keyboard has no function row:
**Fn+1 … Fn+0 = F1 … F10, Fn+- = F11, Fn+= = F12**. For ⇧F1 … ⇧F10 hold Shift *first*, then Fn and the digit.
Everything is also in the command palette (**Ctrl+Space**), the trackball clicks keys and hints, and keys can be
changed in the manager with `b`.

```
sshtermy              # start
sshtermy --version    # version, commit, build date
sshtermy --update     # update to the newest version
sshtermy -window      # own window instead of the terminal (automatic when started without a terminal)
sshtermy -tui         # always in the terminal, never an own window
sshtermy --no-intro   # without the start animation (permanently: Settings › Motion “off”)
sshtermy --debug      # write a debug log for this run (debug.log in the config folder)
sshtermy backup [file]   # full backup of all data (asks for the master password)
sshtermy restore <file>  # restore a full backup (replaces all data, keeps the old folder)
sshtermy mcp             # MCP server for AI clients (stdin/stdout), see “sshTermy as MCP server”
```

## Usage

📖 **Manual & FAQ** (setup, AI privacy, network devices, moving to a new computer, MCP): [db9979.github.io/sshtermy/manual.html](https://db9979.github.io/sshtermy/manual.html) · palette › Help › “Manual & FAQ”

🖨 **Printable quick reference** (A4, 2 pages): [English PDF](https://db9979.github.io/sshtermy/handout/sshtermy-quickref-en.pdf) · [German PDF](https://db9979.github.io/sshtermy/handout/sshtermy-kurzreferenz-de.pdf)

| Where | Key | Effect |
|---|---|---|
| Manager | ⏎ / n / e / d / / / t / s / y / l | connect / new / edit / delete / search / tunnel / AI settings / sync / session logs |
| | Space / * | mark a connection / mark all visible ones; then e (bulk change), v and d apply to all marked ones, Esc clears |
| | right-click | menu for a connection or folder: connect, edit, move, tunnel, SSH key, quick commands, Wake-on-LAN, mark, delete; for folders expand/collapse, new connection here, open as grid |
| | Ctrl+Z | undo the last delete (connection, folder, runbook; 10 s, or click the toast) |
| Connection form | Ctrl+T / Ctrl+S | test the connection (reachable, host key, login) / save |
| Closed tab | ⏎ / e / k / x / w | reconnect / edit the connection / compare and accept a changed host key / close the tab / wake up (Wake-on-LAN) |
| | i / a / b | import connections / choose a color scheme (26 themes, live preview) / customize shortcuts |
| | c / q | create or copy an SSH key / edit quick commands |
| | m / f / z | broadcast to several hosts / network scan / schedules (Ctrl+N new, Ctrl+E edit, Ctrl+R now, Ctrl+O on/off, ⏎ last result) |
| | w / g | workspaces (saved split layouts) / open a folder as a grid |
| Folders | o / r / v / d | create folder (“A / B” nested) / edit (name, AI house rules) / move / dissolve (content stays) |
| | ← → Space / - + | collapse/expand folder / collapse or expand all |
| | Shift+↑↓ (Alt+↑↓), drag with the mouse | change the order, drag into a folder |
| | # , /#tag | filter by tag, search by name, host, folder, tags |
| Broadcast | Space / g / a, Tab, Ctrl+S, p | select host / folder / all, hosts·source·command, check and confirm, trial run on/off |
| | ←→ / ↑↓ / z / c / r / x | host column / scroll / enlarge / copy output / again / cancel |
| Network scan | Ctrl+S, Space / a, ⏎ | start scan, select hits, create as connections |
| Terminal | F2 | goal mode (agent) |
| | F3 | ask AI (Esc stops the answer, n new question, f follow-up, u use the fix after an error) |
| | F4 | tunnels on/off (⏎ or click), a start with connection, open a web service in the browser |
| | F5 | files (SFTP): Tab side, ⏎/double-click open or view, c copy (up/down), e own editor, o default app, Space/a mark, Ctrl/Shift+click mark, drag copies, right-click menu, F2 rename, y copy path, s sort, v preview, n folder, d delete, . hidden; in the viewer / search, f follow, w wrap |
| | F6 | host overview: r/⏎ query, t graph range 5 min/1 h, z large, l log view, c containers, b back up switch config, ↑↓ state, d diff, v view |
| | F7 / F8 | previous / next tab |
| | Alt+T | all tabs (list: jump, close, rename, duplicate, move) |
| | Alt+S | search in all open tabs, ⏎ jumps to the matching line |
| | Alt+N / Alt+W | new tab / close tab (also `+` and `×` in the tab bar, right-click a tab for its menu, hover it for target, duration and latency) |
| | Alt+L | local shell in a new tab (also in the manager; palette: `local`, `pwsh`, `cmd`, `wsl`, `zsh` …) |
| | F9 / ⇧F1…F8 | quick commands: list / run directly (`{{name}}` in the command asks for a value) |
| | F10 | back to the manager (sessions keep running) |
| | ⇧F9 | key bar explains the keys of the current area |
| | ⇧F10 | move the tab to its own terminal window (Windows Terminal/console, Terminal.app/iTerm2, Linux terminals, tmux; in sshTermy's own window a second own window); the session stays open, closing the window or ⏎ brings it back. Own terminal: `SSHTERMY_TERMINAL="kitty {cmd}"` |
| | Ctrl+F | search the scrollback (Ctrl+E only error lines, Tab error list) |
| | Ctrl+T | choose a layout: one, two side by side/stacked, large + two, three, four, six (keys 1…7); inside, ⇧+arrow resizes, `=` equal sizes, `z` zoom, `d` dim the others, `w` workspaces |
| | Alt+I | split view: type into all visible panes at once |
| | Ctrl+arrow / Alt+⇧+arrow / Alt+Z | split view: switch pane / move divider / zoom the active pane. Mouse: drag a divider (double-click = equal sizes), drag a title onto another pane = swap, double-click a title = zoom |
| | Tab or → | accept ghost text |
| | Alt+1…4 | insert follow-up command (without Enter; red ones twice) |
| | Alt+0 | after a failure: the AI explains the cause and suggests a fix |
| | Alt+E | log view: anomalies in the logs (⏎ lines, 1…4 check, k AI explains, b AI status report, x/X hide, n what's new, t timeline, z period, w log watch, s syslog setup on switches) |
| | Alt+K / Alt+J | AI companion: level off · quiet · chatty · coach / companion sidebar |
| Companion sidebar | ↑↓ / ⏎ / a / c | choose suggestion / insert / run (dangerous ones twice) / copy |
| | m / l / s / ? / r | mute topic / remember fact / level / last request sent / recap |
| | p / g / n / x | spotted intent: plan / agent takes over / do it myself / don't ask again |
| | Ctrl+click / right-click | open URL in the browser / menu for a link, IP or host: connect (directly or via the server), ping from the server or from here, copy |
| | Paste | multi-line shows a preview first (⏎ paste, o without the last Enter, Esc cancel); palette “Paste guard off” |
| Goal mode | ↑↓ / Ctrl+S / Ctrl+D | choose runbook / save goal as runbook / delete runbook |
| Goal mode, file step | ⏎ / v / s | create the file on the host / view it / skip; at the end `r` deletes the work folder |
| | ⏎ / i / e / s / r / x | run / only insert (dry run) / edit / skip / suggest again / end |
| | Tab | step by step → agent *Careful* → *Normal* → *Full* → *Plan first* |
| Agent mode | Esc / ? / v / x | pause / give an instruction / view the script / end; at the end `s` save as runbook, `r` delete the scripts on the host, `c` copy the result, `l` read it in full |
| Goal/agent/plan panel, Ask AI | PgUp / PgDn, mouse wheel | scroll long content (summary, plan, answer) |
| | drag / double-click / triple-click, right-click | select text in the panel (goal, steps, output, error messages) and copy it right away (clipboard + OSC 52): word / line; right-click copies the selection again |
| Plan mode | ↑↓ / Space / e / v / ? | choose step / remove or restore / edit / view script / re-plan |
| | Tab / ⏎ / x | how it runs (step by step or agent level) / approve and run / discard; Ctrl+S saves the plan as a runbook |
| AI inputs | ⇧⏎ / Ctrl+J | new line in goal, question and palette (long texts wrap automatically, the field grows) |
| Logs | / / Tab / f / s | search (output too) / by day or by host / filter today · 7 days · failures / sort hosts |
| | z / Z / b / x / d / a | AI summary / summarize all matches / change report / export (md, txt, cast) / delete / recording on/off |
| AI settings | Ctrl+T / Ctrl+L | test endpoint / choose a model from the endpoint's list |
| Everywhere | Ctrl+Space | command palette: ⏎ start as goal, Tab ask AI, ↑↓ choose suggestion/tunnel/connection; `log` = AI analyzes output, `color` = color rules |
| | F1 / F12 / Ctrl+Q | help / about + update / quit (asks while connections are open; Ctrl+Q again = quit) |
| | F12 → l / F12 → w | switch language (English ↔ German) / what's new |
| Own window | Ctrl+⇧V · ⇧Insert (Windows also Ctrl+V, Mac ⌘V) / Ctrl + · Ctrl − · Ctrl 0 (Mac: ⌘) | paste / font size bigger · smaller · default (also Ctrl + mouse wheel, remembered); F11 full screen; on the Mac ⌘ acts as Alt, ⌥ keeps typing @ [ ] { } \| ~ |

Why function keys instead of `Ctrl+A`? In bash/zsh `Ctrl+A` means “beginning of line” and it is the prefix of GNU screen.
sshTermy only binds keys that shells and full-screen programs practically never need.

**Customize keys** (b in the manager or palette → “Customize shortcuts”): pick an action, ⏎, press the new key.
Del resets an action, r resets all. Duplicate bindings are marked red. Alt+⏎ is blocked because
Windows Terminal uses it for full screen. Your bindings travel to your other computers with the sync. `l` switches
to “this computer only”: keys you set then apply only here (marked ◆) and win over the synced ones, e.g. on a
uConsole without an F row or a Mac terminal without Alt; r in that mode removes them again.

**Language:** palette (Ctrl+Space) → “Language” or F12 → `l` switches between English and German, immediately and
without a restart. The choice is stored in the settings (synced to your other computers) and additionally, unencrypted,
in the file `language` in the config folder so that the unlock screen already knows it. Without that file the system
language decides (`LANG`/`LC_ALL`, on Windows the display language): German on a German system, otherwise English.
The instructions to the AI stay in German internally; with English selected, the AI answers in English.

**Appearance:** a in the manager switches dark → light → high contrast (dark/light) → color-blind (dark/light) → Matrix (green) → Amber (retro) → Neon (or
palette → Settings › "Appearance"). **Customize colors** in the same submenu: pick a template, then change each color
with ←/→ (hue), +/− (brightness) or as a hex value; the whole UI previews it live, a contrast check warns about hard to
read colors, Ctrl+S saves (synced to your other computers).

**Icons:** Settings › "Icons: …" offers four styles (synced to your other computers). Three work with any terminal font because
they only use characters Cascadia Mono, Menlo and co. have themselves: **text tags** (default) like `[+]` `[k]` `[/]`,
with prompt characters in the manager (`$` Linux, `%` macOS, `>` Windows, `#` switch); **Unicode shapes** like
`+ ¤ ◎ ◈ ¶`; **minimal** with `▸` for actions, `»` for submenus and `◆` for the AI. The fourth style, **Nerd Font**,
shows real pictograms but needs a Nerd Font in the terminal (e.g. "Cascadia Mono NF"). sshTermy first shows a test
line: boxes or "?" there mean the font is missing (Mac: `brew install --cask font-caskaydia-mono-nerd-font`, then pick
it in the terminal profile).

The font size belongs to the terminal window: Ctrl + plus/minus in Windows Terminal, ⌘ + / − on macOS.

**Import** (i in the manager): sshTermy looks for `~/.ssh/config` (with `Include`, `Host *` defaults and
`ProxyJump`), the PuTTY sessions in the registry and `%APPDATA%\MobaXterm\MobaXterm.ini` by itself. New targets are
preselected, existing ones deselected; ⏎ imports. With f you load a file from somewhere else, e.g. a
PuTTY export (`reg export HKCU\Software\SimonTatham\PuTTY putty.reg`), a portable `MobaXterm.ini` or an
`.mxtsessions` file. Passwords are not imported, sshTermy asks for them on the first connect. sshTermy reads PuTTY keys
(`.ppk`, version 2 and 3, with or without passphrase) directly, no Pageant needed.

**Telnet and serial console:** in the connection form, “Connection type” switches between SSH, Telnet and
“Serial (console cable)”, e.g. for an Aruba switch on a USB console cable. Port “automatic” uses the only USB
console cable that is plugged in, so a synced connection also works on the other computer (COM5 on Windows,
`/dev/cu.usbserial-…` on macOS, `/dev/ttyUSB0` on Linux). With “This computer only” a fixed port applies only
here and the other computers keep theirs. Default 9600 baud 8N1. If user and password are stored,
sshTermy types them as soon as the device asks (and answers “Press any key to continue”). In the palette,
`telnet 10.0.0.5`, `COM3` or `console /dev/ttyUSB0 115200` connects right away; palette → “Send break” sends a serial
break. Host overview (F6), config backup, quick commands and the AI work as with SSH; files, tunnels and broadcast
need SSH. Linux: the user needs to be in the `dialout` group. Telnet is unencrypted, use it only in trusted networks.

**Local shell:** Alt+L (or the first entry under Alt+N, or palette → “Open local shell”) opens the shell of your own
computer in a tab: PowerShell 7 (`pwsh`), otherwise Windows PowerShell or cmd on Windows (ConPTY, Windows 10 1809 or
newer), your login shell on macOS and `$SHELL` on Linux. Typing `pwsh`, `cmd`, `wsl -d Ubuntu` or `zsh` in the palette
starts that one. As a saved connection (“Connection type: Local”) you can set the shell and a start folder;
left empty, each computer uses its own default, so a synced entry works on Windows and Mac alike. With
“This computer only” a shell such as `pwsh` applies only on this computer; the others keep theirs. The AI knows whether it
is talking to PowerShell, cmd or a Unix shell; danger rules and the agent levels work as on a server. Files (F5),
tunnels and broadcast need SSH.

**Remote PowerShell (WinRM):** for Windows servers without SSH. In the connection form choose “Connection type:
PowerShell”, enter host, user (`DOMAIN\name` or `name@domain`) and password (or keychain/password manager). Port 5985
(HTTP) is the default; NTLM then encrypts every message. With “HTTPS” sshTermy uses 5986; “Don't check certificate”
is only meant for self-signed certificates in your own network. “basic” works only over HTTPS. In the palette,
`Enter-PSSession srv01`, `Enter-PSSession -ComputerName srv01 -Credential FIRMA\admin -UseSSL`, `etsn srv01` or
`winrm admin@srv01` connects right away. The session feels like `Enter-PSSession`: prompt `[SRV01]: PS C:\…>`,
↑/↓ history, Tab/⇧Tab cycles through completions from the server, `function … {` continues on the next line (`>>`),
`Read-Host` and confirmation prompts get your answer. Variables and the current folder persist. Ctrl+C cancels a
running command by restarting PowerShell in the same folder (variables are lost then). Without a console on the
server, full-screen programs (`edit`, `vim`) do not work. On the server (as administrator): `Enable-PSRemoting -Force`.
Pure Go (WinRS); Negotiate/NTLM works with domain accounts. For servers that only allow Kerberos choose
“kerberos”: enter the computer name (e.g. `srv01.corp.local`, not an IP) and the user as `name@corp.local`; sshTermy
finds the domain controller via DNS (or uses `krb5.conf`/`KRB5_CONFIG`). On Linux/macOS a ticket from `kinit` is
enough, otherwise sshTermy asks for the password (the Windows sign-in itself cannot be used for this).
Alternatively, Windows OpenSSH works over plain SSH (PowerShell as `DefaultShell`), including files and tunnels.

## Screens

The screenshots show the German interface; the English one looks the same.

| Unlock | Connection manager |
|---|---|
| ![](docs/bilder/01-entsperren.png) | ![](docs/bilder/02-manager.png) |
| **Ghost text** | **Dangerous command** |
| ![](docs/bilder/07-ghost.png) | ![](docs/bilder/04-gefahr.png) |
| **Ask AI** | **Tunnels & web services** |
| ![](docs/bilder/05-frage.png) | ![](docs/bilder/06-tunnel.png) |

**Command palette (Ctrl+Space):** one field for everything; text becomes a goal or a question. Without text it stays short: **Frequent** on top (in the terminal goal mode, host overview, search, split, new tab), then the recently used connections and one folder per area: **Quick commands & tunnels ›, AI › (goal, agent, ask AI, task, companion), This session › (host, files, containers, recording), Search & logs › (scrollback, all tabs, error lines, log glance, session logs, folder scan, log watch), Tabs & view › (tabs, split ‹ value ›, zoom, workspaces), Several hosts & network ›, Team & sharing ›, Connections & data › (new, SSH key, import, export, full backup)** (the number of entries on the right). Keys that only work in the manager are not shown in the terminal. ⏎ or → opens a folder, Esc or ← goes back. Typing shows matches from all folders directly.
Toggles live in the **[:] Settings ›** submenu in four groups: General, Appearance, Security, Setup (each toggle shows "on"/"off" and the palette stays open while you flip them). Shortcuts, tour, news, update and “Offer updates” are under **[?] Help & info ›**, the debug log and performance overlay there under **Troubleshooting ›**. Entries with several values show them as ‹ value ›: ⏎, → or a click moves to the next one, ← back. AI, Logs & audit (session log, log watch, syslog, audit), Network devices (command sets, config rules) and relay have their own subfolders. Motion “off” also turns off the start animation. Typing still finds items inside the submenus directly, even single values like “Matrix”; Esc or ← goes back.

![Command palette](docs/bilder/09-palette.png)

| Files (SFTP, F5) | Runbooks in goal mode |
|---|---|
| ![](docs/bilder/12-dateien.png) | ![](docs/bilder/11-runbooks.png) |
| **Test the AI endpoint, choose a model** | **Agent mode: works on its own, writes scripts** |
| ![](docs/bilder/10-ki-test.png) | ![](docs/bilder/16-agent.png) |
| **Split view: large + two** | **Tab list (Alt+T)** |
| ![](docs/bilder/17-geteilt.png) | ![](docs/bilder/18-tabs.png) |
| **Session logs by day** | |
| ![](docs/bilder/19-protokolle.png) | |
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
  unreadable for other accounts and computers. The master password remains as recovery (moving, macOS). Switch it on or off any time in “Change master password”.
- File `store.vault` in `%APPDATA%\sshtermy` (macOS `~/Library/Application Support/sshtermy`, Linux
  `~/.config/sshtermy`), permissions 0600, written atomically. `SSHTERMY_HOME` for a portable installation.
- Automatic lock after idle time (`LockAfterMin`), the DEK is zeroed when locking (best effort).
- `domain.Secret` never reveals itself in logs and error messages (`••••••`).

### SSH

- **Host keys:** trust on first use with a SHA-256 fingerprint in the dialog, own `known_hosts`. A **changed**
  host key aborts the connection, without “connect anyway” (possible man-in-the-middle).
- Agent auth preferred: private keys stay in the agent. Embedded keys are only stored encrypted in the store.
- Tunnels bind to `127.0.0.1` by default. SOCKS5 resolves names on the server (no local DNS leaks).
- **SSH keys (`c` in the manager):** generates ed25519 or RSA-4096 keys (like `ssh-keygen`, optionally with a
  passphrase, as a file in `~/.ssh` or embedded), adds them to `~/.ssh/authorized_keys` like `ssh-copy-id` and
  switches the connection only after a successful test login with the key.
- **Keychain:** a password or key passphrase can live in the macOS Keychain, Windows Credential Manager or the
  Secret Service (Linux, `secret-tool`) instead of the store. Each computer has its own entry; if it is missing
  after a sync, sshTermy asks once and saves the answer there.

### AI

- The AI **never types by itself**. Every command is a suggestion; only your confirmation sends it to the PTY.
- **Fixed danger rules** (`internal/safety`), never the model, decide what is red: `rm -rf`, `find -delete`, `dd`, `mkfs`,
  `curl | sh`, `… | base64 -d | sh`, reboot, flushing the firewall, changing users, `git reset --hard`, removing packages, Docker prune,
  stopping the SSH service, deleting system files, taking the network down, `iptables -P … DROP`, `kubectl delete`, `DROP DATABASE`, `zfs destroy`, `rsync --delete`;
  on Windows `Remove-Item -Recurse`, `Format-Volume`, `iex (iwr …)`, stopping WinRM, `Disable-NetAdapter` and more.
  Every command is checked against all rule groups (Unix, Windows, network devices), multi-line input line by line: a switch that was taken for a server cannot hide `erase startup-config`.
  Red commands need two confirmations; edited commands are checked again. `sudo` is marked, also behind `&&`, `;` or `|`.
- **Reading secrets asks too:** `/etc/shadow`, private keys, `.env`, `kubectl get secret`, `docker inspect`, `printenv` or a switch's `show running-config` change nothing, but their output would go to the AI. The agent and MCP always ask first; plan mode skips them while exploring.
- **Dialogs that pop up by themselves** (host key, password, MCP request, files from the server) ignore keys for one second, so a `j` from `journalctl` you were typing confirms nothing. If another dialog is open, they wait instead of replacing it.
- **Paste guard:** the same rules check every pasted line. A hit shows the preview with the
  red line and needs two confirmations, even if the multi-line preview is turned off.
- **Prompt injection:** everything from the terminal is passed as material in a `<<< … >>>` block whose markers are
  defused inside the content; the system prompt says material is never an instruction.
- **Strict parsing:** only the expected JSON, no control characters (ESC sequences from the model never reach
  the screen), length limits, `<think>` from reasoning models is removed.
- **Secrets** (private keys, AWS/GitHub/OpenAI keys, bearer tokens, `password=…`, `user:pass@`) are removed before
  sending; there is no ghost text at a password prompt.
- The API key is never sent via `http://` to a foreign host (unless explicitly allowed).
- **AI privacy** (Settings › AI): “all providers”, “local AI only” (only endpoints on your own network: localhost,
  private IPs, `.local`/`.lan`/`.internal` names; cloud fallbacks are skipped) or “AI off”. Per connection the tags
  `ai-local` or `no-ai` (also `ki-lokal`, `ki-aus`) do the same; the stricter setting wins. In the same folder
  “Show last AI request” shows exactly what was last sent, to which host and model.
- Rate limit in goal mode (default 6 steps/min, at most 40 steps per goal).

### sshTermy as MCP server

AI clients such as Claude Desktop or Claude Code can use your saved connections through the Model Context Protocol.
Turn it on in Settings › AI › “MCP access for AI clients”: **read only** (list connections and open tabs,
read a tab, run read-only commands like `df -h` or `show interfaces`) or **with confirmation** (any other command
runs only after you press `y` in sshTermy; dangerous commands need `y` twice). Commands with `sudo` or that read secrets (`/etc/shadow`, keys, `.env`, `show running-config` …) always ask, even if they only read. sshTermy must be running and unlocked;
`sshtermy mcp` is only a bridge and holds no data. Connections tagged `no-ai`/`ai-local`, or AI privacy “AI off”, are
never shown. Secrets in output are redacted, every call lands in the audit log.

```
claude mcp add sshtermy -- sshtermy mcp
```

Claude Desktop (`claude_desktop_config.json`): `{"mcpServers": {"sshtermy": {"command": "sshtermy", "args": ["mcp"]}}}`

### AI providers and fallback

- **Provider** in the AI settings (`s`): Spark / any OpenAI-compatible server (vLLM, SGLang, LiteLLM, LM Studio),
  Ollama, OpenAI or Claude (Anthropic Messages API). ←/→ switches and prefills address and model; Ctrl+L lists
  the provider's models, Ctrl+T tests.
- **Fallback providers** (settings → “Fallback providers”, or `r` in the AI status): up to three, tried in order
  when the endpoint does not answer. A failed endpoint is skipped for 5 minutes or until the background check
  reaches it again. The status dot turns yellow and shows “fallback: Claude” while a fallback answers.
- Cloud providers always need an API key; keys are stored encrypted like passwords.

### Faster and more focused AI

- **Every function has its own role:** ghost text, follow-up commands, goal mode, agent, plan, companion, explain,
  log diagnosis, report and “adapt for all” each send their own system prompt with their own token limit and temperature.
- **House rules** (AI settings → “House rules”, and per folder with `e` in the manager → “AI house rules”): free text
  that goes along with every AI task, e.g. “Debian 12, never suggest reboot”. Folder rules apply to the subfolders too.
  They never override the safety rules.
- **Fast model** (AI settings): optionally a second, small model on the same endpoint for ghost text, follow-up commands
  and the companion; goal mode, agent, plan and diagnosis stay on the main model. If the fast model fails, the main model
  answers and the fast one is skipped for 5 minutes.
- **Quicker answers:** the fixed part of the request (system profile, recent commands, screen) comes first and what you
  just typed comes last, so vLLM, SGLang and Ollama can reuse their prefix cache. Ghost text sends only the last 25 screen
  lines, follow-ups 40. No new request while the shown suggestion still fits, and the same input within 3 minutes is answered
  from memory. Short answers ask reasoning models not to think long (Qwen, gpt-oss, OpenAI o/gpt-5, Ollama thinking models).
- **Kept warm:** while tabs are open, a one-token request every 4 minutes keeps your own model loaded (Ollama unloads
  after 5 minutes). Cloud providers are not pinged.
- **AI status** (click the dot) shows the response time per task (average and last), so you see what is slow.

### Error detection and session log

- **When is a command done?** After Enter, as soon as an empty prompt is back and the output settles. Its
  output is the text between the command line and the new prompt.
- **Exit code:** if the shell reports it via shell integration (OSC 133;D or 633;D, as in VS Code, WezTerm, iTerm2),
  the real code counts (130 = Ctrl+C is not a failure). For bash, e.g. this in `~/.bashrc` is enough:
  `PROMPT_COMMAND='printf "\033]133;D;%s\007" $?'"${PROMPT_COMMAND:+;$PROMPT_COMMAND}"`.
  Without shell integration sshTermy estimates from typical error messages in the last lines (`command not found`,
  `No such file`, `Permission denied`, `E: …`, `% Invalid input` on switches and more). This is only an offer, never a block.
- **Logs** are stored in `logs/` in the config folder (synced only with the archive switched on, see below), one file per session,
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
- A build without that key (self-built without `UPDATE_PUBKEY`) says so in the About dialog (F12) and asks before
  installing an update; `--update` prints a warning.
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
make bench        # performance: terminal throughput, memory per tab, manager with 500 connections
make loadtest     # load test: 1000 hosts (store, manager, palette, broadcast, sync, MCP)
make build        # bin/sshtermy
make dist         # all platforms to dist/ + checksums.txt
make fuzz         # fuzz tests for parsers of data from foreign devices (FUZZTIME=5m for longer)
make vuln         # known vulnerabilities in Go and the libraries (govulncheck)
make golden       # rewrite the screen tests after an intended UI change
make handout      # printable quick reference site/handout/*.pdf (Node + Playwright; content in tools/handout/content.mjs)
                  # TestHandoutKeysCurrent fails when keys or help change until this has run again
```

**Screen tests:** `internal/ui/golden_test.go` draws every view (manager, forms, dialogs, terminal with panels,
split view …) at a fixed size in German and English, as plain text without colors, and compares it with
`internal/ui/testdata/golden/*.txt`. It also checks that every screen fills the window exactly. After an
intended change run `make golden` and look at the diff of the `.txt` files before committing. On a merge
conflict in these files simply run `make golden` again after the merge. If CI fails, the job artifact
`golden-actual` contains the screens as they look now. New view: add a line to `goldenCases`.

**CI** runs the tests on Linux, Windows and macOS, plus the race detector, `govulncheck` and a short fuzz run.
Dependabot proposes library updates weekly. Signing updates (one-time setup): [docs/signatur.md](docs/signatur.md) (German).

**Interface texts** are written in German in the code and go through `i18n.T("…")` (formats: `i18n.F`, fixed
errors: `i18n.NewError`). Every new text needs an entry in `internal/i18n/en.go`; `go test ./internal/i18n`
reports missing entries and mismatched placeholders.

Test locally without a real server and without a GPU:

```
cd tools/dev/devsshd && go run .          # test SSH server on 127.0.0.1:2222 (any password)
python3 tools/dev/fakellm.py              # test LLM on 127.0.0.1:11434 (/v1/chat/completions)
SSHTERMY_HOME=/tmp/sth go run ./cmd/sshtermy
```

The terminal emulator is a patched copy in `third_party/vt` (faster scrolling, compact scrollback),
see `third_party/vt/README.sshtermy.md`.

## Next steps (suggestions)

1. Windows resources (icon, version info) and code signing of the `.exe`, macOS notarization.
2. macOS keychain as a second unlock path (like DPAPI on Windows).

## Relay: servers behind NAT/firewall

In short (details in [docs/vermittler.md](docs/vermittler.md), German):

```
# 1. on a reachable machine (VPS, Pi, NAS)
sshtermy relay start --url https://relay.example.org:8022
sshtermy relay code --anker             # line for a server
sshtermy relay code                     # line for sshTermy

# 2. on the server that should become reachable
sshtermy anchor koppeln 'https://relay.example.org:8022#termy=…&fp=…'
sshtermy anchor dienst | sudo tee /etc/systemd/system/sshtermy-anker.service
sudo systemctl enable --now sshtermy-anker
```

3. In sshTermy: Ctrl+Space → “Pair / manage relays”, paste the line. “Systems via relay” lists all servers with an
   online dot, ⏎ creates the connection or connects.

The relay only passes encrypted bytes. The anchor reports the host key; if it does not match, sshTermy aborts.
Without a relay nothing changes.

## Sync between computers

Press **y** in the manager. The wizard only asks for what is needed and checks every entry live right away
(name, port, certificate, login, folder, write permission, existing sync file). If something fails, the line
below says what to do, e.g. where to enable WebDAV in DSM or QTS.

![Sync wizard](docs/bilder/13-sync.png)

| Location | What you enter | Notes |
|---|---|---|
| **Folder (cloud)** | a folder in OneDrive, Dropbox, iCloud Drive, the Nextcloud client, Syncthing or on a network drive | Cloud folders that are found are suggested. Conflict copies of the cloud service are merged automatically. |
| **NAS / WebDAV: Synology** | name/IP, user, password | Install the “WebDAV Server” package, enable HTTPS. sshTermy finds port 5006/5005 by itself. Folder e.g. `/home/sshtermy`. |
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
wins per entry, deleted entries are remembered as tombstones (180 days), and “last used”
does not count as a change. This way changes on two computers are not lost as long as they affect different entries.
Syncing happens at start, a few seconds after every change and every 5 minutes; manually via the palette
(“Sync now”). All settings travel along: look, colors, icons, language, keys, AI, lock, notifications, monitoring
limits, danger check, paste preview, session logs, intro animation and so on. Your own command sets sync too; on another computer
a changed or new set only takes effect after you take it over there (Settings › Network devices › Command sets, `f`, with a diff), and a
set deleted on one computer is moved aside to `befehlssaetze/.geloescht` on the others. Only computer-specific things stay local:
DPAPI, the sync settings themselves, key file paths, paired relays, team folder paths, the "mouse" switch (depends on
the terminal program), audit/debug log, MCP server, editor, the font size of the own window and everything marked
“this computer only” (keys, shell, serial port). Passwords in the OS keychain stay on each computer; only the switch
travels, and each computer asks once. Nerd Font icons sync too, but on another computer they only turn on after the
test, since the font may be missing there. Schedules sync per entry and run only on their computer. With “Command
history” (on by default) the favorite suggestions (F9) count the commands from all computers; each computer only
writes its own count. Below the form the wizard lists the computers in the sync with their last sync and what
travels or stays.

**Archive (optional):** With the “Archive” switch in the sync wizard (y), the config backups of your switches (F6)
and the session logs also travel to your other computers. They are stored in the subfolder `sshtermy-archiv/` next to
the sync file, one encrypted file per backup or log (key derived from the sync key; the file name only shows kind and
day, no host names). Every file is written only once, so there are no conflicts. “Keep” (30, 90 or 365 days) sets how
long entries stay in the archive; local copies are kept. A deleted log also disappears on the other computers. Running
sessions go into the archive after they end; the log view shows which computer a log came from.

To try it without a NAS: `go run ./tools/dev/devdav` (http on port 5005) or `go run ./tools/dev/devdav -tls -addr 127.0.0.1:5006`
(self-signed), user `dominik`, password `nas-pw`.

## Sharing with the team

Ctrl+Space → “Share with the team”: choose connections (Space, `g` folder, `a` all), then password and options.
sshTermy writes an encrypted `.sshteam` file (Argon2id + AES-256-GCM, nothing readable without the password,
not even the sender). Jump hosts are added automatically; tunnels, runbooks and quick commands are optional.
**Never included:** passwords, embedded keys, passphrases, key paths outside `~/`, user names (optional),
learned rejected suggestions, AI and sync settings. Leave the password empty and sshTermy creates one.

The colleague uses Ctrl+Space → “Import team package”: file, password, target folder (default “Team”). A
preview shows new and updated entries. Importing the same package again updates the entries but keeps the
colleague's own passwords, keys and user names. For safety, new entries arrive without agent or X11
forwarding, without a local shell command and without a `cmd:` password source; turn those on yourself if needed. If the team changes the address of a connection that has your password, key or agent forwarding stored, sshTermy asks before using the new address.

### Team folders: kept up to date

When sharing, put the file into a folder everyone can reach (network drive, OneDrive or Teams folder) or enter an
https/WebDAV address (`https://user:password@nas:5006/team/acme.sshteam`) and turn on “Keep up to date”. sshTermy
then rewrites the file whenever the connections change (checked every minute). If you shared from a folder in the
manager, the whole folder is shared: new connections in it are included automatically.

Colleagues leave “Subscribe” on when importing. sshTermy then re-reads the file every 5 minutes and applies changes:
new connections are added, changed ones updated, and ones the sender deleted disappear here too. Their own
passwords, keys and user names stay. A package never removes protection: a “prod” tag or the device type
“network device” stays even if the sender removed it. Changed quick commands show the command on the first key
press and only run on the second; new or changed runbooks wait before any schedule runs them; tunnels from the
team never start by themselves. Ctrl+Space → “Team folders” shows the status, checks right away or ends a
subscription (the connections stay). Team folders are local only and not part of the sync.

## License

sshTermy is released under the [MIT License](LICENSE): you may use, copy, change and pass it on, also commercially,
as long as the copyright notice and the license text stay included. Idea: Dominik Bornhäußer.
