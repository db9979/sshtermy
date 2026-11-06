#!/bin/sh
# Schreibt die winget-Manifeste (portable .exe) für ein Release:
#   packaging/winget.sh 1.0.42 dist/checksums.txt out/
# Ergebnis: out/manifests/d/db9979/sshTermy/1.0.42/*.yaml, so wie im Repo
# microsoft/winget-pkgs. Eingereicht wird mit .github/workflows/winget.yml.
set -eu
ver="$1"; sums="$2"; out="$3"
id="db9979.sshTermy"
sha() {
	s="$(awk -v n="$1" '{f=$2; sub(/^\*/, "", f); if (f == n) print toupper($1)}' "$sums")"
	[ -n "$s" ] || { echo "winget.sh: keine Prüfsumme für $1" >&2; exit 1; }
	echo "$s"
}
u="https://github.com/db9979/sshtermy/releases/download/v$ver"
d="$out/manifests/d/db9979/sshTermy/$ver"
mkdir -p "$d"
mv_="1.6.0"
hdr="# yaml-language-server: \$schema=https://aka.ms/winget-manifest"

cat >"$d/$id.yaml" <<YML
$hdr.version.$mv_.schema.json
PackageIdentifier: $id
PackageVersion: $ver
DefaultLocale: en-US
ManifestType: version
ManifestVersion: $mv_
YML

cat >"$d/$id.installer.yaml" <<YML
$hdr.installer.$mv_.schema.json
PackageIdentifier: $id
PackageVersion: $ver
InstallerType: portable
Commands:
- sshtermy
ReleaseDate: $(date -u +%Y-%m-%d)
Installers:
- Architecture: x64
  InstallerUrl: $u/sshtermy_windows_amd64.exe
  InstallerSha256: $(sha sshtermy_windows_amd64.exe)
- Architecture: x86
  InstallerUrl: $u/sshtermy_windows_386.exe
  InstallerSha256: $(sha sshtermy_windows_386.exe)
- Architecture: arm64
  InstallerUrl: $u/sshtermy_windows_arm64.exe
  InstallerSha256: $(sha sshtermy_windows_arm64.exe)
ManifestType: installer
ManifestVersion: $mv_
YML

cat >"$d/$id.locale.en-US.yaml" <<YML
$hdr.defaultLocale.$mv_.schema.json
PackageIdentifier: $id
PackageVersion: $ver
PackageLocale: en-US
Publisher: Dominik Bornhäußer
PublisherUrl: https://github.com/db9979
PackageName: sshTermy
PackageUrl: https://db9979.github.io/sshtermy/
License: Freeware
ShortDescription: SSH terminal with an AI copilot
Description: Terminal application with connection manager, encrypted store, SFTP, tunnels, broadcast and an AI copilot for any OpenAI-compatible endpoint.
Moniker: sshtermy
Tags:
- ssh
- terminal
- tui
- sftp
- ai
ReleaseNotesUrl: https://github.com/db9979/sshtermy/releases/tag/v$ver
ManifestType: defaultLocale
ManifestVersion: $mv_
YML

cat >"$d/$id.locale.de-DE.yaml" <<YML
$hdr.locale.$mv_.schema.json
PackageIdentifier: $id
PackageVersion: $ver
PackageLocale: de-DE
Publisher: Dominik Bornhäußer
PackageName: sshTermy
PackageUrl: https://db9979.github.io/sshtermy/de/
License: Freeware
ShortDescription: SSH-Terminal mit KI-Copilot
ManifestType: locale
ManifestVersion: $mv_
YML
echo "$d"
