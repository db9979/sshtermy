#!/bin/sh
# Baut ein flaches apt-Repository aus .deb-Dateien (für GitHub Pages):
#   packaging/apt-repo.sh _site/apt     (die .deb liegen schon dort)
# Mit APT_GPG_PRIVATE_KEY (ASCII-armored, ohne Passphrase) wird signiert:
# InRelease, Release.gpg und der öffentliche Schlüssel sshtermy.gpg.
# Ohne Schlüssel bleibt das Repo unsigniert (Einbinden mit [trusted=yes]).
set -eu
dir="$1"
cd "$dir"
rm -f Release InRelease Release.gpg
apt-ftparchive packages . >Packages
gzip -9kf Packages
apt-ftparchive \
	-o APT::FTPArchive::Release::Origin=sshTermy \
	-o APT::FTPArchive::Release::Label=sshTermy \
	-o APT::FTPArchive::Release::Suite=stable \
	-o APT::FTPArchive::Release::Codename=stable \
	-o APT::FTPArchive::Release::Description="sshTermy – SSH terminal with an AI copilot" \
	release . >Release
if [ -n "${APT_GPG_PRIVATE_KEY:-}" ]; then
	export GNUPGHOME="$(mktemp -d)"
	printf '%s\n' "$APT_GPG_PRIVATE_KEY" | gpg --batch --import 2>/dev/null
	gpg --batch --yes --pinentry-mode loopback --clearsign -o InRelease Release
	gpg --batch --yes --pinentry-mode loopback -abs -o Release.gpg Release
	gpg --batch --export >sshtermy.gpg
	rm -rf "$GNUPGHOME"
	echo "apt: signiert"
else
	echo "apt: ohne Signatur (Secret APT_GPG_PRIVATE_KEY fehlt)"
fi
