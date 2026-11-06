#!/bin/sh
# Baut ein .deb aus einer fertigen Programmdatei (kein Kompilieren hier).
#   packaging/deb.sh 1.0.42 amd64 dist/sshtermy_linux_amd64 dist
# Ergebnis: dist/sshtermy_1.0.42_amd64.deb (Programm unter /usr/bin/sshtermy).
set -eu
ver="$1"; arch="$2"; bin="$3"; out="$4"

root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT INT TERM
chmod 755 "$root"
install -Dm755 "$bin" "$root/usr/bin/sshtermy"
install -d "$root/usr/share/doc/sshtermy" "$root/DEBIAN"
cat >"$root/usr/share/doc/sshtermy/copyright" <<'TXT'
sshTermy – SSH terminal with an AI copilot
Idea: Dominik Bornhäußer
Source: https://github.com/db9979/sshtermy
License: MIT

TXT
cat "$(dirname "$0")/../LICENSE" >>"$root/usr/share/doc/sshtermy/copyright"
size="$(du -sk "$root/usr" | cut -f1)"
cat >"$root/DEBIAN/control" <<TXT
Package: sshtermy
Version: $ver
Architecture: $arch
Maintainer: Dominik Bornhäußer <noreply@github.com>
Installed-Size: $size
Section: net
Priority: optional
Homepage: https://db9979.github.io/sshtermy/
Description: SSH terminal with an AI copilot
 sshTermy is a terminal application (TUI) with a connection manager,
 encrypted store, SFTP, tunnels, broadcast and an AI copilot for any
 OpenAI-compatible endpoint. Updates come through apt.
TXT
mkdir -p "$out"
dpkg-deb --root-owner-group --build "$root" "$out/sshtermy_${ver}_${arch}.deb" >/dev/null
echo "$out/sshtermy_${ver}_${arch}.deb"
