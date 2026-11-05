#!/bin/sh
# sshTermy installieren (macOS und Linux):
#
#   curl -fsSL https://raw.githubusercontent.com/db9979/sshtermy/main/install.sh | sh
#
# Lädt die neueste Version aus den GitHub Releases, prüft die SHA-256-Summe und
# legt sie ab:
#   macOS: ~/Applications/sshTermy.app (Doppelklick öffnet sshTermy im Terminal)
#   beide: Befehl ~/.local/bin/sshtermy
# Über curl geladene Dateien bekommen keine Quarantäne-Markierung, deshalb
# blockt Gatekeeper die App nicht. Auto-Update funktioniert danach wie gewohnt.
set -eu

REPO="db9979/sshtermy"
BASE="https://github.com/$REPO/releases/latest/download"

say() { printf '%s\n' "$*"; }
die() { printf 'sshTermy: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) die "nur macOS und Linux; für Windows die .exe aus den Releases nehmen" ;;
esac
case "$(uname -m)" in
arm64 | aarch64) arch=arm64 ;;
x86_64 | amd64) arch=amd64 ;;
*) die "Prozessor $(uname -m) wird nicht unterstützt" ;;
esac
# Rosetta: ein x86-Terminal auf Apple Silicon meldet x86_64.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then
	arch=arm64
fi
asset="sshtermy_${os}_${arch}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Lade $asset …"
curl -fsSL -o "$tmp/$asset" "$BASE/$asset" || die "Download fehlgeschlagen"
curl -fsSL -o "$tmp/checksums.txt" "$BASE/checksums.txt" || die "checksums.txt fehlt"

want="$(awk -v n="$asset" '{f=$2; sub(/^\*/, "", f); if (f == n) print tolower($1)}' "$tmp/checksums.txt")"
[ -n "$want" ] || die "keine Prüfsumme für $asset"
if command -v shasum >/dev/null 2>&1; then
	got="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
else
	got="$(sha256sum "$tmp/$asset" | awk '{print $1}')"
fi
[ "$got" = "$want" ] || die "Prüfsumme stimmt nicht, Download verworfen"
chmod 755 "$tmp/$asset"

if [ "$os" = darwin ]; then
	app="$HOME/Applications/sshTermy.app"
	c="$app/Contents"
	mkdir -p "$c/MacOS" "$c/Resources"
	# Programmdatei unter Resources: Contents/MacOS/sshTermy (Starter) und
	# sshtermy würden sich auf dem Mac-Dateisystem (ohne Groß/Klein) beißen.
	bin="$c/Resources/sshtermy"
	mv -f "$tmp/$asset" "$bin"

	cat >"$c/MacOS/sshTermy" <<'EOF'
#!/bin/sh
# Startet sshTermy in einem Terminalfenster (iTerm, falls installiert).
BIN="$(cd "$(dirname "$0")/../Resources" && pwd)/sshtermy"
for t in "/Applications/iTerm.app" "$HOME/Applications/iTerm.app"; do
	[ -d "$t" ] && exec open -a "$t" "$BIN"
done
exec open -a Terminal "$BIN"
EOF
	chmod 755 "$c/MacOS/sshTermy"

	cat >"$c/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>sshTermy</string>
	<key>CFBundleDisplayName</key><string>sshTermy</string>
	<key>CFBundleIdentifier</key><string>com.github.db9979.sshtermy</string>
	<key>CFBundleExecutable</key><string>sshTermy</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF
	xattr -dr com.apple.quarantine "$app" 2>/dev/null || true
	# Finder/Launchpad sollen die neue App gleich sehen.
	/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app" >/dev/null 2>&1 || true
	say "App: $app (im Finder unter Programme, oder per Spotlight „sshTermy“)"
else
	bin="$HOME/.local/share/sshtermy/sshtermy"
	mkdir -p "$(dirname "$bin")"
	mv -f "$tmp/$asset" "$bin"
fi

mkdir -p "$HOME/.local/bin"
ln -sf "$bin" "$HOME/.local/bin/sshtermy"
say "Befehl: ~/.local/bin/sshtermy ($("$bin" --version 2>/dev/null | head -1 || true))"
case ":$PATH:" in
*":$HOME/.local/bin:"*) ;;
*) say "Hinweis: ~/.local/bin ist nicht im PATH. Für zsh: echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc" ;;
esac
say "Fertig."
