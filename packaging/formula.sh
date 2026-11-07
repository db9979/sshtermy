#!/bin/sh
# Schreibt die Homebrew-Formel für ein Release (Tap = dieses Repo):
#   packaging/formula.sh 1.0.42 dist/checksums.txt > Formula/sshtermy.rb
# Installation: brew tap db9979/sshtermy https://github.com/db9979/sshtermy
#               brew install sshtermy
set -eu
ver="$1"; sums="$2"
sha() {
	s="$(awk -v n="$1" '{f=$2; sub(/^\*/, "", f); if (f == n) print tolower($1)}' "$sums")"
	[ -n "$s" ] || { echo "formula.sh: keine Prüfsumme für $1" >&2; exit 1; }
	echo "$s"
}
u="https://github.com/db9979/sshtermy/releases/download/v$ver"
cat <<RB
# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "$ver"

  on_macos do
    on_arm do
      url "$u/sshtermy_darwin_arm64"
      sha256 "$(sha sshtermy_darwin_arm64)"
    end
    on_intel do
      url "$u/sshtermy_darwin_amd64"
      sha256 "$(sha sshtermy_darwin_amd64)"
    end
  end

  on_linux do
    on_arm do
      url "$u/sshtermy_linux_arm64"
      sha256 "$(sha sshtermy_linux_arm64)"
    end
    on_intel do
      url "$u/sshtermy_linux_amd64"
      sha256 "$(sha sshtermy_linux_amd64)"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
RB
